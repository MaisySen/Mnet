package control

// Capacity evidence for actual owning Store retained-history validation and
// publication. Counts are benchmark fixtures, NOT production admission policy.
import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkRegistrarRetainedHistoryPublication(b *testing.B) {
	for _, count := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("records-%d", count), func(b *testing.B) {
			s, err := Open(filepath.Join(b.TempDir(), "state.json"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { s.Close() })
			raw, err := os.ReadFile("testdata/registrar-receipt-v2.json")
			if err != nil {
				b.Fatal(err)
			}
			var vector struct {
				Record registrarRecord `json:"record"`
			}
			if err := json.Unmarshal(raw, &vector); err != nil {
				b.Fatal(err)
			}
			s.data.Registrar = &registrarState{Version: 2, Records: map[string]*registrarRecord{}, ConsumedProofs: map[string]string{}}
			token := func(kind string, i int) string {
				hash := sha256.Sum256([]byte(fmt.Sprintf("PUBLIC-benchmark-%s-%d", kind, i)))
				return base64.RawURLEncoding.EncodeToString(hash[:])
			}
			for i := range count {
				r := vector.Record
				consumption := *r.Consumption
				r.Consumption = &consumption
				r.NodeID = fmt.Sprintf("node-history-%d", i)
				r.Phase = "revoked"
				r.ReceiptID = token("receipt", i)
				c := &r.Consumption.Challenge
				c.ID = token("challenge", i)
				c.Binding.HandoffID = token("handoff", i)
				c.Binding.AttemptID = token("attempt", i)
				c.Binding.ReserveID = token("reserve", i)
				c.Binding.DevicePublicKey = token("public-key", i)
				r.Scope.HandoffID = c.Binding.HandoffID
				r.Scope.AttemptID = c.Binding.AttemptID
				r.Scope.ReserveID = c.Binding.ReserveID
				pub, err := base64.RawURLEncoding.DecodeString(c.Binding.DevicePublicKey)
				if err != nil {
					b.Fatal(err)
				}
				r.DevicePublicKey = base64.StdEncoding.EncodeToString(pub)
				r.Consumption.TranscriptDigest = registrarTranscriptDigest(*c)
				r.ReceiptDigest = registrarReceiptDigest(r)
				claim := registrarClaimKey(r.NetworkID, r.DevicePublicKey)
				s.data.Registrar.Records[claim] = &r
				s.data.Registrar.ConsumedProofs[registrarProofKey(*c)] = claim
			}
			if err := s.validateRegistrarState(); err != nil {
				b.Fatal(err)
			}
			s.mu.Lock()
			err = s.saveLocked()
			s.mu.Unlock()
			if err != nil {
				b.Fatal(err)
			}
			info, err := os.Stat(s.path)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.mu.Lock()
				err := s.saveLocked()
				s.mu.Unlock()
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(info.Size()), "state-bytes")
		})
	}
}
