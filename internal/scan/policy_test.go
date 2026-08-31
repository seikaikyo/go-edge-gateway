package scan

import (
	"errors"
	"net"
	"testing"

	"github.com/seikaikyo/go-common/modbus"
)

func mustPolicy(t *testing.T, cidrs []string) *Policy {
	t.Helper()
	p, err := NewPolicy(cidrs)
	if err != nil {
		t.Fatalf("NewPolicy(%v): %v", cidrs, err)
	}
	return p
}

func TestNewPolicyRejectsBadCIDR(t *testing.T) {
	tests := []struct {
		name  string
		cidrs []string
	}{
		{"not a cidr", []string{"192.168.1.1"}},
		{"garbage", []string{"not-a-network"}},
		{"bad mask", []string{"10.0.0.0/64"}},
		{"only blanks", []string{"  ", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewPolicy(tt.cidrs); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestCheckHost(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		host    string
		wantErr error
	}{
		{"loopback allowed by default", nil, "127.0.0.1", nil},
		{"private allowed by default", nil, "192.168.1.50", nil},
		{"ipv6 loopback allowed by default", nil, "::1", nil},
		{"public address denied by default", nil, "8.8.8.8", ErrTargetNotAllowed},
		{"public ipv6 denied by default", nil, "2001:4860:4860::8888", ErrTargetNotAllowed},
		{"link local denied by default", nil, "169.254.169.254", ErrTargetNotAllowed},
		{"link local denied even when listed", []string{"169.254.0.0/16"}, "169.254.169.254", ErrTargetNotAllowed},
		{"ipv6 link local denied even when listed", []string{"fe80::/10"}, "fe80::1", ErrTargetNotAllowed},
		{"narrow list allows member", []string{"10.20.30.0/24"}, "10.20.30.7", nil},
		{"narrow list denies neighbour", []string{"10.20.30.0/24"}, "10.20.31.7", ErrTargetNotAllowed},
		{"narrow list denies other private range", []string{"10.20.30.0/24"}, "192.168.1.1", ErrTargetNotAllowed},
		{"ipv4 mapped ipv6 denied", nil, "::ffff:8.8.8.8", ErrTargetNotAllowed},
		{"bracketed ipv6 allowed", nil, "[::1]", nil},
		{"empty host invalid", nil, "", ErrTargetInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustPolicy(t, tt.cidrs)
			// No resolver in these cases: every host is a literal address.
			p.lookupIP = func(string) ([]net.IP, error) {
				t.Fatalf("unexpected DNS lookup for %q", tt.host)
				return nil, nil
			}

			err := p.CheckHost(tt.host)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckHost(%q) = %v, want %v", tt.host, err, tt.wantErr)
			}
		})
	}
}

func TestCheckHostResolvesNames(t *testing.T) {
	tests := []struct {
		name     string
		resolved []net.IP
		lookErr  error
		wantErr  error
	}{
		{"single allowed address", []net.IP{net.ParseIP("192.168.1.10")}, nil, nil},
		{"single denied address", []net.IP{net.ParseIP("8.8.8.8")}, nil, ErrTargetNotAllowed},
		{"one denied among allowed", []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("8.8.8.8")}, nil, ErrTargetNotAllowed},
		{"metadata address", []net.IP{net.ParseIP("169.254.169.254")}, nil, ErrTargetNotAllowed},
		{"lookup failure", nil, errors.New("nxdomain"), ErrTargetInvalid},
		{"empty result", []net.IP{}, nil, ErrTargetInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustPolicy(t, nil)
			p.lookupIP = func(string) ([]net.IP, error) { return tt.resolved, tt.lookErr }

			err := p.CheckHost("plc.example.internal")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckHost = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckSerialPort(t *testing.T) {
	available := []string{"/dev/ttyUSB0", "/dev/cu.usbserial-1420"}

	tests := []struct {
		name    string
		port    string
		wantErr error
	}{
		{"available port", "/dev/ttyUSB0", nil},
		{"second available port", "/dev/cu.usbserial-1420", nil},
		{"unlisted device", "/dev/ttyUSB9", ErrSerialNotAllowed},
		{"arbitrary file", "/etc/passwd", ErrSerialNotAllowed},
		{"block device", "/dev/sda", ErrSerialNotAllowed},
		{"traversal", "/dev/../etc/shadow", ErrSerialNotAllowed},
		{"prefix of a real port", "/dev/ttyUSB", ErrSerialNotAllowed},
		{"empty", "", ErrSerialNotAllowed},
		{"whitespace only", "   ", ErrSerialNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustPolicy(t, nil)
			p.listSerialPorts = func() []string { return available }

			err := p.CheckSerialPort(tt.port)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckSerialPort(%q) = %v, want %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestCheckSerialPortWithNoDevices(t *testing.T) {
	p := mustPolicy(t, nil)
	p.listSerialPorts = func() []string { return nil }

	if err := p.CheckSerialPort("/dev/ttyUSB0"); !errors.Is(err, ErrSerialNotAllowed) {
		t.Fatalf("CheckSerialPort = %v, want %v", err, ErrSerialNotAllowed)
	}
}

func TestCheckRequestDispatchesOnMode(t *testing.T) {
	p := mustPolicy(t, nil)
	p.listSerialPorts = func() []string { return []string{"/dev/ttyUSB0"} }

	tests := []struct {
		name    string
		req     modbus.ScanRequest
		wantErr error
	}{
		{"tcp allowed", modbus.ScanRequest{Mode: "tcp", Host: "192.168.0.1"}, nil},
		{"tcp denied", modbus.ScanRequest{Mode: "tcp", Host: "1.1.1.1"}, ErrTargetNotAllowed},
		{"rtu allowed", modbus.ScanRequest{Mode: "rtu", SerialPort: "/dev/ttyUSB0"}, nil},
		{"rtu denied", modbus.ScanRequest{Mode: "rtu", SerialPort: "/dev/ttyUSB1"}, ErrSerialNotAllowed},
		{"rtu does not fall through to host check", modbus.ScanRequest{Mode: "rtu", SerialPort: "/etc/passwd", Host: "127.0.0.1"}, ErrSerialNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			if err := p.CheckRequest(&req); !errors.Is(err, tt.wantErr) {
				t.Fatalf("CheckRequest = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
