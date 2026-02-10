package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const marketCacheRoot = "data/market_cache"

type klineCacheRecord struct {
	OpenTime int64   `json:"open_time"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

type cacheSummary struct {
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
	StartTS   int64  `json:"start_ts"`
	EndTS     int64  `json:"end_ts"`
	Count     int    `json:"count"`
	SizeBytes int64  `json:"size_bytes"`
}

func cacheFilePath(symbol, timeframe string) string {
	return filepath.Join(marketCacheRoot, symbol, fmt.Sprintf("%s.jsonl", timeframe))
}

// handleListMarketCache lists cached kline files with optional filters.
func (s *Server) handleListMarketCache(c *gin.Context) {
	symbolFilter := c.Query("symbol")
	tfFilter := c.Query("timeframe")
	startTS, _ := strconv.ParseInt(c.DefaultQuery("start_ts", "0"), 10, 64)
	endTS, _ := strconv.ParseInt(c.DefaultQuery("end_ts", "0"), 10, 64)

	var summaries []cacheSummary

	if _, err := os.Stat(marketCacheRoot); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{"items": summaries})
		return
	}

	// Walk cache directory
	err := filepath.Walk(marketCacheRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".jsonl" {
			return nil
		}

		sym := filepath.Base(filepath.Dir(path))
		tf := filepath.Base(path[:len(path)-len(filepath.Ext(path))])

		if symbolFilter != "" && sym != symbolFilter {
			return nil
		}
		if tfFilter != "" && tf != tfFilter {
			return nil
		}

		start, end, count := int64(0), int64(0), 0
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var rec klineCacheRecord
			if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
				continue
			}
			if count == 0 {
				start = rec.OpenTime
			}
			end = rec.OpenTime
			count++
		}
		if startTS > 0 && end < startTS {
			return nil
		}
		if endTS > 0 && start > endTS {
			return nil
		}

		summaries = append(summaries, cacheSummary{
			Symbol:    sym,
			Timeframe: tf,
			StartTS:   start,
			EndTS:     end,
			Count:     count,
			SizeBytes: info.Size(),
		})
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list cache"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": summaries})
}

type cacheDeleteRequest struct {
	Symbol    string `json:"symbol" binding:"required"`
	Timeframe string `json:"timeframe" binding:"required"`
	StartTS   int64  `json:"start_ts"` // optional
	EndTS     int64  `json:"end_ts"`   // optional
}

// handleDeleteMarketCache deletes cached klines within a time range.
func (s *Server) handleDeleteMarketCache(c *gin.Context) {
	var req cacheDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "Invalid request parameters")
		return
	}

	path := cacheFilePath(req.Symbol, req.Timeframe)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "No cache file"})
		return
	}

	tmp := path + ".tmp"
	in, err := os.Open(path)
	if err != nil {
		SafeInternalError(c, "Open cache file", err)
		return
	}
	defer in.Close()

	out, err := os.Create(tmp)
	if err != nil {
		SafeInternalError(c, "Create temp file", err)
		return
	}

	scanner := bufio.NewScanner(in)
	writer := bufio.NewWriter(out)
	kept := 0
	for scanner.Scan() {
		var rec klineCacheRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if (req.StartTS > 0 && rec.OpenTime < req.StartTS) || (req.EndTS > 0 && rec.OpenTime > req.EndTS) {
			// keep outside range
			writer.Write(scanner.Bytes())
			writer.WriteByte('\n')
			kept++
		}
	}
	writer.Flush()
	out.Close()

	if kept == 0 {
		// remove file
		os.Remove(path)
		os.Remove(tmp)
	} else {
		os.Remove(path)
		os.Rename(tmp, path)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Cache updated"})
}

// handleDownloadMarketCache streams cached klines within optional time window.
func (s *Server) handleDownloadMarketCache(c *gin.Context) {
	symbol := c.Query("symbol")
	tf := c.Query("timeframe")
	if symbol == "" || tf == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol and timeframe are required"})
		return
	}
	startTS, _ := strconv.ParseInt(c.DefaultQuery("start_ts", "0"), 10, 64)
	endTS, _ := strconv.ParseInt(c.DefaultQuery("end_ts", "0"), 10, 64)

	path := cacheFilePath(symbol, tf)
	f, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cache not found"})
		return
	}
	defer f.Close()

	c.Header("Content-Type", "application/jsonl")
	filename := fmt.Sprintf("%s_%s_%s.jsonl", symbol, tf, time.Now().Format("20060102150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec klineCacheRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if startTS > 0 && rec.OpenTime < startTS {
			continue
		}
		if endTS > 0 && rec.OpenTime > endTS {
			continue
		}
		c.Writer.Write(scanner.Bytes())
		c.Writer.Write([]byte("\n"))
	}
}
