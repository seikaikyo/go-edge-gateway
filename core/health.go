package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// HealthServer exposes an HTTP /health endpoint for the gateway.
type HealthServer struct {
	gw     *Gateway
	server *http.Server
	logger *slog.Logger
}

// NewHealthServer creates a health server on the given port.
func NewHealthServer(gw *Gateway, port int, logger *slog.Logger) *HealthServer {
	hs := &HealthServer{gw: gw, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", hs.handleHealth)

	hs.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	return hs
}

// ListenAndServe starts the health HTTP server.
func (hs *HealthServer) ListenAndServe() error {
	hs.logger.Info("health server starting", "addr", hs.server.Addr)
	return hs.server.ListenAndServe()
}

// Close shuts down the health server.
func (hs *HealthServer) Close() error {
	return hs.server.Close()
}

func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := struct {
		Status  string                          `json:"status"`
		Gateway string                          `json:"gateway"`
		Plugins map[string]any                  `json:"plugins"`
	}{
		Status:  "ok",
		Gateway: hs.gw.cfg.Gateway.Name,
		Plugins: make(map[string]any),
	}

	allOK := true
	for name, h := range hs.gw.Health() {
		if !h.OK {
			allOK = false
		}
		status.Plugins[name] = h
	}

	if !allOK {
		status.Status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	if !allOK {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}
