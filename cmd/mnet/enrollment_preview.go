package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"

	"fintex.au/mnet/internal/multies"
)

func enrollmentPreviewOrigin(args []string) (string, error) {
	if len(args) != 2 || args[0] != "--server" {
		return "", errors.New("usage: mnet multies-enroll-preview --server TRUSTED_MULTIES_ORIGIN (consent only; no network joining)")
	}
	if _, e := multies.NewBridgeClient(args[1]); e != nil {
		return "", errors.New("explicit Multies origin is invalid")
	}
	return args[1], nil
}
func preparedPublicIdentity(raw []byte) ([]byte, error) {
	fail := errors.New("local public identity is invalid; browser was not opened")
	if len(raw) > 1024 {
		return nil, fail
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	v, e := d.Token()
	if e != nil || v != json.Delim('{') {
		return nil, fail
	}
	name, e := d.Token()
	if e != nil || name != "public_key" {
		return nil, fail
	}
	var key string
	if d.Decode(&key) != nil || d.More() {
		return nil, fail
	}
	v, e = d.Token()
	if e != nil || v != json.Delim('}') {
		return nil, fail
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, fail
	}
	b, e := base64.StdEncoding.Strict().DecodeString(key)
	if e != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != key || bytes.Equal(b, make([]byte, 32)) {
		return nil, fail
	}
	return b, nil
}

// runEnrollmentPreview opens the actual separate Multies consent page only.
// No callback listener or completion authority exists, and the ephemeral attempt
// is deliberately not a crash-recoverable enrollment record. Do not reuse it as one.
func runEnrollmentPreview(ctx context.Context, local *http.Client, base, origin string, open func(context.Context, string) error, out io.Writer) error {
	if ctx == nil || ctx.Err() != nil || local == nil || open == nil || out == nil {
		return multies.ErrDenied
	}
	if _, e := multies.NewBridgeClient(origin); e != nil {
		return multies.ErrDenied
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, base+"/localapi/identity", nil)
	if e != nil {
		return multies.ErrDenied
	}
	response, e := local.Do(req)
	if e != nil {
		return errLocalDaemon
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("local identity preparation was refused; browser was not opened")
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 1025))
	if e != nil {
		return multies.ErrDenied
	}
	public, e := preparedPublicIdentity(raw)
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var attempt [32]byte
	if _, e = rand.Read(attempt[:]); e != nil {
		return multies.ErrDenied
	}
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if platform != "linux" && platform != "windows" && platform != "macos" {
		return errors.New("enrollment preview is unsupported on this platform")
	}
	key := base64.RawURLEncoding.EncodeToString(public)
	u, e := url.Parse(origin)
	if e != nil {
		return multies.ErrDenied
	}
	u.Path = "/mnet/enroll"
	u.RawPath = ""
	u.RawQuery = url.Values{"attemptId": {base64.RawURLEncoding.EncodeToString(attempt[:])}, "devicePublicKey": {key}, "platform": {platform}}.Encode()
	if _, e = fmt.Fprintln(out, "Separate joining-consent preview. No computer will join a network.\nCompare this full public device key in Multies:", key); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e = open(ctx, u.String()); e != nil {
		return errors.New("browser did not open; no joining result is known")
	}
	_, e = fmt.Fprintln(out, "Browser opened for a separate decision. Mnet has not received or saved enrollment authority.\nThe registrar is not connected. No tunnel, screen, keyboard or mouse access was started.")
	return e
}
