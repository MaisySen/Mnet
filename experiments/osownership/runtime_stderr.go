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
	Kind            string `json:"kind"`
	Token           string `json:"token,omitempty"`
	Errno           int    `json:"errno,omitempty"`
	RecognizedLines int    `json:"recognized_lines"`
	UnparsedBytes   bool   `json:"unparsed_bytes"`
	LastStrictPhase string `json:"last_strict_phase"`
}

func classifyRuntimeStderr(b []byte, complete bool) RuntimeStderrEvidence {
	r := RuntimeStderrEvidence{Kind: "unclassified", UnparsedBytes: len(b) > 0, LastStrictPhase: "unknown"}
	if len(b) > 8192 {
		b = b[:8192]
	}
	lines := bytes.Split(b, []byte{'\n'})
	prefixOpen := true
	phaseCount := 0
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		if len(line) > 1024 {
			prefixOpen = false
			continue
		}
		if prefixOpen {
			var p phaseRecord
			d := json.NewDecoder(bytes.NewReader(line))
			d.DisallowUnknownFields()
			if d.Decode(&p) == nil && d.Decode(&struct{}{}) == io.EOF && p.Version == 1 && allowedPhase(p.Phase) {
				phaseCount++
				if phaseCount > 8 {
					prefixOpen = false
					continue
				}
				r.LastStrictPhase = p.Phase
				continue
			}
			prefixOpen = false
		}
		s := string(line)
		switch {
		case s == "fatal error: newosproc":
			r.Kind = "go-runtime-fatal"
			r.Token = "newosproc"
			r.RecognizedLines++
		case strings.HasPrefix(s, "runtime: failed to create new OS thread (have ") && strings.HasSuffix(s, ")"):
			prefix := "runtime: failed to create new OS thread (have "
			mid := strings.TrimSuffix(strings.TrimPrefix(s, prefix), ")")
			parts := strings.Split(mid, " already; errno=")
			if len(parts) != 2 {
				continue
			}
			threads, e1 := strconv.Atoi(parts[0])
			errno, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil && threads >= 0 && threads <= 1000000 && errno > 0 && errno <= 4095 {
				r.Kind = "go-runtime-fatal"
				r.Token = "newosproc"
				r.Errno = errno
				r.RecognizedLines++
			}
		case s == "fatal error: all goroutines are asleep - deadlock!":
			if r.Token == "" {
				r.Kind = "go-runtime-fatal"
				r.Token = "deadlock"
			}
			r.RecognizedLines++
		case strings.HasPrefix(s, "panic: "):
			if r.Token == "" {
				r.Kind = "go-panic-token"
				r.Token = "panic-prefix"
			}
			r.RecognizedLines++
		}
	}
	if !complete {
		r.Kind = "incomplete-stderr"
	}
	return r
}
