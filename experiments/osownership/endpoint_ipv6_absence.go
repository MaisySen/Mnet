//go:build mnet_enrollment_development

package osownership

// Complete inventory DATA predicate, not an owner or current-state reader.
// Reject explicit AF_INET6 records even when their addresses are missing, and
// reject IPv6 values hidden behind another family. Other shape validation is
// still required by the actual original collector.
func endpointIPv6AddressRouteAbsent(raw RawInventory) bool {
	for _, a := range raw.Addresses {
		if a.Family == 10 || a.Local.Is6() || a.Address.Is6() {
			return false
		}
	}
	for _, r := range raw.Routes {
		if r.Family == 10 || r.Dst.Is6() || r.Source.Is6() || r.Gateway.Is6() || r.PreferredSource.Is6() {
			return false
		}
	}
	return true
}
