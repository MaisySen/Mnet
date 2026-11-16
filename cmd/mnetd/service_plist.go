package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Built on every platform so the generation boundary gets Linux race coverage,
// as well as native Darwin CLI/plutil checks. No launchd effects occur here.
func darwinServicePlist(exe string, o options) (string, error) {
	if err := validateSavedService(exe, o); err != nil {
		return "", err
	}
	// The current daemon configures exactly its requested name. Auto-assigned
	// "utun" is not safe until actual TUN names flow through configuration.
	index := strings.TrimPrefix(o.ifname, "utun")
	n, err := strconv.ParseUint(index, 10, 16)
	if !strings.HasPrefix(o.ifname, "utun") || err != nil || strconv.FormatUint(n, 10) != index {
		return "", fmt.Errorf("print-plist requires an explicit canonical utun interface name, for example -ifname utun9; availability must be checked on the host")
	}
	// validateSavedService permits no XML delimiters, quotes or control bytes.
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Generated only. Enroll first using this same private state directory.
     Review before installation; keep binary and parent directories admin-owned.
     No auth key. Running is not VPN readiness; use authenticated wait/status. -->
<plist version="1.0">
<dict>
  <key>Label</key><string>au.fintex.mnet.mnetd</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>-state</string><string>%s</string>
    <string>-ifname</string><string>%s</string>
    <string>-up</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>Umask</key><integer>63</integer>
</dict>
</plist>
`, exe, o.stateDir, o.ifname), nil
}
