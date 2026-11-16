//go:build !windows

package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestDarwinServicePlistSavedStateOnly(t *testing.T) {
	base := options{stateDir: "/var/lib/mnet", ifname: "utun9"}
	plist, err := darwinServicePlist("/usr/local/bin/mnetd", base)
	if err != nil {
		t.Fatal(err)
	}
	var value struct{ XMLName xml.Name }
	if err := xml.Unmarshal([]byte(plist), &value); err != nil || value.XMLName.Local != "plist" {
		t.Fatal("invalid plist XML", err)
	}
	for _, want := range []string{"<string>/usr/local/bin/mnetd</string>", "<string>-state</string><string>/var/lib/mnet</string>", "<string>-ifname</string><string>utun9</string>", "<string>-up</string>", "<key>Umask</key><integer>63</integer>", "<key>SuccessfulExit</key><false/>", "<key>RunAtLoad</key><true/>"} {
		if !strings.Contains(plist, want) {
			t.Fatal("missing startup contract", want)
		}
	}
	auto := base
	auto.ifname = "utun"
	if out, err := darwinServicePlist("/usr/local/bin/mnetd", auto); err != nil || !strings.Contains(out, "<string>-ifname</string><string>utun</string>") {
		t.Fatal("automatic utun rejected", err)
	}

	for _, name := range []string{"mnet0", "utun-1", "utun01", "utun9junk", "utun65536", "utun9<key>"} {
		o := base
		o.ifname = name
		if s, e := darwinServicePlist("/usr/local/bin/mnetd", o); e == nil || s != "" {
			t.Fatal("unsupported interface accepted")
		}
	}
	for _, exe := range []string{"relative", "/", "/tmp/a b", "/tmp/a&b", "/tmp/a<key>", "/tmp/a/../mnetd"} {
		if s, e := darwinServicePlist(exe, base); e == nil || s != "" {
			t.Fatal("unsafe executable accepted")
		}
	}
	for _, path := range []string{"relative", "/", "/var/../lib/mnet", "/var/a b", "/var/a<key>", "/var/a&b", "/var/%s", "/var/$HOME", "/var/a\n<key>"} {
		o := base
		o.stateDir = path
		if s, e := darwinServicePlist("/usr/local/bin/mnetd", o); e == nil || s != "" {
			t.Fatal("unsafe state accepted")
		}
	}
	for _, change := range []func(*options){func(o *options) { o.authKey = "private-secret" }, func(o *options) { o.server = "https://private.example" }, func(o *options) { o.advRoutes = "10.0.0.0/24" }, func(o *options) { o.sessAuto = true }, func(o *options) { o.fakeTun = true }} {
		o := base
		change(&o)
		s, e := darwinServicePlist("/usr/local/bin/mnetd", o)
		if e == nil || s != "" || strings.Contains(e.Error(), "private-secret") {
			t.Fatal("override accepted/echoed")
		}
	}
}
