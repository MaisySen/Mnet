package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

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
func validDeskArgs(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[0] {
	case "prepare":
		return len(args) == 3
	case "layout-check":
		return true
	case "layout-view":
		return len(args) == 2
	default:
		return false
	}
}
func runDesk(args []string) {
	if !validDeskArgs(args) {
		fatal("usage: mnet desk prepare REQUEST_FILE NEW_PRIVATE_DIR | mnet desk layout-check FILE [--edge DISPLAY EDGE PIXEL] | mnet desk layout-view FILE")
		return
	}
	if args[0] == "prepare" {
		if err := prepareDeskRunFiles(args[1], args[2]); err != nil {
			fatal(err.Error())
			return
		}
		fmt.Println("Drafts written. Review REVIEW.txt and layout.json; no approval, listener or input was created.")
		return
	}
	if args[0] == "layout-view" {
		preview, err := readDeskLayoutPreviewFile(args[1])
		if err != nil {
			fatal(err.Error())
			return
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err = showDeskLayoutPreview(ctx, preview); err != nil {
			fatal(err.Error())
		}
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
