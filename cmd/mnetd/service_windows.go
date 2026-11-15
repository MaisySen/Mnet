//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// Windows service mode, the tailscaled pattern: mnetd -service
// install registers the daemon with the Service Control Manager
// (autostart, runs as LocalSystem so it can manage the tunnel
// interface), mnetd -service remove takes it back out. The SCM then
// starts the binary with -service run; a service session with no
// flags is detected too and routed the same way.
//
// Native Windows CI exercises install/start/stop/restart/crash recovery/
// removal with an offline fixture. Wintun and physical-host VPN service
// validation remain pending.

const (
	serviceName = "mnetd"
	serviceDesc = "Mnet mesh node daemon. Joins this machine to its Mnet network and serves the local mnet CLI."
)

func isServiceSession() bool {
	isSvc, err := svc.IsWindowsService()
	return err == nil && isSvc
}

func runServiceAction(o options) error {
	switch o.serviceAction {
	case "install":
		return installService(o)
	case "remove":
		return removeService()
	case "run":
		return runAsService(o)
	default:
		return fmt.Errorf("unknown -service action %q (install, remove, run)", o.serviceAction)
	}
}

// installService registers mnetd with the SCM. The service starts
// the binary with -service run plus the state directory and -up so
// the node rejoins from its saved state on every boot.
func installService(o options) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	if !filepath.IsAbs(o.stateDir) {
		o.stateDir, err = filepath.Abs(o.stateDir)
		if err != nil {
			return fmt.Errorf("absolute state path: %w", err)
		}
	}
	if o.authKey != "" {
		return fmt.Errorf("initialize saved node state before installing; auth keys must not be stored in service command lines")
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()
	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: "Mnet node daemon",
		Description: serviceDesc,
		StartType:   mgr.StartAutomatic,
	}, "-service", "run", "-state", o.stateDir, "-ifname", o.ifname, "-up")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}}, 86400); err != nil {
		fmt.Fprintf(os.Stderr, "mnetd: service recovery not configured: %v\n", err)
	}
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		// The service itself is registered; event source
		// registration failure is worth saying, not failing over.
		fmt.Fprintf(os.Stderr, "mnetd: event log source not registered: %v\n", err)
	}
	fmt.Printf("mnetd: service %q installed (autostart, state %s)\n", serviceName, o.stateDir)
	return nil
}

func removeService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("service manager (run as Administrator): %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()
	// Best effort stop; it is fine if it was not running.
	_, _ = s.Control(svc.Stop)
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	if err := eventlog.Remove(serviceName); err != nil {
		fmt.Fprintf(os.Stderr, "mnetd: event log source not removed: %v\n", err)
	}
	fmt.Printf("mnetd: service %q removed\n", serviceName)
	return nil
}

// serviceHandler adapts runDaemon to the SCM control channel.
type serviceHandler struct {
	opts options
	elog *eventlog.Log
}

func runAsService(o options) error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		// Without a registered source we still serve; logs fall
		// back to the SCM's own bookkeeping.
		elog = nil
	}
	h := &serviceHandler{opts: o, elog: elog}
	err = svc.Run(serviceName, h)
	if elog != nil {
		defer elog.Close()
	}
	if err != nil {
		return fmt.Errorf("service run: %w", err)
	}
	return nil
}

func (h *serviceHandler) logf(id uint32, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if h.elog != nil {
		_ = h.elog.Info(id, msg)
	}
	log.Print(msg)
}

func (h *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	s <- svc.Status{State: svc.StartPending}

	stop := make(chan struct{})
	ready := make(chan error, 1)
	h.opts.ready = ready
	errCh := make(chan error, 1)
	go func() { errCh <- runDaemon(h.opts, stop) }()

	select {
	case <-ready:
		s <- svc.Status{State: svc.Running, Accepts: accepted}
		h.logf(1, "service started; local API bound")
	case err := <-errCh:
		h.logf(2, "service startup failed: %v", err)
		return true, 1
	}

	loop := true
	for loop {
		select {
		case err := <-errCh:
			if err != nil {
				h.logf(2, "daemon exited with error: %v", err)
				s <- svc.Status{State: svc.Stopped}
				return true, 1
			}
			loop = false
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				close(stop)
				select {
				case <-errCh:
				case <-time.After(20 * time.Second):
					// Do not hang the SCM on a wedged shutdown.
				}
				h.logf(3, "service stopped")
				loop = false
			}
		}
	}
	s <- svc.Status{State: svc.Stopped}
	return false, 0
}
