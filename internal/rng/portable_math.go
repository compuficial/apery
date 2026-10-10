package rng

import "math"

// Portable versions of math.Exp, math.Log, and math.Cos.
//
// The stdlib's results differ per architecture: amd64 runs assembly Exp and
// Log, while on arm64 (and amd64 built with GOAMD64=v3) the compiler fuses
// a*b+c into one FMA instruction, which rounds once instead of twice. That
// breaks byte-identical output, so NormFloat64 and Zipf use these instead.
//
// Log and Cos are Go's pure-Go implementations (Copyright 2009 The Go
// Authors, BSD-style license) with every product that is added to or passed
// on wrapped in float64(), which forces rounding and so rules out fusion. Exp
// ports amd64's assembly, using math.FMA where that fuses. All three match
// amd64 bit for bit except where amd64 is wrong: Exp just below overflow and
// Log of subnormal numbers.

// horner evaluates c[0]*x^(n-1) + ... + c[n-1], rounding every step.
func horner(x float64, c ...float64) float64 {
	p := c[0]
	for _, ci := range c[1:] {
		p = float64(p*x) + ci
	}
	return p
}

// hornerFMA is horner with every step fused, as amd64's Exp evaluates it.
func hornerFMA(x float64, c ...float64) float64 {
	p := c[0]
	for _, ci := range c[1:] {
		p = math.FMA(p, x, ci)
	}
	return p
}

// portableLog is math.Log.
func portableLog(x float64) float64 {
	const (
		ln2Hi = 6.93147180369123816490e-01 // 3fe62e42 fee00000
		ln2Lo = 1.90821492927058770002e-10 // 3dea39ef 35793c76
		l1    = 6.666666666666735130e-01   // 3FE55555 55555593
		l2    = 3.999999999940941908e-01   // 3FD99999 9997FA04
		l3    = 2.857142874366239149e-01   // 3FD24924 94229359
		l4    = 2.222219843214978396e-01   // 3FCC71C5 1D8E78AF
		l5    = 1.818357216161805012e-01   // 3FC74664 96CB03DE
		l6    = 1.531383769920937332e-01   // 3FC39A09 D078C69F
		l7    = 1.479819860511658591e-01   // 3FC2F112 DF3E5244
	)

	switch {
	case math.IsNaN(x) || math.IsInf(x, 1):
		return x
	case x < 0:
		return math.NaN()
	case x == 0:
		return math.Inf(-1)
	}

	// Reduce x to f1 * 2^k with f1 in [sqrt(2)/2, sqrt(2)).
	f1, ki := math.Frexp(x)
	if f1 < math.Sqrt2/2 {
		f1 *= 2
		ki--
	}
	f := f1 - 1
	k := float64(ki)

	// log(1+f) = f - f*f/2 + s*(f*f/2 + R), with s = f/(2+f).
	s := f / (2 + f)
	s2 := s * s
	s4 := s2 * s2
	t1 := float64(s2 * horner(s4, l7, l5, l3, l1))
	t2 := float64(s4 * horner(s4, l6, l4, l2))
	r := t1 + t2
	hfsq := float64(0.5 * f * f)
	return float64(k*ln2Hi) - ((hfsq - (float64(s*(hfsq+r)) + float64(k*ln2Lo))) - f)
}

// portableExp is math.Exp as amd64's assembly computes it on CPUs with FMA
// (every x86 CPU since 2013): a port of SLEEF's exp (Shibata, ISC 2010;
// public domain). Unlike amd64, it returns the finite result rather than
// +Inf for 709.44 < x <= 709.78.
func portableExp(x float64) float64 {
	const (
		log2e    = 1.4426950408889634073599246810018920
		ln2Hi    = 0.69314718055966295651160180568695068359375           // upper bits of ln 2
		ln2Lo    = 0.28235290563031577122588448175013436025525412068e-12 // the rest of ln 2
		overflow = 7.09782712893384e+02
	)

	switch {
	case math.IsNaN(x) || math.IsInf(x, 1):
		return x
	case x > overflow:
		return math.Inf(1)
	}

	// Reduce x = k*ln2 + 16r, rounding halfway cases to even as x86 does.
	kf := math.RoundToEven(log2e * x)
	if kf < -1075 {
		return 0 // rounds to 0; also catches -Inf
	}
	r := math.FMA(-kf, ln2Hi, x)
	r = math.FMA(-kf, ln2Lo, r) / 16

	// e^r - 1 by Taylor series, then e^(16r) by squaring four times with
	// e^2a - 1 = (e^a - 1)(e^a - 1 + 2), adding the 1 back in the last step.
	// float64() keeps each product from fusing with the next +2.
	y := float64(r * hornerFMA(r, expTaylor...))
	for range 3 {
		y = float64(y * (y + 2))
	}
	y = math.FMA(y+2, y, 1)

	// Scale by 2^k with one rounding, as amd64 does except at k = 1024.
	return math.Ldexp(y, int(kf))
}

// expTaylor holds the Taylor coefficients 1/8! down to 1/1!.
var expTaylor = []float64{
	2.4801587301587301587e-5,
	1.9841269841269841270e-4,
	1.3888888888888888889e-3,
	8.3333333333333333333e-3,
	4.1666666666666666667e-2,
	1.6666666666666666667e-1,
	0.5,
	1.0,
}

// sinCoeffs and cosCoeffs approximate sin and cos on [-Pi/4, Pi/4] (from
// Cephes, as in Go's math package).
var (
	sinCoeffs = []float64{
		1.58962301576546568060e-10, // 0x3de5d8fd1fd19ccd
		-2.50507477628578072866e-8, // 0xbe5ae5e5a9291f5d
		2.75573136213857245213e-6,  // 0x3ec71de3567d48a1
		-1.98412698295895385996e-4, // 0xbf2a01a019bfdf03
		8.33333333332211858878e-3,  // 0x3f8111111110f7d0
		-1.66666666666666307295e-1, // 0xbfc5555555555548
	}
	cosCoeffs = []float64{
		-1.13585365213876817300e-11, // 0xbda8fa49a0861a9b
		2.08757008419747316778e-9,   // 0x3e21ee9d7b4e3f05
		-2.75573141792967388112e-7,  // 0xbe927e4f7eac4bc6
		2.48015872888517045348e-5,   // 0x3efa01a019c844f5
		-1.38888888888730564116e-3,  // 0xbf56c16c16c14f91
		4.16666666666665929218e-2,   // 0x3fa555555555554b
	}
)

// portableCos is math.Cos for 0 <= x < 2^29 (no Payne-Hanek reduction).
func portableCos(x float64) float64 {
	const (
		pi4A = 7.85398125648498535156e-1  // 0x3fe921fb40000000, Pi/4 split into three parts
		pi4B = 3.77489470793079817668e-8  // 0x3e64442d00000000
		pi4C = 2.69515142907905952645e-15 // 0x3ce8469898cc5170
	)

	// Reduce to z in [-Pi/4, Pi/4] and octant j, mapping odd octants up.
	j := uint64(x * (4 / math.Pi))
	y := float64(j)
	if j&1 == 1 {
		j++
		y++
	}
	j &= 7
	z := ((x - float64(y*pi4A)) - float64(y*pi4B)) - float64(y*pi4C)

	isNegative := false
	if j > 3 {
		j -= 4
		isNegative = !isNegative
	}
	if j > 1 {
		isNegative = !isNegative
	}

	// Octants 1 and 2 use the sine polynomial, the rest cosine.
	zz := z * z
	if j == 1 || j == 2 {
		y = z + float64(z*zz*horner(zz, sinCoeffs...))
	} else {
		y = 1.0 - float64(0.5*zz) + float64(zz*zz*horner(zz, cosCoeffs...))
	}
	if isNegative {
		y = -y
	}
	return y
}
