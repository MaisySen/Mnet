//go:build mnet_enrollment_development

package osownership

import (
	"context"
	"encoding/binary"
	"slices"
)

// Source owner is not an inertAFReader and implements no fake-only marker.
// Active/last changes occur only on controller ownership lane in resident scope.
type afSourcePassOwner struct {
	controller     *operation1Controller
	sequence, port uint32
	active         bool
	last           *afReadPass
	ownedSetup     *Handle
}
type routeAFRecordingWire struct {
	wire    NetlinkWire
	request []byte
	replies []Datagram
}

func (w *routeAFRecordingWire) Exchange(ctx context.Context, request []byte) ([]Datagram, error) {
	w.request = slices.Clone(request)
	ds, e := w.wire.Exchange(ctx, request)
	for _, d := range ds {
		d.Bytes = slices.Clone(d.Bytes)
		w.replies = append(w.replies, d)
	}
	return ds, e
}

// Parses actual wire output only after NetlinkClient validated complete multipart
// sender/seq/recipient/framing/completion. Source caller additionally supplies
// live route socket+namespace provenance, never an inert reader object.
func collectRouteAFPass(ctx context.Context, o *afSourcePassOwner, wire NetlinkWire) (*afReadPass, error) {
	if o != nil {
		o.last = nil
	}
	if ctx == nil || ctx.Err() != nil || o == nil || !o.active || o.controller == nil || o.port == 0 || o.sequence == ^uint32(0) || wire == nil {
		return nil, afRequest
	}
	o.last = nil
	recording := &routeAFRecordingWire{wire: wire}
	client := NewInMemoryNetlinkClient(recording, o.port)
	client.sequence = o.sequence
	bodies, e := client.Request(ctx, 18, make([]byte, 16), true)
	o.sequence = client.sequence
	if e != nil {
		return nil, e
	}
	if len(bodies) == 0 || len(bodies) > 4096 {
		return nil, afShape
	}
	p := &afReadPass{request: slices.Clone(recording.request), replies: recording.replies, sequence: o.sequence, sourceOwner: o}
	for _, body := range bodies {
		var link LinkRecord
		var e error
		if o.ownedSetup != nil {
			link, e = decodeSetupLink(body, *o.ownedSetup)
		} else {
			link, e = DecodeLink(body)
		}
		if e != nil || !safeInterfaceName(link.Name) {
			return nil, afShape
		}
		p.links = append(p.links, link)
	}
	for _, d := range p.replies {
		b := d.Bytes
		for len(b) > 0 {
			n := int(binary.NativeEndian.Uint32(b))
			p.headers = append(p.headers, afReplyHeader{Length: uint32(n), Type: binary.NativeEndian.Uint16(b[4:]), Flags: binary.NativeEndian.Uint16(b[6:]), Sequence: binary.NativeEndian.Uint32(b[8:]), Recipient: binary.NativeEndian.Uint32(b[12:]), Sender: d.SenderPID, Truncated: d.Truncated})
			b = b[(n+3)&^3:]
		}
	}
	o.last = p
	if _, e = p.requestClaim(); e != nil {
		o.last = nil
		return nil, e
	}
	return p, nil
}
