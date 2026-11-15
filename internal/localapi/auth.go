// Package localapi protects access to the local daemon control interface.
package localapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fintex.au/mnet/internal/privatefs"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

const TokenHeader = "X-Mnet-Local-Token"

func tokenPath(dir string) string { return filepath.Join(dir, "local-api-token") }

// ReadToken reads a private daemon capability without following symlinks.
func ReadToken(dir string) (string, error) {
	path := tokenPath(dir)
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("local API token unavailable; use the daemon's state directory")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("local API token must be a regular file")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("local API token is not readable")
	}
	raw, err := hex.DecodeString(string(b))
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("invalid local API token file")
	}
	return string(b), nil
}

// EnsureToken creates a stable, owner-only capability for the daemon and CLI.
func EnsureToken(dir string) (string, error) {
	if err := privatefs.EnsureDir(dir); err != nil {
		return "", err
	}
	if _, err := os.Lstat(tokenPath(dir)); err == nil {
		if err := privatefs.ProtectFile(tokenPath(dir)); err != nil {
			return "", err
		}
		return ReadToken(dir)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	value := hex.EncodeToString(raw[:])
	f, err := os.OpenFile(tokenPath(dir), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return ReadToken(dir)
	}
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tokenPath(dir))
		}
	}()
	if _, err = f.WriteString(value); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	ok = true
	return value, nil
}

// RequireToken applies to reads too: status can reveal private peer details.
// Browser-origin requests are rejected even if a token were supplied.
func RequireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("Origin") != "" || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "none") {
			http.Error(w, "browser access denied", http.StatusForbidden)
			return
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(token)) != 1 {
			http.Error(w, "local API credential required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Transport adds the private capability only to the daemon's local address.
type Transport struct {
	StateDir string
	Base     http.RoundTripper
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "http" || (r.URL.Host != "mnetd" && r.URL.Host != "127.0.0.1:47373") {
		return nil, fmt.Errorf("refusing non-local daemon address")
	}
	token, err := ReadToken(t.StateDir)
	if err != nil {
		return nil, err
	}
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Set(TokenHeader, token)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}
