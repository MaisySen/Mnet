//go:build mnet_enrollment_development && !(linux || darwin || freebsd || openbsd || netbsd || dragonfly)

package stateowner

import "fmt"

func AcquireExistingDevelopment(path string) (*Lease, error) {
	return nil, fmt.Errorf("existing-only development inspection is unavailable on this platform")
}
