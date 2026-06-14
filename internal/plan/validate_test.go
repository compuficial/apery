package plan

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		plan        *Plan
		expectError bool
		errContains string
	}{
		{
			name:        "nil plan",
			plan:        nil,
			expectError: true,
			errContains: "plan is nil",
		},
		{
			name:        "no entities",
			plan:        &Plan{Seed: 42, Entities: []EntitySpec{}},
			expectError: true,
			errContains: "no entities defined",
		},
		{
			name: "valid simple plan",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid multi-entity plan",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
							{Name: "name", Gen: "pick", Config: map[string]any{"values": []any{"Alice", "Bob"}}},
						},
					},
					{
						Name:  "Order",
						Count: 100,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
							{Name: "amount", Gen: "int", Config: map[string]any{"min": 1, "max": 1000}},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "entity missing name",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
						},
					},
				},
			},
			expectError: true,
			errContains: "missing name",
		},
		{
			name: "entity count zero",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 0,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
						},
					},
				},
			},
			expectError: true,
			errContains: "exactly one of Count or DrivenBy",
		},
		{
			name: "entity count negative",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: -5,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
						},
					},
				},
			},
			expectError: true,
			errContains: "exactly one of Count or DrivenBy",
		},
		{
			name: "entity no fields",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:   "User",
						Count:  10,
						Fields: []FieldSpec{},
					},
				},
			},
			expectError: true,
			errContains: "no fields defined",
		},
		{
			name: "duplicate entity name",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
						},
					},
					{
						Name:  "User",
						Count: 5,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
						},
					},
				},
			},
			expectError: true,
			errContains: "duplicate entity name",
		},
		{
			name: "field missing name",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "", Gen: "seq"},
						},
					},
				},
			},
			expectError: true,
			errContains: "missing name",
		},
		{
			name: "field missing generator",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "id", Gen: ""},
						},
					},
				},
			},
			expectError: true,
			errContains: "missing generator",
		},
		{
			name: "duplicate field name",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{
						Name:  "User",
						Count: 10,
						Fields: []FieldSpec{
							{Name: "id", Gen: "seq"},
							{Name: "id", Gen: "uuid"},
						},
					},
				},
			},
			expectError: true,
			errContains: "duplicate field name",
		},
		// --- DrivenBy validation ---
		{
			name: "valid driven_by plan",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 5,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: false,
		},
		{
			name: "driven_by entity not declared before",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
				},
			},
			expectError: true,
			errContains: "not declared before",
		},
		{
			name: "driven_by field not in parent",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "email", As: "user_email", Min: 1, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "field 'email' does not exist",
		},
		{
			name: "driven_by as conflicts with child field",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "amount", Min: 1, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "conflicts with declared field",
		},
		{
			name: "driven_by min less than 1",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "user_id", Min: 0, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "min must be >= 1",
		},
		{
			name: "driven_by max less than min",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "user_id", Min: 5, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "max must be >= min",
		},
		{
			name: "count and driven_by both set",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", Count: 50, DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "exactly one of Count or DrivenBy",
		},
		{
			name: "neither count nor driven_by",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
				},
			},
			expectError: true,
			errContains: "exactly one of Count or DrivenBy",
		},
		{
			name: "self-referencing driven_by",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "parent_id", Min: 1, Max: 2,
					}, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
				},
			},
			expectError: true,
			errContains: "not declared before",
		},
		{
			name: "reserved config key",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{
						{Name: "id", Gen: "seq", Config: map[string]any{"_store": "bad"}},
					}},
				},
			},
			expectError: true,
			errContains: "reserved for internal use",
		},
		{
			name: "rel_ref entity not declared before",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Order", Count: 10, Fields: []FieldSpec{
						{Name: "user_id", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "id"}},
					}},
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
				},
			},
			expectError: true,
			errContains: "not declared before",
		},
		{
			name: "rel_ref field not in target entity",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", Count: 10, Fields: []FieldSpec{
						{Name: "user_id", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "email"}},
					}},
				},
			},
			expectError: true,
			errContains: "field 'email' does not exist",
		},
		{
			name: "valid rel_ref plan",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", Count: 50, Fields: []FieldSpec{
						{Name: "user_id", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "id"}},
					}},
				},
			},
			expectError: false,
		},
		{
			name: "unique feasibility exceeded",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Item", Count: 3, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Cart", DrivenBy: &DrivenBy{
						Entity: "Item", Field: "id", As: "item_id", Min: 1, Max: 5,
					}, Fields: []FieldSpec{
						{Name: "extra_item", Gen: "rel_ref", Config: map[string]any{
							"entity": "Item", "field": "id", "unique": true,
						}},
					}},
				},
			},
			expectError: true,
			errContains: "unique rel_ref",
		},
		// --- DrivenBy expose + index_as validation ---
		{
			name: "valid driven_by with expose and index_as",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Sub", Count: 10, Fields: []FieldSpec{
						{Name: "id", Gen: "seq"},
						{Name: "total", Gen: "int"},
						{Name: "start", Gen: "time"},
					}},
					{Name: "Recog", DrivenBy: &DrivenBy{
						Entity: "Sub", Field: "id", As: "sub_id", Min: 12, Max: 12,
						Expose: []ParentField{
							{Field: "total", As: "sub_total"},
							{Field: "start"}, // As defaults to "start"
						},
						IndexAs: "i",
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: false,
		},
		{
			name: "expose field not in parent",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Sub", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Recog", DrivenBy: &DrivenBy{
						Entity: "Sub", Field: "id", As: "sub_id", Min: 1, Max: 2,
						Expose: []ParentField{{Field: "nope"}},
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "expose field 'nope' does not exist",
		},
		{
			name: "expose alias conflicts with as",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Sub", Count: 10, Fields: []FieldSpec{
						{Name: "id", Gen: "seq"}, {Name: "total", Gen: "int"},
					}},
					{Name: "Recog", DrivenBy: &DrivenBy{
						Entity: "Sub", Field: "id", As: "sub_id", Min: 1, Max: 2,
						Expose: []ParentField{{Field: "total", As: "sub_id"}},
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "injects \"sub_id\" more than once",
		},
		{
			name: "index_as conflicts with declared field",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Sub", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Recog", DrivenBy: &DrivenBy{
						Entity: "Sub", Field: "id", As: "sub_id", Min: 1, Max: 2,
						IndexAs: "amount",
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "conflicts with declared field",
		},
		{
			name: "expose missing field key",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Sub", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Recog", DrivenBy: &DrivenBy{
						Entity: "Sub", Field: "id", As: "sub_id", Min: 1, Max: 2,
						Expose: []ParentField{{As: "x"}},
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
				},
			},
			expectError: true,
			errContains: "expose entry missing 'field'",
		},
		{
			name: "child reads exposed parent field via template",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "Sub", Count: 10, Fields: []FieldSpec{
						{Name: "id", Gen: "seq"}, {Name: "plan", Gen: "const", Config: map[string]any{"value": "pro"}},
					}},
					{Name: "Recog", DrivenBy: &DrivenBy{
						Entity: "Sub", Field: "id", As: "sub_id", Min: 1, Max: 2,
						Expose:  []ParentField{{Field: "plan"}},
						IndexAs: "i",
					}, Fields: []FieldSpec{
						{Name: "label", Gen: "template", Config: map[string]any{"tpl": "{plan}-{i}"}},
					}},
				},
			},
			expectError: false,
		},
		{
			name: "driven_by references parent As field",
			plan: &Plan{
				Seed: 42,
				Entities: []EntitySpec{
					{Name: "User", Count: 10, Fields: []FieldSpec{{Name: "id", Gen: "seq"}}},
					{Name: "Order", DrivenBy: &DrivenBy{
						Entity: "User", Field: "id", As: "user_id", Min: 1, Max: 3,
					}, Fields: []FieldSpec{{Name: "amount", Gen: "int"}}},
					{Name: "LineItem", DrivenBy: &DrivenBy{
						Entity: "Order", Field: "user_id", As: "order_user_id", Min: 1, Max: 2,
					}, Fields: []FieldSpec{{Name: "qty", Gen: "int"}}},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.plan)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
