//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence && mnet_os_device_preflight

package osownership

func allowedResidentEvidenceMode(mode string) bool {
	return mode == "--osownership-resident-no-tun-source" || mode == "--osownership-resident-device-open-preflight" || mode == endpointResidentMode
}
