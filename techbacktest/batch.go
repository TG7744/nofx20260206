package techbacktest

import (
	"sync"
	"time"
)

// RunBatch runs multiple configs sequentially and returns all results (errors are captured per-item)
func RunBatch(cfgs []Config) []BatchItem {
	items := make([]BatchItem, 0, len(cfgs))
	for _, cfg := range cfgs {
		start := time.Now()
		res, err := Run(cfg)
		item := BatchItem{
			Config:     cfg,
			Result:     res,
			DurationMs: time.Since(start).Milliseconds(),
		}
		if err != nil {
			item.Error = err.Error()
		}
		items = append(items, item)
	}
	return items
}

// RunBatchParallel runs configs with a worker pool (order preserved in output).
// workers<=1 falls back to sequential.
func RunBatchParallel(cfgs []Config, workers int) []BatchItem {
	if workers <= 1 {
		return RunBatch(cfgs)
	}
	type task struct {
		idx int
		cfg Config
	}
	in := make(chan task)
	out := make([]BatchItem, len(cfgs))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range in {
				start := time.Now()
				res, err := Run(t.cfg)
				item := BatchItem{
					Config:     t.cfg,
					Result:     res,
					DurationMs: time.Since(start).Milliseconds(),
				}
				if err != nil {
					item.Error = err.Error()
				}
				out[t.idx] = item
			}
		}()
	}
	for i, cfg := range cfgs {
		in <- task{idx: i, cfg: cfg}
	}
	close(in)
	wg.Wait()
	return out
}

// SummarizeBatch computes aggregate stats over batch items
func SummarizeBatch(items []BatchItem) BatchSummary {
	sum := BatchSummary{Count: len(items)}
	var retSum, ddSum, winSum, pfSum float64
	for _, it := range items {
		sum.TotalRuntime += it.DurationMs
		if it.Error != "" || it.Result == nil {
			sum.Failure++
			continue
		}
		sum.Success++
		stats := it.Result.Stats
		retSum += stats.TotalReturn
		ddSum += stats.MaxDrawdown
		winSum += stats.WinRate
		pfSum += stats.ProfitFactor
		if stats.TotalReturn > sum.BestReturn || sum.Success == 1 {
			sum.BestReturn = stats.TotalReturn
		}
		if stats.TotalReturn < sum.WorstReturn || sum.Success == 1 {
			sum.WorstReturn = stats.TotalReturn
		}
	}
	if sum.Success > 0 {
		sum.AvgReturn = retSum / float64(sum.Success)
		sum.AvgDrawdown = ddSum / float64(sum.Success)
		sum.AvgWinRate = winSum / float64(sum.Success)
		sum.AvgPF = pfSum / float64(sum.Success)
	}
	return sum
}
