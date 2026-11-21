//go:build linux && mnet_enrollment_development

package osownership

type ownedSetupDump struct {
	residentNetlink
	handle Handle
}

func (d ownedSetupDump) decodeOwnedSetupLink(b []byte) (LinkRecord, error) {
	return decodeSetupLink(b, d.handle)
}

// Before the creation handle has an ifindex, the exclusive tracked creation
// reads the only candidate TUN by exact preauthorized name, never adopts it.
// Original OS verification/ledger retention still follow before phase admission.
type creationSetupDump struct {
	residentNetlink
	name string
}

func (d creationSetupDump) decodeOwnedSetupLink(b []byte) (LinkRecord, error) {
	if len(b) < 16 {
		return LinkRecord{}, afShape
	}
	name, e := setupSelectionName(b)
	if e != nil {
		return LinkRecord{}, e
	}
	if name != d.name {
		return DecodeLink(b)
	}
	r, e := parseSetupOuter(b)
	if e != nil {
		return LinkRecord{}, e
	}
	if r.Name != d.name {
		return LinkRecord{}, afPhase
	}
	r.setupOuter = &setupOuterRecord{body: append([]byte(nil), b...)}
	return r, nil
}
