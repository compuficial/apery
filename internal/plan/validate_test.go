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
			errContains: "count must be > 0",
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
			errContains: "count must be > 0",
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
