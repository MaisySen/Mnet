//go:build linux && mnet_enrollment_development

package osownership

import (
	"strings"
	"testing"
)

func TestOriginalOffloadHistorySourceCallOrdering(t *testing.T) {
	s := sourceFunction(t, "kernel_linux.go", "openExclusiveNonpersistentTUN")
	create := strings.Index(s, "unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr)")
	offload := strings.Index(s, "unix.IoctlSetInt(fd, unix.TUNSETOFFLOAD, 0)")
	history := strings.Index(s, "attempt.offloadHistory = &originalOffloadHistory{")
	if create < 0 || offload <= create || history <= offload || !strings.Contains(s[offload:history], "return e") || strings.Contains(s, "IoctlSetPointerInt") {
		t.Fatal("history not after integer-form success")
	}
	if !strings.Contains(sourceFunction(t, "runtime_custody_source_live_linux.go", "validateCompletion"), "offloadHistory.matches(") {
		t.Fatal("actual collector omitted original history")
	}
}
func TestOriginalOffloadHistoryRejectsForeignOriginal(t *testing.T) {
	for _, fault := range []string{"valid", "attempt", "custody", "namespace", "name", "request", "argument", "create", "close", "replaced-record", "nil"} {
		t.Run(fault, func(t *testing.T) {
			a := &originalAttempt{custody: &fileCustody{}, createRequest: 0x9001}
			ns := Namespace{Device: 7, Inode: 8, Isolated: true}
			h := &originalOffloadHistory{attempt: a, custody: a.custody, namespace: ns, name: "mno-own", request: 0x400454d0}
			a.offloadHistory = h
			switch fault {
			case "attempt":
				h.attempt = &originalAttempt{}
			case "custody":
				h.custody = &fileCustody{}
			case "namespace":
				h.namespace.Inode++
			case "name":
				h.name = "mno-other"
			case "request":
				h.request++
			case "argument":
				h.argument = 1
			case "create":
				a.createRequest = 0x1001
			case "close":
				a.closeAttempted = true
			case "replaced-record":
				a.offloadHistory = &originalOffloadHistory{}
			case "nil":
				h = nil
			}
			if got := h.matches(a, a.custody, ns, "mno-own"); got != (fault == "valid") {
				t.Fatal("history binding", fault, got)
			}
		})
	}
}
