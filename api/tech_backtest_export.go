package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// handleTechBacktestExport exports trades as CSV
func (s *Server) handleTechBacktestExport(c *gin.Context) {
	id := c.Param("id")
	if s.techBTManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tech backtest manager unavailable"})
		return
	}
	rec, ok := s.techBTManager.Get(id)
	if !ok || rec.Result == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"techbt_%s_trades.csv\"", id))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"entry_time", "exit_time", "entry_px", "exit_px", "qty", "pnl", "pnl_pct", "side"})
	for _, t := range rec.Result.Trades {
		_ = w.Write([]string{
			strconv.FormatInt(t.EntryTime, 10),
			strconv.FormatInt(t.ExitTime, 10),
			fmt.Sprintf("%.6f", t.EntryPx),
			fmt.Sprintf("%.6f", t.ExitPx),
			fmt.Sprintf("%.6f", t.Qty),
			fmt.Sprintf("%.6f", t.PnL),
			fmt.Sprintf("%.4f", t.PnLPct),
			t.Side,
		})
	}
	w.Flush()
}
