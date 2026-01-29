package writer

import "testing"

func TestOrderedMapKeysAndGet(t *testing.T) {
	record := NewOrderedMap()
	record.Set("a", 1)
	record.Set("b", 2)

	keys := record.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Fatalf("unexpected keys: %#v", keys)
	}

	val, ok := record.Get("a")
	if !ok || val != 1 {
		t.Fatalf("expected value 1 for key a, got %v", val)
	}
}

func TestOrderedMapCloneAndPrepend(t *testing.T) {
	record := NewOrderedMap()
	record.Set("id", 1)
	record.Set("name", "alice")

	clone := record.Clone()
	clone.Prepend("_entity", "User")

	if _, ok := record.Get("_entity"); ok {
		t.Fatal("expected original record to remain unchanged")
	}

	keys := clone.Keys()
	if len(keys) != 3 || keys[0] != "_entity" {
		t.Fatalf("unexpected keys after prepend: %#v", keys)
	}
}
