//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fintex.au/mnet/internal/desk"
	"golang.org/x/sys/unix"
)

func TestDeskPreparationWritesOnlyNewPrivateDrafts(t *testing.T) {
	root := t.TempDir()
	request := filepath.Join(root, "request.json")
	evidence := func(pin string) desk.PreparationEndpoint {
		return desk.PreparationEndpoint{Inspection: desk.Inspection{LocalEvidenceOnly: true, CertificateKey: strings.Repeat(pin, 64), Keyboard: strings.Repeat("c", 64), Width: 1024, Height: 768}, Certificate: filepath.Join(root, pin, "cert.pem"), PrivateKey: filepath.Join(root, pin, "key.pem")}
	}
	value := desk.Preparation{Version: 1, Primary: evidence("a"), Recipient: evidence("b"), Address: "127.0.0.1:5910", Edge: desk.Right, LifetimeSeconds: 60}
	data, _ := json.Marshal(value)
	if err := os.WriteFile(request, data, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "drafts")
	if err := prepareDeskRunFiles(request, output); err != nil {
		t.Fatal(err)
	}
	if err := prepareDeskRunFiles(request, output); err == nil {
		t.Fatal("existing output replaced")
	}
	for _, relative := range []string{"", "primary", "recipient", "primary/replay", "recipient/replay", "primary/run.json", "recipient/run.json", "layout.json", "REVIEW.txt"} {
		info, err := os.Stat(filepath.Join(output, relative))
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("draft not private", relative, info, err)
		}
	}
	for _, role := range []string{"primary", "recipient"} {
		file, err := os.Open(filepath.Join(output, role, "run.json"))
		if err != nil {
			t.Fatal(err)
		}
		spec, _, err := desk.DecodeRun(file)
		file.Close()
		if err != nil || spec.Role != role {
			t.Fatal(spec, err)
		}
		entries, err := os.ReadDir(filepath.Join(output, role, "replay"))
		if err != nil || len(entries) != 0 {
			t.Fatal("preparation consumed approval/replay", err)
		}
		if _, err = os.Stat(spec.PrivateKey); !os.IsNotExist(err) {
			t.Fatal("preparation read or created key material", err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(output, "primary", "run.json"))
	if len(before) == 0 {
		t.Fatal("failed duplicate removed drafts")
	}
	link := filepath.Join(root, "linked-request")
	if err := os.Symlink(request, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, root} {
		if err := prepareDeskRunFiles(path, filepath.Join(root, "refused")); err == nil {
			t.Fatal("special input accepted", path)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "refused")); !os.IsNotExist(err) {
		t.Fatal("refused source left output")
	}
	writable := filepath.Join(root, "shared")
	if err := os.Mkdir(writable, 0777); err != nil {
		t.Fatal(err)
	}
	os.Chmod(writable, 0777)
	if err := prepareDeskRunFiles(request, filepath.Join(writable, "drafts")); err == nil {
		t.Fatal("shared writable parent accepted")
	}
}
