package enrollmentpending

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func TestWorkflowRecordMalformedRefusesWithoutRewrite(t *testing.T) {
	j, b, _ := bootstrapReady(t, "https://registrar.invalid")
	r := *j.data
	now := time.Now().UnixMilli()
	b.CheckedAtMS = now
	b.ReserveDeadlineMS = now + 30000
	b.AccessUntilMS = now + 5000
	c := contract.Challenge{ID: code(), Nonce: code(), EphemeralPublicKey: code(), ExpiresAtMS: now + 5000, Binding: b}
	rawC, _ := json.Marshal(contract.ChallengeWire{Version: 4, Challenge: c})
	w := workflowData{ReserveID: b.ReserveID, StartedMS: now, TimeoutMS: 1000, MaxChallengeMS: 10000, MaxReserveMS: 60000, MaxAgeMS: 20000, MaxLeaseMS: 10000, UncertaintyMS: 1, ChallengeJSON: string(rawC)}
	r.Version = 4
	r.Phase = "workflow-challenge-data"
	r.WorkflowJSON = workflowEncode(w)
	if !validRecord(r) {
		t.Fatal("fixture invalid")
	}
	dir, path := j.owner.Parent(), j.path
	j.Close()
	for _, kind := range []string{"missing", "null", "unknown", "duplicate", "overflow", "phase", "principal", "reserve", "receipt-before-submit", "bad-response", "empty-submit", "cross-purpose"} {
		t.Run(kind, func(t *testing.T) {
			bad := r
			data := w
			switch kind {
			case "missing":
				bad.WorkflowJSON = ""
			case "null":
				bad.WorkflowJSON = "null"
			case "unknown":
				bad.WorkflowJSON = strings.Replace(bad.WorkflowJSON, "{", `{"unknown":true,`, 1)
			case "duplicate":
				bad.WorkflowJSON = strings.Replace(bad.WorkflowJSON, "{", `{"started_ms":1,`, 1)
			case "overflow":
				data.TimeoutMS = math.MaxInt64
				bad.WorkflowJSON = workflowEncode(data)
			case "phase":
				bad.Phase = "exchanged-data"
			case "principal":
				changed := c
				changed.Binding.PrincipalID = "other"
				raw, _ := json.Marshal(contract.ChallengeWire{Version: 4, Challenge: changed})
				data.ChallengeJSON = string(raw)
				bad.WorkflowJSON = workflowEncode(data)
			case "reserve":
				data.ReserveID = code() + "x"
				bad.WorkflowJSON = workflowEncode(data)
			case "receipt-before-submit":
				data.ReceiptJSON = `{}`
				bad.WorkflowJSON = workflowEncode(data)
			case "bad-response":
				data.ResponseDigest = "bad"
				bad.Phase = "workflow-submit-may-have-started"
				bad.WorkflowJSON = workflowEncode(data)
			case "empty-submit":
				bad.Phase = "workflow-submit-may-have-started"
			case "cross-purpose":
				bad.BootstrapJSON = `{}`
			}
			raw, _ := json.Marshal(bad)
			var parsed record
			if strictRecord(raw, &parsed) && validRecord(parsed) {
				t.Fatal("malformed workflow accepted", kind)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(dir)
			if err == nil {
				opened.Close()
				t.Fatal("corrupt file opened")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatal("corrupt intent rewritten")
			}
		})
	}
}
