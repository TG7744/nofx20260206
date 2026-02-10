package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleTechBacktestBatchStatus returns batch job with progress
func (s *Server) handleTechBacktestBatchStatus(c *gin.Context) {
	if s.techBTBatchMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "batch manager unavailable"})
		return
	}
	id := c.Param("id")
	job, ok := s.techBTBatchMgr.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}
