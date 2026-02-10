package api

import (
	"net/http"

	"nofx/techbacktest"

	"github.com/gin-gonic/gin"
)

type techBacktestBatchRequest struct {
	Items    []techBacktestRequest `json:"items" binding:"required"`
	Parallel int                   `json:"parallel"`
}

// handleTechBacktestBatch runs multiple configs sequentially for quick网格/对比
func (s *Server) handleTechBacktestBatch(c *gin.Context) {
	var req techBacktestBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	cfgs := make([]techbacktest.Config, 0, len(req.Items))
	for _, r := range req.Items {
		cfgs = append(cfgs, techbacktest.Config{
			Symbol:           r.Symbol,
			Timeframe:        r.Timeframe,
			Start:            parseTimeOrZero(r.Start),
			End:              parseTimeOrZero(r.End),
			InitialBalance:   r.InitialBalance,
			FeeBps:           r.FeeBps,
			SlippageBps:      r.SlippageBps,
			Leverage:         r.Leverage,
			Strategy:         r.Strategy,
			StopLossPct:      r.StopLossPct,
			TakeProfitPct:    r.TakeProfitPct,
			TrailingStopPct:  r.TrailingStopPct,
			SupertrendPeriod: r.SupertrendPeriod,
			SupertrendMult:   r.SupertrendMult,
		})
	}

	p := req.Parallel
	if p <= 0 {
		p = 1
	}
	async := c.DefaultQuery("async", "false") == "true"

	if async {
		job := s.techBTBatchMgr.Create(len(cfgs), p)
		s.techBTBatchMgr.RunAsyncProgress(job, cfgs)
		c.JSON(http.StatusOK, gin.H{
			"batch_id": job.ID,
			"status":   job.Status,
			"total":    job.Total,
			"parallel": job.Parallel,
		})
		return
	}

	items := techbacktest.RunBatchParallel(cfgs, p)
	c.JSON(http.StatusOK, gin.H{
		"items":    items,
		"summary":  techbacktest.SummarizeBatch(items),
		"parallel": p,
	})
}
