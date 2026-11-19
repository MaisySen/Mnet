package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const fileChunk = 256 * 1024

// DefaultMaxFileBytes bounds one incoming transfer unless explicitly raised.
const DefaultMaxFileBytes int64 = 1 << 30

// DefaultFileIdleTimeout bounds silence between incoming data/FIN frames.
const DefaultFileIdleTimeout = 60 * time.Second

// FileReceiver handles incoming file transfer streams. Files land in
// Dir under their offered base name. An empty Dir disables reception; no
// implicit process working-directory destination is allowed.
type FileReceiver struct {
	// Internal transaction seam for deterministic fault/lifecycle checks. External
	// callers retain the path backend; this does not enable provider reception.
	beginStorage func(dir, name string) (receiveTransaction, error)

	// RequireApproval selects per-offer local consent. Its zero value preserves
	// legacy explicitly configured-directory reception. Consumer callers must opt
	// in; neither viewing nor a control grant substitutes for this decision.
	RequireApproval bool
	Approval        *FileApprovalGate

	Mux         *Mux
	Dir         string
	MaxBytes    int64         // <=0 uses DefaultMaxFileBytes
	IdleTimeout time.Duration // <=0 uses DefaultFileIdleTimeout
	OnDone      func(name string, size int64)
	// Authorized checks current session access at storage boundaries. Nil keeps
	// the receiver's existing explicit destination policy without an extra gate.
	Authorized func() bool
}

// Handle runs one incoming file stream to completion.
func (r FileReceiver) Handle(s Stream) {
	defer s.release()
	// An authorization callback can block while this session ends. Check both
	// sides of it so a late true result cannot grant work to a retired mux.
	allowed := func() bool {
		select {
		case <-r.Mux.Closed():
			return false
		default:
		}
		if r.Authorized != nil && !r.Authorized() {
			return false
		}
		select {
		case <-r.Mux.Closed():
			return false
		default:
			return true
		}
	}
	if !allowed() {
		return
	}
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
	if !safeTransferName(name) {
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
	if !allowed() {
		return
	}
	destination := fileReceiveDestination{dir: r.Dir}
	if r.RequireApproval {
		var ok bool
		destination, ok = r.approvedDestination(s, offer, allowed)
		if !ok {
			// Retiring a local prompt must not wait for a peer to drain output.
			if err := r.Mux.controlJSON(s.ID, FileAccept{OK: false, Reason: "file approval unavailable or declined"}); err != nil {
				r.Mux.tryReset(s.ID)
			}
			return
		}
		if !allowed() {
			return
		}
	}
	if strings.TrimSpace(destination.dir) == "" {
		refuse("file reception is not configured")
		return
	}

	begin := r.beginStorage
	if begin == nil {
		begin = beginPathReceive
	}
	var storage receiveTransaction
	var err error
	if destination.export != nil {
		storage, err = beginExportReceive(destination, offer, r.Mux.Closed(), allowed)
	} else {
		storage, err = begin(destination.dir, name)
	}
	if err != nil {
		if errors.Is(err, errReceiveDestination) {
			refuse("destination already exists or is inaccessible")
		} else {
			refuse("receiver storage unavailable")
		}
		return
	}
	defer storage.Abort()
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
			r.Mux.tryReset(s.ID)
			return
		}
		fr, err := s.Recv(remaining)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if !errors.Is(err, ErrStreamReset) && !errors.Is(err, io.ErrClosedPipe) {
				r.Mux.tryReset(s.ID)
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
		if !allowed() {
			return
		}
		written, err := storage.Write(fr.Payload)
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
	if !allowed() {
		return
	}

	if err = storage.Finalize(); err != nil {
		ack(false, digest)
		return
	}
	if !allowed() {
		return
	}
	if err = storage.Publish(); err != nil {
		ack(false, digest)
		return
	}
	ack(true, digest)
	if r.OnDone != nil && allowed() {
		r.OnDone(name, n)
	}
}

// SendFile offers and sends a file over a new dynamic stream and
// waits for the final ack.
func SendFile(m *Mux, path string) error {
	select {
	case <-m.Closed():
		return net.ErrClosed
	default:
	}
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
	if err := copyOwnedFileBytes(m.Closed(), h, f, st.Size()); err != nil {
		return err
	}
	current, err := f.Stat()
	if err != nil {
		return err
	}
	if current.Size() != st.Size() {
		return fmt.Errorf("session: source size changed during hashing")
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
	complete := false
	defer func() {
		if !complete {
			m.retireFileOutbound(s.ID)
			m.tryReset(s.ID)
		}
		s.release()
	}()
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

	// Read terminal receiver replies concurrently with copying. Otherwise a reset
	// is observed only after the entire source has been queued. This goroutine is
	// joined on every return; stream release wakes its blocking Recv.
	type fileReply struct {
		frame Frame
		err   error
	}
	replies := make(chan fileReply, 1)
	canceled := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		frame, err := s.Recv(0)
		close(canceled)
		m.wmu.Lock()
		m.cond.Broadcast()
		m.wmu.Unlock()
		replies <- fileReply{frame, err}
	}()
	defer func() { s.release(); <-readerDone }()
	if err := copyOwnedFileBytes(canceled, fileStreamWriter{mux: m, id: s.ID, canceled: canceled}, f, st.Size()-accept.Offset); err != nil {
		return err
	}
	current, err = f.Stat()
	if err != nil {
		return err
	}
	if current.Size() != st.Size() {
		return fmt.Errorf("session: source size changed during transfer")
	}
	if err := m.sendWithCancel(s.ID, msgFin, PriBulk, nil, true, canceled); err != nil {
		return err
	}
	// Only the final acknowledgement wait has a30second deadline. Time spent
	// transferring a large file does not consume that allowance.
	var reply fileReply
	select {
	case reply = <-replies:
	case <-time.After(30 * time.Second):
		return fmt.Errorf("session: transfer sent, ack unread: timeout")
	}
	if reply.err != nil {
		return fmt.Errorf("session: transfer sent, ack unread: %w", reply.err)
	}
	ack := reply.frame
	var fa FileAck
	if err := json.Unmarshal(ack.Payload, &fa); err != nil {
		return err
	}
	if !fa.OK || !strings.EqualFold(fa.SHA256, offer.SHA256) {
		return fmt.Errorf("session: receiver digest mismatch")
	}
	complete = true
	return nil
}

// Use one portable filename policy on every host. In particular, a remote
// offer must not select Windows alternate streams, device names or normalized
// aliases such as a trailing dot/space, even when tested on a Unix host.
func safeTransferName(name string) bool {
	if !utf8.ValidString(name) || name == "" || name == "." || name == ".." || len(name) > 255 || filepath.Base(name) != name || strings.ContainsAny(name, "/\\:<>\"|?*") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
	}
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	switch stem {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		return false
	}
	if stem == "COM¹" || stem == "COM²" || stem == "COM³" || stem == "LPT¹" || stem == "LPT²" || stem == "LPT³" {
		return false
	}
	return true
}

// copyOwnedFileBytes is bounded to the offered file snapshot. Never read/hash
// appended bytes or continue into another read/write after known session End.
// A filesystem Read/Write already dispatched must still return on its own.
func copyOwnedFileBytes(closed <-chan struct{}, dst io.Writer, src io.Reader, size int64) error {
	if size < 0 {
		return fmt.Errorf("session: invalid source size")
	}
	buf := make([]byte, fileChunk)
	for size > 0 {
		select {
		case <-closed:
			return net.ErrClosed
		default:
		}
		chunk := int64(len(buf))
		if size < chunk {
			chunk = size
		}
		n, err := src.Read(buf[:chunk])
		select {
		case <-closed:
			return net.ErrClosed
		default:
		}
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			if werr != nil {
				return werr
			}
			if written != n {
				return io.ErrShortWrite
			}
			size -= int64(n)
		}
		if err != nil {
			if err == io.EOF && size == 0 {
				return nil
			}
			if err == io.EOF {
				return io.ErrUnexpectedEOF
			}
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	select {
	case <-closed:
		return net.ErrClosed
	default:
		return nil
	}
}

type fileStreamWriter struct {
	mux      *Mux
	id       uint32
	canceled <-chan struct{}
}

func (w fileStreamWriter) Write(p []byte) (int, error) {
	if err := w.mux.sendWithCancel(w.id, msgData, PriBulk, p, true, w.canceled); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Drop only this failed transfer's not-yet-dispatched output. A frame already
// handed to conn.Write cannot be recalled without ending the shared transport.
func (m *Mux) retireFileOutbound(id uint32) {
	m.wmu.Lock()
	defer m.wmu.Unlock()
	for pri, queue := range m.pending {
		keep := queue[:0]
		for _, frame := range queue {
			if frame.StreamID == id {
				m.queuedBytes -= headerLen + len(frame.Payload)
			} else {
				keep = append(keep, frame)
			}
		}
		clear(queue[len(keep):])
		m.pending[pri] = keep
	}
	m.cond.Broadcast()
}
