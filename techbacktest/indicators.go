package techbacktest

import "math"

// Simple indicator helpers (minimal, to avoid pulling external deps)

func ema(values []float64, period int) []float64 {
	if period <= 0 || len(values) == 0 {
		return nil
	}
	k := 2.0 / float64(period+1)
	out := make([]float64, len(values))
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}

func sma(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	out := make([]float64, len(values))
	var sum float64
	for i := 0; i < len(values); i++ {
		sum += values[i]
		if i >= period {
			sum -= values[i-period]
		}
		if i >= period-1 {
			out[i] = sum / float64(period)
		}
	}
	return out
}

func stddev(values []float64, period int, ma []float64) []float64 {
	if period <= 0 || len(values) < period || len(ma) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	for i := period - 1; i < len(values); i++ {
		mean := ma[i]
		var sum float64
		for j := i - period + 1; j <= i; j++ {
			diff := values[j] - mean
			sum += diff * diff
		}
		out[i] = math.Sqrt(sum / float64(period))
	}
	return out
}

func rsi(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period+1 {
		return nil
	}
	out := make([]float64, len(values))
	var gain, loss float64
	for i := 1; i <= period; i++ {
		diff := values[i] - values[i-1]
		if diff >= 0 {
			gain += diff
		} else {
			loss -= diff
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	rs := 0.0
	if avgLoss != 0 {
		rs = avgGain / avgLoss
	}
	out[period] = 100 - 100/(1+rs)
	for i := period + 1; i < len(values); i++ {
		diff := values[i] - values[i-1]
		if diff >= 0 {
			avgGain = (avgGain*float64(period-1) + diff) / float64(period)
			avgLoss = (avgLoss * float64(period-1)) / float64(period)
		} else {
			avgGain = (avgGain * float64(period-1)) / float64(period)
			avgLoss = (avgLoss*float64(period-1) - diff) / float64(period)
		}
		if avgLoss != 0 {
			rs = avgGain / avgLoss
			out[i] = 100 - 100/(1+rs)
		} else {
			out[i] = 100
		}
	}
	return out
}

func macd(values []float64, fast, slow, signal int) (macdLine, signalLine []float64) {
	emaFast := ema(values, fast)
	emaSlow := ema(values, slow)
	if len(emaFast) == 0 || len(emaSlow) == 0 {
		return nil, nil
	}
	n := len(values)
	macdLine = make([]float64, n)
	for i := 0; i < n; i++ {
		if i < len(emaFast) && i < len(emaSlow) {
			macdLine[i] = emaFast[i] - emaSlow[i]
		}
	}
	signalLine = ema(macdLine, signal)
	return macdLine, signalLine
}

func boll(values []float64, period int, mult float64) (upper, middle, lower []float64) {
	middle = sma(values, period)
	if len(middle) == 0 {
		return nil, nil, nil
	}
	std := stddev(values, period, middle)
	n := len(values)
	upper = make([]float64, n)
	lower = make([]float64, n)
	for i := 0; i < n; i++ {
		if i < period-1 || i >= len(std) {
			continue
		}
		upper[i] = middle[i] + mult*std[i]
		lower[i] = middle[i] - mult*std[i]
	}
	return
}

// CCI: (typical price - SMA(tp)) / (0.015 * mean deviation)
func cci(hlc3 []float64, period int) []float64 {
	if period <= 0 || len(hlc3) < period {
		return nil
	}
	smaVals := sma(hlc3, period)
	out := make([]float64, len(hlc3))
	for i := period - 1; i < len(hlc3); i++ {
		ma := smaVals[i]
		var devSum float64
		for j := i - period + 1; j <= i; j++ {
			devSum += math.Abs(hlc3[j] - ma)
		}
		meanDev := devSum / float64(period)
		if meanDev == 0 {
			out[i] = 0
		} else {
			out[i] = (hlc3[i] - ma) / (0.015 * meanDev)
		}
	}
	return out
}

// ATR (Wilder)
func atr(high, low, close []float64, period int) []float64 {
	n := len(close)
	out := make([]float64, n)
	if n == 0 || period <= 0 {
		return out
	}
	trPrev := high[0] - low[0]
	out[0] = trPrev
	for i := 1; i < n; i++ {
		tr := math.Max(high[i]-low[i], math.Max(math.Abs(high[i]-close[i-1]), math.Abs(low[i]-close[i-1])))
		if i < period {
			out[i] = ((out[i-1] * float64(i)) + tr) / float64(i+1)
		} else if i == period {
			sum := 0.0
			for j := 0; j <= i; j++ {
				if j == 0 {
					sum += trPrev
				} else {
					t := math.Max(high[j]-low[j], math.Max(math.Abs(high[j]-close[j-1]), math.Abs(low[j]-close[j-1])))
					sum += t
				}
			}
			out[i] = sum / float64(period)
		} else {
			out[i] = (out[i-1]*float64(period-1) + tr) / float64(period)
		}
		trPrev = tr
	}
	return out
}

// VWAP cumulative typical price * volume / cumulative volume
func vwap(high, low, close, volume []float64) []float64 {
	n := len(close)
	out := make([]float64, n)
	cpv := 0.0
	cvol := 0.0
	for i := 0; i < n; i++ {
		tp := (high[i] + low[i] + close[i]) / 3
		cpv += tp * volume[i]
		cvol += volume[i]
		if cvol > 0 {
			out[i] = cpv / cvol
		}
	}
	return out
}

// Supertrend (basic version using ATR multiplier)
// returns line values; positive trend above price, negative trend below (sign with trend direction)
func supertrend(high, low, close []float64, period int, mult float64) []float64 {
	n := len(close)
	out := make([]float64, n)
	if n == 0 || period <= 0 {
		return out
	}
	atrVals := atr(high, low, close, period)
	upperBand := make([]float64, n)
	lowerBand := make([]float64, n)
	for i := 0; i < n; i++ {
		hl2 := (high[i] + low[i]) / 2
		upperBand[i] = hl2 + mult*atrVals[i]
		lowerBand[i] = hl2 - mult*atrVals[i]
	}
	trendUp := true
	for i := 0; i < n; i++ {
		if i == 0 {
			out[i] = upperBand[i]
			trendUp = close[i] >= upperBand[i]
			continue
		}
		if close[i] > upperBand[i-1] {
			trendUp = true
		} else if close[i] < lowerBand[i-1] {
			trendUp = false
		}
		if trendUp {
			if lowerBand[i] < lowerBand[i-1] {
				lowerBand[i] = lowerBand[i-1]
			}
			out[i] = lowerBand[i]
		} else {
			if upperBand[i] > upperBand[i-1] {
				upperBand[i] = upperBand[i-1]
			}
			out[i] = upperBand[i]
		}
		if trendUp {
			out[i] = lowerBand[i]
		} else {
			out[i] = upperBand[i]
		}
	}
	return out
}
