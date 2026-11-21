package enrollmentcontract

import (
	"crypto/sha256"
	"encoding/base64"
)

// SourceDescriptorBytes reproduces Multies reserveDescriptorBytes version 1
// using the separately selected service/reserve and own-PKCE response DATA.
// Ticket is intentionally excluded. It authenticates nothing and creates no
// reserve. This is JavaScript JSON.stringify string-array encoding, NOT Go's
// HTML-escaped JSON or the separate length-prefixed possession transcript.
func SourceDescriptorBytes(own SourceExchange, serviceID, reserveID string) ([]byte, error) {
	return sourceDescriptorBytes(own.ID, own.Launch, own.Mapping, serviceID, reserveID)
}
func sourceDescriptorBytes(handoffID string, l SourceLaunch, m SourceMapping, serviceID, reserveID string) ([]byte, error) {
	if !validSourceLaunch(l) || !validSourceMapping(m) || !sourceID.MatchString(serviceID) {
		return nil, ErrInvalid
	}
	for _, s := range []string{handoffID, reserveID} {
		if _, err := digest(s); err != nil {
			return nil, err
		}
	}
	prefix := []string{"mnet-reserve-descriptor-v1", serviceID, handoffID, reserveID}
	suffix := []string{l.Purpose, l.AttemptID, l.DevicePublicKey, l.CredentialDigest, l.Challenge, l.State, l.RedirectURI, l.Platform, m.WorkspaceID, m.Issuer, m.RegistrarOrigin, m.Audience, m.MappingRevision, m.NetworkID}
	out := []byte{'['}
	for _, s := range prefix {
		if len(out) > 1 {
			out = append(out, ',')
		}
		out = appendJSONString(out, s)
	}
	out = append(out, ',', '1')
	for _, s := range suffix {
		out = append(out, ',')
		out = appendJSONString(out, s)
	}
	return append(out, ']'), nil
}
func SourceDescriptorDigest(own SourceExchange, serviceID, reserveID string) (string, error) {
	raw, err := SourceDescriptorBytes(own, serviceID, reserveID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// All input strings have already passed UTF8/shape validators. JSON.stringify
// emits valid non-control Unicode directly, including HTML-sensitive runes.
func appendJSONString(out []byte, s string) []byte {
	const hex = "0123456789abcdef"
	out = append(out, '"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			out = append(out, '\\', byte(r))
		case '\b':
			out = append(out, '\\', 'b')
		case '\f':
			out = append(out, '\\', 'f')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			if r < 0x20 {
				out = append(out, '\\', 'u', '0', '0', hex[byte(r)>>4], hex[byte(r)&15])
			} else {
				out = append(out, string(r)...)
			}
		}
	}
	return append(out, '"')
}
