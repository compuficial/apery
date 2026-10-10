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
	if _, err := New(SeedFromInt64(8)).Read(buf); err != nil {
		t.Fatal(err)
	}

	src := rand.New(rand.NewPCG(uint64(SeedFromInt64(8)), 0))
	want := make([]byte, 16)
	binary.LittleEndian.PutUint64(want, src.Uint64())
	binary.LittleEndian.PutUint64(want[8:], src.Uint64())
	if !bytes.Equal(buf, want[:12]) {
		t.Fatalf("Read = %x, want %x", buf, want[:12])
	}
}

// In the reference build the sampler must match math/rand/v2's draw for
// draw, so output is unchanged from v1.2.0, which used it. Elsewhere the
// stdlib itself rounds differently; TestDigestsArePortable pins the
// reference results there.
func TestZipfMatchesStdlib(t *testing.T) {
	if !isReferenceBuild() {
		t.Skip("the stdlib sampler is the reference only on amd64 with GOAMD64=v1")
	}
	params := rand.New(rand.NewPCG(3, 4))
	imaxes := []uint64{0, 1, 10, 1000, 1_000_000, 1 << 40, 1 << 62}
	for i := range 210 {
		s, v, imax := 1.001+params.Float64()*4, 1+params.Float64()*100, imaxes[i%len(imaxes)]
		seed := DeriveIndex(SeedFromInt64(9), int64(i))
		z, r := NewZipf(s, v, imax), New(seed)
		want := rand.NewZipf(rand.New(rand.NewPCG(uint64(seed), 0)), s, v, imax)
		for j := range 2000 {
			if got, w := z.Uint64(r), want.Uint64(); got != w {
				t.Fatalf("s=%v v=%v imax=%d draw %d: got %d, want %d", s, v, imax, j, got, w)
			}
		}
	}
}

// Draws stay in [0, imax] at the edges of the parameter space.
func TestZipfStaysInRange(t *testing.T) {
	cases := []struct {
		s, v float64
		imax uint64
	}{
		{1.0001, 1, 1 << 62}, {1.0001, 1, 10}, {1000, 1, 100}, {1.5, 1e9, 1000},
		{2, 1, 0}, {1.1, 1, math.MaxUint32}, {4, 50, 1 << 40},
	}
	r := New(SeedFromInt64(4))
	for _, c := range cases {
		z := NewZipf(c.s, c.v, c.imax)
		for range 20_000 {
			if k := z.Uint64(r); k > c.imax {
				t.Fatalf("Zipf(s=%v, v=%v, imax=%d) drew %d", c.s, c.v, c.imax, k)
			}
		}
	}
}

// Output must be bit-identical on every platform. arm64 (and amd64 with
// GOAMD64=v3) may fuse a*b+c into one FMA, and the stdlib's Exp, Log, and Cos
// round differently per architecture. The digests were recorded on amd64
// (GOAMD64=v1) and must hold wherever CI runs. Draws hide most one-ulp drift
// in rounding, so the sampler's float internals are pinned too. The Zipf
// draw digests match the stdlib sampler that v1.2.0 used and must never
// change.
func TestDigestsArePortable(t *testing.T) {
	z := NewZipf(1.1, 1, 1_000_000)
	ties := expTies()
	cases := []struct {
		name string
		draw func(r *Rng) float64
		want uint64
	}{
		{"FloatRange", func(r *Rng) float64 { return r.FloatRange(-100, 100) }, 0x595a90a762f6dc6f},
		{"FloatRangeHuge", func(r *Rng) float64 { return r.FloatRange(-math.MaxFloat64, math.MaxFloat64) }, 0x932b0cdf487b42a2},
		{"Normal", func(r *Rng) float64 { return r.Normal(37, 0.5) }, 0x6f5491f3cc05cc22},

		{"Exp", func(r *Rng) float64 { return portableExp(float64(r.Float64()*1460) - 750) }, 0x5b8c076787c7c027},
		{"ExpTies", func(r *Rng) float64 { return portableExp(ties[r.Intn(len(ties))]) }, 0x49b05444f7ad36f2},
		{"Log", func(r *Rng) float64 { return portableLog(math.Float64frombits(r.r.Uint64() >> 1)) }, 0x4606b1b7fbca835c},
		{"Cos", func(r *Rng) float64 { return portableCos(float64(r.Float64() * 1000)) }, 0x4a2fe60b546fba39},
		{"LogGamma", func(r *Rng) float64 { return logGamma(1 + float64(r.Float64()*1e6)) }, 0xb8a1e1e752bd6df0},
		{"LogGammaShifted", func(r *Rng) float64 { return logGamma(1 + float64(r.Float64()*7)) }, 0x80467f1eac0b342},

		{"LogNormal", func(r *Rng) float64 { return r.LogNormal(3, 0.5) }, 0x54737f37a9166cb4},
		{"Exponential", func(r *Rng) float64 { return r.Exponential(300) }, 0x26b84e2ec5244deb},
		{"PoissonMultiply", func(r *Rng) float64 { return float64(r.Poisson(3.5)) }, 0x172f0932ec2d3057},
		{"PoissonSwitch", func(r *Rng) float64 { return float64(r.Poisson(10)) }, 0xa4c144cef58e5840},
		{"PoissonPTRS", func(r *Rng) float64 { return float64(r.Poisson(50)) }, 0x7e05ecc9348eae5c},
		{"PoissonHuge", func(r *Rng) float64 { return float64(r.Poisson(1e9)) }, 0x403bbdd83b59581f},

		{"ZipfH", func(r *Rng) float64 { return z.h(float64(r.Float64() * 1e6)) }, 0xb30157e1a19b6288},
		{"ZipfInvertUr", func(r *Rng) float64 { ur, _ := z.invert(r.Float64()); return ur }, 0x145010440d523d9},
		{"ZipfInvertX", func(r *Rng) float64 { _, x := z.invert(r.Float64()); return x }, 0xc561c70f48247dda},
		{"ZipfBound", func(r *Rng) float64 { return z.bound(float64(r.Intn(1_000_001))) }, 0xe2867e6705b5024},
		{"ZipfSpan", func(r *Rng) float64 { return randomZipf(r).span }, 0x8eee4f53ab0a752},
		{"ZipfSqueeze", func(r *Rng) float64 { return randomZipf(r).squeeze }, 0x6de85bf5cc78e29},

		{"ZipfDefault", zipfDraws(1.1, 1, 100), 0xda36309eb0150c46},
		{"ZipfRelRef", zipfDraws(1.5, 1, 100), 0x47d0cac9ee657539},
		{"ZipfOffset", zipfDraws(2, 10, 1000), 0x448452da107ab89e},
		{"ZipfHuge", zipfDraws(1.01, 1, 1<<40), 0x35598dda0cab55fe},
		{"ZipfSteep", zipfDraws(8, 1, 100), 0xa35cc284f948abf8},
		{"ZipfUnderflow", zipfDraws(200, 1, 1_000_000), 0xe3dbd3f783edc725},
		{"ZipfLargeV", zipfDraws(3, 1000, 10), 0xdb0c108d5ffdc3f},
		{"ZipfSingle", zipfDraws(1.5, 1, 0), 0xe3dbd3f783edc725},
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

// randomZipf returns a sampler with parameters drawn from r.
func randomZipf(r *Rng) *Zipf {
	return NewZipf(1.001+float64(r.Float64()*4), 1+float64(r.Float64()*100), uint64(r.IntRange(0, 1<<40)))
}

// zipfDraws returns a draw func over one Zipf sampler.
func zipfDraws(s, v float64, imax uint64) func(r *Rng) float64 {
	z := NewZipf(s, v, imax)
	return func(r *Rng) float64 { return float64(z.Uint64(r)) }
}
