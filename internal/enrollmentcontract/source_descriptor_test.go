package enrollmentcontract

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

func TestSourceDescriptorMatchesActualMultiesEncoder(t *testing.T) {
	raw, err := os.ReadFile("testdata/multies-descriptor-source-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FixtureOnly bool `json:"fixtureOnly"`
		Vectors     []struct {
			Name        string         `json:"name"`
			Exchange    SourceExchange `json:"exchange"`
			ServiceID   string         `json:"serviceId"`
			ReserveID   string         `json:"reserveId"`
			BytesBase64 string         `json:"bytesBase64"`
			Digest      string         `json:"digest"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || !fixture.FixtureOnly || len(fixture.Vectors) != 2 {
		t.Fatal("invalid source fixture", err)
	}
	for _, v := range fixture.Vectors {
		t.Run(v.Name, func(t *testing.T) {
			raw, err := SourceDescriptorBytes(v.Exchange, v.ServiceID, v.ReserveID)
			if err != nil {
				t.Fatal(err)
			}
			if base64.StdEncoding.EncodeToString(raw) != v.BytesBase64 {
				t.Fatal("Multies JS and native descriptor bytes differ")
			}
			digest, err := SourceDescriptorDigest(v.Exchange, v.ServiceID, v.ReserveID)
			if err != nil || digest != v.Digest {
				t.Fatal("descriptor digest differs", err)
			}
			// Ticket is a transport secret, not part of immutable descriptor identity.
			changed := v.Exchange
			changed.Ticket = "not-used"
			again, err := SourceDescriptorDigest(changed, v.ServiceID, v.ReserveID)
			if err != nil || again != digest {
				t.Fatal("ticket changed immutable descriptor")
			}
			changed = v.Exchange
			changed.Mapping.Audience += "changed"
			again, err = SourceDescriptorDigest(changed, v.ServiceID, v.ReserveID)
			if err != nil || again == digest {
				t.Fatal("binding mutation not reflected")
			}
		})
	}
}
func TestJSONStringMatchesJSNotHTMLOrGoEscaping(t *testing.T) {
	// Includes a literal backslash-u sequence, which must not be confused with an
	// actual U+2028 by a naive replacement of Go encoding/json output.
	input := "<&>\"\\literal\\u2028\u2028\u2029\b\f\n\r\t\x01台灣"
	expected := "\"<&>\\\"\\\\literal\\\\u2028\u2028\u2029\\b\\f\\n\\r\\t\\u0001台灣\""
	if got := string(appendJSONString(nil, input)); got != expected {
		t.Fatalf("JSON bytes differ: %q", got)
	}
}
