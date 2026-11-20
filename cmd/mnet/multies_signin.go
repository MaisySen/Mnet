package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"fintex.au/mnet/internal/multies"
)

func multiesSignInOrigin(args []string) (string, error) {
	if len(args) != 2 || args[0] != "--server" || args[1] == "" {
		return "", errors.New("usage: mnet multies-signin --server TRUSTED_MULTIES_ORIGIN (experimental; no VPN enrollment)")
	}
	if _, err := multies.NewBridgeClient(args[1]); err != nil {
		return "", errors.New("Multies bridge origin is invalid")
	}
	return args[1], nil
}
func openMultiesBrowser(ctx context.Context, target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "linux":
		command = "xdg-open"
		args = []string{target}
	case "darwin":
		command = "open"
		args = []string{target}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", target}
	default:
		return errors.New("system browser opening is unsupported")
	}
	launch, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Direct argv only. Never pass a browser URL to a shell or inherit its output.
	if exec.CommandContext(launch, command, args...).Run() != nil {
		return errors.New("system browser could not be opened")
	}
	return nil
}

// This explicit experimental command prepares only a local daemon identity. It
// cannot manufacture account authority, start the daemon, enroll a VPN peer,
// persist a Multies credential or open a remote desktop/input session.
func runMultiesSignIn(ctx context.Context, local *http.Client, base, origin string, open func(context.Context, string) error) (multies.ApprovedDeviceList, error) {
	if open == nil {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	bridge, err := multies.NewBridgeClient(origin)
	if err != nil {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	if ctx == nil || ctx.Err() != nil || local == nil {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/localapi/identity", nil)
	if err != nil {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	response, err := local.Do(req)
	if err != nil {
		return multies.ApprovedDeviceList{}, errLocalDaemon
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return multies.ApprovedDeviceList{}, errors.New("local device identity preparation was refused")
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	if err != nil || len(encoded) > 1024 {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	var identity struct {
		PublicKey string `json:"public_key"`
	}
	if json.Unmarshal(encoded, &identity) != nil {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(identity.PublicKey)
	if err != nil || len(raw) != 32 || base64.StdEncoding.EncodeToString(raw) != identity.PublicKey {
		return multies.ApprovedDeviceList{}, errors.New("local device identity was not confirmed; browser was not opened")
	}
	sum := sha256.Sum256(raw)
	key := base64.RawURLEncoding.EncodeToString(sum[:])
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	fmt.Println("Experimental Multies bridge. This does not connect the VPN or enable remote input.")
	fmt.Println("Compare this exact device key in the browser:", key)
	c, err := bridge.SignIn(ctx, key, platform, func(target string) error { return open(ctx, target) })
	if err != nil {
		return multies.ApprovedDeviceList{}, errors.New("Multies sign in did not finish; no local credential was saved")
	}
	if _, err = bridge.Authorize(ctx, c, multies.List); err != nil {
		return multies.ApprovedDeviceList{}, multies.ErrDenied
	}
	return bridge.Devices(ctx, c)
}
