// Package scan provides on-demand Modbus device scanning via HTTP API.
// Core scan logic lives in go-common/modbus; this package adds job management,
// HTTP handlers, and an embedded React UI.
package scan

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/seikaikyo/go-common/modbus"
	"github.com/seikaikyo/go-common/response"
)

// Job represents an async scan job.
type Job struct {
	ID        string            `json:"job_id"`
	Status    string            `json:"status"` // running, completed, failed
	Request   modbus.ScanRequest `json:"request"`
	Result    *modbus.ScanResult `json:"result,omitempty"`
	Error     string            `json:"error,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

type jobStore struct {
	mu   sync.RWMutex
	jobs map[string]*Job
	seq  int
}

var store = &jobStore{jobs: make(map[string]*Job)}

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

func (s *jobStore) get(id string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[id]
}

func (s *jobStore) list() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

// Router returns the chi router for scan API endpoints.
func Router() chi.Router {
	r := chi.NewRouter()

	r.Post("/scan", handleScan)
	r.Post("/scan/quick", handleQuickScan)
	r.Post("/read", handleRead)
	r.Get("/jobs", handleListJobs)
	r.Get("/jobs/{id}", handleGetJob)
	r.Get("/serial/ports", handleListSerialPorts)
	r.Post("/scan/jobs/{id}/to-config", handleToConfig)

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

func handleScan(w http.ResponseWriter, r *http.Request) {
	var req modbus.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if msg := validateScanRequest(&req); msg != "" {
		response.Err(w, http.StatusBadRequest, msg)
		return
	}

	job := store.create(req)

	go func() {
		slog.Info("scan started", "job_id", job.ID, "mode", req.Mode, "host", req.Host, "serial", req.SerialPort)
		result, err := modbus.Scan(req)

		store.mu.Lock()
		defer store.mu.Unlock()

		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
			slog.Error("scan failed", "job_id", job.ID, "error", err)
		} else {
			job.Status = "completed"
			job.Result = result
			slog.Info("scan completed", "job_id", job.ID,
				"responsive", result.Summary.Responsive,
				"dynamic", result.Summary.Dynamic,
				"duration_ms", result.DurationMs,
			)
		}
	}()

	response.OK(w, map[string]any{
		"job_id":  job.ID,
		"status":  job.Status,
		"message": "scan started, poll GET /api/jobs/" + job.ID + " for results",
	})
}

func handleQuickScan(w http.ResponseWriter, r *http.Request) {
	var req modbus.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if msg := validateScanRequest(&req); msg != "" {
		response.Err(w, http.StatusBadRequest, msg)
		return
	}
	req.ScanTypes = []string{"holding"}
	if req.AddressEnd > 999 {
		req.AddressEnd = 999
	}
	req.Samples = 1

	job := store.create(req)

	go func() {
		slog.Info("quick scan started", "job_id", job.ID, "host", req.Host)
		result, err := modbus.Scan(req)

		store.mu.Lock()
		defer store.mu.Unlock()

		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = "completed"
			job.Result = result
		}
	}()

	response.OK(w, map[string]any{
		"job_id":  job.ID,
		"status":  job.Status,
		"message": "quick scan started",
	})
}

func handleRead(w http.ResponseWriter, r *http.Request) {
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
		response.Err(w, http.StatusBadGateway, "connect failed: "+err.Error())
		return
	}
	defer conn.Close()

	data, err := modbus.ReadBatch(conn.Client, req.Type, req.Address, req.Count)
	if err != nil {
		response.Err(w, http.StatusBadGateway, "read failed: "+err.Error())
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

func handleListSerialPorts(w http.ResponseWriter, r *http.Request) {
	ports := modbus.ListSerialPorts()
	response.OK(w, map[string]any{"ports": ports})
}

func deviceLabel(req modbus.ScanRequest) string {
	if req.Mode == "rtu" {
		return fmt.Sprintf("rtu:%s@%d", req.SerialPort, req.BaudRate)
	}
	return fmt.Sprintf("%s:%d", req.Host, req.Port)
}

func handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := store.list()
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

func handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job := store.get(id)
	if job == nil {
		response.Err(w, http.StatusNotFound, "job not found")
		return
	}
	response.OK(w, job)
}
