//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/binary"
)

// Actual routeSocket is read-only until a separately reviewed typed mutation
// sender exists. No denylist: SETLINK19, DELLINK17 and ALL other commands fail.
func validateReadOnlyRouteRequest(request []byte, localPID uint32) error {
	if len(request) < 16 || int(binary.NativeEndian.Uint32(request)) != len(request) || binary.NativeEndian.Uint16(request[6:]) != 0x301 || binary.NativeEndian.Uint32(request[8:]) == 0 || localPID == 0 || binary.NativeEndian.Uint32(request[12:]) != localPID {
		return ErrConflict
	}
	kind := binary.NativeEndian.Uint16(request[4:])
	switch kind {
	case 18, 22, 26, 34:
	default:
		return ErrConflict
	}
	header, e := DumpRequestHeader(kind)
	if e != nil || !bytes.Equal(request[16:], header) {
		return ErrConflict
	}
	return nil
}

// This pure gate is used immediately before actual Sendto and by fake sends.
func guardReadOnlyRouteSend(request []byte, localPID uint32, send func() error) error {
	if send == nil {
		return ErrConflict
	}
	if e := validateReadOnlyRouteRequest(request, localPID); e != nil {
		return e
	}
	return send()
}
