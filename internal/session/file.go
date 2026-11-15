package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const fileChunk = 256 * 1024

// FileReceiver handles incoming file transfer streams. Files land in
// Dir (default: current directory) under their offered base name.
type FileReceiver struct {
	Mux    *Mux
	Dir    string
	OnDone func(name string, size int64)
}

// Handle runs one incoming file stream to completion.
func (r FileReceiver) Handle(s Stream) {
	var offer FileOffer
	if err := json.Unmarshal(s.OpenJSON, &offer); err != nil {
		return
	}
	dir := r.Dir
	if dir == "" {
		dir = "."
	}
	name := filepath.Base(offer.Name)
	if name == "." || name == "/" || name == "" {
		return
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		r.Mux.SendJSON(s.ID, PriBulk, FileAccept{OK: false, Reason: err.Error()})
		return
	}
	defer f.Close()
	if err := r.Mux.SendJSON(s.ID, PriBulk, FileAccept{OK: true}); err != nil {
		return
	}
	h := sha256.New()
	var n int64
	for {
		fr, err := s.Recv(0)
		if err != nil {
			break // FIN: transfer stream closed by sender
		}
		if fr.Type != msgData {
			continue
		}
		// An ack/accept from the sender side arrives as JSON DATA;
		// chunk data is raw. Chunks are the only DATA we expect here.
		f.Write(fr.Payload)
		h.Write(fr.Payload)
		n += int64(len(fr.Payload))
	}
	digest := hex.EncodeToString(h.Sum(nil))
	ok := n == offer.Size && (offer.SHA256 == "" || digest == offer.SHA256)
	r.Mux.SendJSON(s.ID, PriBulk, FileAck{OK: ok, SHA256: digest})
	if ok && r.OnDone != nil {
		r.OnDone(name, n)
	}
}

// SendFile offers and sends a file over a new dynamic stream and
// waits for the final ack.
func SendFile(m *Mux, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	offer := FileOffer{
		Name:   filepath.Base(path),
		Size:   st.Size(),
		SHA256: hex.EncodeToString(h.Sum(nil)),
	}
	s, err := m.Open(PriBulk, offer)
	if err != nil {
		return err
	}
	// Wait for the accept.
	acc, err := s.Recv(30 * time.Second)
	if err != nil {
		return err
	}
	var accept FileAccept
	if err := json.Unmarshal(acc.Payload, &accept); err != nil {
		return fmt.Errorf("session: bad accept: %w", err)
	}
	if !accept.OK {
		return fmt.Errorf("session: file refused: %s", accept.Reason)
	}
	if accept.Offset > 0 {
		if _, err := f.Seek(accept.Offset, 0); err != nil {
			return err
		}
	}
	buf := make([]byte, fileChunk)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if werr := m.SendBytes(s.ID, PriBulk, buf[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if err := m.send(s.ID, msgFin, PriBulk, nil); err != nil {
		return err
	}
	// FIN half-closes our side; the receiver's ack still arrives as
	// DATA on the same stream id, which the mux routes to s.
	ack, err := s.Recv(30 * time.Second)
	if err != nil {
		return fmt.Errorf("session: transfer sent, ack unread: %w", err)
	}
	var fa FileAck
	if err := json.Unmarshal(ack.Payload, &fa); err != nil {
		return err
	}
	if !fa.OK {
		return fmt.Errorf("session: receiver digest mismatch")
	}
	return nil
}
