package api

import (
	"net/http"
	"time"

	"nofx/logger"
	"nofx/techbacktest"

	"github.com/gin-gonic/gin"
)

type techBacktestRequest struct {
	Symbol           string                      `json:"symbol" binding:"required"`
	Timeframe        string                      `json:"timeframe" binding:"required"`
	Start            string                      `json:"start"` // ISO8601
	End              string                      `json:"end"`   // ISO8601
	InitialBalance   float64                     `json:"initial_balance"`
	FeeBps           float64                     `json:"fee_bps"`
	SlippageBps      float64                     `json:"slippage_bps"`
	Leverage         float64                     `json:"leverage"`
	Strategy         techbacktest.StrategyConfig `json:"strategy" binding:"required"`
	StopLossPct      float64                     `json:"stop_loss_pct"`
	TakeProfitPct    float64                     `json:"take_profit_pct"`
	TrailingStopPct  float64                     `json:"trailing_stop_pct"`
	SupertrendPeriod int                         `json:"supertrend_period"`
	SupertrendMult   float64                     `json:"supertrend_mult"`
}

func (s *Server) handleTechBacktestRun(c *gin.Context) {
	var req techBacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	startTime := parseTimeOrZero(req.Start)
	endTime := parseTimeOrZero(req.End)

	cfg := techbacktest.Config{
		Symbol:           req.Symbol,
		Timeframe:        req.Timeframe,
		Start:            startTime,
		End:              endTime,
		InitialBalance:   req.InitialBalance,
		FeeBps:           req.FeeBps,
		SlippageBps:      req.SlippageBps,
		Leverage:         req.Leverage,
		Strategy:         req.Strategy,
		StopLossPct:      req.StopLossPct,
		TakeProfitPct:    req.TakeProfitPct,
		TrailingStopPct:  req.TrailingStopPct,
		SupertrendPeriod: req.SupertrendPeriod,
		SupertrendMult:   req.SupertrendMult,
	}

	async := c.DefaultQuery("async", "false") == "true"
	rec := s.techBTManager.Create(cfg)

	if async {
		s.techBTManager.RunAsync(rec)
		c.JSON(http.StatusOK, gin.H{"run_id": rec.ID, "status": rec.Status})
		return
	}

	res, err := s.techBTManager.RunSync(rec)
	if err != nil {
		logger.Errorf("Tech backtest failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func parseTimeOrZero(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
