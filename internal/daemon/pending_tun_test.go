package daemon

import (
	"golang.zx2c4.com/wireguard/tun"
	"sync/atomic"
	"testing"
)

type closeTrackedTUN struct {
	tun.Device
	closes atomic.Int32
}

func (t *closeTrackedTUN) Close() error { t.closes.Add(1); return nil }

func TestCloseReleasesPendingInjectedTunnel(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	pending := &closeTrackedTUN{}
	if err := d.SetTUN(pending); err != nil {
		t.Fatal(err)
	}
	d.Close()
	d.Close()
	if pending.closes.Load() != 1 {
		t.Fatalf("pending tunnel closes=%d, want1", pending.closes.Load())
	}
}

func TestPendingTunnelReplacementRefused(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	first, second := &closeTrackedTUN{}, &closeTrackedTUN{}
	if err := d.SetTUN(first); err != nil {
		t.Fatal(err)
	}
	if err := d.SetTUN(second); err == nil {
		t.Fatal("unconsumed tunnel silently replaced")
	}
	if first.closes.Load() != 0 || second.closes.Load() != 0 {
		t.Fatal("refusal closed caller-owned or existing tunnel")
	}
}

func TestDownReleasesPendingAndAllowsReplacement(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	first, second := &closeTrackedTUN{}, &closeTrackedTUN{}
	if d.SetTUN(first) != nil {
		t.Fatal("first injection failed")
	}
	d.Down()
	if first.closes.Load() != 1 {
		t.Fatal("Down leaked pending tunnel")
	}
	if d.SetTUN(second) != nil {
		t.Fatal("Down did not permit new injection")
	}
	d.Close()
	if second.closes.Load() != 1 {
		t.Fatal("Close leaked replacement tunnel")
	}
}
