package suggest

import "testing"

func TestClosest(t *testing.T) {
	candidates := []string{"min", "max", "Order", "OrderLine"}
	cases := map[string]string{
		"mni":        "min",   // transposition counts as two edits
		"maxx":       "max",   // one insertion
		"ORDER":      "",      // case matters: four edits from "Order"
		"Ordr":       "Order", // one deletion
		"OrderLines": "OrderLine",
		"customer":   "", // nothing close
		"":           "", // three edits from the shortest candidate
	}
	for name, want := range cases {
		if got := Closest(name, candidates); got != want {
			t.Errorf("Closest(%q) = %q, want %q", name, got, want)
		}
	}
	if got := Closest("mox", []string{"max", "mix"}); got != "max" {
		t.Errorf("tie: Closest = %q, want the first candidate", got)
	}
}
