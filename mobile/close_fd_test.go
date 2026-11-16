//go:build linux

package mobile

import (
	"os"
	"syscall"
	"testing"
)

func TestClosedMobileStartConsumesFD(t *testing.T) {
	n, err := NewNode(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	fd, err := syscall.Dup(int(w.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Start(fd, "http://127.0.0.1:1", "fixture", "node", ""); err == nil {
		t.Fatal("closed mobile Start accepted")
	}
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != syscall.EBADF {
		syscall.Close(fd)
		t.Fatal("rejected mobile Start leaked consumed fd", err)
	}
	if _, err := n.Prepare("http://127.0.0.1:1", "fixture", "node", ""); err == nil {
		t.Fatal("closed mobile Prepare revived")
	}
}
