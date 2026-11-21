package enrollmentidentity

import "testing"

func TestExactIdentitySyntax(t *testing.T) {
	for _, s := range []string{"https://identity.test", "https://identity.test:443", "https://identity.test/issuer", "https://identity.test/path%2Fid"} {
		if !ValidIssuer(s) {
			t.Fatal("exact issuer refused")
		}
	}
	if ValidOrigin("https://identity.test/issuer") || ValidOrigin("https://identity.test/") {
		t.Fatal("origin path accepted")
	}
	for _, s := range []string{"http://127.0.0.1:1234", "https://user:pw@identity.test", "https://identity.test?", "https://identity.test#", "https://identity.test:", "https://identity.test:65536", "https://identity.test\n", string([]byte{0xff})} {
		if ValidOrigin(s) || ValidIssuer(s) {
			t.Fatal("invalid identity accepted")
		}
	}
}
