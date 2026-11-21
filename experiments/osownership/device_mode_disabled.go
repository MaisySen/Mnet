//go:build mnet_enrollment_development && (!mnet_os_device_preflight || !mnet_os_readonly_evidence)

package osownership

func allowedResidentEvidenceMode(mode string) bool {
	return mode == "--osownership-resident-no-tun-source"
}
