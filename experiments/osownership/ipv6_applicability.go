//go:build mnet_enrollment_development

package osownership

import (
	"errors"
	"fmt"
)

// AF_SPEC family records are framed attributes, not a name/kind heuristic.
// Absence of family10 is no inet6-device state, never 'IPv6 disabled'.
func LinkIPv6Applicable(l LinkRecord) (bool, error) {
	b, ok := l.Raw[26]
	if !ok {
		return false, errors.New("missing AF_SPEC applicability")
	}
	a, e := decodeAttributes(b)
	if e != nil {
		return false, e
	}
	for k := range a {
		if k != 2 && k != 10 {
			return false, fmt.Errorf("unreviewed AF_SPEC family %d", k)
		}
	}
	if _, ok = a[2]; !ok {
		return false, errors.New("missing IPv4 AF_SPEC")
	}
	for _, nested := range a {
		if _, e = decodeAttributes(nested); e != nil {
			return false, e
		}
	}
	_, yes := a[10]
	return yes, nil
}

const ipv6DirPrefix = "@ipv6-directory/"

func validateIPv6Directory(name string, applicable bool, s SysctlSnapshot) error {
	key := ipv6DirPrefix + name
	status, ok := s[key]
	if !ok {
		return errors.New("IPv6 directory evidence missing")
	}
	if applicable && status != "present" || !applicable && status != "ENOENT" {
		return errors.New("AF_SPEC/directory mismatch")
	}
	for _, leaf := range []string{"forwarding", "disable_ipv6", "addr_gen_mode"} {
		k := "/proc/sys/net/ipv6/conf/" + name + "/" + leaf
		v, present := s[k]
		if !applicable {
			if present {
				return errors.New("IPv6 leaf present without AF_SPEC")
			}
			continue
		}
		if !present {
			return errors.New("IPv6 leaf missing")
		}
		if leaf == "forwarding" && v != "0" {
			return errors.New("IPv6 forwarding enabled")
		}
		if leaf == "disable_ipv6" && v != "0" && v != "1" {
			return errors.New("invalid disable_ipv6")
		}
		if leaf == "addr_gen_mode" && v != "0" && v != "1" && v != "2" && v != "3" {
			return errors.New("invalid addr_gen_mode")
		}
	}
	return nil
}
