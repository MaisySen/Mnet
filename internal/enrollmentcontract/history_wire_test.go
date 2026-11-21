package enrollmentcontract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoryWireStrictSeparateHistoricalPurpose(t *testing.T) {
	doc := nativeDocumentFixture(t)
	envelopes := []any{HistoryRequestWire{Version: 1, Challenge: doc.Challenge}, HistoryReceiptWire{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest}}
	decode := func(i int, raw []byte) error {
		if i == 0 {
			_, err := DecodeHistoryRequestWire(raw)
			return err
		}
		_, err := DecodeHistoryReceiptWire(raw, doc.Challenge)
		return err
	}
	for i, envelope := range envelopes {
		raw, _ := json.Marshal(envelope)
		if err := decode(i, raw); err != nil {
			t.Fatal(err)
		}
		if decode(1-i, raw) == nil {
			t.Fatal("cross-purpose history wire")
		}
		for _, bad := range [][]byte{nil, []byte("null"), append(raw, raw...), bytes.Repeat([]byte(" "), ProofWireMaxBytes+1), append([]byte{0xff}, raw...), []byte(strings.Replace(string(raw), `"version":1`, `"version":1.0`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"Version":1`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"version":null`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1)), append([]byte(`{"current":true,`), raw[1:]...)} {
			if decode(i, bad) == nil {
				t.Fatal("malformed history accepted", i)
			}
		}
		var object map[string]json.RawMessage
		json.Unmarshal(raw, &object)
		for field := range object {
			var changed map[string]json.RawMessage
			json.Unmarshal(raw, &changed)
			delete(changed, field)
			bad, _ := json.Marshal(changed)
			if decode(i, bad) == nil {
				t.Fatal("missing history field", field)
			}
		}
	}
	receipt := HistoryReceiptWire{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest}
	receipt.Receipt.NodeID += "wrong"
	bad, _ := json.Marshal(receipt)
	if _, err := DecodeHistoryReceiptWire(bad, doc.Challenge); err == nil {
		t.Fatal("receipt projection accepted")
	}
	for _, old := range []any{WorkflowReceiptWire{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest, ConfirmationMAC: doc.Challenge.Nonce}, ConfirmationWire{Version: 4, ChallengeID: doc.Challenge.ID, ConfirmationMAC: doc.Challenge.Nonce}} {
		raw, _ := json.Marshal(old)
		if _, err := DecodeHistoryReceiptWire(raw, doc.Challenge); err == nil {
			t.Fatal("crypto/workflow purpose accepted as history")
		}
	}
}
