//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestLinuxServiceUnitSavedIdentityOnly(t *testing.T) {
	base := options{stateDir: "/var/lib/mnet", ifname: "mnet0"}
	unit, err := linuxServiceUnit("/usr/local/bin/mnetd", base)
	if err != nil || !strings.Contains(unit, "ExecStart=/usr/local/bin/mnetd -state /var/lib/mnet -ifname mnet0 -up\n") || !strings.Contains(unit, "UMask=0077") || !strings.Contains(unit, "Restart=on-failure") {
		t.Fatal("unit missing saved-state startup/recovery", err)
	}
	for _, state := range []string{"relative", "/", "/var/lib/../mnet", "/var/lib/a b", "/var/lib/%n", "/var/lib/$HOME", "/var/lib/a\nExecStart=/bad", "/var/lib/\"quoted"} {
		o := base
		o.stateDir = state
		if u, e := linuxServiceUnit("/usr/local/bin/mnetd", o); e == nil || u != "" {
			t.Fatal("unsafe state path generated unit")
		}
	}
	for _, exe := range []string{"mnetd", "/tmp/mnet$d", "/tmp/mnet%f", "/tmp/mnet d"} {
		if u, e := linuxServiceUnit(exe, base); e == nil || u != "" {
			t.Fatal("unsafe executable generated unit")
		}
	}
	for _, change := range []func(*options){func(o *options) { o.authKey = "private-key" }, func(o *options) { o.server = "https://control.example" }, func(o *options) { o.network = "home" }, func(o *options) { o.advRoutes = "192.168.1.0/24" }, func(o *options) { o.sessAuto = true }, func(o *options) { o.fakeTun = true }, func(o *options) { o.ifname = "mnet%0" }, func(o *options) { o.ifname = ".." }, func(o *options) { o.ifname = strings.Repeat("x", 16) }, func(o *options) { o.proofDir = "/other" }} {
		o := base
		change(&o)
		u, e := linuxServiceUnit("/usr/local/bin/mnetd", o)
		if e == nil || u != "" || strings.Contains(e.Error(), "private-key") {
			t.Fatal("override accepted or secret echoed")
		}
	}
}

func TestLinuxServiceActionsNeverInstall(t *testing.T) {
	for _, action := range []string{"install", "remove", "run", "unknown"} {
		if err := runServiceAction(options{serviceAction: action}); err == nil {
			t.Fatal("Linux mutation action accepted")
		}
	}
}
