package rng

import (
	"math"
	"math/rand/v2"
	"runtime"
	"runtime/debug"
	"testing"
)

// isReferenceBuild reports whether this build's stdlib is the reference the
// ports reproduce: amd64 compiled without FMA fusion (GOAMD64=v1, the
// default). Exp also assumes an FMA-capable CPU, as every x86 CPU since 2013
// is.
func isReferenceBuild() bool {
	if runtime.GOARCH != "amd64" {
		return false
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range bi.Settings {
		if s.Key == "GOAMD64" {
			return s.Value == "v1"
		}
	}
	return false
}

// ulps returns how many units in the last place a and b are apart.
func ulps(a, b float64) uint64 {
	x, y := int64(math.Float64bits(a)), int64(math.Float64bits(b))
	if x > y {
		return uint64(x - y)
	}
	return uint64(y - x)
}

// expTies returns inputs where log2e*x lands exactly halfway between two
// integers, so the reduction's round-half-to-even matters. It stops below
// 709.44, where amd64's math.Exp overflows early.
func expTies() []float64 {
	var ties []float64
	for n := -1075; n <= 1022; n++ {
		half := float64(n) + 0.5
		x := half * math.Ln2
		for range 64 {
			x = math.Nextafter(x, math.Inf(-1))
		}
		for range 128 {
			if math.Log2E*x == half {
				ties = append(ties, x)
			}
			x = math.Nextafter(x, math.Inf(1))
		}
	}
	return ties
}

// The ports must equal the stdlib in the reference build, whose math.Exp,
// math.Log, and math.Cos they reproduce. Elsewhere the stdlib drifts by an
// ulp or two.
func TestPortableMathMatchesStdlib(t *testing.T) {
	src := rand.New(rand.NewPCG(1, 2))
	ties := expTies()
	cases := []struct {
		name      string
		port, std func(float64) float64
		input     func() float64
		drift     uint64 // allowed ulps outside the reference build
	}{
		// Up to 709.4: above it amd64 overflows early (see portableExp).
		{"Exp", portableExp, math.Exp, func() float64 { return float64(src.Float64()*1459.4) - 750 }, 2},
		{"ExpTies", portableExp, math.Exp, func() float64 { return ties[src.IntN(len(ties))] }, 2},
		// Every positive normal float64, plus +Inf and NaN. amd64's
		// assembly is wrong for subnormals; see TestPortableMathSpecialCases.
		{"Log", portableLog, math.Log, func() float64 { return math.Float64frombits(src.Uint64()>>1 | 1<<52) }, 1},
		{"Cos", portableCos, math.Cos, func() float64 { return float64(src.Float64() * 1000) }, 1},
	}
	for _, c := range cases {
		tolerance := c.drift
		if isReferenceBuild() {
			tolerance = 0
		}
		for range 1_000_000 {
			x := c.input()
			got, want := c.port(x), c.std(x)
			if math.IsNaN(got) && math.IsNaN(want) {
				continue
			}
			if d := ulps(got, want); d > tolerance {
				t.Fatalf("%s(%v) = %v, stdlib %v (%d ulps apart)", c.name, x, got, want, d)
			}
		}
	}
}

func TestPortableMathSpecialCases(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	cases := []struct {
		name      string
		got, want float64
	}{
		{"Exp(NaN)", portableExp(nan), nan},
		{"Exp(+Inf)", portableExp(inf), inf},
		{"Exp(-Inf)", portableExp(-inf), 0},
		{"Exp(0)", portableExp(0), 1},
		{"Exp(710)", portableExp(710), inf},
		{"Exp(-746)", portableExp(-746), 0},
		{"Exp(-1e10)", portableExp(-1e10), 0}, // far below underflow
		{"Log(NaN)", portableLog(nan), nan},
		{"Log(+Inf)", portableLog(inf), inf},
		{"Log(-1)", portableLog(-1), nan},
		{"Log(0)", portableLog(0), -inf},
		{"Log(1)", portableLog(1), 0},
		{"Cos(0)", portableCos(0), 1},
	}
	for _, c := range cases {
		if math.Float64bits(c.got) != math.Float64bits(c.want) && !(math.IsNaN(c.got) && math.IsNaN(c.want)) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	// Where amd64's assembly is wrong, the ports must be right: Exp
	// overflows early and Log mishandles subnormals there.
	if got, want := portableExp(709.7), 1.6549840276802644e+308; ulps(got, want) > 2 {
		t.Errorf("Exp(709.7) = %v, want %v", got, want)
	}
	if got, want := portableLog(math.SmallestNonzeroFloat64), -1074*math.Ln2; ulps(got, want) > 1 {
		t.Errorf("Log(2^-1074) = %v, want %v", got, want)
	}
}
