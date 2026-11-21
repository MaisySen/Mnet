package control

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// Independent frozen pre-extraction recipe, no new shared helper. Proves byte
// compatibility with actual stored v1/v2, including raw unnormalized strings.
func oldReceiptRecipe(r registrarRecord) []byte {
	domain := "mnet/store-enrollment-receipt/v1"
	if r.ReceiptEncodingVersion == 2 {
		domain = "mnet/store-enrollment-receipt/v2"
	}
	fields := [][]byte{}
	for _, s := range []string{domain, r.Scope.Issuer, r.Scope.RegistrarOrigin, r.Scope.Audience, r.Scope.ServiceID, r.Scope.WorkspaceID, r.Scope.MappingRevision, r.Scope.HandoffID, r.Scope.AttemptID, r.Scope.ReserveID, r.DescriptorDigest, "1", r.ReceiptID, r.NodeID, r.NetworkID, r.DevicePublicKey, r.CredentialDigest} {
		fields = append(fields, []byte(s))
	}
	for _, n := range []int64{r.CommittedAtMS, r.AccessExpiresAtMS} {
		fields = append(fields, binary.BigEndian.AppendUint64(nil, uint64(n)))
	}
	if r.ReceiptEncodingVersion == 2 && r.Consumption != nil {
		fields = append(fields, []byte(r.Consumption.Challenge.ID), []byte(r.Consumption.TranscriptDigest))
	}
	var out []byte
	for _, f := range fields {
		out = binary.BigEndian.AppendUint32(out, uint32(len(f)))
		out = append(out, f...)
	}
	return out
}
func TestRegistrarReceiptSharedCodecPreservesFrozenHistoricalBytes(t *testing.T) {
	s := registrarConsumptionFixture(t)
	original := *registrarFixtureRecord(s)
	for _, version := range []int{1, 2} {
		for _, suffix := range []string{"", "<>&-é-\u2028-\u2029", "\x00\nraw", " ", "世界"} {
			r := original
			r.ReceiptEncodingVersion = version
			r.Scope.Audience += suffix
			r.NodeID += suffix
			r.CommittedAtMS = -123
			r.DescriptorEncodingVersion = 27
			if !bytes.Equal(oldReceiptRecipe(r), registrarReceiptBytes(r)) {
				t.Fatal("shared DATA codec changed historical recipe", version)
			}
		}
	}
	r := original
	r.Consumption = nil
	if !bytes.Equal(oldReceiptRecipe(r), registrarReceiptBytes(r)) {
		t.Fatal("invalid missing-consumption compatibility changed")
	}
}
