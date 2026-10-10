package registry

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRegisterAndGet(t *testing.T) {
	name := "test_gen_register"
	factory := func(config map[string]any) (Generator, error) {
		return &SeqGenerator{current: 0, step: 1}, nil
	}

	if err := Register(name, factory); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := Register(name, factory); err == nil {
		t.Fatal("expected duplicate registration error")
	}

	gen, err := Get(name, map[string]any{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gen == nil {
		t.Fatal("expected generator, got nil")
	}
}

func TestRegisterErrors(t *testing.T) {
	if err := Register("", func(map[string]any) (Generator, error) { return nil, nil }); err == nil {
		t.Fatal("expected error for empty name")
	}

	if err := Register("test_nil_factory", nil); err == nil {
		t.Fatal("expected error for nil factory")
	}

	_, err := Get("missing_generator", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing generator")
	}
}

func TestMustRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	MustRegister("", func(map[string]any) (Generator, error) { return nil, nil })
}

func TestMustRegisterInfo_PanicsForUnknownGenerator(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unregistered generator name")
		}
	}()
	MustRegisterInfo("nonexistent_generator_xyz", GeneratorInfo{
		Name:        "nonexistent_generator_xyz",
		Description: "test",
	})
}

func TestListGenerators_Sorted(t *testing.T) {
	infos := ListGenerators()
	if len(infos) == 0 {
		t.Fatal("ListGenerators returned empty list")
	}
	for i := 1; i < len(infos); i++ {
		if infos[i].Name < infos[i-1].Name {
			t.Errorf("not sorted: %q before %q", infos[i-1].Name, infos[i].Name)
		}
	}
}

func TestGetInfo(t *testing.T) {
	info, ok := GetInfo("seq")
	if !ok {
		t.Fatal("GetInfo(seq) returned false")
	}
	if info.Name != "seq" {
		t.Errorf("info.Name = %q, want seq", info.Name)
	}
	if info.Description == "" {
		t.Error("info.Description is empty")
	}
}

func TestGetInfo_NotFound(t *testing.T) {
	_, ok := GetInfo("does_not_exist")
	if ok {
		t.Fatal("GetInfo returned true for nonexistent generator")
	}
}

func TestAllGeneratorsHaveInfo(t *testing.T) {
	infos := ListGenerators()
	if len(infos) == 0 {
		t.Fatal("ListGenerators returned empty list")
	}
	for _, info := range infos {
		if info.Description == "" {
			t.Errorf("generator %q has empty description", info.Name)
		}
		if info.Example == "" {
			t.Errorf("generator %q has empty example", info.Name)
		}
	}
}

func TestGet_RejectsUnknownConfigKey(t *testing.T) {
	_, err := Get("int", map[string]any{"mn": 1})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	for _, want := range []string{`unknown config key "mn"`, `did you mean "min"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestGet_AllowsReservedKeys(t *testing.T) {
	if _, err := Get("int", map[string]any{EnvKey: NewEnv("")}); err != nil {
		t.Fatalf("reserved key rejected: %v", err)
	}
}

// Every documented example must build under strict key checking, which also
// proves ConfigKeys lists every key the examples use.
func TestGeneratorExamplesBuild(t *testing.T) {
	for _, info := range ListGenerators() {
		t.Run(info.Name, func(t *testing.T) {
			var fields []struct {
				Gen    string         `yaml:"gen"`
				Config map[string]any `yaml:"config"`
			}
			if err := yaml.Unmarshal([]byte(info.Example), &fields); err != nil {
				t.Fatalf("example is not valid YAML: %v", err)
			}
			for _, f := range fields {
				if _, err := Get(f.Gen, f.Config); err != nil {
					t.Fatalf("example does not build: %v", err)
				}
			}
		})
	}
}

func spec(gen string, config map[string]any) map[string]any {
	return map[string]any{"gen": gen, "config": config}
}

// Composites may only nest generators whose output is a pure function of the
// row seed; stateful, store-reading, and (outside switch) row-aware
// generators must be top-level fields.
func TestCompositeNestingRules(t *testing.T) {
	seq := spec("seq", nil)
	ref := spec("rel_ref", map[string]any{"entity": "User", "field": "id"})
	tpl := spec("template", map[string]any{"tpl": "{a}"})
	pick := spec("pick", map[string]any{"values": []any{"x"}})

	tests := []struct {
		name    string
		gen     string
		config  map[string]any
		wantErr string
	}{
		{"object seq", "object", map[string]any{"fields": map[string]any{"n": seq}}, "per-row state"},
		{"object rel_ref", "object", map[string]any{"fields": map[string]any{"n": ref}}, "other entities"},
		{"object template", "object", map[string]any{"fields": map[string]any{"n": tpl}}, "current row"},
		{"list seq", "list", map[string]any{"len": 2, "item": seq}, "per-row state"},
		{"one_of template", "one_of", map[string]any{"generators": []any{tpl}}, "current row"},
		{"switch seq", "switch", map[string]any{"key": "a", "cases": map[string]any{"x": seq}}, "per-row state"},
		{"switch rel_ref", "switch", map[string]any{"key": "a", "cases": map[string]any{"x": ref}}, "other entities"},
		{"switch template", "switch", map[string]any{"key": "a", "cases": map[string]any{"x": tpl}}, ""},
		{"object of object", "object", map[string]any{"fields": map[string]any{
			"inner": spec("object", map[string]any{"fields": map[string]any{"p": pick}}),
		}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Get(tt.gen, tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// A misspelled key in a sub-generator spec must not silently drop its config.
func TestCompositeSpecRejectsUnknownKeys(t *testing.T) {
	_, err := Get("object", map[string]any{"fields": map[string]any{
		"age": map[string]any{"gen": "int", "confg": map[string]any{"min": 1}},
	}})
	if err == nil || !strings.Contains(err.Error(), `"confg"`) {
		t.Fatalf("error = %v, want unknown key confg", err)
	}

	_, err = Get("object", map[string]any{"fields": map[string]any{
		"age": map[string]any{"gen": "int", "config": map[string]any{"mni": 1}},
	}})
	if err == nil || !strings.Contains(err.Error(), `did you mean "min"`) {
		t.Fatalf("error = %v, want suggestion for nested config key", err)
	}
}

// Every unknown key is reported at once, not just the first.
func TestGet_ReportsAllUnknownKeys(t *testing.T) {
	_, err := Get("int", map[string]any{"mn": 1, "maxx": 2})
	want := `int: unknown config keys "maxx" (did you mean "max"?), "mn" (did you mean "min"?)`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %s", err, want)
	}
}
