package registry

import "testing"

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
