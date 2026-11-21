package enrollmentcontract

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowWireStrictIndependentPurposes(t *testing.T) {
	raw, err := os.ReadFile("testdata/multies-provenance-source-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Exchange SourceExchange `json:"exchange"`
		Reserve  SourceReserve  `json:"reserve"`
	}
	if json.Unmarshal(raw, &source) != nil {
		t.Fatal("source fixture")
	}
	request := SourceReserveRequest{Ticket: source.Exchange.Ticket, HandoffID: source.Exchange.ID, ReserveID: source.Reserve.ReserveID, Launch: source.Exchange.Launch, Mapping: source.Exchange.Mapping}
	doc := nativeDocumentFixture(t)
	envelopes := []any{WorkflowBeginWire{Version: 1, Request: request}, WorkflowSubmitWire{Version: 1, AttemptID: doc.Challenge.Binding.AttemptID, ChallengeID: doc.Challenge.ID, ResponseMAC: doc.Challenge.Nonce}, WorkflowReceiptWire{Version: 1, Receipt: doc.Receipt, ReceiptDigest: doc.ReceiptDigest, ConfirmationMAC: doc.Challenge.Nonce}}
	decode := func(i int, raw []byte) error {
		switch i {
		case 0:
			_, err := DecodeWorkflowBeginWire(raw, source.Reserve.ServiceID)
			return err
		case 1:
			_, err := DecodeWorkflowSubmitWire(raw)
			return err
		default:
			_, err := DecodeWorkflowReceiptWire(raw, doc.Challenge)
			return err
		}
	}
	for i, envelope := range envelopes {
		raw, _ := json.Marshal(envelope)
		if err := decode(i, raw); err != nil {
			t.Fatal("new wire roundtrip", i, err)
		}
		for j := range envelopes {
			if j != i && decode(j, raw) == nil {
				t.Fatal("cross-purpose accepted")
			}
		}
		for _, bad := range [][]byte{nil, []byte("null"), append(append([]byte{}, raw...), raw...), []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"version":1.0`, 1)), []byte(strings.Replace(string(raw), `"version":1`, `"Version":1`, 1)), append([]byte{0xff}, raw...), bytes.Repeat([]byte(" "), ProofWireMaxBytes+1)} {
			if decode(i, bad) == nil {
				t.Fatal("malformed envelope accepted", i)
			}
		}
		for _, bad := range [][]byte{[]byte(strings.Replace(string(raw), `"version":1`, `"version":null`, 1)), append([]byte(`{"unknown":true,`), raw[1:]...)} {
			if decode(i, bad) == nil {
				t.Fatal("unknown/null accepted", i)
			}
		}
		var root map[string]json.RawMessage
		json.Unmarshal(raw, &root)
		for name := range root {
			var object map[string]json.RawMessage
			json.Unmarshal(raw, &object)
			delete(object, name)
			bad, _ := json.Marshal(object)
			if decode(i, bad) == nil {
				t.Fatal("missing required field", i, name)
			}
		}
	}
	receipt := envelopes[2].(WorkflowReceiptWire)
	typ := reflect.TypeOf(receipt.Receipt)
	for i := 0; i < typ.NumField(); i++ {
		changed := receipt
		field := reflect.ValueOf(&changed.Receipt).Elem().Field(i)
		if field.Kind() == reflect.String {
			field.SetString(field.String() + "x")
		} else {
			field.SetInt(field.Int() + 1)
		}
		raw, _ := json.Marshal(changed)
		if _, err := DecodeWorkflowReceiptWire(raw, doc.Challenge); err == nil {
			t.Fatal("changed receipt accepted", typ.Field(i).Name)
		}
	}
}
