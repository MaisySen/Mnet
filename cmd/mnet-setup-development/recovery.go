//go:build mnet_enrollment_development

package main

import (
	"context"
	"fintex.au/mnet/internal/daemon"
	"html/template"
	"os"
)

func writeDevelopmentRecoveryPage(root string, i daemon.DevelopmentRecoveryInspection, r *daemon.DevelopmentRecoveryResult) error {
	title := "Retained attempt inspected. Not connected."
	body := "This is local historical inspection only. It did not contact a service, recover authority or start a network. A lost browser listener cannot be resumed from an old page."
	if i.JoiningSaved {
		title = "Joining saved. Not connected."
		body = "The original local joining result is saved. Its presence does not verify a current network or authorize another connection."
	}
	if r != nil && r.HistoricalRecovered {
		title = "Joining history recovered. Not connected."
		body = "An authenticated lookup recovered the same original attempt's historical receipt. This is not a new approval, proof confirmation, saved joining credential or live network."
		if r.JoiningFinalized {
			title = "Original joining finalized. Not connected."
			body = "The existing prepared attempt was finalized within its original approved interval. This did not start or verify a network."
		}
	}
	if i.BookkeepingPending {
		title = "Joining saved. Bookkeeping pending. Not connected."
		body = "The exact original joining document is already saved, but its local completion bookkeeping is pending. Inspection did not repair it or verify a current network."
	}
	if r != nil && r.BookkeepingCompleted {
		title = "Original bookkeeping completed. Not connected."
		body = "Only completion bookkeeping for the exact already-saved joining document was finished. The original State, key, receipt and expiry were unchanged. No service was contacted and no network was verified or started."
	}
	switch i.LocalConnectionPhase {
	case "setup-reserved":
		body += " The local development setup slice was reserved but no ready result was confirmed. Inspection only, no retry or connection is allowed."
	case "ready":
		body += " This new-setup-only local development workflow has one unspent connection slice. Limits are total 16, setup reserves 2, later one-shot 14, not receipt quota. Explicit --connect-existing-once is required and only works within the unchanged original interval."
	case "connection-reserved":
		body += " The entire 14-request local development connection slice was spent. This does not prove any network started or stopped. No retry, refund or crash resume is allowed."
	}
	if i.OriginalLeaseEnded {
		body += " The original approved interval has ended. There are no network controls."
	}
	file, err := os.CreateTemp("", "mnet-retained-result-*.html")
	if err != nil {
		return err
	}
	name := file.Name()
	// A separate temporary display file, never a write inside inspected State/root.
	page := template.Must(template.New("recovery").Parse(`<!doctype html><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><title>Mnet retained attempt</title><style>body{font:17px system-ui;background:#f4f5f7;color:#202124;padding:48px 20px}main{max-width:640px;margin:auto;background:white;border:1px solid #ddd;border-radius:18px;padding:30px}p{line-height:1.6}small{color:#596171}</style><main><small>DISPOSABLE DEVELOPMENT RECOVERY</small><h1>{{.Title}}</h1><p>{{.Body}}</p><p>Retained phase: {{.Phase}}</p><p>Screen, keyboard and mouse access are unavailable. No automatic retry, reconnect, new joining attempt or service was started.</p><small>Historical result only. You can close this tab.</small></main>`))
	err = page.Execute(file, struct{ Title, Body, Phase string }{title, body, i.Phase})
	closeErr := file.Close()
	if err != nil {
		os.Remove(name)
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return runDevelopmentBrowser(context.Background(), name, nil)
}
