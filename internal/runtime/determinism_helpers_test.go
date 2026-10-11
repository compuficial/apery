package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/writer"
)

var update = flag.Bool("update", false, "update golden files")

const goldenDir = "testdata/golden"
const goldenSeed int64 = 12345

var scalarPlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Scalars", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq", Config: map[string]any{"start": 1, "step": 1}},
			{Name: "counter", Gen: "int", Config: map[string]any{"min": 0, "max": 1000}},
			{Name: "score", Gen: "float", Config: map[string]any{"min": -100.0, "max": 100.0}},
			{Name: "active", Gen: "bool", Config: map[string]any{}},
			{Name: "color", Gen: "pick", Config: map[string]any{
				"values":  []any{"red", "green", "blue"},
				"weights": []any{5.0, 3.0, 2.0},
			}},
			{Name: "tag", Gen: "const", Config: map[string]any{"value": "v1"}},
			{Name: "uid", Gen: "uuid", Config: map[string]any{}},
			{Name: "ulid", Gen: "ulid", Config: map[string]any{}},
			{Name: "ts", Gen: "time", Config: map[string]any{
				"start": "2020-01-01T00:00:00Z", "end": "2025-12-31T00:00:00Z",
			}},
			{Name: "code", Gen: "regex", Config: map[string]any{"pattern": "[A-Z]{3}-[0-9]{4}"}},
			{Name: "iq", Gen: "normal_int", Config: map[string]any{
				"mu": 100.0, "sigma": 15.0, "clamp_min": 50, "clamp_max": 150,
			}},
			{Name: "temp", Gen: "normal_float", Config: map[string]any{
				"mu": 37.0, "sigma": 0.5,
			}},
			{Name: "rank", Gen: "zipf", Config: map[string]any{"s": 1.5, "v": 1, "imax": 100}},
			{Name: "local_ts", Gen: "time", Config: map[string]any{
				"start": "2024-01-01", "end": "2024-12-31", "tz": "America/New_York",
			}},
			{Name: "nickname", Gen: "pick", Config: map[string]any{
				"values": []any{"Ace", "Bee", "Cy"},
			}, NullRate: 0.25},
			{Name: "price", Gen: "lognormal", Config: map[string]any{"mu": 3.0, "sigma": 0.5}},
			{Name: "price_cents", Gen: "lognormal", Config: map[string]any{"mu": 8.0, "sigma": 0.5, "round": true}},
			{Name: "wait", Gen: "exponential", Config: map[string]any{"mean": 300.0, "clamp_max": 3600.0}},
			{Name: "visits", Gen: "poisson", Config: map[string]any{"lambda": 3.5}},
			{Name: "views", Gen: "poisson", Config: map[string]any{"lambda": 250.0}},
			{Name: "logged_at", Gen: "time_seq", Config: map[string]any{
				"start": "2024-06-01T00:00:00Z", "step": "90s", "jitter": "30s",
			}},
		}},
	},
}

var compositePlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Composites", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "address", Gen: "object", Config: map[string]any{
				"fields": map[string]any{
					"city": map[string]any{"gen": "pick", "config": map[string]any{
						"values": []any{"NYC", "LA", "CHI"},
					}},
					"zip": map[string]any{"gen": "int", "config": map[string]any{
						"min": 10000, "max": 99999,
					}},
				},
			}},
			{Name: "tags", Gen: "list", Config: map[string]any{
				"item": map[string]any{"gen": "pick", "config": map[string]any{
					"values": []any{"alpha", "beta", "gamma"},
				}},
				"min_len": 1,
				"max_len": 4,
			}},
			{Name: "skills", Gen: "sample", Config: map[string]any{
				"values": []any{"Go", "Python", "Rust", "Java", "C++"},
				"min_n":  1,
				"max_n":  3,
			}},
			{Name: "value", Gen: "one_of", Config: map[string]any{
				"generators": []any{
					map[string]any{"gen": "int", "config": map[string]any{"min": 1, "max": 100}},
					map[string]any{"gen": "float", "config": map[string]any{"min": 0.0, "max": 1.0}},
				},
				"weights": []any{7.0, 3.0},
			}},
			// Composite children with null_rate: an object field and the
			// items of a nested list.
			{Name: "contact", Gen: "object", Config: map[string]any{
				"fields": map[string]any{
					"email": map[string]any{"gen": "regex", "config": map[string]any{
						"pattern": "[a-z]{5}@example\\.com",
					}, "null_rate": 0.4},
					"phones": map[string]any{"gen": "list", "config": map[string]any{
						"item": map[string]any{"gen": "regex", "config": map[string]any{
							"pattern": "555-[0-9]{4}",
						}, "null_rate": 0.2},
						"min_len": 1,
						"max_len": 3,
					}},
				},
			}},
		}},
	},
}

var rowAwarePlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "RowAware", Count: 10000, Fields: []plan.FieldSpec{
			{Name: "active", Gen: "bool", Config: map[string]any{}},
			{Name: "department", Gen: "pick", Config: map[string]any{
				"values": []any{"engineering", "sales", "support"},
			}},
			{Name: "score", Gen: "int", Config: map[string]any{"min": 1, "max": 100}},
			{Name: "greeting", Gen: "template", Config: map[string]any{
				"tpl": "Dept {department}, score={score}",
			}},
			{Name: "access", Gen: "switch", Config: map[string]any{
				"key": "department",
				"cases": map[string]any{
					"engineering": map[string]any{"gen": "const", "config": map[string]any{"value": "full"}},
					"sales":       map[string]any{"gen": "const", "config": map[string]any{"value": "read-only"}},
				},
				"default": map[string]any{"gen": "const", "config": map[string]any{"value": "limited"}},
			}},
			// Nulls flow into row-aware fields: expr gives null, template "".
			{Name: "bonus", Gen: "int", Config: map[string]any{"min": 1, "max": 10}, NullRate: 0.3},
			{Name: "bonus_score", Gen: "expr", Config: map[string]any{"expr": "{bonus} * {score}"}},
			{Name: "bonus_note", Gen: "template", Config: map[string]any{"tpl": "bonus={bonus}"}},
			{Name: "perk", Gen: "switch", Config: map[string]any{
				"key": "department",
				"cases": map[string]any{
					"engineering": map[string]any{"gen": "template", "config": map[string]any{
						"tpl": "{department} laptop",
					}, "null_rate": 0.5},
				},
				"default": map[string]any{"gen": "const", "config": map[string]any{"value": "none"}},
			}},
		}},
	},
}

var relationalPlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Customer", Count: 100, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "name", Gen: "regex", Config: map[string]any{"pattern": "[A-Z][a-z]{4,8}"}},
		}},
		{Name: "Product", Count: 50, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "price", Gen: "int", Config: map[string]any{"min": 100, "max": 9999}},
		}},
		{Name: "Order", DrivenBy: &plan.DrivenBy{
			Entity: "Customer", Field: "id", As: "customer_id", Min: 1, Max: 5,
		}, Fields: []plan.FieldSpec{
			{Name: "order_id", Gen: "seq"},
			{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Product", "field": "id",
			}},
			{Name: "amount", Gen: "int", Config: map[string]any{"min": 1, "max": 10}},
			{Name: "placed_at", Gen: "time_seq", Config: map[string]any{
				"start": "2024-01-01", "step": "1h", "jitter": "59m",
			}},
		}},
		{Name: "Review", Count: 200, Fields: []plan.FieldSpec{
			{Name: "customer_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Customer", "field": "id", "distribution": "zipf", "s": 1.5,
			}},
			{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Product", "field": "id",
			}},
			{Name: "rating", Gen: "int", Config: map[string]any{"min": 1, "max": 5}},
			{Name: "referrer_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Customer", "field": "id",
			}, NullRate: 0.4},
		}},
		// Unique draws: one batch per standalone entity, one per parent for
		// driven_by. Both must be independent of chunk size and workers.
		{Name: "Featured", Count: 50, Fields: []plan.FieldSpec{
			{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Product", "field": "id", "unique": true,
			}},
		}},
		{Name: "Tagging", DrivenBy: &plan.DrivenBy{
			Entity: "Customer", Field: "id", As: "customer_id", Min: 1, Max: 4,
		}, Fields: []plan.FieldSpec{
			{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Product", "field": "id", "unique": true, "distribution": "zipf",
			}},
		}},
		// Exponential gaps build on the row before: each entity runs in order
		// on one worker, standalone and driven_by (beside unique draws).
		{Name: "Visit", Count: 300, Fields: []plan.FieldSpec{
			{Name: "customer_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Customer", "field": "id",
			}},
			{Name: "seen_at", Gen: "time_seq", Config: map[string]any{
				"start": "2024-01-01", "step": "10m", "gaps": "exponential",
			}},
		}},
		{Name: "Bookmark", DrivenBy: &plan.DrivenBy{
			Entity: "Customer", Field: "id", As: "customer_id", Min: 1, Max: 3,
		}, Fields: []plan.FieldSpec{
			{Name: "product_id", Gen: "rel_ref", Config: map[string]any{
				"entity": "Product", "field": "id", "unique": true,
			}},
			{Name: "saved_at", Gen: "time_seq", Config: map[string]any{
				"start": "2024-01-01", "step": "1h", "gaps": "exponential",
			}},
		}},
	},
}

// dependentPlan exercises cross-row dependent values (issue #1): exposed parent
// columns, the child index, and the expr + date_offset generators. Count is
// small so recognition rows land in the 10-line golden spot-check.
var dependentPlan = plan.Plan{
	Seed: goldenSeed,
	Entities: []plan.EntitySpec{
		{Name: "Subscription", Count: 6, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "start_date", Gen: "time", Config: map[string]any{
				"start": "2024-01-01", "end": "2024-12-31", "format": "2006-01-02",
			}},
			{Name: "total", Gen: "int", Config: map[string]any{"min": 1200, "max": 120000}},
		}},
		{Name: "Recognition", DrivenBy: &plan.DrivenBy{
			Entity: "Subscription", Field: "id", As: "subscription_id", Min: 12, Max: 12,
			Expose: []plan.ParentField{
				{Field: "start_date", As: "sub_start"},
				{Field: "total", As: "sub_total"},
			},
			IndexAs: "event_index",
		}, Fields: []plan.FieldSpec{
			{Name: "recognized_at", Gen: "date_offset", Config: map[string]any{
				"base": "{sub_start}", "amount": "{event_index}", "unit": "months", "format": "2006-01-02",
			}},
			{Name: "amount", Gen: "expr", Config: map[string]any{"expr": "{sub_total} / 12"}},
			{Name: "amount_cents", Gen: "expr", Config: map[string]any{"expr": "round({sub_total} / 12)"}},
		}},
	},
}

// canonicalPlans is the ordered set of plans used by golden and stress tests.
var canonicalPlans = []struct {
	name string
	plan *plan.Plan
}{
	{"scalar", &scalarPlan},
	{"composite", &compositePlan},
	{"row_aware", &rowAwarePlan},
	{"relational", &relationalPlan},
	{"dependent", &dependentPlan},
}

// runPlanWithOpts runs a plan into a buffer and returns the raw JSONL output bytes.
// Note: Executor.Run() auto-closes the writer via defer, so we don't call w.Close() here.
func runPlanWithOpts(t *testing.T, p *plan.Plan, opts ...Option) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := writer.NewJSONLWriterFromWriter(&buf)
	e := New(w, opts...)
	if err := e.Run(t.Context(), p); err != nil {
		t.Fatalf("executor.Run: %v", err)
	}
	return buf.Bytes()
}

// dumpOutput saves data for debugging a failure and returns where. It uses
// the system temp dir because t.TempDir is removed when the test ends.
func dumpOutput(name string, data []byte) string {
	f, err := os.CreateTemp("", "apery-"+name+"-*.jsonl")
	if err != nil {
		return "(not saved: " + err.Error() + ")"
	}
	_, err = f.Write(data)
	if err = errors.Join(err, f.Close()); err != nil {
		return "(not saved: " + err.Error() + ")"
	}
	return f.Name()
}

// computeDigest returns the SHA-256 hex string of the given bytes.
func computeDigest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// firstNLines returns the first n lines from data (including trailing newlines).
func firstNLines(data []byte, n int) []byte {
	var count int
	for i, b := range data {
		if b == '\n' {
			count++
			if count == n {
				return data[:i+1]
			}
		}
	}
	return data
}

// writeGolden writes digest and spot-check files to the golden dir.
func writeGolden(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}

	digest := computeDigest(data)
	digestPath := filepath.Join(goldenDir, name+".digest")
	if err := os.WriteFile(digestPath, []byte(digest+"\n"), 0644); err != nil {
		t.Fatalf("write digest: %v", err)
	}

	spotPath := filepath.Join(goldenDir, name+".jsonl")
	if err := os.WriteFile(spotPath, firstNLines(data, 10), 0644); err != nil {
		t.Fatalf("write spot-check: %v", err)
	}

	t.Logf("updated golden files for %s (digest: %s)", name, digest[:16])
}

// compareGolden reads golden files and compares against actual data.
func compareGolden(t *testing.T, name string, data []byte) {
	t.Helper()

	digestPath := filepath.Join(goldenDir, name+".digest")
	wantDigestBytes, err := os.ReadFile(digestPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file not found: %s — run with -update to generate", digestPath)
		}
		t.Fatalf("read digest: %v", err)
	}
	wantDigest := bytes.TrimSpace(wantDigestBytes)
	gotDigest := computeDigest(data)

	if gotDigest != string(wantDigest) {
		t.Fatalf("digest mismatch for %s:\n  want: %s\n  got:  %s\n  actual output dumped to: %s",
			name, wantDigest, gotDigest, dumpOutput(name+".actual", data))
	}

	spotPath := filepath.Join(goldenDir, name+".jsonl")
	wantSpot, err := os.ReadFile(spotPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file not found: %s — run with -update to generate", spotPath)
		}
		t.Fatalf("read spot-check: %v", err)
	}

	gotSpot := firstNLines(data, 10)
	if !bytes.Equal(gotSpot, wantSpot) {
		t.Errorf("spot-check mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s",
			name, wantSpot, gotSpot)
	}
}
