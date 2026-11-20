//go:build linux

package main

import (
	"errors"
	"fmt"
	"strings"

	"fintex.au/mnet/internal/desk"
)

// This finite native review supports the same single-source/single-recipient
// arrangement as the launcher. Do not truncate extra displays into an apparently
// complete review or render untrusted descriptive labels as verified identity.
func deskArrangementReview(binding desk.Binding, layout *desk.Compiled) (string, error) {
	if err := binding.Validate(layout); err != nil {
		return "", err
	}
	displays := layout.Displays()
	if len(displays) != 2 {
		return "", errors.New("native desk review requires exactly two displays; no truncated arrangement")
	}
	var source, target desk.Display
	for _, d := range displays {
		switch {
		case d.Device == binding.Primary:
			source = d
		case d.Device == binding.Recipient && d.ID == binding.Display:
			target = d
		default:
			return "", errors.New("native desk review display role mismatch")
		}
	}
	if source.ID == "" || target.ID == "" {
		return "", errors.New("native desk review requires one source and one exact recipient display")
	}
	var edges []string
	for _, link := range layout.Links() {
		if link.From == source.ID && link.To == target.ID {
			edges = append(edges, fmt.Sprintf("%s edge, shared layout span [%d,%d)", link.Edge, link.Start, link.End))
		}
	}
	if len(edges) != 1 {
		return "", errors.New("native desk review needs one exact shared source edge")
	}
	return fmt.Sprintf("Exact arrangement, not an editable preview:\nPrimary %s: layout (%d,%d) %dx%d; native %dx%d px\nRecipient %s: layout (%d,%d) %dx%d; native %dx%d px\nPrimary to recipient: %s\nLayout units are not native pixels. Empty gaps/corners do not switch.", source.ID, source.X, source.Y, source.Width, source.Height, source.PixelWidth, source.PixelHeight, target.ID, target.X, target.Y, target.Width, target.Height, target.PixelWidth, target.PixelHeight, strings.Join(edges, "")), nil
}
