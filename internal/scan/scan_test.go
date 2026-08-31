package scan

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/seikaikyo/go-common/modbus"
)

// leakyTerms are substrings that must never reach a client. They are what a Go
// network or file error carries, and they tell a caller whether a port was
// closed, filtered, or open.
var leakyTerms = []string{
	"connection refused",
	"dial tcp",
	"i/o timeout",
	"no such file",
	"permission denied",
	"no route to host",
	"connect serial",
	"deadline exceeded",
}

// closedLoopbackPort returns a loopback port with nothing listening on it, so
// a connection attempt fails immediately without touching the network.
func closedLoopbackPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func testRouter(t *testing.T, cidrs []string, serialPorts []string) chi.Router {
	t.Helper()
	p, err := NewPolicy(cidrs)
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}
	p.listSerialPorts = func() []string { return serialPorts }
	p.lookupIP = func(string) ([]net.IP, error) { return nil, nil }
	return Router(p)
}

func post(t *testing.T, r chi.Router, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func assertNoLeak(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, term := range leakyTerms {
		if strings.Contains(lower, term) {
			t.Fatalf("response leaks transport detail %q: %s", term, body)
		}
	}
}

func TestScanEndpointsEnforceHostAllowList(t *testing.T) {
	r := testRouter(t, []string{"127.0.0.0/8"}, nil)
	closed := strconv.Itoa(closedLoopbackPort(t))

	hosts := []struct {
		name string
		host string
		port string
		// denied is false for the one target inside the allow list; that case
		// only asserts the policy did not reject it.
		denied bool
	}{
		{"allowed loopback", "127.0.0.1", closed, false},
		{"public host", "8.8.8.8", "502", true},
		{"metadata host", "169.254.169.254", "80", true},
		{"private range outside list", "10.1.2.3", "502", true},
		{"other private range", "192.168.1.10", "502", true},
	}

	for _, path := range []string{"/scan", "/scan/quick", "/read"} {
		for _, h := range hosts {
			t.Run(path+" "+h.name, func(t *testing.T) {
				body := `{"mode":"tcp","host":"` + h.host + `","port":` + h.port + `}`
				rec := post(t, r, path, body)

				if !h.denied {
					if rec.Code == http.StatusForbidden {
						t.Fatalf("allowed host rejected: %s", rec.Body.String())
					}
					return
				}
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusForbidden, rec.Body.String())
				}
				assertNoLeak(t, rec.Body.String())
				if strings.Contains(rec.Body.String(), h.host) {
					t.Fatalf("denial echoes the target back: %s", rec.Body.String())
				}
			})
		}
	}
}

func TestScanEndpointsEnforceSerialAllowList(t *testing.T) {
	r := testRouter(t, nil, []string{"/dev/ttyUSB0"})

	tests := []struct {
		name   string
		serial string
	}{
		{"unlisted device", "/dev/ttyUSB1"},
		{"arbitrary path", "/etc/passwd"},
		{"block device", "/dev/sda"},
		{"traversal", "/dev/../etc/shadow"},
	}

	for _, path := range []string{"/scan", "/scan/quick", "/read"} {
		for _, tt := range tests {
			t.Run(path+" "+tt.name, func(t *testing.T) {
				body := `{"mode":"rtu","serial_port":"` + tt.serial + `"}`
				rec := post(t, r, path, body)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), "available serial devices") {
					t.Fatalf("unexpected message: %s", rec.Body.String())
				}
				assertNoLeak(t, rec.Body.String())
			})
		}
	}
}

// TestReadConnectFailureIsGeneric points /read at a closed loopback port. The
// caller must not be able to tell a refused connection from a timeout.
func TestReadConnectFailureIsGeneric(t *testing.T) {
	r := testRouter(t, []string{"127.0.0.0/8"}, nil)
	port := strconv.Itoa(closedLoopbackPort(t))

	body := `{"mode":"tcp","host":"127.0.0.1","port":` + port + `,"type":"holding","address":0,"count":1}`
	rec := post(t, r, "/read", body)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadGateway, rec.Body.String())
	}

	var env struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error != msgConnectFailed {
		t.Fatalf("error = %q, want %q", env.Error, msgConnectFailed)
	}
	assertNoLeak(t, rec.Body.String())
}

// TestFailedJobErrorIsGeneric covers the async path: GET /jobs hands the job's
// error string to the caller, so it must not carry the transport error.
func TestFailedJobErrorIsGeneric(t *testing.T) {
	a := &api{store: newJobStore()}
	req := modbus.ScanRequest{
		Mode:         "tcp",
		Host:         "127.0.0.1",
		Port:         closedLoopbackPort(t),
		ScanTypes:    []string{"holding"},
		AddressStart: 0,
		AddressEnd:   1,
		Samples:      1,
		TimeoutMs:    100,
	}
	id := a.store.create(req).ID

	a.runScan(id, req, "scan")

	job, ok := a.store.get(id)
	if !ok {
		t.Fatalf("job %q missing after run", id)
	}
	if job.Status != "failed" {
		t.Fatalf("status = %q, want failed", job.Status)
	}
	if job.Error != msgScanFailed {
		t.Fatalf("job error = %q, want %q", job.Error, msgScanFailed)
	}

	rec := httptest.NewRecorder()
	a.handleListJobs(rec, httptest.NewRequest(http.MethodGet, "/jobs", nil))
	assertNoLeak(t, rec.Body.String())

	rec = httptest.NewRecorder()
	a.handleGetJob(rec, httptest.NewRequest(http.MethodGet, "/jobs/"+id, nil))
	assertNoLeak(t, rec.Body.String())
}

// TestRouterRejectsUnvalidatedDefaults guards the default policy: an empty
// allow list must not mean "allow everything".
func TestRouterRejectsUnvalidatedDefaults(t *testing.T) {
	r := testRouter(t, nil, nil)

	rec := post(t, r, "/scan", `{"mode":"tcp","host":"8.8.8.8","port":502}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}
