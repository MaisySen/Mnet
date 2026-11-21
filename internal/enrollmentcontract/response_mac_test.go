package enrollmentcontract

// TEST-ONLY response key schedule contract. Not a production proof verifier.
import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestResponseMACDedicatedLabelIndependentPython(t *testing.T) {
	raw, err := os.ReadFile("testdata/response-mac-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Shared string `json:"shared_hex"`
		Nonce  string `json:"nonce_hex"`
		Info   string `json:"info"`
		PRK    string `json:"prk_hex"`
		Key    string `json:"response_key_hex"`
		MAC    string `json:"response_mac_hex"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	decode := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	shared, nonce := decode(v.Shared), decode(v.Nonce)
	if v.Info != Domain+"/response-mac-key" {
		t.Fatal("response label changed")
	}
	prk, err := hkdf.Extract(sha256.New, shared, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(prk, decode(v.PRK)) {
		t.Fatal("HKDF extract differs from Python")
	}
	key, err := hkdf.Key(sha256.New, shared, nonce, v.Info, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, decode(v.Key)) {
		t.Fatal("response key differs from Python")
	}
	_, transcript := vector(t)
	mac := hmac.New(sha256.New, key)
	mac.Write(transcript)
	expected := decode(v.MAC)
	if !hmac.Equal(mac.Sum(nil), expected) {
		t.Fatal("response MAC differs from independent Python")
	}
	for _, label := range []string{Domain + "/derived-mac-key", Domain + "/server-confirmation-key", "mnet/enrollment-key-possession/experimental-v2/response-mac-key"} {
		wrong, err := hkdf.Key(sha256.New, shared, nonce, label, 32)
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, wrong)
		mac.Write(transcript)
		if hmac.Equal(mac.Sum(nil), expected) {
			t.Fatal("different KDF purpose collided")
		}
	}
	changed := append([]byte(nil), nonce...)
	changed[0] ^= 1
	wrong, err := hkdf.Key(sha256.New, shared, changed, v.Info, 32)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(key, wrong) {
		t.Fatal("nonce unbound to key")
	}
	mac = hmac.New(sha256.New, key)
	mac.Write(append(append([]byte(nil), transcript...), 0))
	if hmac.Equal(mac.Sum(nil), expected) {
		t.Fatal("transcript suffix unbound")
	}
}
