//go:build mnet_enrollment_development

package osownership

// Raw source output stays diagnostic. No Inventory/Ready promotion method.
type ownedSourceDiagnostic struct {
	raw           RawInventory
	sys           SysctlSnapshot
	framing       tunNestedFraming
	effective     []effectiveV4Word
	lineageUsable bool
}

func sourceDiagnosticFromRecords(raw RawInventory, sys SysctlSnapshot, h Handle, usable bool) (ownedSourceDiagnostic, error) {
	var zero ownedSourceDiagnostic
	observed, e := parseEffectiveV4(sys)
	if e != nil {
		return zero, e
	}
	var owned *LinkRecord
	for _, l := range raw.Links {
		if l.Index == h.Link.Index || l.Name == h.Link.Name || l.Alias == h.Link.Alias {
			if owned != nil || l.Index != h.Link.Index || l.Name != h.Link.Name || l.Alias != h.Link.Alias || l.MTU != h.Link.MTU || l.Hardware != 65534 || l.Flags & ^uint32(1|0x40|0x10000) != 0x1090 {
				return zero, ErrConflict
			}
			copy := l
			owned = &copy
		}
	}
	if owned == nil {
		return zero, ErrConflict
	}
	framing, e := parseTUNNested(*owned)
	if e != nil {
		return zero, e
	}
	values, e := effectiveV4Link(*owned, observed)
	if e != nil {
		return zero, e
	}
	clone := SysctlSnapshot{}
	for k, v := range sys {
		clone[k] = v
	}
	return ownedSourceDiagnostic{raw: cloneRawExact(raw), sys: clone, framing: framing, effective: values, lineageUsable: usable}, nil
}
