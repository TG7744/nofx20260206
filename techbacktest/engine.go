package techbacktest

import (
	"fmt"
	"math"
	"nofx/market"
	"nofx/store"
	"time"
)

// Run executes a simple technical backtest synchronously.
func Run(cfg Config) (*Result, error) {
	if cfg.InitialBalance <= 0 {
		cfg.InitialBalance = 1000
	}
	if cfg.FeeBps < 0 {
		cfg.FeeBps = 0
	}
	if cfg.SlippageBps < 0 {
		cfg.SlippageBps = 0
	}
	if cfg.Leverage <= 0 {
		cfg.Leverage = 1
	}
	if cfg.Strategy.Type == "" {
		cfg.Strategy.Type = StrategyEMACross
	}

	indicatorCfg := store.IndicatorConfig{
		Klines: store.KlineConfig{
			PrimaryTimeframe: cfg.Timeframe,
			PrimaryCount:     1000,
		},
		EnableRawKlines: true,
	}

	data, err := market.GetWithTimeframes(cfg.Symbol, []string{cfg.Timeframe}, cfg.Timeframe, 1000, indicatorCfg)
	if err != nil {
		return nil, fmt.Errorf("fetch data: %w", err)
	}
	klines := data.TimeframeData[cfg.Timeframe].Klines
	var closes []float64
	var times []int64
	var hlc3 []float64
	filteredKlines := make([]market.KlineBar, 0, len(klines))
	for _, k := range klines {
		t := time.Unix(k.Time/1000, 0)
		if !cfg.Start.IsZero() && t.Before(cfg.Start) {
			continue
		}
		if !cfg.End.IsZero() && t.After(cfg.End) {
			continue
		}
		filteredKlines = append(filteredKlines, k)
		closes = append(closes, k.Close)
		times = append(times, k.Time)
		hlc3 = append(hlc3, (k.High+k.Low+k.Close)/3)
	}
	if len(closes) < 50 {
		return nil, fmt.Errorf("not enough data after filtering")
	}

	// build slices for indicators
	highs := make([]float64, len(filteredKlines))
	lows := make([]float64, len(filteredKlines))
	vols := make([]float64, len(filteredKlines))
	for i, k := range filteredKlines {
		highs[i] = k.High
		lows[i] = k.Low
		vols[i] = k.Volume
	}

	signals, err := buildSignals(cfg.Strategy, closes, hlc3)
	if err != nil {
		return nil, err
	}

	// indicator overlays for frontend plotting
	fastLen := int(cfg.Strategy.Params["fast"])
	if fastLen == 0 {
		fastLen = 20
	}
	slowLen := int(cfg.Strategy.Params["slow"])
	if slowLen == 0 {
		slowLen = 50
	}
	emaFast := ema(closes, fastLen)
	emaSlow := ema(closes, slowLen)
	bollUpper, bollMid, bollLower := boll(closes, 20, 2)
	macdLine, macdSig := macd(closes, 12, 26, 9)
	macdHist := make([]float64, len(macdLine))
	for i := range macdLine {
		if i < len(macdSig) {
			macdHist[i] = macdLine[i] - macdSig[i]
		}
	}
	rsiSeries := rsi(closes, 14)
	atrSeries := atr(highs, lows, closes, 14)
	vwapSeries := vwap(highs, lows, closes, vols)
	stPeriod := cfg.SupertrendPeriod
	if stPeriod <= 0 {
		stPeriod = 10
	}
	stMult := cfg.SupertrendMult
	if stMult <= 0 {
		stMult = 3
	}
	supertrendSeries := supertrend(highs, lows, closes, stPeriod, stMult)
	atrUpper := make([]float64, len(closes))
	atrLower := make([]float64, len(closes))
	for i := range closes {
		atrUpper[i] = closes[i] + 2*atrSeries[i]
		atrLower[i] = closes[i] - 2*atrSeries[i]
	}

	equity := cfg.InitialBalance
	position := 0.0
	entryPx := 0.0
	highestSinceEntry := 0.0
	equityCurve := make([]EquityPoint, 0, len(closes))
	signalsOut := make([]SignalPoint, 0, 32)
	trades := make([]Trade, 0, 32)
	peak := equity
	maxDD := 0.0
	win, loss := 0, 0
	var grossWin, grossLoss float64

	feeRate := cfg.FeeBps / 10000.0
	slipRate := cfg.SlippageBps / 10000.0

	for i := 0; i < len(closes); i++ {
		sig := signals[i]
		price := closes[i] * (1 + slipRate)

		exit := sig == -1
		// risk exits
		if position != 0 {
			if cfg.StopLossPct > 0 && (price-entryPx)/entryPx <= -cfg.StopLossPct/100 {
				exit = true
			}
			if cfg.TakeProfitPct > 0 && (price-entryPx)/entryPx >= cfg.TakeProfitPct/100 {
				exit = true
			}
			if cfg.TrailingStopPct > 0 {
				if price > highestSinceEntry {
					highestSinceEntry = price
				}
				if highestSinceEntry > 0 && (price-highestSinceEntry)/highestSinceEntry <= -cfg.TrailingStopPct/100 {
					exit = true
				}
			}
		}
		if position != 0 && exit { // close
			pnl := (price-entryPx)*position*cfg.Leverage - math.Abs(price*position*feeRate)
			equity += pnl
			signalsOut = append(signalsOut, SignalPoint{Time: times[i], Price: price, Side: "sell"})
			trades = append(trades, Trade{
				EntryTime: times[i],
				ExitTime:  times[i],
				EntryPx:   entryPx,
				ExitPx:    price,
				Qty:       position,
				PnL:       pnl,
				PnLPct:    (price/entryPx - 1) * 100 * cfg.Leverage,
				Side:      "long",
			})
			if pnl >= 0 {
				win++
				grossWin += pnl
			} else {
				loss++
				grossLoss -= pnl
			}
			position = 0
			entryPx = 0
			highestSinceEntry = 0
		}

		// entry
		if position == 0 && sig == 1 {
			position = equity / price
			entryPx = price
			highestSinceEntry = price
			signalsOut = append(signalsOut, SignalPoint{Time: times[i], Price: price, Side: "buy"})
		}

		// mark to market
		curEquity := equity
		if position != 0 {
			curEquity = equity + (closes[i]-entryPx)*position*cfg.Leverage
		}
		if curEquity > peak {
			peak = curEquity
		}
		dd := (peak - curEquity) / peak
		if dd > maxDD {
			maxDD = dd
		}
		equityCurve = append(equityCurve, EquityPoint{Time: times[i], Equity: curEquity})
	}

	totalReturn := (equityCurve[len(equityCurve)-1].Equity/cfg.InitialBalance - 1) * 100
	winRate := 0.0
	if win+loss > 0 {
		winRate = float64(win) / float64(win+loss) * 100
	}
	pf := 0.0
	if grossLoss > 0 {
		pf = grossWin / grossLoss
	}

	stats := Stats{
		Trades:       len(trades),
		WinRate:      winRate,
		TotalPnL:     equityCurve[len(equityCurve)-1].Equity - cfg.InitialBalance,
		TotalReturn:  totalReturn,
		MaxDrawdown:  maxDD * 100,
		ProfitFactor: pf,
		Sharpe:       0, // left for future: need risk-free & std dev
	}

	return &Result{
		Equity:  equityCurve,
		Trades:  trades,
		Stats:   stats,
		Signals: signalsOut,
		Klines:  filteredKlines,
		Overlay: IndicatorOverlay{
			EMA1:       mapSeries(times, emaFast),
			EMA2:       mapSeries(times, emaSlow),
			BollUpper:  mapSeries(times, bollUpper),
			BollMid:    mapSeries(times, bollMid),
			BollLower:  mapSeries(times, bollLower),
			MACDLine:   mapSeries(times, macdLine),
			MACDSignal: mapSeries(times, macdSig),
			MACDHist:   mapSeries(times, macdHist),
			RSI:        mapSeries(times, rsiSeries),
			ATR:        mapSeries(times, atrSeries),
			VWAP:       mapSeries(times, vwapSeries),
			Supertrend: mapSeries(times, supertrendSeries),
			ATRUpper:   mapSeries(times, atrUpper),
			ATRLower:   mapSeries(times, atrLower),
		},
		Start:     cfg.Start,
		End:       cfg.End,
		Symbol:    cfg.Symbol,
		Timeframe: cfg.Timeframe,
		Strategy:  cfg.Strategy,
	}, nil
}

// buildSignals returns 1 for buy/hold long, -1 for close, 0 otherwise
func buildSignals(strategy StrategyConfig, closes []float64, hlc3 []float64) ([]int, error) {
	n := len(closes)
	out := make([]int, n)

	switch strategy.Type {
	case StrategyEMACross:
		fast := int(strategy.Params["fast"])
		if fast == 0 {
			fast = 20
		}
		slow := int(strategy.Params["slow"])
		if slow == 0 {
			slow = 50
		}
		emaFast := ema(closes, fast)
		emaSlow := ema(closes, slow)
		for i := 1; i < n; i++ {
			if i >= len(emaFast) || i >= len(emaSlow) {
				continue
			}
			prevCross := emaFast[i-1] <= emaSlow[i-1]
			curCross := emaFast[i] > emaSlow[i]
			if prevCross && curCross {
				out[i] = 1
			} else if emaFast[i] < emaSlow[i] {
				out[i] = -1
			}
		}
	case StrategyRSIThreshold:
		period := int(strategy.Params["period"])
		if period == 0 {
			period = 14
		}
		op := strategy.Params["op"] // 1 for <, 2 for >
		val := strategy.Params["value"]
		rsiSeries := rsi(closes, period)
		for i := 0; i < n; i++ {
			if i >= len(rsiSeries) {
				continue
			}
			if op == 2 {
				if rsiSeries[i] > val {
					out[i] = 1
				} else if rsiSeries[i] < val-5 {
					out[i] = -1
				}
			} else {
				if rsiSeries[i] < val {
					out[i] = 1
				} else if rsiSeries[i] > val+5 {
					out[i] = -1
				}
			}
		}
	case StrategyBollBreak:
		period := int(strategy.Params["period"])
		if period == 0 {
			period = 20
		}
		mult := strategy.Params["mult"]
		if mult == 0 {
			mult = 2
		}
		upper, _, lower := boll(closes, period, mult)
		for i := 1; i < n; i++ {
			if i >= len(upper) {
				continue
			}
			if closes[i-1] <= lower[i-1] && closes[i] > lower[i] {
				out[i] = 1 // 突破下轨向上
			} else if closes[i-1] >= upper[i-1] && closes[i] < upper[i] {
				out[i] = -1 // 跌回轨内平仓
			}
		}
	case StrategyMACDFilter:
		fast := int(strategy.Params["fast"])
		if fast == 0 {
			fast = 12
		}
		slow := int(strategy.Params["slow"])
		if slow == 0 {
			slow = 26
		}
		sigp := int(strategy.Params["signal"])
		if sigp == 0 {
			sigp = 9
		}
		line, signalLine := macd(closes, fast, slow, sigp)
		for i := 1; i < n; i++ {
			if i >= len(signalLine) {
				continue
			}
			if line[i-1] <= signalLine[i-1] && line[i] > signalLine[i] {
				out[i] = 1
			} else if line[i-1] >= signalLine[i-1] && line[i] < signalLine[i] {
				out[i] = -1
			}
		}
	case StrategyCCIThreshold:
		period := int(strategy.Params["period"])
		if period == 0 {
			period = 20
		}
		upper := strategy.Params["upper"]
		if upper == 0 {
			upper = 100
		}
		lower := strategy.Params["lower"]
		if lower == 0 {
			lower = -100
		}
		cciSeries := cci(hlc3, period)
		for i := 0; i < n; i++ {
			if i >= len(cciSeries) {
				continue
			}
			if cciSeries[i] < lower {
				out[i] = 1
			} else if cciSeries[i] > upper {
				out[i] = -1
			}
		}
	default:
		return nil, fmt.Errorf("unsupported strategy type")
	}
	return out, nil
}

// helper to map series into SeriesPoint with matching times length
func mapSeries(times []int64, series []float64) []SeriesPoint {
	points := make([]SeriesPoint, 0, len(series))
	for i, v := range series {
		if i >= len(times) {
			break
		}
		points = append(points, SeriesPoint{Time: times[i], Value: v})
	}
	return points
}
