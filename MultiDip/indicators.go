package newstrategy4

import (
	"math"

	ta "github.com/banbox/banta"
)

// EWO simplified: (EMA(short) - EMA(long)) / close * 100
func ewoFromEnv(e *ta.BarEnv, shortLen, longLen int) float64 {
	short := ta.EMA(e.Close, shortLen).Get(0)
	long := ta.EMA(e.Close, longLen).Get(0)
	c := e.Close.Get(0)
	if c == 0 {
		return 0
	}
	return (short - long) / c * 100.0
}

// Simple VWAP lower band estimation
func vwapLowerFromEnv(e *ta.BarEnv, window int) float64 {
	prices := typicalPriceRange(e, window)
	vols := e.Volume.Range(0, window)
	if len(prices) < window || len(vols) < window {
		return 0
	}
	num, den := 0.0, 0.0
	for i := 0; i < window; i++ {
		num += prices[i] * vols[i]
		den += vols[i]
	}
	if den == 0 {
		return 0
	}
	vwap := num / den
	// std of typical price in window
	mean := 0.0
	for i := 0; i < window; i++ {
		mean += prices[i]
	}
	mean /= float64(window)
	varSum := 0.0
	for i := 0; i < window; i++ {
		d := prices[i] - mean
		varSum += d * d
	}
	std := math.Sqrt(varSum / float64(window))
	return vwap - std
}

// amplitude filter: any bar amplitude > pct within window
func amplitudeFlagFromEnv(e *ta.BarEnv, window int, pct float64) bool {
	if e.High.Len() < window {
		return false
	}
	for i := 0; i < window; i++ {
		h := e.High.Get(i)
		l := e.Low.Get(i)
		if l == 0 {
			continue
		}
		if (h-l)/l*100.0 > pct {
			return true
		}
	}
	return false
}

// Williams %R
func williamsRFromEnv(e *ta.BarEnv, period int) float64 {
	hh := e.High.Range(0, period)
	ll := e.Low.Range(0, period)
	if len(hh) < period || len(ll) < period {
		return 0
	}
	highMax := hh[0]
	lowMin := ll[0]
	for _, v := range hh {
		if v > highMax {
			highMax = v
		}
	}
	for _, v := range ll {
		if v < lowMin {
			lowMin = v
		}
	}
	if highMax-lowMin == 0 {
		return 0
	}
	return -100.0 * (highMax - e.Close.Get(0)) / (highMax - lowMin)
}

// ROCR approx: close / close.shift(period)
func rocrFromEnv(e *ta.BarEnv, period int) float64 {
	if e.Close.Len() <= period {
		return 0
	}
	cur := e.Close.Get(0)
	prev := e.Close.Get(period)
	if prev == 0 {
		return 0
	}
	return cur / prev
}

// typical price (H+L+C)/3 range, latest at index 0
func typicalPriceRange(e *ta.BarEnv, window int) []float64 {
	h := e.High.Range(0, window)
	l := e.Low.Range(0, window)
	c := e.Close.Range(0, window)
	n := len(h)
	if len(l) < n {
		n = len(l)
	}
	if len(c) < n {
		n = len(c)
	}
	res := make([]float64, n)
	for i := 0; i < n; i++ {
		res[i] = (h[i] + l[i] + c[i]) / 3.0
	}
	return res
}

// local support within window: pick a local low point
func supLevelFromEnv(e *ta.BarEnv, window int) float64 {
	low := e.Low.Range(0, window)
	if len(low) < window {
		if len(low) == 0 {
			return 0
		}
		return low[len(low)-1]
	}
	for i := 1; i < len(low)-1; i++ {
		if low[i] < low[i-1] && low[i] < low[i+1] {
			return low[i]
		}
	}
	return low[len(low)-1]
}

// StochRSI K (no smoothing)
func stochRSIValue(e *ta.BarEnv, rsiLen, stochLen int) float64 {
	rsi := ta.RSI(e.Close, rsiLen)
	if rsi.Len() < stochLen {
		return math.NaN()
	}
	vals := rsi.Range(0, stochLen)
	cur := rsi.Get(0)
	hi, lo := vals[0], vals[0]
	for _, v := range vals {
		if v > hi {
			hi = v
		}
		if v < lo {
			lo = v
		}
	}
	if math.IsNaN(cur) || hi == lo {
		return math.NaN()
	}
	return 100.0 * (cur - lo) / (hi - lo)
}

// PMAX approx using ATR
func pmaxApprox(e *ta.BarEnv, period, multiplier, length int) float64 {
	ma := ta.EMA(e.Close, length).Get(0)
	atr := ta.ATR(e.High, e.Low, e.Close, period).Get(0)
	return ma + (float64(multiplier)/10.0)*atr
}

// CMF custom
func cmfFromEnv(e *ta.BarEnv, period int) float64 {
	if e.Close.Len() < period {
		return 0
	}
	mfNum, mfDen := 0.0, 0.0
	for i := 0; i < period; i++ {
		h := e.High.Get(i)
		l := e.Low.Get(i)
		c := e.Close.Get(i)
		v := e.Volume.Get(i)
		hl := h - l
		if hl == 0 {
			continue
		}
		mfMult := ((c - l) - (h - c)) / hl
		mf := mfMult * v
		mfNum += mf
		mfDen += v
	}
	if mfDen == 0 {
		return 0
	}
	return mfNum / mfDen
}

// Fisher Transform (single period normalization)
func fisherFromEnv(e *ta.BarEnv, period int) float64 {
	if e.Close.Len() < period {
		return 0
	}
	hi := e.High.Get(0)
	lo := e.Low.Get(0)
	for i := 0; i < period; i++ {
		if e.High.Get(i) > hi {
			hi = e.High.Get(i)
		}
		if e.Low.Get(i) < lo {
			lo = e.Low.Get(i)
		}
	}
	if hi == lo {
		return 0
	}
	v := 2.0*((e.Close.Get(0)-lo)/(hi-lo)) - 1.0
	if v > 0.999 {
		v = 0.999
	}
	if v < -0.999 {
		v = -0.999
	}
	return 0.5 * math.Log((1+v)/(1-v))
} 