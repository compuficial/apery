package runtime

import "testing"

func TestMapEntityStore(t *testing.T) {
	store := newMapEntityStore()

	// Store and retrieve
	store.StoreColumn("User", "id", []any{int64(1), int64(2), int64(3)})
	col, ok := store.GetColumn("User", "id")
	if !ok {
		t.Fatal("expected column to exist")
	}
	if len(col) != 3 {
		t.Fatalf("expected 3 values, got %d", len(col))
	}
	if col[0] != int64(1) {
		t.Fatalf("expected 1, got %v", col[0])
	}

	// Missing entity
	_, ok = store.GetColumn("Order", "id")
	if ok {
		t.Fatal("expected missing column")
	}

	// Missing field
	_, ok = store.GetColumn("User", "email")
	if ok {
		t.Fatal("expected missing column")
	}
}
