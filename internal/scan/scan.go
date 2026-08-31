// Package scan provides on-demand Modbus device scanning via HTTP API.
// Core scan logic lives in go-common/modbus; this package adds job management,
// HTTP handlers, and an embedded React UI.
//
// Every target reaching the Modbus layer is checked against a Policy first:
// TCP hosts against a CIDR allow list, RTU serial ports against the devices the
// host actually exposes. Failures are reported to callers as generic messages
// so the API cannot be used as an internal port-scanning oracle; the underlying
// error is written to the log instead.
package scan

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seikaikyo/go-common/modbus"
	"github.com/seikaikyo/go-common/response"
)

// Client-facing messages. They stay identical across failure causes on purpose:
// "connection refused" and "timed out" must not be distinguishable.
const (
	msgConnectFailed = "device connection failed"
	msgReadFailed    = "device read failed"
	msgScanFailed    = "scan failed"
	msgTargetDenied  = "scan target is not permitted"
	msgSerialDenied  = "serial_port must be one of the available serial devices"
)

// Job represents an async scan job.
type Job struct {
	ID        string             `json:"job_id"`
	Status    string             `json:"status"` // running, completed, failed
	Request   modbus.ScanRequest `json:"request"`
	Result    *modbus.ScanResult `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

type jobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
	seq  int
}

func newJobStore() *jobStore {
	return &jobStore{jobs: make(map[string]*Job)}
}

func (s *jobStore) create(req modbus.ScanRequest) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("scan-%d", s.seq)
	job := &Job{
		ID:        id,
		Status:    "running",
		Request:   req,
		CreatedAt: time.Now(),
	}
	s.jobs[id] = job
	return job
}

// get returns a copy of the job. Readers never hold the live pointer, because
// the scan goroutine keeps writing to it after the handler has returned.
func (s *jobStore) get(id string) (Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[id]
	if !ok {
		return Job{}, false
	}
	return *j, true
}

func (s *jobStore) list() []Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, *j)
	}
	return jobs
}

// finish records the outcome of a scan under the store lock.
func (s *jobStore) finish(id, status, errMsg string, result *modbus.ScanResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return
	}
	j.Status = status
	j.Error = errMsg
	j.Result = result
}

// api holds the handler dependencies: the job store and the target policy.
type api struct {
	store  *jobStore
	policy *Policy
}

// Router returns the chi router for scan API endpoints. The policy governs
// which devices the endpoints may reach; a nil policy falls back to the
// default allow list.
func Router(policy *Policy) chi.Router {
	if policy == nil {
		p, err := NewPolicy(nil)
		if err != nil {
			panic(err)
		}
		policy = p
	}

	a := &api{store: newJobStore(), policy: policy}

	r := chi.NewRouter()

	r.Post("/scan", a.handleScan)
	r.Post("/scan/quick", a.handleQuickScan)
	r.Post("/read", a.handleRead)
	r.Get("/jobs", a.handleListJobs)
	r.Get("/jobs/{id}", a.handleGetJob)
	r.Get("/serial/ports", a.handleListSerialPorts)
	r.Post("/scan/jobs/{id}/to-config", a.handleToConfig)

	return r
}

func validateScanRequest(req *modbus.ScanRequest) string {
	req.ApplyDefaults()
	if req.Mode == "rtu" {
		if req.SerialPort == "" {
			return "serial_port is required for RTU mode"
		}
	} else {
		if req.Host == "" {
			return "host is required for TCP mode"
		}
	}
	return ""
}

// checkTarget applies the policy and writes a generic denial response. It
// returns false when the request must not proceed.
func (a *api) checkTarget(w http.ResponseWriter, r *http.Request, req *modbus.ScanRequest) bool {
	err := a.policy.CheckRequest(req)
	if err == nil {
		return true
	}

	slog.Warn("scan target rejected",
		"path", r.URL.Path,
		"mode", req.Mode,
		"host", req.Host,
		"serial", req.SerialPort,
		"reason", err,
	)

	if errors.Is(err, ErrSerialNotAllowed) {
		response.Err(w, http.StatusBadRequest, msgSerialDenied)
		return false
	}
	response.Err(w, http.StatusForbidden, msgTargetDenied)
	return false
}

func (a *api) handleScan(w http.ResponseWriter, r *http.Request) {
	var req modbus.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if msg := validateScanRequest(&req); msg != "" {
		response.Err(w, http.StatusBadRequest, msg)
		return
	}
	if !a.checkTarget(w, r, &req) {
		return
	}

	job := a.store.create(req)
	id, status := job.ID, job.Status

	go a.runScan(id, req, "scan")

	response.OK(w, map[string]any{
		"job_id":  id,
		"status":  status,
		"message": "scan started, poll GET /api/jobs/" + id + " for results",
	})
}

func (a *api) handleQuickScan(w http.ResponseWriter, r *http.Request) {
	var req modbus.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if msg := validateScanRequest(&req); msg != "" {
		response.Err(w, http.StatusBadRequest, msg)
		return
	}
	if !a.checkTarget(w, r, &req) {
		return
	}
	req.ScanTypes = []string{"holding"}
	if req.AddressEnd > 999 {
		req.AddressEnd = 999
	}
	req.Samples = 1

	job := a.store.create(req)
	id, status := job.ID, job.Status

	go a.runScan(id, req, "quick scan")

	response.OK(w, map[string]any{
		"job_id":  id,
		"status":  status,
		"message": "quick scan started",
	})
}

// runScan executes a scan and records the outcome. Failure detail goes to the
// log; the job carries only a generic message back to the caller.
func (a *api) runScan(jobID string, req modbus.ScanRequest, label string) {
	slog.Info(label+" started", "job_id", jobID, "mode", req.Mode, "host", req.Host, "serial", req.SerialPort)
	result, err := modbus.Scan(req)

	if err != nil {
		a.store.finish(jobID, "failed", msgScanFailed, nil)
		slog.Error(label+" failed", "job_id", jobID, "error", err)
		return
	}

	a.store.finish(jobID, "completed", "", result)
	slog.Info(label+" completed", "job_id", jobID,
		"responsive", result.Summary.Responsive,
		"dynamic", result.Summary.Dynamic,
		"duration_ms", result.DurationMs,
	)
}

func (a *api) handleRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode       string `json:"mode"`
		Host       string `json:"host"`
		Port       int    `json:"port"`
		SerialPort string `json:"serial_port"`
		BaudRate   int    `json:"baud_rate"`
		DataBits   int    `json:"data_bits"`
		StopBits   int    `json:"stop_bits"`
		Parity     string `json:"parity"`
		UnitID     uint8  `json:"unit_id"`
		Type       string `json:"type"`
		Address    uint16 `json:"address"`
		Count      uint16 `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "invalid request body")
		return
	}

	scanReq := modbus.ScanRequest{
		Mode:       req.Mode,
		Host:       req.Host,
		Port:       req.Port,
		SerialPort: req.SerialPort,
		BaudRate:   req.BaudRate,
		DataBits:   req.DataBits,
		StopBits:   req.StopBits,
		Parity:     req.Parity,
		UnitID:     req.UnitID,
		TimeoutMs:  500,
	}
	scanReq.ApplyDefaults()

	if scanReq.Mode == "rtu" && scanReq.SerialPort == "" {
		response.Err(w, http.StatusBadRequest, "serial_port is required for RTU mode")
		return
	}
	if scanReq.Mode == "tcp" && scanReq.Host == "" {
		response.Err(w, http.StatusBadRequest, "host is required for TCP mode")
		return
	}
	if !a.checkTarget(w, r, &scanReq) {
		return
	}

	if req.Type == "" {
		req.Type = "holding"
	}
	if req.Count == 0 {
		req.Count = 1
	}
	if req.Count > 125 {
		req.Count = 125
	}

	conn, err := modbus.NewClient(scanReq)
	if err != nil {
		slog.Warn("read connect failed", "mode", scanReq.Mode, "host", scanReq.Host, "error", err)
		response.Err(w, http.StatusBadGateway, msgConnectFailed)
		return
	}
	defer conn.Close()

	data, err := modbus.ReadBatch(conn.Client, req.Type, req.Address, req.Count)
	if err != nil {
		slog.Warn("read failed", "mode", scanReq.Mode, "host", scanReq.Host, "error", err)
		response.Err(w, http.StatusBadGateway, msgReadFailed)
		return
	}

	values := modbus.BytesToUint16(data)
	response.OK(w, map[string]any{
		"device":  conn.Device,
		"unit_id": scanReq.UnitID,
		"type":    req.Type,
		"address": req.Address,
		"count":   len(values),
		"values":  values,
	})
}

func (a *api) handleListSerialPorts(w http.ResponseWriter, r *http.Request) {
	ports := modbus.ListSerialPorts()
	response.OK(w, map[string]any{"ports": ports})
}

func deviceLabel(req modbus.ScanRequest) string {
	if req.Mode == "rtu" {
		return fmt.Sprintf("rtu:%s@%d", req.SerialPort, req.BaudRate)
	}
	return fmt.Sprintf("%s:%d", req.Host, req.Port)
}

func (a *api) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := a.store.list()
	summaries := make([]map[string]any, len(jobs))
	for i, j := range jobs {
		s := map[string]any{
			"job_id":     j.ID,
			"status":     j.Status,
			"device":     deviceLabel(j.Request),
			"created_at": j.CreatedAt,
		}
		if j.Result != nil {
			s["summary"] = j.Result.Summary
			s["duration_ms"] = j.Result.DurationMs
		}
		if j.Error != "" {
			s["error"] = j.Error
		}
		summaries[i] = s
	}
	response.OK(w, summaries)
}

func (a *api) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, ok := a.store.get(id)
	if !ok {
		response.Err(w, http.StatusNotFound, "job not found")
		return
	}
	response.OK(w, job)
}
