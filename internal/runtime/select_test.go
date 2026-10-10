package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/compuficial/apery/internal/plan"
	"github.com/compuficial/apery/internal/writer"
)

// linesOf keeps the JSONL lines whose _entity is in names, in order.
func linesOf(t *testing.T, data []byte, names []string) []byte {
	t.Helper()
	var out bytes.Buffer
	for line := range bytes.Lines(data) {
		var rec struct {
			Entity string `json:"_entity"`
		}
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatal(err)
		}
		if slices.Contains(names, rec.Entity) {
			out.Write(line)
		}
	}
	return out.Bytes()
}

// A run limited to some entities writes exactly their lines of a full run,
// for every canonical plan, every single entity and every pair, under
// different workers and chunk sizes.
func TestWithEntities_MatchesFullRun(t *testing.T) {
	for _, tc := range canonicalPlans {
		full := runPlanWithOpts(t, tc.plan)
		var subsets [][]string
		for i, a := range tc.plan.Entities {
			subsets = append(subsets, []string{a.Name})
			for _, b := range tc.plan.Entities[i+1:] {
				subsets = append(subsets, []string{a.Name, b.Name})
			}
		}

		for i, names := range subsets {
			opts := []Option{WithEntities(names...)}
			if i%2 == 1 {
				opts = append(opts, WithWorkers(3), WithChunkSize(17))
			}
			got := runPlanWithOpts(t, tc.plan, opts...)
			if want := linesOf(t, full, names); !bytes.Equal(got, want) {
				t.Errorf("%s %v: %d bytes, want %d matching the full run", tc.name, names, len(got), len(want))
			}
		}
	}
}

func TestSelect_Dependencies(t *testing.T) {
	seq := []plan.FieldSpec{{Name: "id", Gen: "seq"}}
	ref := func(entity string) []plan.FieldSpec {
		return []plan.FieldSpec{{Name: "ref", Gen: "rel_ref", Config: map[string]any{"entity": entity, "field": "id"}}}
	}
	p := &plan.Plan{Entities: []plan.EntitySpec{
		{Name: "A", Count: 3, Fields: seq},
		{Name: "B", DrivenBy: &plan.DrivenBy{Entity: "A", Field: "id", As: "a_id", Min: 1, Max: 2}, Fields: seq},
		{Name: "C", Count: 3, Fields: ref("B")},
		{Name: "D", Count: 3, Fields: seq},
		{Name: "E", Count: 3, Fields: ref("A")},
	}}

	cases := []struct {
		names      []string
		want, deps []string
	}{
		{nil, []string{"A", "B", "C", "D", "E"}, nil},
		{[]string{"C"}, []string{"C"}, []string{"A", "B"}},
		{[]string{"E", "D"}, []string{"D", "E"}, []string{"A"}},
		{[]string{"B", "A"}, []string{"A", "B"}, nil},
	}
	for _, c := range cases {
		sel, err := Select(p, c.names)
		if err != nil {
			t.Fatalf("%v: %v", c.names, err)
		}
		if !slices.Equal(sel.Written, c.want) || !slices.Equal(sel.Dependencies, c.deps) {
			t.Errorf("%v: written %v, deps %v; want %v, %v", c.names, sel.Written, sel.Dependencies, c.want, c.deps)
		}
	}
}

func TestSelect_UnknownEntity(t *testing.T) {
	p := singleEntity(1, plan.FieldSpec{Name: "id", Gen: "seq"})
	_, err := Select(p, []string{"V", "Nope"})
	verr, ok := errors.AsType[*plan.ValidationError](err)
	if !ok || len(verr.Issues) != 2 {
		t.Fatalf("error = %v, want two issues", err)
	}
	if got := verr.Issues[0]; got.Code != "unknown_entity" || got.Hint != `did you mean "U"?` {
		t.Errorf("issue = %+v, want unknown_entity suggesting U", got)
	}
	if got := verr.Issues[1]; !strings.Contains(got.Hint, "entities are U") {
		t.Errorf("issue = %+v, want a hint listing the entities", got)
	}

	w := writer.NewJSONLWriterFromWriter(io.Discard)
	if err := New(w, WithEntities("Nope")).Run(t.Context(), p); err == nil {
		t.Error("Run accepted an unknown entity")
	}
}

// A dependency-only entity generates just the columns later entities read,
// and the fields those depend on; the rest never run.
func TestWithEntities_SkipsUnreadFields(t *testing.T) {
	p := &plan.Plan{Seed: 5, Entities: []plan.EntitySpec{
		{Name: "User", Count: 50, Fields: []plan.FieldSpec{
			{Name: "id", Gen: "seq"},
			{Name: "unread", Gen: "__test_counter"},
			{Name: "first", Gen: "pick", Config: map[string]any{"values": []any{"Ann", "Bo"}}},
			{Name: "handle", Gen: "template", Config: map[string]any{"tpl": "@{first}"}},
		}},
		{Name: "Post", Count: 20, Fields: []plan.FieldSpec{
			{Name: "author", Gen: "rel_ref", Config: map[string]any{"entity": "User", "field": "handle"}},
		}},
	}}

	before := generated.Load()
	got := runPlanWithOpts(t, p, WithEntities("Post"))
	if n := generated.Load() - before; n != 0 {
		t.Errorf("the unread field generated %d values, want 0", n)
	}
	if want := linesOf(t, runPlanWithOpts(t, p), []string{"Post"}); !bytes.Equal(got, want) {
		t.Errorf("Post differs from the full run:\n%s\nwant\n%s", got, want)
	}

	ents, err := compile(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := entitiesToRun(ents, map[string]bool{"Post": true})
	var kept []string
	for _, f := range run[0].fields {
		kept = append(kept, f.name)
	}
	if !slices.Equal(kept, []string{"first", "handle"}) {
		t.Errorf("User keeps %v, want [first handle]", kept)
	}
}
