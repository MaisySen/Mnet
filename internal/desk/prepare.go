package desk

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
)

// Inspection is supplied local public evidence, never authenticated identity,
// current consent or network discovery. Preparation does not make it trusted.
type Inspection struct {
	LocalEvidenceOnly bool   `json:"local_evidence_only"`
	Approval          bool   `json:"approval"`
	CertificateKey    string `json:"certificate_key"`
	Keyboard          string `json:"keyboard_hash"`
	Width             int64  `json:"pixel_width"`
	Height            int64  `json:"pixel_height"`
}
type PreparationEndpoint struct {
	Inspection  Inspection `json:"inspection"`
	Certificate string     `json:"certificate_file"`
	PrivateKey  string     `json:"private_key_file"`
}
type Preparation struct {
	Version         int                 `json:"version"`
	Primary         PreparationEndpoint `json:"primary"`
	Recipient       PreparationEndpoint `json:"recipient"`
	Address         string              `json:"recipient_address"`
	Edge            Edge                `json:"source_edge"`
	LifetimeSeconds int                 `json:"lifetime_seconds"`
}
type PreparedRun struct {
	DraftOnly bool    `json:"draft_only"`
	Primary   RunSpec `json:"primary"`
	Recipient RunSpec `json:"recipient"`
}

func DecodePreparation(r io.Reader) (Preparation, error) {
	var request Preparation
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil || len(data) > 65536 {
		return request, errors.New("desk preparation document unreadable or exceeds64KiB")
	}
	if err = validateJSON(data); err != nil {
		return request, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return Preparation{}, errors.New("desk preparation schema invalid")
	}
	return request, nil
}

// Prepare builds reviewable drafts. It neither loads private keys nor invokes
// the run launcher. Each launcher independently rechecks its local evidence.
func Prepare(request Preparation) (PreparedRun, error) { return prepare(request, rand.Reader) }
func prepare(request Preparation, random io.Reader) (PreparedRun, error) {
	fail := func() (PreparedRun, error) {
		return PreparedRun{}, errors.New("desk preparation requires exact independent device evidence and local absolute identity paths")
	}
	if request.Version != 1 {
		return fail()
	}
	for _, endpoint := range []PreparationEndpoint{request.Primary, request.Recipient} {
		e := endpoint.Inspection
		if !e.LocalEvidenceOnly || e.Approval || !digest(e.CertificateKey) || !digest(e.Keyboard) || e.Width < 64 || e.Height < 64 || e.Width > 32767 || e.Height > 32767 || !filepath.IsAbs(endpoint.Certificate) || !filepath.IsAbs(endpoint.PrivateKey) || endpoint.Certificate == endpoint.PrivateKey {
			return fail()
		}
	}
	a, b := request.Primary.Inspection, request.Recipient.Inspection
	if a.CertificateKey == b.CertificateKey || a.Keyboard != b.Keyboard {
		return fail()
	}
	source := Display{ID: "primary", Device: a.CertificateKey, Width: a.Width, Height: a.Height, PixelWidth: a.Width, PixelHeight: a.Height}
	target := Display{ID: "recipient", Device: b.CertificateKey, Width: b.Width, Height: b.Height, PixelWidth: b.Width, PixelHeight: b.Height}
	switch request.Edge {
	case Right:
		target.X = source.Width
	case Left:
		target.X = -target.Width
	case Bottom:
		target.Y = source.Height
	case Top:
		target.Y = -target.Height
	default:
		return fail()
	}
	layout := Layout{Version: 1, Displays: []Display{source, target}}
	compiled, err := Compile(layout)
	if err != nil {
		return PreparedRun{}, err
	}
	// Validate all nonrandom fields before consuming entropy or returning drafts.
	binding := Binding{Run: string(bytes.Repeat([]byte("0"), 64)), Primary: a.CertificateKey, Recipient: b.CertificateKey, Layout: compiled.Fingerprint(), Keyboard: a.Keyboard, Display: target.ID}
	primary := RunSpec{Version: 1, Role: "primary", Address: request.Address, Certificate: request.Primary.Certificate, PrivateKey: request.Primary.PrivateKey, RunDirectory: "replay", LifetimeSeconds: request.LifetimeSeconds, SourceDisplay: source.ID, Edge: request.Edge, Binding: binding, Layout: layout}
	recipient := primary
	recipient.Role = "recipient"
	recipient.Certificate = request.Recipient.Certificate
	recipient.PrivateKey = request.Recipient.PrivateKey
	if _, err = primary.Validate(); err != nil {
		return PreparedRun{}, err
	}
	if _, err = recipient.Validate(); err != nil {
		return PreparedRun{}, err
	}
	var run [32]byte
	if random == nil {
		return PreparedRun{}, errors.New("desk run randomness unavailable")
	}
	if _, err = io.ReadFull(random, run[:]); err != nil {
		return PreparedRun{}, errors.New("desk run randomness unavailable")
	}
	primary.Binding.Run = hex.EncodeToString(run[:])
	recipient.Binding.Run = primary.Binding.Run
	return PreparedRun{DraftOnly: true, Primary: primary, Recipient: recipient}, nil
}
