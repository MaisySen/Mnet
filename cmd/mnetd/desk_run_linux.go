//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"fintex.au/mnet/internal/desk"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"golang.org/x/sys/unix"
)

// readDeskLocalFile never follows symlinks and bounds reads before parsing.
// Private key and replay state must belong to this user and be owner-only.
func readDeskLocalFile(path string, private bool, limit int64) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("desk local file unavailable")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("desk local file must be regular")
	}
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if private && (info.Mode().Perm()&0077 != 0 || stat.Uid != uint32(os.Geteuid())) {
		return nil, errors.New("desk private key must be owner-only and owned by this user")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("desk local file unreadable or too large")
	}
	return data, nil
}
func loadDeskCertificate(spec desk.RunSpec) (tls.Certificate, error) {
	public, err := readDeskLocalFile(spec.Certificate, false, 65536)
	if err != nil {
		return tls.Certificate{}, err
	}
	private, err := readDeskLocalFile(spec.PrivateKey, true, 65536)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert, err := tls.X509KeyPair(public, private)
	if err != nil || len(cert.Certificate) != 1 {
		return tls.Certificate{}, errors.New("desk requires exactly one matching local certificate/key")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, err
	}
	expected := spec.Binding.Primary
	if spec.Role == "recipient" {
		expected = spec.Binding.Recipient
	}
	if desk.CertificateKey(leaf) != expected {
		return tls.Certificate{}, errors.New("desk own certificate differs from independently supplied role pin")
	}
	cert.Leaf = leaf
	return cert, nil
}

// Reserve before presenting review. Decline/failure also consumes this run.
// The caller explicitly supplies an existing private directory; never erase it
// or silently fall back to a new directory if durable reuse protection fails.
func reserveDeskRun(dir, run string) error {
	fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("desk explicit replay directory unavailable")
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("desk replay directory must be owner-only and owned by this user")
	}
	// A validated run is a lowercase256bit digest. Defensive check keeps this
	// helper safe even when called outside the decoded invocation.
	if len(run) != 64 {
		return errors.New("invalid desk run token")
	}
	for _, c := range run {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return errors.New("invalid desk run token")
		}
	}
	record, err := unix.Openat(fd, run, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return errors.New("desk run already consumed or replay storage unavailable")
	}
	syncErr := unix.Fsync(record)
	closeErr := unix.Close(record)
	return errors.Join(syncErr, closeErr, unix.Fsync(fd))
}
func deskRunDisplay(spec desk.RunSpec) error {
	if onWayland() || os.Getenv("DISPLAY") == "" {
		return errors.New("desk launch requires local X11")
	}
	c, err := xgb.NewConn()
	if err != nil {
		return err
	}
	defer c.Close()
	keyboard, err := deskKeyboardFingerprint(c)
	if err != nil || keyboard != spec.Binding.Keyboard {
		return errors.New("desk run keyboard differs from local desktop")
	}
	setup := xproto.Setup(c)
	screen := setup.DefaultScreen(c)
	if !deskStatusRGB(screen) {
		return errors.New("desk launch requires supported 24-bit TrueColor X11")
	}
	if len(setup.Roots) != 1 {
		return errors.New("desk launch supports one X screen only")
	}
	id, key := spec.SourceDisplay, spec.Binding.Primary
	if spec.Role == "recipient" {
		id, key = spec.Binding.Display, spec.Binding.Recipient
	}
	for _, d := range spec.Layout.Displays {
		if d.ID == id && d.Device == key && d.PixelWidth == int64(screen.WidthInPixels) && d.PixelHeight == int64(screen.HeightInPixels) && d.PixelWidth >= 64 && d.PixelHeight >= 64 && d.PixelWidth <= 32767 && d.PixelHeight <= 32767 {
			return nil
		}
	}
	return errors.New("desk run actual local geometry differs")
}

type deskRunReview struct {
	deskReviewUI
	seconds int
	edge    desk.Edge
}

func (r deskRunReview) Review(ctx context.Context, title, text string) error {
	edge := ""
	if title == "Mnet Shared Desk: review recipient" {
		edge = "Selected source edge: " + string(r.edge) + "\n"
	}
	return r.deskReviewUI.Review(ctx, title, fmt.Sprintf("One engineering attempt. Deadline includes review and waiting: %d seconds from launch.\n%sClosing the launcher stops this run. Decline/failure consumes its run token.\n\n", r.seconds, edge)+text)
}
func runDeskFile(parent context.Context, path string, status io.Writer) error {
	if parent == nil || status == nil {
		return errors.New("desk launcher context/status absent")
	}
	started := time.Now()
	data, err := readDeskLocalFile(path, false, 65536)
	if err != nil {
		return err
	}
	// Resolve local file paths against the explicitly selected config, never the
	// daemon state directory. Nothing is downloaded or discovered from a peer.
	spec, layout, err := desk.DecodeRun(bytes.NewReader(data))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(parent, started.Add(time.Duration(spec.LifetimeSeconds)*time.Second))
	defer cancel()
	base := filepath.Dir(path)
	for _, p := range []*string{&spec.Certificate, &spec.PrivateKey, &spec.RunDirectory} {
		if !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
	cert, err := loadDeskCertificate(spec)
	if err != nil {
		return err
	}
	if err = deskRunDisplay(spec); err != nil {
		return err
	}
	ui, err := newDeskReviewUI()
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err = reserveDeskRun(spec.RunDirectory, spec.Binding.Run); err != nil {
		return err
	}
	ui = deskRunReview{ui, spec.LifetimeSeconds, spec.Edge}
	fmt.Fprintln(status, "Shared Desk engineering run: local review required; no workspace identity, video, clipboard, files or recording")
	if spec.Role == "recipient" {
		return serveDeskRun(ctx, spec, layout, cert, ui, status)
	}
	return sendDeskRun(ctx, spec, layout, cert, ui, status)
}

func deskRunTLS(cert tls.Certificate, server bool) *tls.Config {
	c := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, NextProtos: []string{"mnet-desk-1"}}
	if server {
		c.ClientAuth = tls.RequireAnyClientCert
	} else {
		c.InsecureSkipVerify = true
	} // Exact peer public key is verified by desk before any input.
	return c
}
func closeOnDeskContext(ctx context.Context, closer io.Closer) func() {
	done := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			closer.Close()
		case <-done:
			if ctx.Err() != nil {
				closer.Close()
			}
		}
	}()
	return func() { close(done); <-joined }
}
func serveDeskRun(ctx context.Context, spec desk.RunSpec, layout *desk.Compiled, cert tls.Certificate, ui deskReviewUI, status io.Writer) (result error) {
	live := func() bool { return ctx.Err() == nil }
	// Review precedes opening a port. The waiting safety owner provides a local
	// stop while no native injector exists, then hands off before native setup.
	if err := reviewDeskRun(ctx, "recipient", spec.Binding, layout, live, ui); err != nil {
		return err
	}
	geometry, err := newDeskRunGeometryGuard(ctx, spec)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, geometry.Close()) }()
	ctx = geometry.ctx
	window, err := startDeskRunStatus(ctx, spec, "waiting")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, window.Close()) }()
	ctx = window.ctx
	safety, err := newDeskSafety(spec.Binding.Keyboard)
	if err != nil {
		return err
	}
	var handoff atomic.Bool
	safetyClosed := false
	defer func() {
		if !safetyClosed {
			result = errors.Join(result, safety.Close())
		}
	}()
	waiting, withdraw := context.WithCancel(ctx)
	defer withdraw()
	watchDone := make(chan struct{})
	watchJoined := make(chan struct{})
	go func() {
		defer close(watchJoined)
		select {
		case <-watchDone:
		case <-safety.Stopped():
			if !handoff.Load() {
				withdraw()
			}
		}
	}()
	defer func() { close(watchDone); <-watchJoined }()
	approvedLive := func() bool { return waiting.Err() == nil }
	approval, err := desk.NewApproval(spec.Binding, layout, approvedLive, func(current func() bool) (desk.NativeInput, error) {
		if !current() {
			return nil, errors.New("desk recipient retired before input")
		}
		handoff.Store(true)
		err := closeDeskSafetyForHandoff(safety)
		safetyClosed = true
		if err != nil {
			return nil, err
		}
		if !current() {
			return nil, errors.New("desk recipient retired during handoff")
		}
		input, err := startDeskProcessInput(spec.Binding.Keyboard, current)
		if err != nil {
			return nil, err
		}
		for _, d := range spec.Layout.Displays {
			if d.ID == spec.Binding.Display && (int64(input.process.width) != d.PixelWidth || int64(input.process.height) != d.PixelHeight) {
				return nil, errors.Join(errors.New("desk recipient geometry changed during approval"), input.Release())
			}
		}
		if err := window.Set("ready"); err != nil {
			return nil, errors.Join(err, input.Release())
		}
		return &deskStatusInput{input, window}, nil
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", spec.Address)
	if err != nil {
		return errors.New("desk exact local listen address unavailable")
	}
	defer listener.Close()
	joinListener := closeOnDeskContext(waiting, listener)
	defer joinListener()
	fmt.Fprintln(status, "Shared Desk recipient approved: waiting for one exact-key connection; local stop or process exit cancels")
	conn, err := listener.Accept()
	listener.Close()
	if err != nil {
		return errors.New("desk recipient canceled or accept failed")
	}
	defer conn.Close()
	joinConn := closeOnDeskContext(waiting, conn)
	defer joinConn()
	return approval.ServePair(tls.Server(conn, deskRunTLS(cert, true)))
}
func sendDeskRun(ctx context.Context, spec desk.RunSpec, layout *desk.Compiled, cert tls.Certificate, ui deskReviewUI, status io.Writer) (result error) {
	live := func() bool { return ctx.Err() == nil }
	if err := reviewDeskRun(ctx, "primary", spec.Binding, layout, live, deskEdgeReview{ui, spec.Edge}); err != nil {
		return err
	}
	window, err := startDeskRunStatus(ctx, spec, "armed")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, window.Close()) }()
	ctx = window.ctx
	var capture atomic.Pointer[deskPrimaryCapture]
	board, err := desk.NewSwitchboard(layout, spec.Binding.Primary, spec.SourceDisplay, live, func() {
		if c := capture.Load(); c != nil {
			c.disable()
		}
	})
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, board.Stop()) }()
	c, err := waitDeskEdge(ctx, board, spec.Edge, spec.Binding, spec.Binding.Keyboard, live, func(binding desk.Binding) (desk.NativeInput, error) {
		if err := window.Set("connecting"); err != nil {
			return nil, err
		}
		conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", spec.Address)
		if err != nil {
			return nil, errors.New("desk exact recipient unavailable; no retry")
		}
		client, err := desk.ConnectPair(tls.Client(conn, deskRunTLS(cert, false)), binding, layout, live)
		if err != nil {
			conn.Close()
		}
		return client, err
	})
	if err != nil {
		return err
	}
	capture.Store(c)
	if err := window.Set("forwarding"); err != nil {
		return errors.Join(err, c.Close())
	}
	fmt.Fprintln(status, "Shared Desk sending raw input: local stop, return edge, peer loss, deadline or process exit ends this run")
	select {
	case <-ctx.Done():
	case <-c.done:
	}
	err = c.Close()
	// A local requested stop can race an input acknowledgement that already
	// completed. Still require confirmed remote and local cleanup; only the
	// known input-authority diagnostic becomes an expected cancellation.
	if ctx.Err() != nil && errors.Is(c.runErr, desk.ErrAuthorityRetired) {
		return errors.Join(c.err, board.Stop())
	}
	return err
}

// inspectDeskLocal is read-only local evidence. It reserves no desktop owner,
// opens no network listener, reads no private key and creates no approval.
func inspectDeskLocal(path string, status io.Writer) error {
	if onWayland() || os.Getenv("DISPLAY") == "" {
		return errors.New("desk inspection requires local X11")
	}
	data, err := readDeskLocalFile(path, false, 65536)
	if err != nil {
		return err
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return errors.New("desk inspection requires exactly one PEM public certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	c, err := xgb.NewConn()
	if err != nil {
		return err
	}
	defer c.Close()
	keyboard, err := deskKeyboardFingerprint(c)
	if err != nil {
		return err
	}
	setup := xproto.Setup(c)
	screen := setup.DefaultScreen(c)
	if len(setup.Roots) != 1 {
		return errors.New("desk inspection supports one X screen only")
	}
	return json.NewEncoder(status).Encode(struct {
		LocalEvidenceOnly bool   `json:"local_evidence_only"`
		Approval          bool   `json:"approval"`
		CertificateKey    string `json:"certificate_key"`
		Keyboard          string `json:"keyboard_hash"`
		Width             uint16 `json:"pixel_width"`
		Height            uint16 `json:"pixel_height"`
	}{true, false, desk.CertificateKey(cert), keyboard, screen.WidthInPixels, screen.HeightInPixels})
}
