package desk

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
)

// RunSpec is an explicit one-shot engineering invocation, not enrollment or a
// stored grant. Certificate/private key paths are local and never sent on wire.
// The same independently accepted Binding and Layout must reach both people.
type RunSpec struct {
	Version         int     `json:"version"`
	Role            string  `json:"role"`
	Address         string  `json:"address"`
	Certificate     string  `json:"certificate_file"`
	PrivateKey      string  `json:"private_key_file"`
	RunDirectory    string  `json:"run_directory"`
	LifetimeSeconds int     `json:"lifetime_seconds"`
	SourceDisplay   string  `json:"source_display"`
	Edge            Edge    `json:"edge"`
	Binding         Binding `json:"binding"`
	Layout          Layout  `json:"layout"`
}

func DecodeRun(r io.Reader) (RunSpec, *Compiled, error) {
	var spec RunSpec
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil || len(data) > 65536 {
		return spec, nil, errors.New("desk run document unreadable or exceeds64KiB")
	}
	if err = validateJSON(data); err != nil {
		return spec, nil, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&spec); err != nil {
		return spec, nil, errors.New("desk run schema invalid")
	}
	layout, err := spec.Validate()
	return spec, layout, err
}
func (s RunSpec) Validate() (*Compiled, error) {
	if s.Version != 1 || (s.Role != "primary" && s.Role != "recipient") || s.LifetimeSeconds < 1 || s.LifetimeSeconds > 600 || s.Certificate == "" || s.PrivateKey == "" || s.RunDirectory == "" {
		return nil, errors.New("desk run explicit role, lifetime1to600seconds and local identity/replay paths required")
	}
	host, port, err := net.SplitHostPort(s.Address)
	p, pe := strconv.Atoi(port)
	ip := net.ParseIP(host)
	if err != nil || pe != nil || p < 1 || p > 65535 || ip == nil || (!ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()) {
		return nil, errors.New("desk run requires an explicit unicast IP and nonzero port; no DNS or wildcard listener")
	}
	if len(s.Layout.Displays) != 2 {
		return nil, errors.New("desk engineering launcher requires exactly one source and one recipient display")
	}
	layout, err := Compile(s.Layout)
	if err != nil {
		return nil, err
	}
	if err = s.Binding.Validate(layout); err != nil {
		return nil, err
	}
	if !digest(s.Binding.Primary) || !digest(s.Binding.Recipient) {
		return nil, errors.New("desk run exact public-key pins required")
	}
	if s.Edge != Left && s.Edge != Right && s.Edge != Top && s.Edge != Bottom {
		return nil, errors.New("desk run exact source edge required")
	}
	var source Display
	for _, d := range s.Layout.Displays {
		if d.ID == s.SourceDisplay && d.Device == s.Binding.Primary {
			source = d
		}
	}
	if source.ID == "" {
		return nil, errors.New("desk run source absent")
	}
	// This launcher enables only one pair and a single destination on that edge.
	// General offline layouts may describe more, but cannot widen this grant.
	found := false
	for _, link := range layout.Links() {
		if link.From == source.ID && link.Edge == s.Edge {
			if link.To != s.Binding.Display {
				return nil, errors.New("desk run edge names another destination")
			}
			found = true
		}
	}
	if !found {
		return nil, errors.New("desk run edge not connected to exact recipient")
	}
	return layout, nil
}
