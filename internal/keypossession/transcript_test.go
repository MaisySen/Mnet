package keypossession

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestBinaryTranscriptIndependentVector(t *testing.T) {
	c := Challenge{Binding: Binding{RegistrarOrigin: "https://registrar.test", Audience: "enroll<&>", Issuer: "https://issuer.test/path", Subject: "person<&>雪", PersonSessionID: "session", WorkspaceID: "workspace", NetworkID: "network", MappingRevision: "revision-2", AuthorizationID: "authorization"}, ExpiresUnixMilli: 1795228800000}
	for i := range 32 {
		c.ID[i] = byte(i)
		c.Nonce[i] = byte(i + 32)
		c.EphemeralPublicKey[i] = byte(i + 64)
		c.Binding.DevicePublicKey[i] = byte(i + 96)
	}
	raw, e := os.ReadFile("testdata/transcript-v2.hex")
	if e != nil {
		t.Fatal(e)
	}
	want, e := hex.DecodeString(strings.TrimSpace(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(transcript(c), want) {
		t.Fatal("binary transcript differs from independent Python vector")
	}
	changed := c
	changed.Binding.Subject = "person"
	changed.Binding.PersonSessionID = "<&>雪session"
	if bytes.Equal(transcript(c), transcript(changed)) {
		t.Fatal("field boundaries ambiguous")
	}
}
func TestExactIssuerPathAndRevisionAreBound(t *testing.T) {
	secret, b, now := fixture(t)
	b.Issuer += "/issuer"
	p, c, e := New(b, now)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Cancel()
	wrong := b
	wrong.Issuer += "/other"
	if _, e = Respond(secret, c, wrong, now); e == nil {
		t.Fatal("issuer mismatch accepted")
	}
	wrong = b
	wrong.MappingRevision += "-old"
	if _, e = Respond(secret, c, wrong, now); e == nil {
		t.Fatal("mapping revision mismatch accepted")
	}
	response, e := Respond(secret, c, b, now)
	if e != nil || p.Verify(c, response, now) != nil {
		t.Fatal("exact issuer/revision refused")
	}
}
