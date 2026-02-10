package techbacktest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RunStatus string

const (
	StatusPending RunStatus = "pending"
	StatusRunning RunStatus = "running"
	StatusDone    RunStatus = "done"
	StatusFailed  RunStatus = "failed"
)

type RunRecord struct {
	ID        string    `json:"id"`
	Status    RunStatus `json:"status"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Error     string    `json:"error,omitempty"`
	Result    *Result   `json:"result,omitempty"`
	Config    Config    `json:"config"`
}

type Manager struct {
	mu   sync.Mutex
	runs map[string]*RunRecord
	root string
}

func NewManager() *Manager {
	root := filepath.Join("data", "tech_backtest_runs")
	_ = os.MkdirAll(root, 0o755)
	m := &Manager{runs: make(map[string]*RunRecord), root: root}
	m.loadExisting()
	return m
}

func (m *Manager) Create(cfg Config) *RunRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := time.Now().UTC().Format("20060102_150405.000")
	rec := &RunRecord{
		ID:        id,
		Status:    StatusPending,
		StartedAt: time.Now().UTC(),
		Config:    cfg,
	}
	m.runs[id] = rec
	m.save(rec)
	return rec
}

func (m *Manager) List() []*RunRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*RunRecord, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, r)
	}
	return out
}

func (m *Manager) Get(id string) (*RunRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	return r, ok
}

func (m *Manager) RunAsync(rec *RunRecord) {
	go m.run(rec)
}

// RunSync executes the backtest in current goroutine and persists result/status.
func (m *Manager) RunSync(rec *RunRecord) (*Result, error) {
	return m.run(rec)
}

// run is the shared executor for both sync/async modes.
func (m *Manager) run(rec *RunRecord) (*Result, error) {
	m.mu.Lock()
	rec.Status = StatusRunning
	m.mu.Unlock()

	res, err := Run(rec.Config)

	m.mu.Lock()
	defer m.mu.Unlock()
	rec.EndedAt = time.Now().UTC()
	if err != nil {
		rec.Status = StatusFailed
		rec.Error = err.Error()
		m.save(rec)
		return nil, err
	}

	rec.Status = StatusDone
	rec.Result = res
	m.save(rec)
	return res, nil
}

// ---------- persistence ----------
func (m *Manager) save(rec *RunRecord) {
	path := filepath.Join(m.root, rec.ID+".json")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rec)
}

func (m *Manager) loadExisting() {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(m.root, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rec RunRecord
		if json.Unmarshal(b, &rec) == nil {
			m.runs[rec.ID] = &rec
		}
	}
}
