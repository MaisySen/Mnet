package enrollmentcontract

// Separate immutable-history DATA protocol. Neither envelope carries a current
// access flag, lease renewal, proof confirmation or finalization permit.
type HistoryRequestWire struct {
	Version   int       `json:"version"`
	Challenge Challenge `json:"challenge"`
}
type HistoryReceiptWire struct {
	Version       int       `json:"version"`
	Receipt       ReceiptV2 `json:"receipt"`
	ReceiptDigest string    `json:"receipt_digest"`
}

func (HistoryRequestWire) String() string   { return "[private immutable history request DATA]" }
func (HistoryRequestWire) GoString() string { return "[private immutable history request DATA]" }
func (HistoryReceiptWire) String() string   { return "[private immutable receipt history DATA]" }
func (HistoryReceiptWire) GoString() string { return "[private immutable receipt history DATA]" }
func DecodeHistoryRequestWire(raw []byte) (HistoryRequestWire, error) {
	var w HistoryRequestWire
	if !proofWireExact(raw, &w) || w.Version != 1 {
		return HistoryRequestWire{}, ErrInvalid
	}
	if _, err := Encode(w.Challenge); err != nil {
		return HistoryRequestWire{}, err
	}
	return w, nil
}
func DecodeHistoryReceiptWire(raw []byte, pinned Challenge) (HistoryReceiptWire, error) {
	var w HistoryReceiptWire
	if !proofWireExact(raw, &w) || w.Version != 1 {
		return HistoryReceiptWire{}, ErrInvalid
	}
	expected, err := ReceiptV2ForChallenge(pinned, w.Receipt.NodeID, w.Receipt.ReceiptID, w.Receipt.CommittedAtMS)
	if err != nil || expected != w.Receipt || ReceiptV2Digest(expected) != w.ReceiptDigest {
		return HistoryReceiptWire{}, ErrInvalid
	}
	return w, nil
}
