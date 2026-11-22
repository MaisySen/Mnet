//go:build mnet_enrollment_development

package osownership

import "encoding/binary"

// Two-phase bootstrap: ADMISSION pins the actual inactive WG pair readback.
// FLOW_START follows only after that exact retained device has started. Both
// bodies are channel DATA, not standalone native or original-I/O authority.
const endpointFlowStart byte = 8
const endpointFlowBytes = 40

type endpointFlowBody struct {
	BridgeGeneration uint64
	PairDigest       [32]byte
}

func (b endpointFlowBody) encode() ([]byte, error) {
	if b.BridgeGeneration < 2 || b.PairDigest == ([32]byte{}) {
		return nil, errEndpointFrame
	}
	p := make([]byte, endpointFlowBytes)
	binary.BigEndian.PutUint64(p, b.BridgeGeneration)
	copy(p[8:], b.PairDigest[:])
	return p, nil
}
func decodeEndpointFlow(p []byte) (b endpointFlowBody, e error) {
	if len(p) != endpointFlowBytes {
		return b, errEndpointFrame
	}
	b.BridgeGeneration = binary.BigEndian.Uint64(p)
	copy(b.PairDigest[:], p[8:])
	_, e = b.encode()
	return b, e
}
func (b endpointFlowBody) matches(a endpointAdmissionBody) bool {
	return b.BridgeGeneration == a.BridgeGeneration && b.PairDigest == a.BridgePairDigest
}
