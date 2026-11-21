package enrollmentpending

// Local two-file protocol only. Caller owns actual native lock, current policy,
// confirmed proof provenance and sole State publication. No remote I/O here.
import (
	"bytes"
	"encoding/json"
	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/stateowner"
	"io"
	"os"
	"reflect"
	"time"
)

type finalizationData struct {
	ID                    string `json:"id"`
	OriginalStateRevision string `json:"original_state_revision"`
	PolicyRevision        string `json:"policy_revision"`
	ReceiptDigest         string `json:"receipt_digest"`
}

func decodeFinalization(r record) (finalizationData, bool) {
	var f finalizationData
	d := json.NewDecoder(bytes.NewReader([]byte(r.FinalizationJSON)))
	d.UseNumber()
	if !shape(d, reflect.TypeOf(f), 0) {
		return f, false
	}
	if _, err := d.Token(); err != io.EOF {
		return f, false
	}
	if json.Unmarshal([]byte(r.FinalizationJSON), &f) != nil {
		return f, false
	}
	return f, token(f.ID) && token(f.OriginalStateRevision) && token(f.PolicyRevision) && token(f.ReceiptDigest)
}
func finalizationDocument(r record) (contract.NativeCredentialDocument, finalizationData, error) {
	var empty contract.NativeCredentialDocument
	f, ok := decodeFinalization(r)
	if !ok {
		return empty, f, ErrInvalid
	}
	w, ok := decodeWorkflow(r)
	if !ok {
		return empty, f, ErrInvalid
	}
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if err != nil {
		return empty, f, err
	}
	var receipt contract.HistoryReceiptWire
	if r.Version == 7 {
		h, ok := decodeHistory(r)
		if !ok {
			return empty, f, ErrInvalid
		}
		receipt, err = contract.DecodeHistoryReceiptWire([]byte(h.ReceiptJSON), c)
	} else {
		var wire contract.WorkflowReceiptWire
		wire, err = contract.DecodeWorkflowReceiptWire([]byte(w.ReceiptJSON), c)
		receipt = contract.HistoryReceiptWire{Version: 1, Receipt: wire.Receipt, ReceiptDigest: wire.ReceiptDigest}
	}
	if err != nil || receipt.ReceiptDigest != f.ReceiptDigest {
		return empty, f, ErrInvalid
	}
	doc := contract.NativeCredentialDocument{Version: 1, Kind: contract.NativeCredentialKind, Secret: r.Credential, FinalizationID: f.ID, Challenge: c, Receipt: receipt.Receipt, ReceiptDigest: receipt.ReceiptDigest}
	return doc, f, contract.CheckNativeCredentialDocument(doc)
}
func validFinalizationRecord(r record) bool {
	if r.Phase != "native-finalization-prepared" && r.Phase != "native-finalization-completed" {
		return false
	}
	old := r
	old.Version = 4
	old.Phase = "workflow-receipt-data"
	old.FinalizationJSON = ""
	p, err := config.ValidateLocalV2(r.Policy)
	if err != nil || !validWorkflowRecord(old, p) {
		return false
	}
	_, _, err = finalizationDocument(r)
	return err == nil
}

// FinalizeWorkflow requires original live runtime receipt and exact provided
// authenticated projection AND private SAME-runtime crypto-confirmed readiness.
// Actual native caller must check its retained owner/policy inside publish. This method is
// not a constructor of receipt provenance or a public credential getter.
// Marker is durable before secret transfer to the narrow sole-State callback.
func (j *Journal) FinalizeWorkflow(receipt contract.WorkflowReceiptWire, originalRevision, policyRevision string, current SourceConfig, publish func(contract.NativeCredentialDocument) error) error {
	if j == nil || j.journal == nil || publish == nil || !token(originalRevision) || !token(policyRevision) {
		return ErrInvalid
	}
	cfg := copyConfig(current)
	now := time.Now()
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	if j.finalizationReady == nil || j.finalizationReady.image != j.data || j.finalizationReady.receipt != receipt || j.data == nil || j.reopened || j.cancel != nil || j.workflow == nil || !((j.data.Version == 4 && j.data.Phase == "workflow-receipt-data") || (j.data.Version == 5 && j.data.Phase == "native-finalization-prepared")) || !live(*j.data, now) || !bootstrapCurrent(*j.data, cfg, now) {
		return ErrInvalid
	}
	w, ok := decodeWorkflow(*j.data)
	c, err := contract.DecodeChallengeWire([]byte(w.ChallengeJSON))
	if !ok || err != nil {
		return ErrInvalid
	}
	pinned, err := contract.DecodeWorkflowReceiptWire([]byte(w.ReceiptJSON), c)
	if err != nil || pinned != receipt || now.UnixMilli() >= receipt.Receipt.AccessExpiresAtMS {
		return ErrInvalid
	}
	r := *j.data
	if r.Version == 4 {
		id, err := opaque()
		if err != nil {
			return err
		}
		f := finalizationData{ID: id, OriginalStateRevision: originalRevision, PolicyRevision: policyRevision, ReceiptDigest: receipt.ReceiptDigest}
		raw, _ := json.Marshal(f)
		r.Version = 5
		r.Phase = "native-finalization-prepared"
		r.FinalizationJSON = string(raw)
		if err := j.commit(r); err != nil {
			return err
		}
		j.finalizationReady.image = j.data
	} else {
		f, ok := decodeFinalization(r)
		if !ok || f.OriginalStateRevision != originalRevision || f.PolicyRevision != policyRevision || f.ReceiptDigest != receipt.ReceiptDigest {
			return ErrInvalid
		}
	}
	doc, _, err := finalizationDocument(r)
	if err != nil {
		return err
	}
	if err := publish(doc); err != nil {
		return err
	}
	r.Phase = "native-finalization-completed"
	err = j.commit(r)
	if err == nil {
		j.finalizationReady = nil
	}
	return err
}

// CorrelateFinalization never constructs/publishes missing native authority. It
// compares the existing sole State document and original revisions against exact
// SAME pending intent. It can finish only historical local bookkeeping AFTER an
// exact State exists. Restart with pending-only/absent-State is deliberately held
// until a separate authenticated SAME-attempt history recovery is implemented.
func (j *Journal) CorrelateFinalization(doc contract.NativeCredentialDocument, originalRevision, policyRevision string, complete bool) error {
	if j == nil || j.journal == nil {
		return ErrInvalid
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	if j.data == nil || (j.data.Version != 5 && j.data.Version != 7) {
		return ErrInvalid
	}
	info, err := os.Lstat(j.path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 || stateowner.SingleLink(j.path, info) != nil {
		return ErrInvalid
	}
	raw, err := os.ReadFile(j.path)
	canonical, _ := json.Marshal(j.data)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ErrInvalid
	}

	expected, f, err := finalizationDocument(*j.data)
	if err != nil || expected != doc || f.OriginalStateRevision != originalRevision || f.PolicyRevision != policyRevision {
		return ErrInvalid
	}
	if j.data.Phase == "native-finalization-completed" {
		return nil
	}
	if !complete || j.data.Phase != "native-finalization-prepared" {
		return ErrInvalid
	}
	r := *j.data
	r.Phase = "native-finalization-completed"
	return j.commit(r)
}
