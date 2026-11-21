package enrollmentcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func wireSeeds(t *testing.T) [][]byte {
	t.Helper()
	c, _ := vector(t)
	out := [][]byte{}
	for _, w := range []any{ChallengeWire{4, c}, ResponseWire{4, c.ID, c.Nonce}, ConfirmationWire{4, c.ID, c.Nonce}} {
		raw, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}
func decodeWireIndex(i int, raw []byte) error {
	switch i {
	case 0:
		_, err := DecodeChallengeWire(raw)
		return err
	case 1:
		_, err := DecodeResponseWire(raw)
		return err
	default:
		_, err := DecodeConfirmationWire(raw)
		return err
	}
}
func TestProofWireRoundTripExactV4AndDisjointPurpose(t *testing.T) {
	seeds := wireSeeds(t)
	c, _ := vector(t)
	got, err := DecodeChallengeWire(seeds[0])
	if err != nil || got != c {
		t.Fatal("challenge roundtrip failed", err)
	}
	for i, raw := range seeds {
		if err := decodeWireIndex(i, raw); err != nil {
			t.Fatal(err)
		}
		for j := range seeds {
			if i != j && decodeWireIndex(j, raw) == nil {
				t.Fatal("cross-purpose envelope accepted")
			}
		}
	}
	for _, v := range []any{ChallengeWire{4, c}, ResponseWire{4, c.ID, c.Nonce}, ConfirmationWire{4, c.ID, c.Nonce}} {
		if strings.Contains(fmt.Sprintf("%v %#v", v, v), c.ID) {
			t.Fatal("formatter exposed challenge")
		}
	}
}
func TestProofWireEveryNestedFieldRequiredExactNonNull(t *testing.T) {
	for i, seed := range wireSeeds(t) {
		var root any
		json.Unmarshal(seed, &root)
		var walk func(any)
		walk = func(v any) {
			object, ok := v.(map[string]any)
			if !ok {
				return
			}
			for key, original := range object {
				object[key] = nil
				raw, _ := json.Marshal(root)
				if decodeWireIndex(i, raw) == nil {
					t.Fatal("null accepted", key)
				}
				delete(object, key)
				raw, _ = json.Marshal(root)
				if decodeWireIndex(i, raw) == nil {
					t.Fatal("missing accepted", key)
				}
				object[key] = original
				upper := strings.ToUpper(key)
				delete(object, key)
				object[upper] = original
				raw, _ = json.Marshal(root)
				if decodeWireIndex(i, raw) == nil {
					t.Fatal("case alias accepted", key)
				}
				delete(object, upper)
				object[key] = original
				walk(original)
			}
		}
		walk(root)
	}
}
func TestProofWireMalformedVersionsDuplicatesNumbersAndBounds(t *testing.T) {
	for i, seed := range wireSeeds(t) {
		for _, raw := range [][]byte{nil, []byte(`null`), []byte(`[]`), append(append([]byte(nil), seed...), []byte(`{}`)...), append([]byte(`{"version":4,`), seed[1:]...), bytes.Replace(seed, []byte(`"version":4`), []byte(`"version":4.0`), 1), bytes.Replace(seed, []byte(`"version":4`), []byte(`"version":4e0`), 1), bytes.Replace(seed, []byte(`"version":4`), []byte(`"version":3`), 1), append([]byte(`{"unknown":1,`), seed[1:]...), bytes.Repeat([]byte(" "), ProofWireMaxBytes+1), append(append([]byte(nil), seed...), 0xff)} {
			if decodeWireIndex(i, raw) == nil {
				t.Fatal("malformed accepted", i)
			}
		}
	}
	seeds := wireSeeds(t)
	for _, number := range []string{"1.5", "1e3", "9223372036854775808", "null", "\"123\""} {
		var object map[string]json.RawMessage
		json.Unmarshal(seeds[0], &object)
		var challenge map[string]json.RawMessage
		json.Unmarshal(object["challenge"], &challenge)
		challenge["expires_at_ms"] = json.RawMessage(number)
		object["challenge"], _ = json.Marshal(challenge)
		raw, _ := json.Marshal(object)
		if _, err := DecodeChallengeWire(raw); err == nil {
			t.Fatal("invalid integer accepted")
		}
	}
}
func TestProofWireCanonicalNonzeroMACAndIDs(t *testing.T) {
	for i, seed := range wireSeeds(t)[1:] {
		for _, field := range []string{"challenge_id", map[int]string{0: "response_mac", 1: "confirmation_mac"}[i]} {
			for _, bad := range []string{"", strings.Repeat("A", 43), strings.Repeat("B", 44), "not-a-token"} {
				var object map[string]any
				json.Unmarshal(seed, &object)
				object[field] = bad
				raw, _ := json.Marshal(object)
				if decodeWireIndex(i+1, raw) == nil {
					t.Fatal("noncanonical token accepted")
				}
			}
		}
	}
}
func FuzzProofWireDecoders(f *testing.F) {
	c := ChallengeWire{}
	raw, _ := json.Marshal(c)
	f.Add(raw)
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > ProofWireMaxBytes+1 {
			t.Skip()
		}
		DecodeChallengeWire(raw)
		DecodeResponseWire(raw)
		DecodeConfirmationWire(raw)
	})
}
