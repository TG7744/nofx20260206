package techbacktest

import (
	"fmt"
	"sync"
	"time"
)

// BatchManager manages async batch jobs with in-memory storage.
type BatchManager struct {
	mu   sync.Mutex
	jobs map[string]*BatchJob
	seq  int64
}

func NewBatchManager() *BatchManager {
	return &BatchManager{
		jobs: make(map[string]*BatchJob),
	}
}

func (m *BatchManager) nextID() string {
	m.seq++
	return time.Now().UTC().Format("20060102_150405") + "-" + fmtSeq(m.seq)
}

// Create creates a batch job record
func (m *BatchManager) Create(total int, parallel int) *BatchJob {
	if parallel <= 0 {
		parallel = 1
	}
	job := &BatchJob{
		ID:        m.nextID(),
		Status:    StatusPending,
		StartedAt: time.Now().UTC(),
		Items:     make([]BatchItem, total),
		Parallel:  parallel,
		Total:     total,
	}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()
	return job
}

// Get returns job by id
func (m *BatchManager) Get(id string) (*BatchJob, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	return j, ok
}

// RunAsync executes configs and updates job progress
func (m *BatchManager) RunAsync(job *BatchJob, cfgs []Config) {
	go func() {
		m.updateStatus(job.ID, StatusRunning)
		items := RunBatchParallel(cfgs, job.Parallel)

		m.mu.Lock()
		if j, ok := m.jobs[job.ID]; ok {
			j.Items = items
			j.Summary = SummarizeBatch(items)
			j.Status = StatusDone
			j.Done = j.Total
			j.EndedAt = time.Now().UTC()
		}
		m.mu.Unlock()
	}()
}

// RunAsyncProgress executes with progress updates per item
func (m *BatchManager) RunAsyncProgress(job *BatchJob, cfgs []Config) {
	go func() {
		m.updateStatus(job.ID, StatusRunning)
		type task struct {
			idx int
			cfg Config
		}
		in := make(chan task)
		var wg sync.WaitGroup
		for i := 0; i < job.Parallel; i++ {
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
					m.mu.Lock()
					if j, ok := m.jobs[job.ID]; ok {
						j.Items[t.idx] = item
						j.Done++
					}
					m.mu.Unlock()
				}
			}()
		}
		for i, cfg := range cfgs {
			in <- task{idx: i, cfg: cfg}
		}
		close(in)
		wg.Wait()

		m.mu.Lock()
		if j, ok := m.jobs[job.ID]; ok {
			j.Summary = SummarizeBatch(j.Items)
			j.Status = StatusDone
			j.EndedAt = time.Now().UTC()
		}
		m.mu.Unlock()
	}()
}

func (m *BatchManager) updateStatus(id string, status RunStatus) {
	m.mu.Lock()
	if j, ok := m.jobs[id]; ok {
		j.Status = status
	}
	m.mu.Unlock()
}

// fmtSeq renders incremental sequence padded to 3 digits
func fmtSeq(n int64) string {
	if n < 10 {
		return "00" + fmt.Sprint(n)
	}
	if n < 100 {
		return "0" + fmt.Sprint(n)
	}
	return fmt.Sprint(n)
}
