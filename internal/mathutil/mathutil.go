package mathutil

import (
	"math"
)

// Logit returns log(p/(1-p)), clamping p to [0.001, 0.999].
func Logit(p float64) float64 {
	p = math.Max(0.001, math.Min(0.999, p))
	return math.Log(p / (1 - p))
}

// Sigmoid returns 1/(1+exp(-x)).
func Sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// NormalCDF calculates the cumulative distribution function of the standard normal distribution.
// P(Z <= z) where Z ~ N(0,1)
func NormalCDF(z float64) float64 {
	return 0.5 * (1 + math.Erf(z/math.Sqrt2))
}

// NormalInvCDF calculates the inverse CDF (quantile function) of the standard normal distribution.
// Returns z such that P(Z <= z) = p.
// Uses Peter Acklam's 3-region rational approximation (error < 1.5e-8).
func NormalInvCDF(p float64) float64 {
	if p <= 0 {
		return -10 // Clamp to reasonable minimum
	}
	if p >= 1 {
		return 10 // Clamp to reasonable maximum
	}
	if p == 0.5 {
		return 0
	}

	const (
		a1 = -3.969683028665376e+01
		a2 = 2.209460984245205e+02
		a3 = -2.759285104469687e+02
		a4 = 1.383577518672690e+02
		a5 = -3.066479806614716e+01
		a6 = 2.506628277459239e+00

		b1 = -5.447609879822406e+01
		b2 = 1.615858368580409e+02
		b3 = -1.556989798598866e+02
		b4 = 6.680131188771972e+01
		b5 = -1.328068155288572e+01

		c1 = -7.784894002430293e-03
		c2 = -3.223964580411365e-01
		c3 = -2.400758277161838e+00
		c4 = -2.549732539343734e+00
		c5 = 4.374664141464968e+00
		c6 = 2.938163982698783e+00

		d1 = 7.784695709041462e-03
		d2 = 3.224671290700398e-01
		d3 = 2.445134137142996e+00
		d4 = 3.754408661907416e+00

		pLow  = 0.02425
		pHigh = 1 - pLow
	)

	var q float64

	if p < pLow {
		// Rational approximation for lower region
		q = math.Sqrt(-2 * math.Log(p))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	} else if p <= pHigh {
		// Rational approximation for central region
		q = p - 0.5
		r := q * q
		return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
	} else {
		// Rational approximation for upper region
		q = math.Sqrt(-2 * math.Log(1-p))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
}

// RegBetaI computes the regularized incomplete beta function I_x(a,b)
// using the Lentz continued fraction method.
func RegBetaI(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}

	// Use symmetry: I_x(a,b) = 1 - I_{1-x}(b,a) when x > (a+1)/(a+b+2)
	if x > (a+1)/(a+b+2) {
		return 1 - RegBetaI(b, a, 1-x)
	}

	// Log of the beta function prefix: x^a * (1-x)^b / (a * B(a,b))
	lnPrefix := a*math.Log(x) + b*math.Log(1-x) -
		math.Log(a) - (lgamma(a) + lgamma(b) - lgamma(a+b))

	prefix := math.Exp(lnPrefix)

	// Lentz continued fraction
	const maxIter = 200
	const eps = 1e-14
	const tiny = 1e-30

	f := 1.0 + tiny
	c := f
	d := 0.0

	for i := 0; i <= maxIter; i++ {
		var an float64
		m := i / 2
		if i == 0 {
			an = 1.0
		} else if i%2 == 0 {
			// even terms
			fm := float64(m)
			an = fm * (b - fm) * x / ((a + 2*fm - 1) * (a + 2*fm))
		} else {
			// odd terms
			fm := float64(m)
			an = -((a + fm) * (a + b + fm) * x) / ((a + 2*fm) * (a + 2*fm + 1))
		}

		d = 1.0 + an*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		d = 1.0 / d

		c = 1.0 + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}

		f *= c * d

		if math.Abs(c*d-1) < eps {
			break
		}
	}

	return prefix * (f - 1)
}

// lgamma wraps math.Lgamma discarding the sign.
func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// TDistCDF computes the CDF of Student's t-distribution with df degrees of freedom.
func TDistCDF(t, df float64) float64 {
	x := df / (df + t*t)
	beta := RegBetaI(df/2, 0.5, x)
	if t >= 0 {
		return 1 - 0.5*beta
	}
	return 0.5 * beta
}

// WinsorizeLogits caps logit outliers at ±maxSD robust standard deviations
// from the median. Uses mean absolute deviation from median as a robust spread
// estimator (σ ≈ 1.2533 × MAD_mean). Modifies the logits slice in place.
// Requires at least 3 values.
func WinsorizeLogits(logits, weights []float64, maxSD float64) {
	if len(logits) < 3 || len(logits) != len(weights) {
		return
	}

	n := len(logits)

	// Find median (robust center)
	sorted := make([]float64, n)
	copy(sorted, logits)
	sortFloat64s(sorted)
	median := sorted[n/2] // For odd n; close enough for even n

	// Mean absolute deviation from median (robust spread estimator)
	var sumAbsDev float64
	for _, v := range logits {
		d := v - median
		if d < 0 {
			d = -d
		}
		sumAbsDev += d
	}
	meanAbsDev := sumAbsDev / float64(n)

	// Scale to estimate σ (for normal data: σ ≈ 1.2533 × mean_abs_dev)
	robustSD := 1.2533 * meanAbsDev
	if robustSD < 0.01 {
		return // All values essentially identical
	}

	lo := median - maxSD*robustSD
	hi := median + maxSD*robustSD
	for i := range logits {
		logits[i] = math.Max(lo, math.Min(hi, logits[i]))
	}
}

// sortFloat64s sorts a slice of float64 in ascending order (insertion sort for small n).
func sortFloat64s(a []float64) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}

// TDistInvCDF computes the inverse CDF of Student's t-distribution via bisection.
func TDistInvCDF(p, df float64) float64 {
	if p <= 0 {
		return -100
	}
	if p >= 1 {
		return 100
	}
	if p == 0.5 {
		return 0
	}

	lo, hi := -100.0, 100.0
	for i := 0; i < 100; i++ {
		mid := (lo + hi) / 2
		if TDistCDF(mid, df) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}
