package core

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

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
//
// listenHost is the interface to bind. Use ListenHost to resolve it; binding
// anything other than loopback exposes the scan API to the network and is only
// safe together with a configured API token.
func NewAPIServer(gw *Gateway, listenHost string, port int, logger *slog.Logger) (*APIServer, error) {
	s := &APIServer{gw: gw, logger: logger}

	policy, err := scan.NewPolicy(gw.cfg.Scan.AllowedTargets)
	if err != nil {
		return nil, err
	}

	token := APIToken(gw.cfg)
	switch {
	case token == "":
		logger.Warn("api token not configured; /api endpoints will refuse every request",
			"token_env", gw.cfg.API.TokenEnv)
	case !IsLoopbackHost(listenHost):
		logger.Warn("api bound to a non-loopback address", "listen", listenHost)
	}

	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

	// Health endpoint stays unauthenticated for platform health checks.
	r.Get("/health", s.handleHealth)

	// Scan API under /api/ — authenticated, JSON-only, same-origin.
	r.Route("/api", func(r chi.Router) {
		r.Use(requestGuard(gw.cfg.API.AllowedOrigins))
		r.Use(tokenAuth(token, logger))
		r.Mount("/", scan.Router(policy))
	})

	// Embedded scanner UI (SPA fallback)
	r.HandleFunc("/*", scan.StaticHandler())
	r.HandleFunc("/", scan.StaticHandler())

	s.server = &http.Server{
		Addr:              net.JoinHostPort(listenHost, strconv.Itoa(port)),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

// Handler exposes the routed handler, chiefly so tests can exercise the
// middleware chain without opening a socket.
func (s *APIServer) Handler() http.Handler {
	return s.server.Handler
}

// Addr reports the resolved bind address.
func (s *APIServer) Addr() string {
	return s.server.Addr
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
