//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_process_launch

package osownership

import (
	"encoding/json"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"reflect"
	"strconv"
	"strings"
)

func numericCurrentNamespace() (Namespace, error) {
	f, e := os.Open("/proc/thread-self/ns/net")
	if e != nil {
		return Namespace{}, e
	}
	defer f.Close()
	var s unix.Stat_t
	if e = unix.Fstat(int(f.Fd()), &s); e != nil {
		return Namespace{}, e
	}
	return Namespace{Device: uint64(s.Dev), Inode: s.Ino}, nil
}
func emitClientBootstrapEvidence() error {
	if e := emitEvidencePhase("final-entry"); e != nil {
		return e
	}
	// FIRST final-client evidence operation: raw fd enumeration before any
	// os.ReadFile/os.Open/JSON stdout can initialize runtime poll descriptors.
	fds, e := clientFDInventorySource()
	if e != nil {
		return opFailure("client-fd-inventory", "runner", e)
	}
	if e = validateFDAllowlist(fds); e != nil {
		_ = json.NewEncoder(os.Stdout).Encode(ClientBootstrapEvidence{FailureClass: "harness-fd-inventory", FDs: fds})
		return e
	}
	if e := emitEvidencePhase("fd-validated"); e != nil {
		return e
	}
	b, e := rawReadBounded("/proc/self/status", 16384)
	if e != nil {
		return opFailure("client-status", "runner", e)
	}
	if e = validateClientStatus(string(b)); e != nil {
		return e
	}
	status := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb", "NoNewPrivs", "Seccomp", "Seccomp_filters":
			status[kv[0]] = strings.TrimSpace(kv[1])
		}
	}
	ns, e := rawCurrentNamespace()
	if e != nil {
		return opFailure("client-namespace", "runner", e)
	}
	finalFDs, e := clientFDInventorySource()
	if e != nil {
		return e
	}
	if e = validateFDAllowlist(finalFDs); e != nil {
		return e
	}
	if !reflect.DeepEqual(fds, finalFDs) {
		return &FDInventoryHarnessError{}
	}
	filterBaseline, e := strconv.Atoi(os.Getenv("MNO_CLIENT_FILTER_BASELINE"))
	if e != nil || filterBaseline < 0 {
		return opFailure("client-filter-count", "runner", ErrConflict)
	}
	out := ClientBootstrapEvidence{FilterBaseline: filterBaseline, PID: os.Getpid(), ParentPID: os.Getppid(), UID: os.Getuid(), Status: status, FDs: fds, Namespace: ns, ProbeDenied: map[string]int{}}
	// Signal0 cannot deliver any signal. EPERM proves this specific real filter
	// path refuses owner probing; no crash/termination signal is used. No unshare,
	// setns, namespace creation or network mutation probes are attempted.
	e = unix.Kill(os.Getppid(), 0)
	if e != unix.EPERM {
		return opFailure("owner-signal0", "custody", errors.New("owner signal0 unexpectedly permitted"))
	}
	out.ProbeDenied["owner-signal0"] = int(unix.EPERM)
	return opFailure("client-output", "runner", json.NewEncoder(os.Stdout).Encode(out))
}
func rawCurrentNamespace() (Namespace, error) {
	fd, e := unix.Open("/proc/thread-self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if e != nil {
		return Namespace{}, e
	}
	defer unix.Close(fd)
	var s unix.Stat_t
	if e = unix.Fstat(fd, &s); e != nil {
		return Namespace{}, e
	}
	return Namespace{Device: uint64(s.Dev), Inode: s.Ino}, nil
}
