package techbacktest

import (
	"time"

	"nofx/market"
)

// StrategyType defines the built-in strategy categories
type StrategyType string

const (
	StrategyEMACross     StrategyType = "ema_cross"
	StrategyRSIThreshold StrategyType = "rsi_threshold"
	StrategyBollBreak    StrategyType = "boll_breakout"
	StrategyMACDFilter   StrategyType = "macd_filter"
	StrategyCCIThreshold StrategyType = "cci_threshold"
)

// StrategyConfig describes a simple technical strategy
// For now we support two quick templates:
// - ema_cross: params["fast"], params["slow"]
// - rsi_threshold: params["period"], params["op"] ("<" or ">"), params["value"]
type StrategyConfig struct {
	Type   StrategyType       `json:"type"`
	Params map[string]float64 `json:"params,omitempty"`
}

// Config is the request payload for a backtest run
type Config struct {
	Symbol          string         `json:"symbol"`
	Timeframe       string         `json:"timeframe"`
	Start           time.Time      `json:"start"`
	End             time.Time      `json:"end"`
	InitialBalance  float64        `json:"initial_balance"`
	FeeBps          float64        `json:"fee_bps"`
	SlippageBps     float64        `json:"slippage_bps"`
	Leverage        float64        `json:"leverage"`
	Strategy        StrategyConfig `json:"strategy"`
	StopLossPct     float64        `json:"stop_loss_pct,omitempty"`
	TakeProfitPct   float64        `json:"take_profit_pct,omitempty"`
	TrailingStopPct float64        `json:"trailing_stop_pct,omitempty"`
	// Overlay params
	SupertrendPeriod int     `json:"supertrend_period,omitempty"`
	SupertrendMult   float64 `json:"supertrend_mult,omitempty"`
}

type EquityPoint struct {
	Time   int64   `json:"time"`
	Equity float64 `json:"equity"`
}

type Trade struct {
	EntryTime int64   `json:"entry_time"`
	ExitTime  int64   `json:"exit_time"`
	EntryPx   float64 `json:"entry_px"`
	ExitPx    float64 `json:"exit_px"`
	Qty       float64 `json:"qty"`
	PnL       float64 `json:"pnl"`
	PnLPct    float64 `json:"pnl_pct"`
	Side      string  `json:"side"`
}

type Stats struct {
	Trades       int     `json:"trades"`
	WinRate      float64 `json:"win_rate"`
	TotalPnL     float64 `json:"total_pnl"`
	TotalReturn  float64 `json:"total_return"`
	MaxDrawdown  float64 `json:"max_drawdown"`
	ProfitFactor float64 `json:"profit_factor"`
	Sharpe       float64 `json:"sharpe"`
}

type Result struct {
	Equity    []EquityPoint     `json:"equity"`
	Trades    []Trade           `json:"trades"`
	Stats     Stats             `json:"stats"`
	Start     time.Time         `json:"start"`
	End       time.Time         `json:"end"`
	Symbol    string            `json:"symbol"`
	Timeframe string            `json:"timeframe"`
	Strategy  StrategyConfig    `json:"strategy"`
	Signals   []SignalPoint     `json:"signals,omitempty"`
	Klines    []market.KlineBar `json:"klines,omitempty"`
	Overlay   IndicatorOverlay  `json:"overlay,omitempty"`
}

type SignalPoint struct {
	Time  int64   `json:"time"`
	Price float64 `json:"price"`
	Side  string  `json:"side"` // "buy" or "sell"
}

type SeriesPoint struct {
	Time  int64   `json:"time"`
	Value float64 `json:"value"`
}

type IndicatorOverlay struct {
	EMA1       []SeriesPoint `json:"ema1,omitempty"`
	EMA2       []SeriesPoint `json:"ema2,omitempty"`
	BollUpper  []SeriesPoint `json:"boll_upper,omitempty"`
	BollMid    []SeriesPoint `json:"boll_mid,omitempty"`
	BollLower  []SeriesPoint `json:"boll_lower,omitempty"`
	MACDLine   []SeriesPoint `json:"macd_line,omitempty"`
	MACDSignal []SeriesPoint `json:"macd_signal,omitempty"`
	MACDHist   []SeriesPoint `json:"macd_hist,omitempty"`
	RSI        []SeriesPoint `json:"rsi,omitempty"`
	ATR        []SeriesPoint `json:"atr,omitempty"`
	VWAP       []SeriesPoint `json:"vwap,omitempty"`
	Supertrend []SeriesPoint `json:"supertrend,omitempty"`
	ATRUpper   []SeriesPoint `json:"atr_upper,omitempty"`
	ATRLower   []SeriesPoint `json:"atr_lower,omitempty"`
}

// BatchItem is a single backtest run result used for批量/网格对比
type BatchItem struct {
	Config     Config  `json:"config"`
	Result     *Result `json:"result,omitempty"`
	Error      string  `json:"error,omitempty"`
	DurationMs int64   `json:"duration_ms"`
}

// BatchSummary aggregates batch metrics for quick comparison
type BatchSummary struct {
	Count        int     `json:"count"`
	Success      int     `json:"success"`
	Failure      int     `json:"failure"`
	BestReturn   float64 `json:"best_return"`
	WorstReturn  float64 `json:"worst_return"`
	AvgReturn    float64 `json:"avg_return"`
	AvgDrawdown  float64 `json:"avg_drawdown"`
	AvgWinRate   float64 `json:"avg_win_rate"`
	AvgPF        float64 `json:"avg_profit_factor"`
	TotalRuntime int64   `json:"total_runtime_ms"`
}

// BatchJob represents a running or finished batch task
type BatchJob struct {
	ID        string       `json:"id"`
	Status    RunStatus    `json:"status"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   time.Time    `json:"ended_at"`
	Items     []BatchItem  `json:"items"`
	Summary   BatchSummary `json:"summary"`
	Parallel  int          `json:"parallel"`
	Total     int          `json:"total"`
	Done      int          `json:"done"`
	Error     string       `json:"error,omitempty"`
}
