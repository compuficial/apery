// Package rng provides deterministic random number generation with hierarchical seed derivation.
//
// The Derive function creates child seeds from parent seeds using FNV-1a hashing,
// enabling reproducible randomness across nested contexts (entity → field → row).
// This ensures that the same plan with the same root seed always produces identical
// output, which is critical for Apery's determinism guarantees.
package rng

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
)

// Rng is a seeded random number generator
type Rng struct {
	src  *rand.PCG
	r    *rand.Rand
	seed Seed
}

// Seed represents a deterministic RNG seed.
type Seed uint64

// SeedFromInt64 converts an int64 seed into a Seed.
func SeedFromInt64(value int64) Seed {
	return Seed(uint64(value))
}

// New creates a new RNG with a seed.
func New(seed Seed) *Rng {
	src := rand.NewPCG(uint64(seed), 0)
	return &Rng{src: src, r: rand.New(src), seed: seed}
}

// Reseed restarts the RNG at seed, giving the same stream as New(seed)
// without allocating. Hot loops reuse one Rng per field this way.
func (r *Rng) Reseed(seed Seed) {
	r.src.Seed(uint64(seed), 0)
	r.seed = seed
}

// GetSeed returns rng seed
func (r *Rng) GetSeed() Seed {
	return r.seed
}

// Intn returns a random int in [0,n)
func (r *Rng) Intn(n int) int {
	return r.r.IntN(n)
}

// IntRange returns a random int64 in [min, max]. The span is computed in
// uint64 so ranges covering all of int64 don't overflow.
func (r *Rng) IntRange(min int64, max int64) int64 {
	span := uint64(max - min)
	if span == math.MaxUint64 {
		return int64(r.r.Uint64())
	}
	return min + int64(r.r.Uint64N(span+1))
}

// Float64 returns a random float in [0,1.0)
func (r *Rng) Float64() float64 {
	return r.r.Float64()
}

// FloatRange returns a random float64 in [min, max]
func (r *Rng) FloatRange(min float64, max float64) float64 {
	hi := math.Nextafter(max, math.Inf(1))
	if math.IsInf(hi, 1) {
		hi = max
	}

	// Ranges wider than MaxFloat64 interpolate without forming the width,
	// which would overflow to +Inf. float64() rounds each product so no
	// platform fuses it into an FMA (see Normal).
	width := hi - min
	u := r.r.Float64()
	if math.IsInf(width, 1) {
		return float64(min*(1-u)) + float64(hi*u)
	}
	return min + float64(u*width)
}

// Derive creates a child seed from parent + label.
func Derive(parent Seed, label string) Seed {
	hf := fnv.New64a()
	hf.Write([]byte(label))
	return Seed(mix64(uint64(parent) ^ hf.Sum64()))
}

// DeriveIndex creates a child seed from parent + index.
func DeriveIndex(parent Seed, index int64) Seed {
	return Seed(mix64(uint64(parent) ^ uint64(index)))
}

func mix64(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

// NormFloat64 returns a normally distributed float64 with mean 0 and stddev 1
// using the Box-Muller transform. It uses portable Log and Cos so the result
// is identical on every architecture.
func (r *Rng) NormFloat64() float64 {
	u1 := r.Float64()
	for u1 == 0 { // log(0) would yield an infinite sample
		u1 = r.Float64()
	}
	u2 := r.Float64()
	return math.Sqrt(-2*portableLog(u1)) * portableCos(2*math.Pi*u2)
}

// Normal returns a normally distributed float64 with mean mu and stddev
// sigma. The float64() conversion forces rounding of the product: Go may
// otherwise fuse a*b+c into one FMA on arm64 (or amd64 with GOAMD64=v3),
// which changes the last bit and breaks cross-platform determinism.
func (r *Rng) Normal(mu, sigma float64) float64 {
	return mu + float64(sigma*r.NormFloat64())
}

// Read fills p with random bytes, implementing io.Reader. Each 64-bit draw
// supplies 8 bytes, little-endian.
func (r *Rng) Read(p []byte) (int, error) {
	for i := 0; i < len(p); i += 8 {
		word := r.r.Uint64()
		for j := i; j < len(p) && j < i+8; j++ {
			p[j] = byte(word)
			word >>= 8
		}
	}

	return len(p), nil
}

// Zipf samples a Zipf distribution with skewness s (> 1), offset v (>= 1),
// and maximum value imax. The stdlib sampler precomputes constants and binds
// to one source, so it is rebuilt only when a different Rng is passed in;
// callers that reseed one Rng per row pay that cost once. Not safe for
// concurrent use.
type Zipf struct {
	s, v    float64
	imax    uint64
	src     *rand.Rand
	sampler *rand.Zipf
}

// NewZipf creates a Zipf sampler.
func NewZipf(s float64, v float64, imax uint64) *Zipf {
	return &Zipf{s: s, v: v, imax: imax}
}

// Uint64 draws one value in [0, imax] from r.
func (z *Zipf) Uint64(r *Rng) uint64 {
	if z.src != r.r {
		z.src = r.r
		z.sampler = rand.NewZipf(r.r, z.s, z.v, z.imax)
	}
	return z.sampler.Uint64()
}
