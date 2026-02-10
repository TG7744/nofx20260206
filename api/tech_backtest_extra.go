package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleTechBacktestRuns lists run records (lightweight)
func (s *Server) handleTechBacktestRuns(c *gin.Context) {
	if s.techBTManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tech backtest manager unavailable"})
		return
	}
	runs := s.techBTManager.List()
	c.JSON(http.StatusOK, gin.H{"items": runs})
}

// handleTechBacktestResult returns specific run result
func (s *Server) handleTechBacktestResult(c *gin.Context) {
	if s.techBTManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "tech backtest manager unavailable"})
		return
	}
	id := c.Param("id")
	rec, ok := s.techBTManager.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}
