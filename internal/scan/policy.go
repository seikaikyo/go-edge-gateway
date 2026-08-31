package scan

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/seikaikyo/go-common/modbus"
)

// Errors returned by target validation. Handlers map these to status codes and
// to a generic client-facing message; the wrapped detail stays in the log.
var (
	ErrTargetNotAllowed = errors.New("scan target is not in the allow list")
	ErrTargetInvalid    = errors.New("scan target is not a valid host")
	ErrSerialNotAllowed = errors.New("serial port is not an available device")
)

// DefaultAllowedTargets is the fallback CIDR allow list: loopback plus the
// RFC1918 private ranges. An edge gateway talks to plant-floor equipment, so
// anything routable on the public internet is out of scope by default.
var DefaultAllowedTargets = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// deniedTargets are refused whatever the configuration says. Link-local covers
// the cloud instance metadata endpoint (169.254.169.254).
var deniedTargets = []netip.Prefix{
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("fe80::/10"),
}

// Policy decides which scan targets a request may reach.
type Policy struct {
	allowed []netip.Prefix

	// lookupIP resolves a hostname. Overridable in tests.
	lookupIP func(host string) ([]net.IP, error)

	// listSerialPorts enumerates usable serial devices. Overridable in tests.
	listSerialPorts func() []string
}

// NewPolicy builds a policy from a CIDR allow list. An empty list falls back to
// DefaultAllowedTargets. Unparsable entries are reported rather than ignored,
// so a typo cannot silently widen or narrow the allow list.
func NewPolicy(cidrs []string) (*Policy, error) {
	if len(cidrs) == 0 {
		cidrs = DefaultAllowedTargets
	}

	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		p, err := netip.ParsePrefix(c)
		if err != nil {
			return nil, fmt.Errorf("scan allowed_targets %q: %w", c, err)
		}
		prefixes = append(prefixes, p.Masked())
	}
	if len(prefixes) == 0 {
		return nil, errors.New("scan allowed_targets resolved to an empty list")
	}

	return &Policy{
		allowed:         prefixes,
		lookupIP:        net.LookupIP,
		listSerialPorts: modbus.ListSerialPorts,
	}, nil
}

// CheckHost validates a TCP scan target. Literal addresses are checked
// directly; hostnames are resolved and every returned address must pass, so a
// name pointing at one allowed and one denied address is refused.
func (p *Policy) CheckHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return ErrTargetInvalid
	}
	host = strings.Trim(host, "[]")

	if addr, err := netip.ParseAddr(host); err == nil {
		return p.checkAddr(addr)
	}

	ips, err := p.lookupIP(host)
	if err != nil || len(ips) == 0 {
		return ErrTargetInvalid
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return ErrTargetInvalid
		}
		if err := p.checkAddr(addr.Unmap()); err != nil {
			return err
		}
	}
	return nil
}

func (p *Policy) checkAddr(addr netip.Addr) error {
	if !addr.IsValid() {
		return ErrTargetInvalid
	}
	addr = addr.Unmap()

	for _, denied := range deniedTargets {
		if denied.Contains(addr) {
			return ErrTargetNotAllowed
		}
	}
	for _, allowed := range p.allowed {
		if allowed.Contains(addr) {
			return nil
		}
	}
	return ErrTargetNotAllowed
}

// CheckSerialPort validates an RTU serial device path. Only paths currently
// enumerated by the host are accepted, so an arbitrary string never reaches
// the O_RDWR open inside the RTU handler.
func (p *Policy) CheckSerialPort(port string) error {
	port = strings.TrimSpace(port)
	if port == "" {
		return ErrSerialNotAllowed
	}
	for _, available := range p.listSerialPorts() {
		if port == available {
			return nil
		}
	}
	return ErrSerialNotAllowed
}

// CheckRequest validates a scan request against the policy, dispatching on mode.
func (p *Policy) CheckRequest(req *modbus.ScanRequest) error {
	if req.Mode == "rtu" {
		return p.CheckSerialPort(req.SerialPort)
	}
	return p.CheckHost(req.Host)
}
