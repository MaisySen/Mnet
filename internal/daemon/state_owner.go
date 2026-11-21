package daemon

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"unicode/utf8"

	"fintex.au/mnet/internal/privatefs"
	"fintex.au/mnet/internal/stateowner"
)

var ErrStateUncertain = errors.New("native state publication uncertain; Close and reopen required")
var ErrStateStale = errors.New("native state snapshot is stale or belongs to another directory")

type stateTemp interface {
	Name() string
	Write([]byte) (int, error)
	Sync() error
	Close() error
}
type stateFS struct {
	create  func(string) (stateTemp, error)
	rename  func(string, string) error
	syncDir func(string) error
}
type nativeStateOwner struct {
	lease *stateowner.Lease
	path  string
	saved State
	fs    stateFS
	fault bool
}

func openStateOwner(dir string) (*nativeStateOwner, error) {
	if dir == "" {
		return nil, ErrStateStale
	}
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrStateStale
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Acquire creates and syncs missing directory entries BEFORE EnsureDir can
	// pre-create them through MkdirAll. Then protect the dedicated directory.
	// Keep the SAME stable daemon.lock as the old process entry point.
	lease, _, err := stateowner.Acquire(filepath.Join(dir, "daemon"))
	if err != nil {
		return nil, err
	}
	if err := privatefs.EnsureDir(dir); err != nil {
		lease.Close()
		return nil, err
	}
	if err := stateowner.SyncDir(lease.Parent()); err != nil {
		lease.Close()
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			lease.Close()
		}
	}()
	o := &nativeStateOwner{lease: lease, path: filepath.Join(lease.Parent(), "state.json"), fs: stateFS{create: func(dir string) (stateTemp, error) { return os.CreateTemp(dir, ".mnet-native-*") }, rename: stateowner.Rename, syncDir: stateowner.SyncDir}}
	o.saved.revisionPath = o.path
	info, err := os.Lstat(o.path)
	if os.IsNotExist(err) {
		if err := guardNativeStateHistory(o); err != nil {
			return nil, err
		}
		ok = true
		return o, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || stateowner.SingleLink(o.path, info) != nil || info.Size() > 1<<20 {
		return nil, ErrStateStale
	}
	if err := privatefs.ProtectFile(o.path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(o.path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		f.Close()
		return nil, err
	}
	if !validStateJSON(raw) || unmarshalOwnedState(raw, &o.saved) != nil || o.saved.validateIdentity() != nil {
		f.Close()
		return nil, errors.New("invalid native identity state; contents were not changed")
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := o.fs.syncDir(lease.Parent()); err != nil {
		return nil, err
	}
	if err := lease.Check(); err != nil {
		return nil, err
	}
	o.saved.revisionPath = o.path
	o.saved.revision = sha256.Sum256(raw)
	o.saved.revisionExists = true
	if err := guardNativeStateHistory(o); err != nil {
		return nil, err
	}
	ok = true
	return o, nil
}
func (o *nativeStateOwner) health() error {
	if o.fault {
		return ErrStateUncertain
	}
	if err := o.lease.Check(); err != nil {
		o.fault = true
		return ErrStateUncertain
	}
	return nil
}
func cloneState(s State) State {
	s.AdvertiseRoutes = append([]string(nil), s.AdvertiseRoutes...)
	if s.native != nil {
		n := *s.native
		s.native = &n
	}
	return s
}
func (o *nativeStateOwner) save(s *State) error { return o.saveChecked(s, nil, nil) }
func (o *nativeStateOwner) saveChecked(s *State, before, after func() error) error {
	if err := o.health(); err != nil {
		return err
	}
	if s.nativeDetached {
		return ErrStateStale
	}
	if o.saved.native != nil && !reflect.DeepEqual(o.saved.native, s.native) {
		return ErrStateStale
	}
	if err := s.validateIdentity(); err != nil {
		return err
	}
	if o.saved.PrivateKey != "" && (s.PrivateKey != o.saved.PrivateKey || s.PublicKey != o.saved.PublicKey) {
		return errors.New("native identity replacement requires separate recovery; state was not changed")
	}
	raw, err := marshalOwnedState(s)
	if err != nil || !validStateJSON(raw) {
		return errors.New("invalid native state size")
	}
	if info, err := os.Lstat(o.path); err == nil {
		if !info.Mode().IsRegular() || stateowner.SingleLink(o.path, info) != nil {
			o.fault = true
			return ErrStateUncertain
		}
		existing, err := os.ReadFile(o.path)
		if err != nil {
			return err
		}
		if !o.saved.revisionExists || sha256.Sum256(existing) != o.saved.revision {
			o.fault = true
			return ErrStateUncertain
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if o.saved.revisionExists {
		o.fault = true
		return ErrStateUncertain
	}
	f, err := o.fs.create(o.lease.Parent())
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
	if err := o.health(); err != nil {
		return err
	}
	source, err := os.Lstat(tmp)
	if err != nil {
		return err
	}
	old, oldErr := os.Lstat(o.path)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return oldErr
	}
	if before != nil {
		if err := before(); err != nil {
			return err
		}
	}
	if err := o.fs.rename(tmp, o.path); err != nil {
		target, targetErr := os.Lstat(o.path)
		left, leftErr := os.Lstat(tmp)
		sameTarget := (oldErr == nil && targetErr == nil && os.SameFile(old, target)) || (os.IsNotExist(oldErr) && os.IsNotExist(targetErr))
		sameSource := leftErr == nil && os.SameFile(source, left)
		if !sameTarget || !sameSource {
			o.fault = true
			return ErrStateUncertain
		}
		return err
	}
	if err := o.fs.syncDir(o.lease.Parent()); err != nil {
		o.fault = true
		return ErrStateUncertain
	}
	if err := o.health(); err != nil {
		return err
	}
	if after != nil {
		if err := after(); err != nil {
			o.fault = true
			return ErrStateUncertain
		}
	}
	s.revisionPath = o.path
	s.revision = sha256.Sum256(raw)
	s.revisionExists = true
	o.saved = cloneState(*s)
	return nil
}

// Legacy fields may be absent; malformed/duplicate/unknown/case-alias/null fields
// never become zero-value identity defaults. Preserve optional legacy metadata.
func validStateJSON(raw []byte) bool {
	if len(raw) == 0 || len(raw) > 1<<20 || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	v, err := d.Token()
	if err != nil || v != json.Delim('{') {
		return false
	}
	fields := map[string]reflect.Kind{}
	typ := reflect.TypeOf(State{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := f.Tag.Get("json")
		for n, c := range name {
			if c == ',' {
				name = name[:n]
				break
			}
		}
		fields[name] = f.Type.Kind()
	}
	fields["native_enrollment_v1"] = reflect.String
	seen := map[string]bool{}
	for d.More() {
		v, err = d.Token()
		name, ok := v.(string)
		kind, known := fields[name]
		if err != nil || !ok || !known || seen[name] {
			return false
		}
		seen[name] = true
		v, err = d.Token()
		if err != nil {
			return false
		}
		switch kind {
		case reflect.String:
			value, ok := v.(string)
			if !ok {
				return false
			}
			if name == "native_enrollment_v1" {
				if _, err := decodeSavedCredential([]byte(value)); err != nil {
					return false
				}
			}
		case reflect.Bool:
			if _, ok := v.(bool); !ok {
				return false
			}
		case reflect.Slice:
			if v != json.Delim('[') {
				return false
			}
			count := 0
			for d.More() {
				count++
				if count > 4096 {
					return false
				}
				v, err = d.Token()
				if _, ok := v.(string); err != nil || !ok {
					return false
				}
			}
			v, err = d.Token()
			if err != nil || v != json.Delim(']') {
				return false
			}
		default:
			return false
		}
	}
	v, err = d.Token()
	if err != nil || v != json.Delim('}') {
		return false
	}
	_, err = d.Token()
	return err == io.EOF
}
