package ifacectl

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Serving owns only the exact NAT rules it adds. Forwarding/firewall policy
// belongs to the operator: never change shared sysctls or adopt/delete another
// application's rule. Caller serializes start/close with device lifecycle.
type Serving struct {
	iface, tag     string
	owned          [][]string
	command        func(...string) ([]byte, error)
	readForwarding func() ([]byte, error)
}

func NewServing(iface string) (*Serving, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("automatic subnet serving currently supports managed Linux interfaces only")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	return &Serving{iface: iface, tag: "mnet-" + hex.EncodeToString(nonce[:]), readForwarding: func() ([]byte, error) { return os.ReadFile("/proc/sys/net/ipv4/ip_forward") }, command: func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "iptables", args...).CombinedOutput()
	}}, nil
}

// Start supports bounded private IPv4 subnets, not exit/default routing.
// Require pre-enabled forwarding so Down need not guess who now owns a global
// setting. NAT is destination-scoped and cannot hairpin back into our TUN.
func (s *Serving) Start(routes []string) error {
	if s == nil {
		return fmt.Errorf("serving owner unavailable")
	}
	if len(s.owned) != 0 {
		return fmt.Errorf("serving already started")
	}
	if s.iface == "" || strings.ContainsAny(s.iface, " \t\r\n") {
		return fmt.Errorf("invalid serving interface")
	}
	if len(routes) == 0 || len(routes) > 256 {
		return fmt.Errorf("subnet serving requires 1 to 256 private IPv4 routes")
	}
	var rules [][]string
	seen := map[string]bool{}
	for _, raw := range routes {
		p, err := netip.ParsePrefix(raw)
		if err != nil || !p.Addr().Is4() {
			return fmt.Errorf("unsupported serving prefix %q", raw)
		}
		p = p.Masked()
		private := false
		for _, rawBlock := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
			block := netip.MustParsePrefix(rawBlock)
			if p.Bits() >= block.Bits() && block.Contains(p.Addr()) {
				private = true
				break
			}
		}
		if !private {
			return fmt.Errorf("serving requires RFC1918 IPv4 prefix: %s", p)
		}
		if seen[p.String()] {
			continue
		}
		seen[p.String()] = true
		rules = append(rules, []string{"-s", "100.64.0.0/10", "-d", p.String(), "!", "-o", s.iface, "-m", "comment", "--comment", s.tag, "-j", "MASQUERADE"})
	}
	forwarding, err := s.readForwarding()
	if err != nil {
		return fmt.Errorf("read IPv4 forwarding prerequisite: %w", err)
	}
	if strings.TrimSpace(string(forwarding)) != "1" {
		return fmt.Errorf("subnet serving requires operator-enabled IPv4 forwarding; no sysctl was changed")
	}
	for _, rule := range rules {
		out, err := s.command(append([]string{"-w", "2", "-t", "nat", "-A", "POSTROUTING"}, rule...)...)
		if err != nil {
			return errors.Join(fmt.Errorf("add owned subnet NAT rule: %w: %s", err, out), s.Close())
		}
		s.owned = append(s.owned, rule)
	}
	return nil
}

// Close deletes by the full unique rule specification, never by an index or
// broad source match. Failed cleanup stays tracked for another attempt.
func (s *Serving) Close() error {
	if s == nil {
		return nil
	}
	var pending [][]string
	var errs error
	for i := len(s.owned) - 1; i >= 0; i-- {
		rule := s.owned[i]
		out, err := s.command(append([]string{"-w", "2", "-t", "nat", "-D", "POSTROUTING"}, rule...)...)
		if err != nil {
			pending = append(pending, rule)
			errs = errors.Join(errs, fmt.Errorf("remove owned subnet NAT rule: %w: %s", err, out))
		}
	}
	s.owned = pending
	return errs
}
