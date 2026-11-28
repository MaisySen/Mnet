// Package handoffview renders the local browser's Mnet handoff and setup results.
package handoffview

import (
	"html/template"
	"io"
	"net/http"
)

const contentPolicy = "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

var page = template.Must(template.New("handoff").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<title>{{.Title}} · Mnet</title>
<style>
:root{color-scheme:light dark;background:#f5f5f7;color:#1d1d1f;font:16px system-ui,sans-serif}
body{min-height:100vh;box-sizing:border-box;margin:0;display:grid;place-items:center;padding:24px}
main{box-sizing:border-box;width:min(100%,30rem);background:#fff;border:1px solid #d2d2d7;border-radius:16px;padding:clamp(22px,6vw,36px)}
header{display:flex;align-items:center;gap:10px;font-size:1.125rem;font-weight:650}
.label{display:block;margin-top:22px;font-size:.875rem;color:#5a5a5f}
h1{font-size:clamp(1.5rem,4vw,2rem);line-height:1.2;margin:22px 0 12px}
.label+h1{margin-top:8px}
p{margin:0;line-height:1.55;color:#5a5a5f}
footer{margin-top:24px;font-size:.875rem;color:#5a5a5f}
@media(prefers-color-scheme:dark){:root{background:#151517;color:#f5f5f7}main{background:#222225;border-color:#454549}p,footer,.label{color:#bcbcc2}}
</style></head><body><main>
<header><svg aria-hidden="true" width="30" height="30" viewBox="0 0 100 100"><g transform="translate(0 0.75)"><polyline points="20,79 20,39 50,67 80,39 80,79" fill="none" stroke="currentColor" stroke-width="21" stroke-linecap="round" stroke-linejoin="round"/><circle cx="50" cy="20" r="11" fill="#FF6A1A"/></g></svg>Mnet</header>
{{with .Label}}<small class="label">{{.}}</small>{{end}}
<h1>{{.Title}}</h1><p>{{.Message}}</p><footer>You can close this tab.</footer>
</main></body></html>`))

// Write sends a self-contained page without including any callback parameters.
func Write(w http.ResponseWriter, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", contentPolicy)
	_ = render(w, "", title, message)
}

// WriteSetupResult writes a local result file for the disposable setup app.
func WriteSetupResult(w io.Writer, title, message string) error {
	return render(w, "Disposable development setup", title, message)
}

func render(w io.Writer, label, title, message string) error {
	return page.Execute(w, struct{ Label, Title, Message string }{label, title, message})
}
