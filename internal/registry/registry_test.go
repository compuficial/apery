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
