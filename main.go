// Apery is a deterministic synthetic data generator built on declarative plans.
//
// It generates schema-driven synthetic data using a plan-based approach where the
// same plan with the same seed always produces identical output. The system uses
// a registry of pluggable generators (seq, pick, bool, int, float, uuid) and
// supports multiple output formats.
package main

import (
	"apery/internal/plan"
	"apery/internal/runtime"
	"apery/internal/writer"
	"fmt"
)

func main() {
	fmt.Println("Apery starting...")

	p1 := plan.Plan{
		Seed: 4,
		Entities: []plan.EntitySpec{
			{
				Name:  "User",
				Count: 20,
				Fields: []plan.FieldSpec{
					{Name: "id", Gen: "seq"},
					{Name: "employee_number", Gen: "seq"},
					{Name: "is_active", Gen: "bool", Config: map[string]any{"probability": 0.7}},
					{Name: "department", Gen: "pick", Config: map[string]any{"values": []any{"engineering", "sales"}}},
					{Name: "department_code", Gen: "int", Config: map[string]any{"max": 100}},
					{Name: "idn", Gen: "uuid"},
					{Name: "timestamp", Gen: "time", Config: map[string]any{"format": "2006-01-02"}},
				},
			},
		},
	}

	w, err := writer.NewJSONLWriter("output.jsonl")
	if err != nil {
		fmt.Println("Error creating writer:", err)
		return
	}

	executor := runtime.New(w)
	err = executor.Run(&p1)
	if err != nil {
		fmt.Println("Error:", err)
	}
}
