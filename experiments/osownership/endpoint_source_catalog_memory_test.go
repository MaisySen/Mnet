//go:build linux && mnet_enrollment_development

package osownership

import (
	"net/netip"
	"strings"
	"testing"
)

// DATA only: no source pass, installed packet gate, controller, native object,
// callback, context/timer, syscall, proc/file access or new process is created.
func TestEndpointReadinessDataMemory(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    endpointPollResult
		read bool
	}{
		{"timeout", endpointPollResult{}, false},
		{"pipe-input-only", endpointPollResult{Input: 1}, false},
		{"pipe-output-only", endpointPollResult{Output: 4}, false},
		{"tun-output-only", endpointPollResult{TUN: 4}, false},
		{"tun-priority-only", endpointPollResult{TUN: 2}, false},
		{"tun-readable", endpointPollResult{TUN: 1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var hint endpointReadHint
			hint.replace(tc.p)
			if hint.take() != tc.read || hint.take() {
				t.Fatal("hint was wrong or reusable")
			}
			hint.replace(endpointPollResult{TUN: 1})
			hint.replace(endpointPollResult{})
			if hint.take() {
				t.Fatal("timeout retained stale hint")
			}
		})
	}
	// A readiness hint is deliberately just DATA; no authority-bearing source
	// object is available here to perform any original I/O from this hint.
}
func TestEndpointIPv6AbsenceDataMemory(t *testing.T) {
	v4 := netip.MustParseAddr("100.64.0.1")
	v6 := netip.MustParseAddr("2001:db8::1")
	for _, tc := range []struct {
		name   string
		raw    RawInventory
		absent bool
	}{
		{"empty", RawInventory{}, true},
		{"v4", RawInventory{Addresses: []AddressRecord{{Family: 2, Local: v4, Address: v4}}, Routes: []RouteRecord{{Family: 2, Dst: v4}}}, true},
		{"v6-address-family-even-empty", RawInventory{Addresses: []AddressRecord{{Family: 10}}}, false},
		{"v6-route-family-even-empty", RawInventory{Routes: []RouteRecord{{Family: 10}}}, false},
		{"v6-local-wrong-family", RawInventory{Addresses: []AddressRecord{{Family: 2, Local: v6}}}, false},
		{"v6-address-wrong-family", RawInventory{Addresses: []AddressRecord{{Family: 2, Address: v6}}}, false},
		{"v6-route-dst", RawInventory{Routes: []RouteRecord{{Family: 2, Dst: v6}}}, false},
		{"v6-route-source", RawInventory{Routes: []RouteRecord{{Family: 2, Source: v6}}}, false},
		{"v6-route-gateway", RawInventory{Routes: []RouteRecord{{Family: 2, Gateway: v6}}}, false},
		{"v6-route-prefsrc", RawInventory{Routes: []RouteRecord{{Family: 2, PreferredSource: v6}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if endpointIPv6AddressRouteAbsent(tc.raw) != tc.absent {
				t.Fatal("absence predicate")
			}
		})
	}
}
func TestEndpointPolicyCatalogDataMemory(t *testing.T) {
	if len(endpointIPv4ConsumerCatalog) != 33 || len(endpointIPv6ConsumerCatalog) != 60 {
		t.Fatal("catalog count")
	}
	for _, family := range []int{4, 6} {
		specs := endpointIPv4ConsumerCatalog[:]
		names := ownedTUNIPv4Words[:]
		if family == 6 {
			specs = endpointIPv6ConsumerCatalog[:]
			names = ownedTUNIPv6Words[:]
		}
		for i, spec := range specs {
			if spec.Index != i || spec.Name != names[i] || spec.Group == "" || spec.References == "" || endpointPolicyMissingEvidence(spec) == "" {
				t.Fatal("source catalog mislabeled or semantic gap waived", family, i)
			}
		}
	}
	for _, i := range []int{29, 56} {
		spec := endpointIPv6ConsumerCatalog[i]
		if !strings.Contains(endpointPolicyMissingEvidence(spec), "ALL") || !strings.Contains(spec.References, "ndisc.c") {
			t.Fatal("ND event dependency missing", i)
		}
	}
	if endpointIPv6ConsumerCatalog[29].Group == endpointIPv6ConsumerCatalog[56].Group || endpointIPv4ConsumerCatalog[10].Group != "diagnostic-unverified" {
		t.Fatal("distinct semantic categories erased")
	}
}
