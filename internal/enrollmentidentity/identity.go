// Package enrollmentidentity defines exact operator identity syntax shared by
// inactive enrollment contracts. Syntax never authenticates a service or person.
package enrollmentidentity

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidOrigin requires an explicitly spelled HTTPS origin, without a path.
// Explicit default ports remain distinct; no normalization or resolution occurs.
func ValidOrigin(s string) bool { return endpoint(s, true) }

// ValidIssuer permits the exact issuer path as part of its immutable identity.
func ValidIssuer(s string) bool { return endpoint(s, false) }
func endpoint(s string, origin bool) bool {
	if s == "" || len(s) > 2048 || !utf8.ValidString(s) || strings.ContainsAny(s, "?#") || strings.HasSuffix(s, ":") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	if u.Port() != "" {
		n, e := strconv.Atoi(u.Port())
		if e != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if origin && (u.Path != "" || u.RawPath != "") {
		return false
	}
	return u.String() == s
}
