package core

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

func newTestServer(t *testing.T, token string, listen string, origins []string) *APIServer {
	t.Helper()

	t.Setenv(DefaultTokenEnv, token)

	cfg := &Config{}
	cfg.Gateway.Name = "test-gw"
	cfg.Gateway.HealthPort = 8080
	cfg.API.TokenEnv = DefaultTokenEnv
	cfg.API.AllowedOrigins = origins
	cfg.Scan.AllowedTargets = []string{"127.0.0.0/8"}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gw := NewGateway(cfg, logger)

	api, err := NewAPIServer(gw, listen, cfg.Gateway.HealthPort, logger)
	if err != nil {
		t.Fatalf("NewAPIServer: %v", err)
	}
	return api
}

func TestAPIRequiresToken(t *testing.T) {
	tests := []struct {
		name       string
		token      string // value placed in the environment
		header     string
		headerVal  string
		wantStatus int
	}{
		{"no token configured refuses", "", "Authorization", "Bearer " + testToken, http.StatusServiceUnavailable},
		{"missing credential", testToken, "", "", http.StatusUnauthorized},
		{"wrong token", testToken, "Authorization", "Bearer wrong-token-value-here", http.StatusUnauthorized},
		{"empty bearer", testToken, "Authorization", "Bearer ", http.StatusUnauthorized},
		{"wrong scheme", testToken, "Authorization", "Basic " + testToken, http.StatusUnauthorized},
		{"token prefix only", testToken, "Authorization", "Bearer " + testToken[:8], http.StatusUnauthorized},
		{"correct bearer", testToken, "Authorization", "Bearer " + testToken, http.StatusOK},
		{"correct header token", testToken, "X-API-Token", testToken, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestServer(t, tt.token, DefaultListenHost, nil)

			req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.headerVal)
			}
			rec := httptest.NewRecorder()
			api.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestShortTokenTreatedAsUnconfigured(t *testing.T) {
	api := newTestServer(t, "short", DefaultListenHost, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	req.Header.Set("Authorization", "Bearer short")
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthStaysUnauthenticated(t *testing.T) {
	api := newTestServer(t, testToken, DefaultListenHost, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	api.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequestGuard(t *testing.T) {
	// Body is valid JSON but omits host, so a request clearing every guard
	// lands on the handler's own validation with 400.
	const body = `{"mode":"tcp"}`

	tests := []struct {
		name        string
		contentType string
		origin      string
		allowed     []string
		wantStatus  int
	}{
		{"json accepted", "application/json", "", nil, http.StatusBadRequest},
		{"json with charset accepted", "application/json; charset=utf-8", "", nil, http.StatusBadRequest},
		{"text plain refused", "text/plain", "", nil, http.StatusUnsupportedMediaType},
		{"form refused", "application/x-www-form-urlencoded", "", nil, http.StatusUnsupportedMediaType},
		{"multipart refused", "multipart/form-data; boundary=x", "", nil, http.StatusUnsupportedMediaType},
		{"missing content type refused", "", "", nil, http.StatusUnsupportedMediaType},
		{"cross site origin refused", "application/json", "https://evil.example", nil, http.StatusForbidden},
		{"same origin accepted", "application/json", "http://example.test", nil, http.StatusBadRequest},
		{"configured origin accepted", "application/json", "https://ui.example", []string{"https://ui.example"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestServer(t, testToken, DefaultListenHost, tt.allowed)

			req := httptest.NewRequest(http.MethodPost, "/api/scan", strings.NewReader(body))
			req.Host = "example.test"
			req.Header.Set("Authorization", "Bearer "+testToken)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rec := httptest.NewRecorder()
			api.Handler().ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListenHostDefaultsToLoopback(t *testing.T) {
	cfg := &Config{}

	tests := []struct {
		name  string
		flag  string
		env   string
		cfgIn string
		want  string
	}{
		{"nothing set", "", "", "", DefaultListenHost},
		{"config only", "", "", "10.0.0.5", "10.0.0.5"},
		{"env beats config", "", "0.0.0.0", "10.0.0.5", "0.0.0.0"},
		{"flag beats env", "192.168.1.9", "0.0.0.0", "10.0.0.5", "192.168.1.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ListenEnv, tt.env)
			cfg.API.Listen = tt.cfgIn
			if got := ListenHost(cfg, tt.flag); got != tt.want {
				t.Fatalf("ListenHost = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServerBindsLoopbackByDefault(t *testing.T) {
	api := newTestServer(t, testToken, DefaultListenHost, nil)
	if got := api.Addr(); got != "127.0.0.1:8080" {
		t.Fatalf("Addr = %q, want 127.0.0.1:8080", got)
	}
}
