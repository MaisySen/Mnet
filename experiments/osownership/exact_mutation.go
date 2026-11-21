//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"errors"
)

// ExactMutation has no exported payload/kind fields; callers cannot turn an
// encoded route delete into DELLINK. Namespace/handle checks are still the
// adapter's responsibility immediately before sending. TUN never uses DELLINK.
type ExactMutation struct {
	kind     uint16
	payload  []byte
	identity Handle
}

func BuildExactMutation(h Handle, op Operation) (ExactMutation, error) {
	var m ExactMutation
	var e error
	if h.Link.Index <= 0 || !h.Namespace.Isolated || h.Link.Kind != "tun" {
		return m, ErrConflict
	}
	m.identity = h
	switch op.Kind {
	case "set-alias-mtu-exact":
		if op.Name != h.Link.Name || op.Alias != h.Link.Alias || op.MTU != MTU {
			return m, ErrConflict
		}
		m.kind = 16
		m.payload, e = EncodeLink(h.Link.Index, h.Link.Name, h.Link.Alias, h.Link.MTU, false)
	case "link-up-exact":
		if !h.TagConfirmed {
			return m, ErrConflict
		}
		m.kind = 16
		m.payload, e = EncodeLink(h.Link.Index, h.Link.Name, h.Link.Alias, h.Link.MTU, true)
	case "address-add-exclusive", "address-del-exact":
		if op.Address != h.Address.Prefix {
			return m, ErrConflict
		}
		m.kind = 20
		if op.Kind == "address-del-exact" {
			m.kind = 21
		}
		m.payload, e = EncodeAddress(h.Link.Index, h.Address.Prefix.Addr())
	case "route-add-exclusive", "route-del-exact":
		r := op.Route
		if op.Kind == "route-add-exclusive" && r.LinkIndex == 0 {
			r.LinkIndex = h.Link.Index
		}
		if r != h.Route {
			return m, ErrConflict
		}
		m.kind = 24
		if op.Kind == "route-del-exact" {
			m.kind = 25
		}
		m.payload, e = EncodeRoute(h.Route)
	default:
		return m, errors.New("no exact rtnetlink encoder for operation")
	}
	return m, e
}
func (c *NetlinkClient) ApplyExact(ctx context.Context, m ExactMutation) error {
	if m.kind == 0 || m.identity.Link.Index <= 0 || len(m.payload) == 0 {
		return ErrConflict
	}
	return c.mutate(ctx, m.kind, m.payload)
}
