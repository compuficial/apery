package rng

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"testing"
)

func TestDeriveDeterministic(t *testing.T) {
	seed := SeedFromInt64(12345)
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
	r := New(SeedFromInt64(1))
	for range 100 {
		val := r.IntRange(5, 5)
		if val != 5 {
			t.Fatalf("expected 5, got %d", val)
		}
	}
}

func TestFloatRangeBounds(t *testing.T) {
	r := New(SeedFromInt64(2))
	min, max := 1.5, 2.5
	for range 100 {
		val := r.FloatRange(min, max)
		if val < min || val > max {
			t.Fatalf("value out of range: %f", val)
		}
	}
}

func TestNormFloat64Finite(t *testing.T) {
	r := New(SeedFromInt64(3))
	val := r.NormFloat64()
	if val != val {
		t.Fatal("expected finite value, got NaN")
	}
}

func TestRead(t *testing.T) {
	r := New(SeedFromInt64(4))
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

// A range spanning all of int64 must not overflow the span computation.
func TestIntRangeFullSpan(t *testing.T) {
	r := New(SeedFromInt64(6))
	for range 100 {
		r.IntRange(math.MinInt64, math.MaxInt64)
	}
}

// Ranges wider than MaxFloat64 must still produce finite in-range values.
func TestFloatRangeHugeSpan(t *testing.T) {
	r := New(SeedFromInt64(7))
	spans := [][2]float64{{-math.MaxFloat64, math.MaxFloat64}, {0, math.MaxFloat64}, {-1e308, 1e308}}
	for _, s := range spans {
		for range 100 {
			v := r.FloatRange(s[0], s[1])
			if math.IsInf(v, 0) || math.IsNaN(v) || v < s[0] || v > s[1] {
				t.Fatalf("FloatRange(%g, %g) = %g", s[0], s[1], v)
			}
		}
	}
}

// Reseed must reproduce exactly the stream New would give for that seed.
func TestReseedMatchesNew(t *testing.T) {
	r := New(SeedFromInt64(1))
	r.Float64()
	r.Reseed(SeedFromInt64(99))
	fresh := New(SeedFromInt64(99))
	if r.GetSeed() != fresh.GetSeed() {
		t.Fatalf("seed = %d, want %d", r.GetSeed(), fresh.GetSeed())
	}
	for i := range 50 {
		if a, b := r.IntRange(0, 1<<40), fresh.IntRange(0, 1<<40); a != b {
			t.Fatalf("draw %d: reseeded %d != fresh %d", i, a, b)
		}
	}
}

// Read fills 8 bytes per 64-bit draw, little-endian.
func TestReadUsesWholeWords(t *testing.T) {
	buf := make([]byte, 12)
	New(SeedFromInt64(8)).Read(buf)

	src := rand.New(rand.NewPCG(uint64(SeedFromInt64(8)), 0))
	want := make([]byte, 16)
	binary.LittleEndian.PutUint64(want, src.Uint64())
	binary.LittleEndian.PutUint64(want[8:], src.Uint64())
	if !bytes.Equal(buf, want[:12]) {
		t.Fatalf("Read = %x, want %x", buf, want[:12])
	}
}

// The cached Zipf sampler must match the stdlib sampler draw for draw, both
// when the Rng is reused (reseeded) and when a new Rng arrives.
func TestZipfMatchesStdlib(t *testing.T) {
	z := NewZipf(1.5, 1, 100)
	r := New(0)
	for i := range int64(200) {
		seed := DeriveIndex(SeedFromInt64(9), i)
		r.Reseed(seed)
		want := rand.NewZipf(rand.New(rand.NewPCG(uint64(seed), 0)), 1.5, 1, 100).Uint64()
		if got := z.Uint64(r); got != want {
			t.Fatalf("draw %d: got %d, want %d", i, got, want)
		}
		if got := z.Uint64(New(seed)); got != want {
			t.Fatalf("draw %d with new Rng: got %d, want %d", i, got, want)
		}
	}
}

// Float output must be bit-identical on every platform. arm64 (and amd64
// with GOAMD64=v3) may fuse a*b+c into one FMA, and the stdlib's Log and Cos
// round differently per architecture. The digests were recorded on amd64
// (GOAMD64=v1) and must hold wherever CI runs.
func TestFloatDigestsArePortable(t *testing.T) {
	cases := []struct {
		name string
		draw func(r *Rng) float64
		want uint64
	}{
		{"FloatRange", func(r *Rng) float64 { return r.FloatRange(-100, 100) }, 0x595a90a762f6dc6f},
		{"FloatRangeHuge", func(r *Rng) float64 { return r.FloatRange(-math.MaxFloat64, math.MaxFloat64) }, 0x932b0cdf487b42a2},
		{"Normal", func(r *Rng) float64 { return r.Normal(37, 0.5) }, 0x6f5491f3cc05cc22},
	}
	for _, c := range cases {
		r := New(SeedFromInt64(11))
		h := fnv.New64a()
		var buf [8]byte
		for range 100_000 {
			binary.LittleEndian.PutUint64(buf[:], math.Float64bits(c.draw(r)))
			h.Write(buf[:])
		}
		if got := h.Sum64(); got != c.want {
			t.Errorf("%s digest = %#x, want %#x", c.name, got, c.want)
		}
	}
}
