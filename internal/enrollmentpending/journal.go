// Package enrollmentpending is a disconnected native pending-intent journal.
// It owns only its own file, not the daemon identity or loopback listener. No
// browser, HTTP, credential installation, tunnel or authority permit exists.
package enrollmentpending

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"sync"
	"time"
	"unicode/utf8"

	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/enrollmentidentity"
	"fintex.au/mnet/internal/privatefs"
	"fintex.au/mnet/internal/stateowner"
)

var ErrInvalid = errors.New("invalid disconnected enrollment intent")
var ErrClosed = errors.New("enrollment intent journal closed")
var ErrUncertain = errors.New("enrollment intent publication uncertain; close and reopen required")
var ErrPending = errors.New("existing enrollment intent requires explicit recovery")

type record struct {
	Version      int                      `json:"version"`
	Phase        string                   `json:"phase"`
	Launch       contract.SourceLaunch    `json:"launch"`
	SourceOrigin string                   `json:"source_origin"`
	Authority    string                   `json:"authority"`
	Allowed      []contract.SourceMapping `json:"allowed"`
	Credential   string                   `json:"credential"`
	Verifier     string                   `json:"verifier"`
	Code         string                   `json:"code"`
	ExchangeJSON string                   `json:"exchange_json"`
	CreatedMS    int64                    `json:"created_ms"`
	ExpiresMS    int64                    `json:"expires_ms"`
}

func (record) String() string   { return "[private native enrollment intent]" }
func (record) GoString() string { return "[private native enrollment intent]" }

type tempFile interface {
	Name() string
	Write([]byte) (int, error)
	Sync() error
	Close() error
}
type filesystem struct {
	create  func(string) (tempFile, error)
	rename  func(string, string) error
	syncDir func(string) error
}
type Journal struct {
	mu       *sync.Mutex
	owner    *stateowner.Lease
	path     string
	fs       filesystem
	data     *record
	closed   bool
	fault    bool
	reopened bool
}

func (Journal) String() string   { return "[private native enrollment journal]" }
func (Journal) GoString() string { return "[private native enrollment journal]" }

// Open requires an explicit dedicated private directory, not a shared user
// folder. It refuses malformed existing state without changing its contents.
// Existing daemon key and process owner remain separately unimplemented gates.
func Open(dir string) (*Journal, error) {
	if err := privatefs.EnsureDir(dir); err != nil {
		return nil, err
	}
	owner, path, err := stateowner.Acquire(dir + string(os.PathSeparator) + "enrollment-pending-v1.json")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			owner.Close()
		}
	}()
	j := &Journal{mu: &sync.Mutex{}, owner: owner, path: path, fs: filesystem{create: func(dir string) (tempFile, error) { return os.CreateTemp(dir, ".mnet-enrollment-*") }, rename: stateowner.Rename, syncDir: stateowner.SyncDir}}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		ok = true
		return j, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || stateowner.SingleLink(path, info) != nil || info.Size() > 65536 {
		return nil, ErrInvalid
	}
	if err := privatefs.ProtectFile(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		f.Close()
		return nil, err
	}
	var r record
	if !strictRecord(raw, &r) || !validRecord(r) {
		f.Close()
		return nil, ErrInvalid
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = j.fs.syncDir(owner.Parent()); err != nil {
		return nil, err
	}
	if err = owner.Check(); err != nil {
		return nil, err
	}
	j.data = &r
	j.reopened = true
	ok = true
	return j, nil
}
func (j *Journal) health() error {
	if j.closed {
		return ErrClosed
	}
	if j.fault {
		return ErrUncertain
	}
	if err := j.owner.Check(); err != nil {
		j.fault = true
		return ErrUncertain
	}
	return nil
}
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	j.data = nil
	return j.owner.Close()
}

// Status exposes no launch, private token or principal. A reopened journal can
// report an incomplete phase but never automatically resumes a listener/request.
func (j *Journal) Status() (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return "", err
	}
	if j.data == nil {
		return "empty", nil
	}
	if j.data.Phase == "prepared" && j.reopened {
		return "prepared-listener-not-owned", nil
	}
	return j.data.Phase, nil
}

// Begin saves fresh native-only secret and PKCE material before returning public
// launch DATA. Caller must independently own the listener and existing durable
// WireGuard key. Passing a key/callback here proves neither fact. No defaults
// for duration, authority or allowed mappings are supplied.
func (j *Journal) Begin(publicKey, platform, callback, sourceOrigin, authority string, allowed []contract.SourceMapping, now, expires time.Time) (contract.SourceLaunch, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return contract.SourceLaunch{}, err
	}
	if j.data != nil {
		return contract.SourceLaunch{}, ErrPending
	}
	r := record{Version: 1, Phase: "prepared", SourceOrigin: sourceOrigin, Authority: authority, Allowed: append([]contract.SourceMapping(nil), allowed...), CreatedMS: now.UnixMilli(), ExpiresMS: expires.UnixMilli()}
	var err error
	for _, field := range []*string{&r.Credential, &r.Verifier, &r.Launch.AttemptID, &r.Launch.State} {
		*field, err = opaque()
		if err != nil {
			return contract.SourceLaunch{}, err
		}
	}
	r.Launch.Version = 1
	r.Launch.Purpose = "mnet-enrollment"
	r.Launch.DevicePublicKey = publicKey
	r.Launch.Platform = platform
	r.Launch.RedirectURI = callback
	r.Launch.CredentialDigest = hash(r.Credential)
	r.Launch.Challenge = hash(r.Verifier)
	if !validRecord(r) || now.IsZero() || expires.IsZero() {
		return contract.SourceLaunch{}, ErrInvalid
	}
	if err := j.commit(r); err != nil {
		return contract.SourceLaunch{}, err
	}
	return r.Launch, nil
}

// RecordCallback accepts DATA only after the future listener has checked exact
// method/host/path/unique query/Origin/state. This independently checks saved
// attempt/state/code too; it does not authenticate an HTTP callback itself.
func (j *Journal) RecordCallback(attempt, state, code string, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	if j.data == nil || j.data.Phase != "prepared" || j.reopened || !live(*j.data, now) || attempt != j.data.Launch.AttemptID || state != j.data.Launch.State || !token(code) {
		return ErrInvalid
	}
	r := *j.data
	r.Phase = "callback-recorded"
	r.Code = code
	return j.commit(r)
}

// MarkExchangeStarted MUST publish before future one-shot network exchange.
// On restart this phase means the server may have consumed it, not retry-safe.
func (j *Journal) MarkExchangeStarted(now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	if j.data == nil || j.data.Phase != "callback-recorded" || !live(*j.data, now) {
		return ErrInvalid
	}
	r := *j.data
	r.Phase = "exchange-may-have-started"
	return j.commit(r)
}

// RecordExchange stores strictly checked response DATA only after explicit
// may-have-started publication. It still proves no authenticated source or PKCE
// HTTP execution; no caller can turn the returned phase into an authority permit.
func (j *Journal) RecordExchange(raw []byte, currentAllowed []contract.SourceMapping, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	if j.data == nil || j.data.Phase != "exchange-may-have-started" || !live(*j.data, now) {
		return ErrInvalid
	}
	r := *j.data
	if _, err := contract.DecodeSourceExchange(raw, r.Launch, r.Authority, r.Allowed, currentAllowed, now); err != nil {
		return ErrInvalid
	}
	r.Phase = "exchanged-data"
	r.ExchangeJSON = string(raw)
	return j.commit(r)
}
func live(r record, now time.Time) bool {
	return !now.IsZero() && now.UnixMilli() >= r.CreatedMS && now.UnixMilli() < r.ExpiresMS
}
func opaque() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
func token(s string) bool {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != s {
		return false
	}
	for _, v := range b {
		if v != 0 {
			return true
		}
	}
	return false
}
func validRecord(r record) bool {
	if r.Version != 1 || r.CreatedMS <= 0 || r.ExpiresMS <= r.CreatedMS || !enrollmentidentity.ValidOrigin(r.SourceOrigin) || !enrollmentidentity.ValidOrigin(r.Authority) || len(r.Allowed) == 0 || len(r.Allowed) > 20 || !token(r.Credential) || !token(r.Verifier) || r.Launch.CredentialDigest != hash(r.Credential) || r.Launch.Challenge != hash(r.Verifier) {
		return false
	}
	// Reuse DATA-only descriptor grammar for launch/mappings, not a fabricated
	// source-authentication result or an authority constructor.
	seen := map[contract.SourceMapping]bool{}
	for _, m := range r.Allowed {
		if seen[m] {
			return false
		}
		seen[m] = true
		own := contract.SourceExchange{ID: r.Launch.AttemptID, Launch: r.Launch, Mapping: m}
		if _, err := contract.SourceDescriptorBytes(own, "journal-shape-check", r.Launch.AttemptID); err != nil {
			return false
		}
	}
	switch r.Phase {
	case "prepared":
		return r.Code == "" && r.ExchangeJSON == ""
	case "callback-recorded", "exchange-may-have-started":
		return token(r.Code) && r.ExchangeJSON == ""
	case "exchanged-data":
		if !token(r.Code) {
			return false
		}
		_, err := contract.DecodeSourceExchange([]byte(r.ExchangeJSON), r.Launch, r.Authority, r.Allowed, r.Allowed, time.UnixMilli(r.CreatedMS))
		return err == nil
	}
	return false
}
func (j *Journal) commit(r record) error {
	if err := j.health(); err != nil {
		return err
	}
	if !validRecord(r) {
		return ErrInvalid
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > 65536 {
		return ErrInvalid
	}
	if info, err := os.Lstat(j.path); err == nil {
		if !info.Mode().IsRegular() || stateowner.SingleLink(j.path, info) != nil {
			return ErrInvalid
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := j.fs.create(j.owner.Parent())
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
		os.Remove(tmp)
	}()
	if err := privatefs.ProtectFile(tmp); err != nil {
		return err
	}
	if n, err := f.Write(raw); err != nil {
		return err
	} else if n != len(raw) {
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	if err := j.health(); err != nil {
		return err
	}
	source, err := os.Lstat(tmp)
	if err != nil {
		return err
	}
	old, oldErr := os.Lstat(j.path)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return oldErr
	}
	if err := j.fs.rename(tmp, j.path); err != nil {
		target, targetErr := os.Lstat(j.path)
		left, leftErr := os.Lstat(tmp)
		unchangedTarget := (oldErr == nil && targetErr == nil && os.SameFile(old, target)) || (os.IsNotExist(oldErr) && os.IsNotExist(targetErr))
		unchangedSource := leftErr == nil && os.SameFile(source, left)
		if !unchangedTarget || !unchangedSource {
			j.fault = true
			return ErrUncertain
		}
		return err
	}
	if err := j.fs.syncDir(j.owner.Parent()); err != nil {
		j.fault = true
		return ErrUncertain
	}
	if err := j.health(); err != nil {
		return err
	}
	j.data = &r
	return nil
}
func strictRecord(raw []byte, out *record) bool {
	if len(raw) == 0 || len(raw) > 65536 || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !shape(d, reflect.TypeOf(*out), 0) {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
func shape(d *json.Decoder, t reflect.Type, depth int) bool {
	if depth > 8 {
		return false
	}
	switch t.Kind() {
	case reflect.Struct:
		v, err := d.Token()
		if err != nil || v != json.Delim('{') {
			return false
		}
		fields := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fields[f.Tag.Get("json")] = f.Type
		}
		seen := map[string]bool{}
		for d.More() {
			v, err = d.Token()
			name, ok := v.(string)
			if err != nil || !ok || seen[name] {
				return false
			}
			field, ok := fields[name]
			if !ok {
				return false
			}
			seen[name] = true
			if !shape(d, field, depth+1) {
				return false
			}
		}
		v, err = d.Token()
		return err == nil && v == json.Delim('}') && len(seen) == len(fields)
	case reflect.Slice:
		v, err := d.Token()
		if err != nil || v != json.Delim('[') {
			return false
		}
		count := 0
		for d.More() {
			count++
			if count > 20 || !shape(d, t.Elem(), depth+1) {
				return false
			}
		}
		v, err = d.Token()
		return err == nil && v == json.Delim(']')
	default:
		v, err := d.Token()
		if err != nil {
			return false
		}
		switch t.Kind() {
		case reflect.String:
			s, ok := v.(string)
			return ok && utf8.ValidString(s)
		case reflect.Int, reflect.Int64:
			n, ok := v.(json.Number)
			if !ok {
				return false
			}
			_, err := n.Int64()
			return err == nil
		}
	}
	return false
}
