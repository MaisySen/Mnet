package control

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"fintex.au/mnet/internal/enrollmentcontract"
)

func TestRegistrarIndexKeysByteExactIndependentPython(t *testing.T) {
	raw, err := os.ReadFile("testdata/registrar-index-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Fields  []string `json:"fields"`
		Encoded string   `json:"encoded_utf8_hex"`
	}
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		b, err := json.Marshal(v.Fields)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(b) != v.Encoded {
			t.Fatal("existing immutable Go JSON index bytes changed")
		}
		if len(v.Fields) == 2 {
			if hex.EncodeToString([]byte(registrarClaimKey(v.Fields[0], v.Fields[1]))) != v.Encoded {
				t.Fatal("claim key differs from independent bytes")
			}
		}
		if len(v.Fields) == 4 {
			if hex.EncodeToString([]byte(registrarScopeKey(v.Fields[0], v.Fields[1], v.Fields[2], v.Fields[3]))) != v.Encoded {
				t.Fatal("actual scope helper differs from independent Python bytes")
			}
		}
		if len(v.Fields) == 3 {
			c := enrollmentcontract.Challenge{ID: v.Fields[2], Binding: enrollmentcontract.Binding{PrincipalAuthority: v.Fields[0], RegistrarOrigin: v.Fields[1]}}
			if hex.EncodeToString([]byte(registrarProofKey(c))) != v.Encoded {
				t.Fatal("proof index differs from independent bytes")
			}
		}
	}
}
