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
					{Name: "department", Gen: "pick", Config: map[string]any{
						"values":  []any{"engineering", "sales", "marketing", "support"},
						"weights": []any{40, 30, 20, 10},
					}},
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
							"geo": map[string]any{"gen": "object", "config": map[string]any{
								"fields": map[string]any{
									"lat": map[string]any{"gen": "float", "config": map[string]any{"min": -90.0, "max": 90.0}},
									"lng": map[string]any{"gen": "float", "config": map[string]any{"min": -180.0, "max": 180.0}},
								},
							}},
						},
					}},
					{Name: "status", Gen: "const", Config: map[string]any{"value": "active"}},
					{Name: "tags", Gen: "list", Config: map[string]any{
						"min_len": 1,
						"max_len": 4,
						"item": map[string]any{"gen": "pick", "config": map[string]any{"values": []any{"admin", "beta", "premium", "internal", "vip"}}},
					}},
					{Name: "skills", Gen: "sample", Config: map[string]any{
						"values": []any{"Go", "Python", "Rust", "TypeScript", "Java", "C++", "Ruby", "Kotlin"},
						"min_n":  2,
						"max_n":  5,
					}},
					{Name: "contact_method", Gen: "one_of", Config: map[string]any{
						"generators": []any{
							map[string]any{"gen": "regex", "config": map[string]any{"pattern": `[a-z]{5,10}@(gmail|yahoo|outlook)\.com`}},
							map[string]any{"gen": "regex", "config": map[string]any{"pattern": `\+1-\d{3}-\d{3}-\d{4}`}},
						},
						"weights": []any{7.0, 3.0},
					}},
					{Name: "greeting", Gen: "template", Config: map[string]any{
						"tpl": "Welcome, employee #{id} from {department}!",
					}},
					{Name: "access_level", Gen: "switch", Config: map[string]any{
						"key": "department",
						"cases": map[string]any{
							"engineering": map[string]any{"gen": "const", "config": map[string]any{"value": "full"}},
							"sales":       map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
							"marketing":   map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
							"support":     map[string]any{"gen": "const", "config": map[string]any{"value": "limited"}},
						},
						"default": map[string]any{"gen": "const", "config": map[string]any{"value": "standard"}},
					}},
				},
			},
			{
				Name:  "Product",
				Count: 500,
				Fields: []apery.FieldSpec{
					{Name: "id", Gen: "seq"},
					{Name: "name", Gen: "regex", Config: map[string]any{"pattern": `[A-Z][a-z]{3,8} [A-Z][a-z]{2,6}`}},
					{Name: "price", Gen: "int", Config: map[string]any{"min": 100, "max": 99999}},
				},
			},
			{
				// 1:M — each User gets 1-5 Orders (driven_by)
				Name: "Order",
				DrivenBy: &apery.DrivenBy{
					Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 5,
				},
				Fields: []apery.FieldSpec{
					{Name: "order_id", Gen: "seq"},
					{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
						"entity": "Product", "field": "id",
					}},
					{Name: "quantity", Gen: "int", Config: map[string]any{"min": 1, "max": 10}},
				},
			},
			{
				// M:1 — Reviews reference Users (zipf) and Products (uniform)
				Name:  "Review",
				Count: 50_000,
				Fields: []apery.FieldSpec{
					{Name: "user_id", Gen: "rel_ref", Config: map[string]any{
						"entity": "User", "field": "id", "distribution": "zipf", "s": 1.5,
					}},
					{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
						"entity": "Product", "field": "id",
					}},
					{Name: "rating", Gen: "int", Config: map[string]any{"min": 1, "max": 5}},
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
