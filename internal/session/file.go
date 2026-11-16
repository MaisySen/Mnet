package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const fileChunk = 256 * 1024

// DefaultMaxFileBytes bounds one incoming transfer unless explicitly raised.
const DefaultMaxFileBytes int64 = 1 << 30

// DefaultFileIdleTimeout bounds silence between incoming data/FIN frames.
const DefaultFileIdleTimeout = 60 * time.Second

// FileReceiver handles incoming file transfer streams. Files land in
// Dir (default: current directory) under their offered base name.
type FileReceiver struct {
	Mux         *Mux
	Dir         string
	MaxBytes    int64         // <=0 uses DefaultMaxFileBytes
	IdleTimeout time.Duration // <=0 uses DefaultFileIdleTimeout
	OnDone      func(name string, size int64)
}

// Handle runs one incoming file stream to completion.
func (r FileReceiver) Handle(s Stream) {
	defer r.Mux.releaseStream(s.ID)
	refuse := func(reason string) { _ = r.Mux.SendJSON(s.ID, PriBulk, FileAccept{OK: false, Reason: reason}) }
	var offer FileOffer
	if err := json.Unmarshal(s.OpenJSON, &offer); err != nil {
		refuse("invalid offer")
		return
	}
	limit := r.MaxBytes
	if limit <= 0 {
		limit = DefaultMaxFileBytes
	}
	name := offer.Name
	if name == "" || name == "." || name == ".." || len(name) > 255 || strings.ContainsAny(name, "/\\\x00") || filepath.Base(name) != name {
		refuse("invalid file name")
		return
	}
	if offer.Size < 0 || offer.Size > limit {
		refuse("file size exceeds receiver policy")
		return
	}
	if offer.SHA256 != "" {
		digest, err := hex.DecodeString(offer.SHA256)
		if err != nil || len(digest) != sha256.Size {
			refuse("invalid SHA256")
			return
		}
	}
	dir := r.Dir
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		refuse(err.Error())
		return
	}
	path := filepath.Join(dir, name)
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		refuse("destination already exists or is inaccessible")
		return
	}
	// Never expose or overwrite the offered destination until the complete
	// bytes, length and digest have been verified. CreateTemp uses mode 0600.
	f, err := os.CreateTemp(dir, ".mnet-transfer-*")
	if err != nil {
		refuse(err.Error())
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if err = r.Mux.SendJSON(s.ID, PriBulk, FileAccept{OK: true}); err != nil {
		return
	}
	ack := func(ok bool, digest string) { _ = r.Mux.SendJSON(s.ID, PriBulk, FileAck{OK: ok, SHA256: digest}) }
	idle := r.IdleTimeout
	if idle <= 0 {
		idle = DefaultFileIdleTimeout
	}
	h := sha256.New()
	var n int64
	progressBy := time.Now().Add(idle)
	for {
		remaining := time.Until(progressBy)
		if remaining <= 0 {
			_ = r.Mux.send(s.ID, msgRst, PriBulk, nil)
			return
		}
		fr, err := s.Recv(remaining)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if !errors.Is(err, ErrStreamReset) && !errors.Is(err, io.ErrClosedPipe) {
				_ = r.Mux.send(s.ID, msgRst, PriBulk, nil)
			}
			return
		}
		if fr.Type != msgData {
			continue
		}
		if int64(len(fr.Payload)) > offer.Size-n {
			ack(false, "")
			return
		}
		written, err := f.Write(fr.Payload)
		if err != nil || written != len(fr.Payload) {
			ack(false, "")
			return
		}
		_, _ = h.Write(fr.Payload)
		n += int64(written)
		if written > 0 {
			progressBy = time.Now().Add(idle)
		}
	}
	digest := hex.EncodeToString(h.Sum(nil))
	if n != offer.Size || (offer.SHA256 != "" && !strings.EqualFold(digest, offer.SHA256)) {
		ack(false, digest)
		return
	}
	if err = f.Sync(); err != nil {
		ack(false, digest)
		return
	}
	if err = f.Close(); err != nil {
		ack(false, digest)
		return
	}
	// Hard-link publication is atomic and fails if another transfer or a
	// symlink already occupies the destination. No replace-on-rename race.
	if err = os.Link(tmp, path); err != nil {
		ack(false, digest)
		return
	}
	ack(true, digest)
	if r.OnDone != nil {
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
	if !st.Mode().IsRegular() {
		return fmt.Errorf("session: only regular files can be sent")
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
	defer m.releaseStream(s.ID)
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
	if accept.Offset < 0 || accept.Offset > st.Size() {
		return fmt.Errorf("session: invalid receiver offset")
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
	if !fa.OK || !strings.EqualFold(fa.SHA256, offer.SHA256) {
		return fmt.Errorf("session: receiver digest mismatch")
	}
	return nil
}
