package enrollmentcontract

// TEST-ONLY pinned key schedule and public credential-digest semantics, not an
// exposed authenticator. A confirmation MAC does not prove durable acceptance.
import (
	"bytes"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestServerConfirmationIndependentPython(t *testing.T) {
	raw, err := os.ReadFile("testdata/server-confirmation-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Info    string `json:"info"`
		Key     string `json:"key_hex"`
		Message string `json:"message_hex"`
		MAC     string `json:"mac_hex"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("testdata/response-mac-v4.json")
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Shared string `json:"shared_hex"`
		Nonce  string `json:"nonce_hex"`
		MAC    string `json:"response_mac_hex"`
		Key    string `json:"response_key_hex"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	decode := func(s string) []byte {
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if v.Info != Domain+"/server-confirmation-key" {
		t.Fatal("confirmation label changed")
	}
	key, err := hkdf.Key(sha256.New, decode(response.Shared), decode(response.Nonce), v.Info, 32)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, decode(v.Key)) || bytes.Equal(key, decode(response.Key)) {
		t.Fatal("confirmation key mismatch or response-key reuse")
	}
	_, transcript := vector(t)
	digest := sha256.Sum256(transcript)
	fields := [][]byte{[]byte(Domain + "/server-confirmation"), digest[:], decode(response.MAC)}
	var message []byte
	for _, f := range fields {
		message = binary.BigEndian.AppendUint32(message, uint32(len(f)))
		message = append(message, f...)
	}
	if !bytes.Equal(message, decode(v.Message)) {
		t.Fatal("confirmation message mismatch")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	if !hmac.Equal(mac.Sum(nil), decode(v.MAC)) {
		t.Fatal("confirmation MAC mismatch")
	}
	for _, mutate := range []func([]byte){func(b []byte) { b[5] ^= 1 }, func(b []byte) { b[len(b)-1] ^= 1 }} {
		changed := append([]byte(nil), message...)
		mutate(changed)
		mac := hmac.New(sha256.New, key)
		mac.Write(changed)
		if hmac.Equal(mac.Sum(nil), decode(v.MAC)) {
			t.Fatal("confirmation ignored transcript/response")
		}
	}
}
func TestPresavedCredentialDigestMatchesTextHashContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/native-credential-digest-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Secret string `json:"secret"`
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(v.Secret)
	if err != nil || len(decoded) != 32 {
		t.Fatal("fixture malformed")
	}
	hash := sha256.Sum256([]byte(v.Secret))
	actual := base64.RawURLEncoding.EncodeToString(hash[:])
	if actual != v.Digest {
		t.Fatal("credential text-digest contract changed")
	}
	wrong := sha256.Sum256(decoded)
	if bytes.Equal(wrong[:], hash[:]) {
		t.Fatal("raw-byte and text digest ambiguously equated")
	}
	// A digest-keyed MAC would let everyone with PUBLIC launch binding forge it.
	// No test or production helper treats the digest as an HMAC secret.
}
