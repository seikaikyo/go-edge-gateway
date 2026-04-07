package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/seikaikyo/go-edge-gateway/internal/scan"
)

// APIServer serves health, scan API, and embedded UI.
type APIServer struct {
	gw     *Gateway
	server *http.Server
	logger *slog.Logger
}

// NewAPIServer creates the HTTP server with all routes.
func NewAPIServer(gw *Gateway, port int, logger *slog.Logger) *APIServer {
	s := &APIServer{gw: gw, logger: logger}

	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	// Health endpoint
	r.Get("/health", s.handleHealth)

	// Scan API under /api/
	r.Route("/api", func(r chi.Router) {
		r.Mount("/", scan.Router())
	})

	// Embedded scanner UI (SPA fallback)
	r.HandleFunc("/*", scan.StaticHandler())
	r.HandleFunc("/", scan.StaticHandler())

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}
	return s
}

// ListenAndServe starts the API server.
func (s *APIServer) ListenAndServe() error {
	s.logger.Info("api server starting", "addr", s.server.Addr)
	return s.server.ListenAndServe()
}

// Close shuts down the API server.
func (s *APIServer) Close() error {
	return s.server.Close()
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := struct {
		Status  string         `json:"status"`
		Gateway string         `json:"gateway"`
		Plugins map[string]any `json:"plugins"`
	}{
		Status:  "ok",
		Gateway: s.gw.cfg.Gateway.Name,
		Plugins: make(map[string]any),
	}

	allOK := true
	for name, h := range s.gw.Health() {
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
