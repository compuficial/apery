package rng

import "math"

// poissonPTRSMin is the mean from which Poisson switches from multiplying
// uniforms (O(lambda) draws) to transformed rejection (O(1)), as NumPy does.
const poissonPTRSMin = 10

// LogNormal returns e^X for X normal with mean mu and standard deviation
// sigma: positive values with a long right tail, like prices or durations.
func (r *Rng) LogNormal(mu, sigma float64) float64 {
	return portableExp(r.Normal(mu, sigma))
}

// Exponential returns an exponentially distributed value with the given
// mean, such as the gap between independent events.
func (r *Rng) Exponential(mean float64) float64 {
	u := r.Float64()
	if u == 0 {
		return 0 // -mean * log(1) would be -0
	}
	return -mean * portableLog(1-u)
}

// Poisson returns a Poisson-distributed count with mean lambda > 0.
func (r *Rng) Poisson(lambda float64) int64 {
	if lambda < poissonPTRSMin {
		return r.poissonMultiply(lambda)
	}
	return r.poissonPTRS(lambda)
}

// poissonMultiply counts uniforms whose running product stays above
// e^-lambda (Knuth).
func (r *Rng) poissonMultiply(lambda float64) int64 {
	limit := portableExp(-lambda)
	var k int64
	for p := r.Float64(); p > limit; p *= r.Float64() {
		k++
	}
	return k
}

// poissonPTRS is Hörmann's transformed rejection with squeeze (1993), as in
// NumPy's random_poisson_ptrs, with every product rounded so no platform
// fuses it.
func (r *Rng) poissonPTRS(lambda float64) int64 {
	sqrtLambda := math.Sqrt(lambda)
	logLambda := portableLog(lambda)
	b := 0.931 + float64(2.53*sqrtLambda)
	a := -0.059 + float64(0.02483*b)
	invAlpha := 1.1239 + 1.1328/(b-3.4)
	vr := 0.9277 - 3.6224/(b-2)

	for {
		u := r.Float64() - 0.5
		v := r.Float64()
		us := 0.5 - math.Abs(u)
		k := math.Floor(float64((2*a/us+b)*u) + lambda + 0.43)

		// Quick accept inside the squeeze, quick reject in the tails.
		if us >= 0.07 && v <= vr {
			return int64(k)
		}
		if k < 0 || (us < 0.013 && v > us) {
			continue
		}

		lhs := portableLog(v) + portableLog(invAlpha) - portableLog(a/(us*us)+b)
		rhs := -lambda + float64(k*logLambda) - logGamma(k+1)
		if lhs <= rhs {
			return int64(k)
		}
	}
}

// logGammaCoeffs are the Stirling series terms of NumPy's random_loggam.
var logGammaCoeffs = []float64{
	-1.39243221690590e+00,
	1.796443723688307e-01,
	-2.955065359477124e-02,
	6.410256410256410e-03,
	-1.917526917526918e-03,
	8.417508417508418e-04,
	-5.952380952380952e-04,
	7.936507936507937e-04,
	-2.777777777777778e-03,
	8.333333333333333e-02,
}

// logGamma is ln Γ(x) for x >= 1 by Stirling's series, shifting x up to 7
// first for accuracy (NumPy's random_loggam). Poisson calls it with k+1, so
// it is ln k!. math.Lgamma isn't used because arm64 fuses it differently.
func logGamma(x float64) float64 {
	const halfLog2Pi = 0.5 * 1.8378770664093453 // ln(2π)/2
	if x == 1 || x == 2 {
		return 0
	}

	var shift int
	if x < 7 {
		shift = int(7 - x)
	}
	x0 := x + float64(shift)
	x2 := (1 / x0) * (1 / x0)
	series := horner(x2, logGammaCoeffs...)
	gl := series/x0 + halfLog2Pi + float64((x0-0.5)*portableLog(x0)) - x0

	// Undo the shift: ln Γ(x) = ln Γ(x+n) - ln(x) - ... - ln(x+n-1).
	for range shift {
		x0--
		gl -= portableLog(x0)
	}
	return gl
}
