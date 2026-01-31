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
	r *rand.Rand
}

// Seed represents a deterministic RNG seed.
type Seed uint64

// SeedFromInt64 converts an int64 seed into a Seed.
func SeedFromInt64(value int64) Seed {
	return Seed(uint64(value))
}

// New creates a new RNG with a seed.
func New(seed Seed) *Rng {
	return &Rng{r: rand.New(rand.NewPCG(uint64(seed), 0))}
}

// Intn returns a random int in [0,n)
func (r *Rng) Intn(n int) int {
	return r.r.IntN(n)
}

// IntRange returns a random int64 in [min, max]
func (r *Rng) IntRange(min int64, max int64) int64 {
	return r.r.Int64N(max-min+1) + min
}

// Float64 returns a random float in [0,1.0)
func (r *Rng) Float64() float64 {
	return r.r.Float64()
}

// FloatRange returns a random float64 in [min, max]
func (r *Rng) FloatRange(min float64, max float64) float64 {
	return min + r.r.Float64()*(math.Nextafter(max, math.Inf(1))-min)
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
// using the Box-Muller transform
func (r *Rng) NormFloat64() float64 {
	u1 := r.Float64()
	u2 := r.Float64()
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

// Read fills p with random bytes, implementing io.Reader
func (r *Rng) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r.Intn(256))
	}

	return len(p), nil
}

// NewZipf creates a Zipf generator with skewness s, offset v, and max value imax.
// s must be > 1, v must be >= 1
func (r *Rng) NewZipf(s float64, v float64, imax uint64) *rand.Zipf {
	return rand.NewZipf(r.r, s, v, imax)
}
