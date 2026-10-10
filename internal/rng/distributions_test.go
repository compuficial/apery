package rng

import (
	"math"
	"testing"
)

const statDraws = 200_000

// meanVar returns the sample mean and variance of n draws.
func meanVar(n int, draw func() float64) (mean, variance float64) {
	var sum, sumSq float64
	for range n {
		x := draw()
		sum += x
		sumSq += x * x
	}
	mean = sum / float64(n)
	return mean, sumSq/float64(n) - mean*mean
}

// within reports whether got is within rel of want.
func within(got, want, rel float64) bool {
	return math.Abs(got-want) <= rel*math.Abs(want)
}

func TestLogNormal_Shape(t *testing.T) {
	r := New(SeedFromInt64(1))
	const mu, sigma = 3.0, 0.5
	below := 0
	logMean, logVar := meanVar(statDraws, func() float64 {
		x := r.LogNormal(mu, sigma)
		if x <= 0 {
			t.Fatalf("LogNormal drew %v, want > 0", x)
		}
		if x < math.Exp(mu) {
			below++
		}
		return math.Log(x)
	})
	if !within(logMean, mu, 0.01) || !within(math.Sqrt(logVar), sigma, 0.01) {
		t.Errorf("log mean %.4f, log stddev %.4f; want %v, %v", logMean, math.Sqrt(logVar), mu, sigma)
	}
	if frac := float64(below) / statDraws; math.Abs(frac-0.5) > 0.01 {
		t.Errorf("%.3f of draws below the median e^mu, want 0.5", frac)
	}
}

func TestExponential_Shape(t *testing.T) {
	r := New(SeedFromInt64(2))
	const mean = 300.0
	got, variance := meanVar(statDraws, func() float64 {
		x := r.Exponential(mean)
		if x < 0 || math.Signbit(x) {
			t.Fatalf("Exponential drew %v, want >= +0", x)
		}
		return x
	})
	if !within(got, mean, 0.01) || !within(math.Sqrt(variance), mean, 0.02) {
		t.Errorf("mean %.2f, stddev %.2f; want both about %v", got, math.Sqrt(variance), mean)
	}
}

// Poisson's mean and variance both equal lambda, on both sides of the switch
// from multiplication to PTRS, and up to very large means.
func TestPoisson_MeanAndVariance(t *testing.T) {
	r := New(SeedFromInt64(3))
	for _, lambda := range []float64{0.05, 1, 3.5, 9.99, 10, 50, 1e4, 1e9} {
		mean, variance := meanVar(statDraws, func() float64 {
			k := r.Poisson(lambda)
			if k < 0 {
				t.Fatalf("Poisson(%v) drew %d", lambda, k)
			}
			return float64(k)
		})
		if !within(mean, lambda, 0.02) || !within(variance, lambda, 0.03) {
			t.Errorf("lambda %v: mean %.4f, variance %.4f", lambda, mean, variance)
		}
	}
}

// The sampled frequencies match the exact probabilities, so a mistake in
// either algorithm shows up as a skewed shape, not just a shifted mean.
func TestPoisson_MatchesPMF(t *testing.T) {
	for _, lambda := range []float64{3.5, 10, 50} {
		r := New(SeedFromInt64(4))
		counts := map[int64]int{}
		for range statDraws {
			counts[r.Poisson(lambda)]++
		}
		for k := range int64(3 * lambda) {
			lgk, _ := math.Lgamma(float64(k + 1))
			p := math.Exp(float64(k)*math.Log(lambda) - lambda - lgk)
			got := float64(counts[k]) / statDraws
			// 4.5 standard errors: deterministic seed, so no flakiness, but
			// loose enough to pass for a correct sampler.
			if tol := 4.5 * math.Sqrt(p*(1-p)/statDraws); math.Abs(got-p) > tol+1e-5 {
				t.Errorf("lambda %v: P(%d) = %.5f, want %.5f ± %.5f", lambda, k, got, p, tol)
			}
		}
	}
}

func TestLogGamma(t *testing.T) {
	for _, x := range []float64{1, 1.5, 2, 2.5, 3, 6.9, 7, 7.5, 10, 100, 1234.5, 1e6, 1e12} {
		want, _ := math.Lgamma(x)
		if got := logGamma(x); math.Abs(got-want) > 1e-12*max(1, math.Abs(want)) {
			t.Errorf("logGamma(%v) = %v, want %v", x, got, want)
		}
	}
}
