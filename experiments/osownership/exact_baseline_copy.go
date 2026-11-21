//go:build mnet_enrollment_development

package osownership

import "slices"

// Structural copy preserves nil vs empty, scalar netip values (including zones),
// attribute bytes and order. No serialization or normalization in this layer.
func cloneRawAttrs(in map[uint16][]byte) map[uint16][]byte {
	if in == nil {
		return nil
	}
	out := make(map[uint16][]byte, len(in))
	for k, v := range in {
		out[k] = slices.Clone(v)
	}
	return out
}
func cloneRawExact(in RawInventory) RawInventory {
	out := in
	out.Links = slices.Clone(in.Links)
	for n := range out.Links {
		out.Links[n].Raw = cloneRawAttrs(in.Links[n].Raw)
	}
	out.Addresses = slices.Clone(in.Addresses)
	for n := range out.Addresses {
		out.Addresses[n].Raw = cloneRawAttrs(in.Addresses[n].Raw)
	}
	out.Routes = slices.Clone(in.Routes)
	for n := range out.Routes {
		out.Routes[n].Raw = cloneRawAttrs(in.Routes[n].Raw)
	}
	out.Rules = slices.Clone(in.Rules)
	for n := range out.Rules {
		out.Rules[n] = slices.Clone(in.Rules[n])
	}
	return out
}
