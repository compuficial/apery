package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile_YAML(t *testing.T) {
	content := `
seed: 42
entities:
  - name: User
    count: 10
    fields:
      - name: id
        gen: seq
      - name: active
        gen: bool
        config:
          probability: 0.8
`
	path := filepath.Join(t.TempDir(), "plan.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if p.Seed != 42 {
		t.Errorf("seed = %d, want 42", p.Seed)
	}
	if len(p.Entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(p.Entities))
	}
	e := p.Entities[0]
	if e.Name != "User" {
		t.Errorf("entity name = %q, want User", e.Name)
	}
	if e.Count != 10 {
		t.Errorf("count = %d, want 10", e.Count)
	}
	if len(e.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(e.Fields))
	}
	if e.Fields[0].Gen != "seq" {
		t.Errorf("field[0].gen = %q, want seq", e.Fields[0].Gen)
	}
	if e.Fields[1].Config["probability"] != 0.8 {
		t.Errorf("field[1].config.probability = %v, want 0.8", e.Fields[1].Config["probability"])
	}
}

func TestLoadFile_JSON(t *testing.T) {
	content := `{
  "seed": 99,
  "entities": [
    {
      "name": "Item",
      "count": 5,
      "fields": [
        {"name": "id", "gen": "seq"}
      ]
    }
  ]
}`
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if p.Seed != 99 {
		t.Errorf("seed = %d, want 99", p.Seed)
	}
	if p.Entities[0].Name != "Item" {
		t.Errorf("entity name = %q, want Item", p.Entities[0].Name)
	}
}

func TestLoadFile_YML_Extension(t *testing.T) {
	content := `
seed: 1
entities:
  - name: A
    count: 1
    fields:
      - name: x
        gen: const
        config:
          value: hello
`
	path := filepath.Join(t.TempDir(), "plan.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if p.Seed != 1 {
		t.Errorf("seed = %d, want 1", p.Seed)
	}
}

func TestLoadFile_UnknownExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.toml")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for .toml extension")
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/plan.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFile_DrivenBy(t *testing.T) {
	content := `
seed: 1
entities:
  - name: User
    count: 10
    fields:
      - name: id
        gen: seq
  - name: Order
    driven_by:
      entity: User
      field: id
      as: user_id
      min: 1
      max: 3
    fields:
      - name: total
        gen: int
        config:
          min: 100
          max: 999
`
	path := filepath.Join(t.TempDir(), "plan.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if len(p.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(p.Entities))
	}
	order := p.Entities[1]
	if order.DrivenBy == nil {
		t.Fatal("DrivenBy is nil")
	}
	if order.DrivenBy.Entity != "User" {
		t.Errorf("driven_by.entity = %q, want User", order.DrivenBy.Entity)
	}
	if order.DrivenBy.As != "user_id" {
		t.Errorf("driven_by.as = %q, want user_id", order.DrivenBy.As)
	}
	if order.DrivenBy.Min != 1 || order.DrivenBy.Max != 3 {
		t.Errorf("driven_by min/max = %d/%d, want 1/3", order.DrivenBy.Min, order.DrivenBy.Max)
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.yaml")
	if err := os.WriteFile(path, []byte("seed: [invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadFile_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte("{invalid}"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
