//go:build mnet_enrollment_development

package osownership

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
)

// This classifier exposes fixed tokens and bounded numeric errno only. Never
// persist arbitrary line text, stack paths, panic values, addresses or env.
// Observed token is NOT a diagnosis of its cause (limits/filter/runner/etc).
type RuntimeStderrEvidence struct {
	LeadingDiagnostic *ChildDiagnostic `json:"leading_diagnostic,omitempty"`
	GenericFatal      bool             `json:"generic_fatal"`
	UnexpectedSignal  bool             `json:"unexpected_signal"`
	SignalHeader      bool             `json:"signal_header"`
	GoroutineHeaders  int              `json:"goroutine_headers"`
	UnparsedLines     int              `json:"unparsed_lines"`
	Kind              string           `json:"kind"`
	Token             string           `json:"token,omitempty"`
	Errno             int              `json:"errno,omitempty"`
	RecognizedLines   int              `json:"recognized_lines"`
	UnparsedBytes     bool             `json:"unparsed_bytes"`
	LastStrictPhase   string           `json:"last_strict_phase"`
}

func classifyRuntimeStderr(b []byte, complete bool) RuntimeStderrEvidence {
	r := RuntimeStderrEvidence{Kind: "unclassified", UnparsedBytes: len(b) > 0, LastStrictPhase: "unknown"}
	if len(b) > 8192 {
		b = b[:8192]
	}
	lines := bytes.Split(b, []byte{'\n'})
	prefixOpen := true
	phaseCount := 0
	diagnosticSeen := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(line) > 1024 {
			prefixOpen = false
			r.UnparsedLines++
			continue
		}
		if prefixOpen {
			var p phaseRecord
			d := json.NewDecoder(bytes.NewReader(line))
			d.DisallowUnknownFields()
			if !diagnosticSeen && d.Decode(&p) == nil && d.Decode(&struct{}{}) == io.EOF && p.Version == 1 && allowedPhase(p.Phase) {
				phaseCount++
				if phaseCount > 8 {
					prefixOpen = false
					r.UnparsedLines++
					continue
				}
				r.LastStrictPhase = p.Phase
				continue
			}
			var diagnostic ChildDiagnostic
			d = json.NewDecoder(bytes.NewReader(line))
			d.DisallowUnknownFields()
			if !diagnosticSeen && phaseCount < 8 && d.Decode(&diagnostic) == nil && d.Decode(&struct{}{}) == io.EOF && validChildDiagnostic(diagnostic) {
				r.LeadingDiagnostic = &diagnostic
				diagnosticSeen = true
				phaseCount++
				continue
			}
			prefixOpen = false
		}
		s := string(line)
		recognized := false
		if strings.HasPrefix(s, "fatal error: ") {
			r.GenericFatal = true
			recognized = true
			if r.Token == "" {
				r.Kind = "go-runtime-fatal"
				r.Token = "generic-fatal-prefix"
			}
		}
		if strings.HasPrefix(s, "unexpected signal") || strings.HasPrefix(s, "runtime: unexpected signal") {
			r.UnexpectedSignal = true
			recognized = true
		}
		if strings.HasPrefix(s, "[signal ") {
			r.SignalHeader = true
			recognized = true
		}
		if strings.HasPrefix(s, "goroutine ") {
			fields := strings.Fields(s)
			if len(fields) >= 3 {
				n, e := strconv.ParseUint(fields[1], 10, 64)
				if e == nil && n > 0 && strings.HasPrefix(fields[2], "[") && strings.HasSuffix(s, ":") {
					r.GoroutineHeaders++
					recognized = true
				}
			}
		}
		switch {
		case s == "fatal error: newosproc":
			r.Kind = "go-runtime-fatal"
			r.Token = "newosproc"
			recognized = true
		case strings.HasPrefix(s, "runtime: failed to create new OS thread (have ") && strings.HasSuffix(s, ")"):
			prefix := "runtime: failed to create new OS thread (have "
			mid := strings.TrimSuffix(strings.TrimPrefix(s, prefix), ")")
			parts := strings.Split(mid, " already; errno=")
			if len(parts) != 2 {
				r.UnparsedLines++
				continue
			}
			threads, e1 := strconv.Atoi(parts[0])
			errno, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil && threads >= 0 && threads <= 1000000 && errno > 0 && errno <= 4095 {
				r.Kind = "go-runtime-fatal"
				r.Token = "newosproc"
				r.Errno = errno
				recognized = true
			}
		case s == "fatal error: all goroutines are asleep - deadlock!":
			if r.Token == "" || r.Token == "generic-fatal-prefix" {
				r.Kind = "go-runtime-fatal"
				r.Token = "deadlock"
			}
			recognized = true
		case strings.HasPrefix(s, "panic: "):
			if r.Token == "" {
				r.Kind = "go-panic-token"
				r.Token = "panic-prefix"
			}
			recognized = true
		}
		if recognized {
			r.RecognizedLines++
		} else {
			r.UnparsedLines++
		}
	}
	r.UnparsedBytes = r.UnparsedLines > 0 || len(b) >= 8192
	if !complete {
		r.Kind = "incomplete-stderr"
	}
	return r
}
