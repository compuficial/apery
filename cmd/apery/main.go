package main

import (
	"apery"
	"apery/internal/runtime"
	"context"
	"log"
)

func main() {
	p1 := apery.Plan{
		Seed: 4,
		Entities: []apery.EntitySpec{
			{
				Name:  "User",
				Count: 20_000,
				Fields: []apery.FieldSpec{
					{Name: "id", Gen: "seq"},
					{Name: "employee_number", Gen: "seq"},
					{Name: "is_active", Gen: "bool", Config: map[string]any{"probability": 0.7}},
					{Name: "department", Gen: "pick", Config: map[string]any{"values": []any{"engineering", "sales"}}},
					{Name: "department_code", Gen: "int", Config: map[string]any{"max": 100}},
					{Name: "idn", Gen: "ulid"},
					{Name: "timestamp", Gen: "time", Config: map[string]any{"format": "2006-01-02"}},
					{Name: "phone", Gen: "regex", Config: map[string]any{"pattern": `\(\d{3}\) \d{3}-\d{4}`}},
					{Name: "sku", Gen: "regex", Config: map[string]any{"pattern": `[A-Z]{2}-\d{6}`}},
					{Name: "license_plate", Gen: "regex", Config: map[string]any{"pattern": `[A-Z]{3}-\d{4}`}},
				{Name: "address", Gen: "object", Config: map[string]any{
					"fields": map[string]any{
						"city":  map[string]any{"gen": "pick", "config": map[string]any{"values": []any{"New York", "Los Angeles", "Chicago", "Houston", "Phoenix"}}},
						"zip":   map[string]any{"gen": "int", "config": map[string]any{"min": 10000, "max": 99999}},
						"suite": map[string]any{"gen": "regex", "config": map[string]any{"pattern": `[A-Z]\d{3}`}},
					},
				}},
				},
			},
		},
	}

	w, err := apery.NewJSONLWriter("output.jsonl")
	// w, err := apery.NewCSVWriter("output.csv")
	if err != nil {
		log.Printf("error creating writer: %v", err)
		return
	}

	if err := apery.Run(context.Background(), &p1, w,
		runtime.WithWorkers(16),
		runtime.WithChunkSize(10000),
	); err != nil {
		log.Printf("error: %v", err)
	}
}
