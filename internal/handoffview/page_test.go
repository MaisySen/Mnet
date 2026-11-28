package handoffview

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteRendersSelfContainedEscapedCallback(t *testing.T) {
	w := httptest.NewRecorder()
	Write(w, "Approval received", "Check result in <Mnet>.")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("callback page lost its response or privacy headers")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("callback page should block external resources")
	}
	if body := w.Body.String(); !strings.Contains(body, "<h1>Approval received</h1>") || !strings.Contains(body, "&lt;Mnet&gt;") {
		t.Fatal("callback page did not render escaped content")
	}
}

func TestSetupResultUsesSameBrandWithDevelopmentLabel(t *testing.T) {
	var page strings.Builder
	if err := WriteSetupResult(&page, "Joining saved. Not connected.", "Setup result <only>."); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Disposable development setup", "<h1>Joining saved. Not connected.</h1>", "Setup result &lt;only&gt;.", "Content-Security-Policy", "aria-hidden=\"true\""} {
		if !strings.Contains(page.String(), want) {
			t.Errorf("missing setup page content %q", want)
		}
	}
}
