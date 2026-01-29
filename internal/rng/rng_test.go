package rng

import (
	"bytes"
	"testing"
)

func TestDeriveDeterministic(t *testing.T) {
	seed := int64(12345)
	label := "entity:user"

	a := Derive(seed, label)
	b := Derive(seed, label)
	if a != b {
		t.Fatalf("expected deterministic derive, got %d and %d", a, b)
	}

	if Derive(seed, "other") == a {
		t.Fatal("expected different label to produce different seed")
	}
}

func TestIntRangeBounds(t *testing.T) {
	r := New(1)
	for i := 0; i < 100; i++ {
		val := r.IntRange(5, 5)
		if val != 5 {
			t.Fatalf("expected 5, got %d", val)
		}
	}
}

func TestFloatRangeBounds(t *testing.T) {
	r := New(2)
	min, max := 1.5, 2.5
	for i := 0; i < 100; i++ {
		val := r.FloatRange(min, max)
		if val < min || val > max {
			t.Fatalf("value out of range: %f", val)
		}
	}
}

func TestNormFloat64Finite(t *testing.T) {
	r := New(3)
	val := r.NormFloat64()
	if val != val {
		t.Fatal("expected finite value, got NaN")
	}
}

func TestRead(t *testing.T) {
	r := New(4)
	buf := make([]byte, 32)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != len(buf) {
		t.Fatalf("expected %d bytes, got %d", len(buf), n)
	}
	if bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("expected non-zero bytes")
	}
}

func TestNewZipf(t *testing.T) {
	r := New(5)
	z := r.NewZipf(1.1, 1, 10)
	if z == nil {
		t.Fatal("expected zipf generator")
	}
}
