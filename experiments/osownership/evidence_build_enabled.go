//go:build linux && mnet_enrollment_development && mnet_os_readonly_evidence

package osownership

// Explicit standalone evidence build, never included in product builds. This
// mechanically permits only fresh namespace/read-only dump and fixed bootstrap
// paths. It leaves every mutation/TUN guard false. Build flag is not authority.
const readOnlyEvidenceBuild = true
