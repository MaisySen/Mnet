//go:build mnet_enrollment_development || multies_native_fixture

package control

import (
	"bytes"
	"fmt"
)

const developmentStoreHeader = "MNET-DISPOSABLE-REGISTRAR-ONLY-STORE-V1\n"
const developmentStoreMaxBytes = 16 * 1024 * 1024

// Incompatible FORMAT fence only, not authentication or hostile same-UID safety.
// Only the tagged dedicated constructor will select this; normal Open cannot.
type developmentRegistrarCodec struct{}

func (developmentRegistrarCodec) maxBytes() int { return developmentStoreMaxBytes }
func (developmentRegistrarCodec) decode(raw []byte) ([]byte, error) {
	if len(raw) > developmentStoreMaxBytes || !bytes.HasPrefix(raw, []byte(developmentStoreHeader)) {
		return nil, fmt.Errorf("dedicated registrar store format required; no legacy conversion")
	}
	payload := raw[len(developmentStoreHeader):]
	if len(payload) == 0 || payload[0] != '{' {
		return nil, fmt.Errorf("dedicated registrar payload missing")
	}
	return payload, nil
}
func (developmentRegistrarCodec) encode(raw []byte) ([]byte, error) {
	if len(raw) == 0 || raw[0] != '{' || len(raw)+len(developmentStoreHeader) > developmentStoreMaxBytes {
		return nil, fmt.Errorf("dedicated registrar payload invalid/too large")
	}
	return append(append([]byte(nil), []byte(developmentStoreHeader)...), raw...), nil
}
