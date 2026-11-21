package enrollmentcontract

// Separate experimental workflow-v1 transport DATA. Older experimental-v4
// bootstrap wire stays unchanged. Parsing is not source/receipt authentication.
type WorkflowBeginWire struct {
	Version int                  `json:"version"`
	Request SourceReserveRequest `json:"request"`
}
type WorkflowSubmitWire struct {
	Version     int    `json:"version"`
	AttemptID   string `json:"attempt_id"`
	ChallengeID string `json:"challenge_id"`
	ResponseMAC string `json:"response_mac"`
}
type WorkflowReceiptWire struct {
	Version         int       `json:"version"`
	Receipt         ReceiptV2 `json:"receipt"`
	ReceiptDigest   string    `json:"receipt_digest"`
	ConfirmationMAC string    `json:"confirmation_mac"`
}

func (WorkflowBeginWire) String() string     { return "[private untrusted workflow begin DATA]" }
func (WorkflowBeginWire) GoString() string   { return "[private untrusted workflow begin DATA]" }
func (WorkflowSubmitWire) String() string    { return "[private untrusted workflow response DATA]" }
func (WorkflowSubmitWire) GoString() string  { return "[private untrusted workflow response DATA]" }
func (WorkflowReceiptWire) String() string   { return "[private untrusted workflow receipt DATA]" }
func (WorkflowReceiptWire) GoString() string { return "[private untrusted workflow receipt DATA]" }
func DecodeWorkflowBeginWire(raw []byte, serviceID string) (WorkflowBeginWire, error) {
	var w WorkflowBeginWire
	if !proofWireExact(raw, &w) || w.Version != 1 {
		return WorkflowBeginWire{}, ErrInvalid
	}
	if _, err := ServiceReserveDescriptorBytes(w.Request, serviceID); err != nil {
		return WorkflowBeginWire{}, err
	}
	return w, nil
}
func DecodeWorkflowSubmitWire(raw []byte) (WorkflowSubmitWire, error) {
	var w WorkflowSubmitWire
	if !proofWireExact(raw, &w) || w.Version != 1 {
		return WorkflowSubmitWire{}, ErrInvalid
	}
	for _, value := range []string{w.AttemptID, w.ChallengeID, w.ResponseMAC} {
		if _, err := digest(value); err != nil {
			return WorkflowSubmitWire{}, err
		}
	}
	return w, nil
}
func DecodeWorkflowReceiptWire(raw []byte, pinned Challenge) (WorkflowReceiptWire, error) {
	var w WorkflowReceiptWire
	if !proofWireExact(raw, &w) || w.Version != 1 {
		return WorkflowReceiptWire{}, ErrInvalid
	}
	expected, err := ReceiptV2ForChallenge(pinned, w.Receipt.NodeID, w.Receipt.ReceiptID, w.Receipt.CommittedAtMS)
	if err != nil || expected != w.Receipt || ReceiptV2Digest(expected) != w.ReceiptDigest {
		return WorkflowReceiptWire{}, ErrInvalid
	}
	if _, err := digest(w.ConfirmationMAC); err != nil {
		return WorkflowReceiptWire{}, err
	}
	return w, nil
}
