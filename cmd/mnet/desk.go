package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"

	"fintex.au/mnet/internal/desk"
)

type deskLayoutReport struct {
	OfflineOnly bool            `json:"offline_only"`
	Active      bool            `json:"active"`
	Displays    int             `json:"displays"`
	Links       []desk.Link     `json:"links"`
	Candidate   *desk.Candidate `json:"candidate,omitempty"`
	EdgeMatched *bool           `json:"edge_matched,omitempty"`
}

// checkDeskLayout is deliberately read-only. IDs from this document are layout
// labels, never current paired-device/key/workspace evidence or active state.
func checkDeskLayout(r io.Reader, query []string) (deskLayoutReport, error) {
	report := deskLayoutReport{OfflineOnly: true}
	if len(query) != 0 && (len(query) != 4 || query[0] != "--edge") {
		return report, errors.New("usage: mnet desk layout-check FILE [--edge DISPLAY EDGE PIXEL]")
	}
	l, err := desk.Decode(r)
	if err != nil {
		return report, err
	}
	c, err := desk.Compile(l)
	if err != nil {
		return report, err
	}
	report.Displays = len(l.Displays)
	report.Links = c.Links()
	if len(query) != 0 {
		along, err := strconv.ParseInt(query[3], 10, 64)
		if err != nil {
			return report, errors.New("edge pixel must be a bounded integer")
		}
		candidate, ok, err := c.MapEdge(query[1], desk.Edge(query[2]), along)
		if err != nil {
			return report, err
		}
		report.EdgeMatched = &ok
		if ok {
			report.Candidate = &candidate
		}
	}
	return report, nil
}
func runDesk(args []string) {
	if len(args) < 2 || args[0] != "layout-check" {
		fatal("usage: mnet desk layout-check FILE [--edge DISPLAY EDGE PIXEL]")
		return
	}
	f, err := os.Open(args[1])
	if err != nil {
		fatal("layout file unavailable")
		return
	}
	defer f.Close()
	report, err := checkDeskLayout(f, args[2:])
	if err != nil {
		fatal(err.Error())
		return
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	must(encoder.Encode(report))
}
