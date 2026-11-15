package ifacectl

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"time"
)

// SubnetRoutes owns only the explicit IPv4 routes it successfully added.
// Caller serializes Sync/Close. It never replaces an existing route. The
// per-instance metric plus interface/protocol/prefix constrain deletion.
// IPv6 and exit/default routing deliberately remain out of scope.
type SubnetRoutes struct {
	iface   string
	metric  string
	owned   map[string]bool
	command func(...string) ([]byte, error)
}

type routeEntry struct {
	Dst      string `json:"dst"`
	Dev      string `json:"dev"`
	Protocol any    `json:"protocol"`
	Metric   uint32 `json:"metric"`
}

func NewSubnetRoutes(iface string) *SubnetRoutes {
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil
	}
	return &SubnetRoutes{iface: iface, metric: strconv.FormatUint(uint64(binary.BigEndian.Uint32(nonce[:])|0x40000000), 10), owned: map[string]bool{}, command: func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "ip", args...).CombinedOutput()
	}}
}

func (r *SubnetRoutes) change(action, prefix string) error {
	out, err := r.command("-4", "route", action, prefix, "dev", r.iface, "proto", "242", "metric", r.metric)
	if err != nil {
		return fmt.Errorf("subnet route %s %s: %w: %s", action, prefix, err, out)
	}
	return nil
}

// Sync removes withdrawn routes before adding new ones. Additions are
// preflighted together; an add failure rolls back this batch's additions.
// A withdrawn route that cannot be removed stays tracked for retry.
func (r *SubnetRoutes) Sync(prefixes []string) error {
	if r == nil {
		return fmt.Errorf("subnet route owner unavailable")
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("managed subnet OS routes currently support Linux only")
	}
	desired := map[string]netip.Prefix{}
	var validation error
	mesh := netip.MustParsePrefix("100.64.0.0/10")
	if len(prefixes) > 1024 {
		validation = fmt.Errorf("subnet input exceeds 1024 entries")
		prefixes = nil // fail closed: withdraw all old ownership, add nothing
	}
	for _, raw := range prefixes {
		p, err := netip.ParsePrefix(raw)
		if err != nil || !p.Addr().Is4() || p.Bits() == 0 {
			validation = errors.Join(validation, fmt.Errorf("unsupported subnet route %q", raw))
			continue
		}
		p = p.Masked()
		private := false
		for _, block := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
			allowed := netip.MustParsePrefix(block)
			if p.Bits() >= allowed.Bits() && allowed.Contains(p.Addr()) {
				private = true
				break
			}
		}
		if !private {
			validation = errors.Join(validation, fmt.Errorf("managed subnet route requires RFC1918 IPv4 prefix: %s", p))
			continue
		}
		if p.Overlaps(mesh) || p.Addr().IsLoopback() || p.Addr().IsMulticast() || p.Addr().IsLinkLocalUnicast() {
			validation = errors.Join(validation, fmt.Errorf("unsafe subnet route %q", raw))
			continue
		}
		if _, exists := desired[p.String()]; !exists && len(desired) >= 256 {
			validation = errors.Join(validation, fmt.Errorf("managed subnet route limit is 256"))
			break
		}
		desired[p.String()] = p
	}
	keys := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for p := range m {
			out = append(out, p)
		}
		sort.Strings(out)
		return out
	}
	for _, prefix := range keys(r.owned) {
		if _, ok := desired[prefix]; !ok {
			if err := r.change("del", prefix); err != nil {
				// Closing a kernel TUN automatically removes its routes. An absent
				// exact route is successful cleanup; never delete a replacement.
				present, e := r.ownedPresent(prefix)
				if e != nil || present {
					return err
				}
			}
			delete(r.owned, prefix)
		}
	}
	// Withdrawal is independent of whether new additions are acceptable.
	if validation != nil {
		return validation
	}
	raw, err := r.command("-N", "-4", "-j", "route", "show", "table", "main")
	if err != nil {
		return fmt.Errorf("inspect subnet routes: %w: %s", err, raw)
	}
	var existing []routeEntry
	if err = json.Unmarshal(raw, &existing); err != nil {
		return fmt.Errorf("parse subnet routes: %w", err)
	}
	for prefix, p := range desired {
		if r.owned[prefix] {
			continue
		}
		for _, entry := range existing {
			if entry.Dst == "default" || entry.Dst == "" {
				continue
			}
			old, e := netip.ParsePrefix(entry.Dst)
			if e != nil {
				if addr, e2 := netip.ParseAddr(entry.Dst); e2 == nil {
					old = netip.PrefixFrom(addr, 32)
				} else {
					return fmt.Errorf("unrecognized existing route %q", entry.Dst)
				}
			}
			if p.Overlaps(old) {
				return fmt.Errorf("subnet %s conflicts with existing route %s on %s", p, entry.Dst, entry.Dev)
			}
		}
	}
	ordered := make([]string, 0, len(desired))
	for p := range desired {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	var added []string
	for _, prefix := range ordered {
		if r.owned[prefix] {
			continue
		}
		if err := r.change("add", prefix); err != nil {
			for _, p := range added {
				if e := r.change("del", p); e == nil {
					delete(r.owned, p)
				}
			}
			return err
		}
		r.owned[prefix] = true
		added = append(added, prefix)
	}
	return nil
}

func (r *SubnetRoutes) ownedPresent(prefix string) (bool, error) {
	raw, err := r.command("-N", "-4", "-j", "route", "show", "table", "main", "exact", prefix)
	if err != nil {
		return false, err
	}
	var entries []routeEntry
	if err = json.Unmarshal(raw, &entries); err != nil {
		return false, err
	}
	metric, _ := strconv.ParseUint(r.metric, 10, 32)
	for _, entry := range entries {
		if entry.Dev == r.iface && entry.Metric == uint32(metric) && fmt.Sprint(entry.Protocol) == "242" {
			return true, nil
		}
	}
	return false, nil
}

func (r *SubnetRoutes) Close() error {
	if r == nil {
		return nil
	}
	return r.Sync(nil)
}
