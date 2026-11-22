//go:build mnet_enrollment_development

package control

import (
	"bytes"
	"encoding/json"
	"fintex.au/mnet/internal/netmap"
	"testing"
)

func TestDevelopmentEndpointCapabilityExactWireShape(t *testing.T) {
	base := map[string]any{"version": 1, "receipt": map[string]any{}, "receipt_digest": "fixture", "endpoints": []string{"192.0.2.1:1"}, "endpoint_capability": netmap.EndpointCapability{Version: 1, Mode: netmap.EndpointModeV1, MTU: 1280}}
	raw, _ := json.Marshal(base)
	var out map[string]any
	if !developmentNodeRequestExact(raw, &out) {
		t.Fatal("new bounded optional declaration shape refused")
	}
	for _, body := range [][]byte{
		bytes.Replace(raw, []byte(`"mtu":1280`), []byte(`"mtu":1280,"mtu":1280`), 1),
		bytes.Replace(raw, []byte(`"mtu":1280`), []byte(`"mtu":1280,"extra":0`), 1),
		bytes.Replace(raw, []byte(`"mtu":1280`), []byte(`"mtu":null`), 1),
	} {
		if bytes.Equal(body, raw) {
			t.Fatal("mutation missed")
		}
		if developmentNodeRequestExact(body, &out) {
			t.Fatal("ambiguous declaration syntax accepted")
		}
	}
	base["endpoint_capability"] = nil
	raw, _ = json.Marshal(base)
	if developmentNodeRequestExact(raw, &out) {
		t.Fatal("present-null declaration accepted")
	}
	delete(base, "endpoint_capability")
	raw, _ = json.Marshal(base)
	if !developmentNodeRequestExact(raw, &out) {
		t.Fatal("existing four-field request changed")
	}
}
