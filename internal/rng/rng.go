// Package rng provides deterministic random number generation with hierarchical seed derivation.
//
// The Derive function creates child seeds from parent seeds using FNV-1a hashing,
// enabling reproducible randomness across nested contexts (entity → field → row).
// This ensures that the same plan with the same root seed always produces identical
// output, which is critical for Apery's determinism guarantees.
package rng

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand/v2"
)

// Rng is a seeded random number generator
type Rng struct {
	r *rand.Rand
}

// New creates a new RNG with a seed
func New(seed int64) *Rng {
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

// Derive creates a child seed from parent + label
func Derive(parent int64, label string) int64 {
	// Fast hash function
	hf := fnv.New64a()

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(parent))
	hf.Write(buf[:])

	hf.Write([]byte(label))

	return int64(hf.Sum64())
}
