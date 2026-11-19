package control

import (
	"fintex.au/mnet/internal/version"
	"fmt"
	"html"
	"net/http"
	"net/url"
)

// handleHealthz is the unauthenticated liveness probe for hosting platforms.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Mnet-Revision", version.Revision)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, landingHTML)
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

// handleApprovePage is the link a client prints while it waits for approval.
// It sends the admin to the dashboard with the pending node highlighted.
func (s *Server) handleApprovePage(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Query().Get("node_id")
	id := html.EscapeString(rawID)
	target := url.URL{Path: "/admin"}
	if rawID == "" {
		id = "Not supplied"
	} else {
		target.Fragment = "pending-" + rawID
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, approveHTML, id, html.EscapeString(target.String()))
}

const landingHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mnet</title>
<style>
:root{font:100% -apple-system,BlinkMacSystemFont,"SF Pro Text","Segoe UI",Roboto,Inter,system-ui,sans-serif;color:#1d1d1f;background:#f5f5f7;color-scheme:light}*{box-sizing:border-box}h1,h2,p,label{overflow-wrap:anywhere}input,select{min-width:0}body{margin:0;padding:32px 24px}.shell{max-width:1040px;margin:auto}header{display:flex;flex-wrap:wrap;gap:16px;align-items:center;justify-content:space-between;padding-bottom:28px;border-bottom:1px solid rgb(0 0 0 / 0.09)}.brand{font-size:1.625rem;font-weight:700;letter-spacing:-.03em}header span{overflow-wrap:anywhere;color:#5a5a5f;font-size:0.875rem}.hero{padding:64px 0 36px;max-width:760px}.eyebrow{font-size:0.75rem;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:#0066cc}h1{font-size:3.25rem;line-height:1.08;letter-spacing:-.045em;margin:16px 0 24px}h2{font-size:1.562rem;letter-spacing:-.025em;margin:8px 0 16px}p{line-height:1.65;color:#5a5a5f}a{color:#0066cc;text-underline-offset:4px}.intro{font-size:1.188rem;max-width:660px}.routes{display:grid;grid-template-columns:1fr 1fr;gap:20px}.card{padding:28px;background:white;border:1px solid rgb(0 0 0 / 0.09);border-radius:16px}.button{display:inline-block;padding:14px 20px;background:#0066cc;color:white;border-radius:10px;text-decoration:none;font-weight:600;margin:8px 0;min-height:50px}.button.secondary{background:#e8e8ed;color:#1d1d1f}.note{font-size:0.875rem}.technical{padding:24px 0}.technical code{background:#e8e8ed;padding:3px 8px;border-radius:6px;color:#1d1d1f;overflow-wrap:anywhere}footer{border-top:1px solid rgb(0 0 0 / 0.09);margin-top:32px;padding-top:24px;color:#5a5a5f;font-size:0.875rem;line-height:1.65}:focus-visible{outline:3px solid #0066cc;outline-offset:4px}@media(max-width:600px){body{padding:24px 18px}.hero{padding:40px 0 24px}h1{font-size:2rem}h2{font-size:1.375rem}.routes{grid-template-columns:1fr}.card{padding:24px}.button{display:block;text-align:center}header span{max-width:100%;text-align:left}}
@media(forced-colors:active){button,.button{border:1px solid ButtonText}button:disabled{border-color:GrayText;color:GrayText}:focus-visible{outline-color:Highlight}}</style>
</head>
<body><div class="shell">
<header><div class="brand"><svg aria-hidden="true" width="40" height="40" style="flex-shrink:0;vertical-align:middle;margin-right:12px" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><g transform="translate(0 0.75)"><polyline points="20,79 20,39 50,67 80,39 80,79" fill="none" stroke="#1D1D1F" stroke-width="21" stroke-linecap="round" stroke-linejoin="round"/><circle cx="50" cy="20" r="11" fill="#FF6A1A"/></g></svg>Mnet</div><span>Private-network development build</span></header>
<main>
<section class="hero"><p class="eyebrow">Your network, clearly connected</p><h1>Your devices.<br>Within reach.</h1><p class="intro">WireGuard-encrypted connections, peer to peer when possible and relayed when needed.</p><p>This control service is for an existing Mnet network. Choose the access you already have.</p></section>
<div class="routes">
<section class="card"><p class="eyebrow">For network members</p><h2>Your approved devices</h2><p>Use the network ID from your operator. Signing in does not enroll a device or grant membership; device approval is not live connectivity.</p><a class="button" href="/account/">View your approved devices</a><p class="note">Read-only account view. This is not Multies sign-in.</p></section>
<section class="card"><p class="eyebrow">For network operators</p><h2>Manage this network</h2><p>Review device approvals, network configuration and explicit account grants. Operator sign-in is separate from member access.</p><a class="button secondary" href="/admin">Open operator dashboard</a><p class="note">Only use operator access assigned to you.</p></section>
</div>
<section class="technical"><h2>Connecting the Mnet app</h2><p>This development build still needs the control-server and network details from your operator. The command-line client uses <code>mnet up</code>. Signing into this website does not connect a VPN device.</p></section>
</main>
<footer>Mnet is provided by <a href="https://fintex.au">Fintex Australia Pty Ltd</a> (ABN 60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au.</footer>
</div></body>
</html>`

const joinPageStyle = `
:root{font:100%% -apple-system,BlinkMacSystemFont,"SF Pro Text","Segoe UI",Roboto,Inter,system-ui,sans-serif;color:#1d1d1f;background:#f5f5f7;color-scheme:light}*{box-sizing:border-box}body{margin:0;padding:48px 24px}.shell{max-width:720px;margin:auto}header{margin-bottom:28px;color:#0066cc;font-weight:700;font-size:1.375rem;letter-spacing:-.03em}.card{background:white;border:1px solid rgb(0 0 0 / 0.09);border-radius:16px;padding:32px}h1{font-size:2rem;line-height:1.2;letter-spacing:-.035em;margin:12px 0 24px}p{line-height:1.65;overflow-wrap:anywhere}.eyebrow{font-size:.75rem;letter-spacing:.08em;text-transform:uppercase;font-weight:700;color:#0066cc}code{display:block;background:#f5f5f7;border-radius:10px;padding:14px 16px;margin-top:8px;overflow-wrap:anywhere;font-size:.875rem}.hint{color:#5a5a5f;font-size:.875rem}a{color:#0066cc;text-underline-offset:4px}.button{display:inline-block;background:#0066cc;color:white;padding:14px 20px;border-radius:10px;text-decoration:none;font-weight:600;margin:12px 0;min-height:50px}:focus-visible{outline:3px solid #0066cc;outline-offset:4px}.back{display:inline-block;margin-top:24px;padding:12px 0}@media(max-width:600px){body{padding:24px 18px}.card{padding:24px 18px}h1{font-size:1.5rem}.button{display:block;text-align:center}}
`

const approveHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Approve device - Mnet</title>
<style>` + joinPageStyle + `@media(forced-colors:active){button,.button{border:1px solid ButtonText}button:disabled{border-color:GrayText;color:GrayText}:focus-visible{outline-color:Highlight}}</style>
</head>
<body><div class="shell"><header><svg aria-hidden="true" width="40" height="40" style="flex-shrink:0;vertical-align:middle;margin-right:12px" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><g transform="translate(0 0.75)"><polyline points="20,79 20,39 50,67 80,39 80,79" fill="none" stroke="#1D1D1F" stroke-width="21" stroke-linecap="round" stroke-linejoin="round"/><circle cx="50" cy="20" r="11" fill="#FF6A1A"/></g></svg>Mnet</header>
<main class="card">
<p class="eyebrow">Device enrollment</p><h1>Review a device request</h1>
<p>Device ID from this link <code>%s</code></p>
<p>Use your assigned operator account to inspect the request in the dashboard, then approve or deny it there.</p>
<a class="button" href="%s">Open operator dashboard</a>
<p class="hint">This link does not check the device's current approval or connection status. Opening it does not approve a device or grant account membership.</p>
</main><a class="back" href="/">Back to Mnet</a></div></body>
</html>`

// Success follows verified identity and persisted approval, not a live VPN check.
const deviceApprovedHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Device approved - Mnet</title><style>` + joinPageStyle + `footer{font-size:.875rem;color:#5a5a5f;line-height:1.65;margin-top:24px;overflow-wrap:anywhere}.identity{overflow-wrap:anywhere}.identity b{font-size:.875em}@media(forced-colors:active){button,.button{border:1px solid ButtonText}button:disabled{border-color:GrayText;color:GrayText}:focus-visible{outline-color:Highlight}}</style></head><body><div class="shell"><header><svg aria-hidden="true" width="40" height="40" style="flex-shrink:0;vertical-align:middle;margin-right:12px" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><g transform="translate(0 0.75)"><polyline points="20,79 20,39 50,67 80,39 80,79" fill="none" stroke="#1D1D1F" stroke-width="21" stroke-linecap="round" stroke-linejoin="round"/><circle cx="50" cy="20" r="11" fill="#FF6A1A"/></g></svg>Mnet</header><main class="card"><p class="eyebrow">Device enrollment</p><h1>Device approved</h1><p class="identity">Signed in as <b>%s</b>.</p><p>Your identity was verified and this device was approved to join the network.</p><p>Return to Mnet to check the connection status. You can close this tab.</p><p class="hint">Device approval does not confirm a live VPN connection or create account membership.</p></main><footer>Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia.</footer></div></body></html>`

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mnet admin</title>
<style>
:root{color-scheme:light;font:100% -apple-system,BlinkMacSystemFont,"SF Pro Text","Segoe UI",Roboto,Inter,system-ui,sans-serif;color:#1d1d1f;background:#f5f5f7}*{box-sizing:border-box}h1,h2,p,label{overflow-wrap:anywhere}input,select{min-width:0}body{margin:0}header{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:24px max(24px,calc((100vw - 1160px)/2));border-bottom:1px solid rgb(0 0 0 / 0.09);background:white}h1{font-size:1.438rem;letter-spacing:-.03em;margin:0}main{padding:28px;max-width:1216px;margin:auto}section{margin-bottom:24px;background:white;border:1px solid rgb(0 0 0 / 0.09);border-radius:16px;padding:24px;min-width:0}h2{font-size:1.438rem;margin:0 0 16px;letter-spacing:-.025em}p{line-height:1.65}a{color:#0066cc;text-underline-offset:4px}table{scroll-padding-inline:8px;display:block;width:100%;max-width:100%;overflow-x:auto;border-collapse:collapse;font-size:0.875rem;margin:20px 0;scrollbar-color:#a1a1a6 #f5f5f7}th,td{text-align:left;padding:14px 12px;border-bottom:1px solid rgb(0 0 0 / 0.09);vertical-align:middle;min-width:90px;max-width:320px;overflow-wrap:anywhere}th{color:#5a5a5f;font-size:0.8125rem;font-weight:650;white-space:nowrap}code{background:#e8e8ed;padding:3px 6px;border-radius:6px;font-size:0.8125rem;overflow-wrap:anywhere}button{background:#0066cc;border:0;color:white;padding:12px 16px;border-radius:10px;cursor:pointer;font:600 .875rem system-ui;min-height:46px}button:hover{background:#0058b0}.row-actions{white-space:nowrap;min-width:180px}.row-actions button{white-space:nowrap;margin:4px 6px 4px 0}button.danger{background:#b00017}button.ghost{background:#e8e8ed;color:#1d1d1f}button:disabled{background:#e8e8ed;color:#5a5a5f;cursor:wait}input,select,textarea{background:white;border:1px solid rgb(0 0 0 / 0.16);color:#1d1d1f;padding:12px;border-radius:10px;font:inherit;min-height:46px;max-width:100%}input[type=checkbox]{min-height:0;accent-color:#0066cc;width:18px;height:18px}form.inline{display:flex;gap:12px;flex-wrap:wrap;align-items:center}form.inline>label{line-height:1.6;font-size:.875rem}form.inline>label>input:not([type=checkbox]),form.inline>label>select{display:block;width:100%;margin-top:6px}textarea{resize:vertical}.badge{display:inline-block;padding:4px 10px;border-radius:999px;font-size:0.75rem;white-space:nowrap}.badge.on{background:#e5f1e8;color:#17692c}.badge.off{background:#e8e8ed;color:#5a5a5f}.badge.wait{background:#fff0d8;color:#8a4b00}.hidden,#login.hidden{display:none!important}#login{max-width:440px;margin:60px auto;padding:28px;border:1px solid rgb(0 0 0 / 0.09);border-radius:16px;background:white;display:flex;flex-direction:column;gap:14px}#login label{font-size:0.875rem;font-weight:650}.key{font-family:ui-monospace,monospace;word-break:break-all}.muted{color:#5a5a5f;font-size:0.875rem;line-height:1.65}footer{padding:24px 28px;color:#5a5a5f;font-size:0.875rem;line-height:1.65;max-width:1216px;margin:auto}details{background:#f5f5f7;border-radius:10px;padding:16px}summary{cursor:pointer;font-weight:600;color:#0066cc;min-height:28px}.admin-nav{display:flex;flex-wrap:wrap;gap:10px;margin-bottom:24px}.admin-nav a{background:#e8e8ed;padding:10px 14px;border-radius:10px;text-decoration:none}.overview{padding:4px 0 20px}.overview h2{font-size:1.875rem}.overview p{max-width:760px;margin:8px 0}.eyebrow{font-size:0.75rem;letter-spacing:.08em;text-transform:uppercase;color:#0066cc;font-weight:700}:focus-visible{outline:3px solid #0066cc;outline-offset:4px}@media(max-width:600px){header{padding:18px;align-items:flex-start;flex-wrap:wrap}main{padding:20px 14px}section{padding:20px 16px}h2{font-size:1.375rem}#login{margin:30px 18px}.overview h2{font-size:1.75rem}form.inline{align-items:stretch}.policy-actions>button{width:100%;min-width:0;overflow-wrap:anywhere}form.inline>input:not([type=checkbox]),form.inline>select,form.inline>button,form.inline>label{width:100%}th,td{padding:12px 10px}.admin-nav a{font-size:0.875rem}footer{padding:20px 18px}}
#memberRead{scroll-margin-block:16px}
.record-label{display:none}
@media(max-width:600px){
  #pending,#nodes,#keys{overflow:visible}
  #pending thead,#nodes thead,#keys thead{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
  #pending tbody,#nodes tbody,#keys tbody{display:block;width:100%}
  #pending tbody tr,#nodes tbody tr,#keys tbody tr{display:block;border:1px solid rgb(0 0 0 / .09);border-radius:10px;background:#f5f5f7;padding:16px;margin-bottom:16px}
  #pending td,#nodes td,#keys td{display:block;min-width:0;max-width:none;border:0;padding:0 0 16px;overflow-wrap:anywhere}
  #pending td:last-child,#nodes td:last-child,#keys td:last-child{padding-bottom:0}
  #pending td:first-child,#nodes td:first-child{font-size:1rem;font-weight:650}
  .record-label{display:block;font-size:.75rem;font-weight:650;color:#5a5a5f;margin-bottom:6px;line-height:1.5}
  #pending .row-actions,#nodes .row-actions,#keys .row-actions{display:flex;flex-wrap:wrap;gap:8px;min-width:0;white-space:normal}
  .row-actions button{flex:1 1 120px;min-width:0;max-width:100%;white-space:normal;overflow-wrap:anywhere;margin:0}
}
@media(forced-colors:active){button,.button{border:1px solid ButtonText}button:disabled{border-color:GrayText;color:GrayText}:focus-visible{outline-color:Highlight}}</style>
</head>
<body>
<header>
<div><svg aria-hidden="true" width="40" height="40" style="display:block;margin-bottom:12px" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><g transform="translate(0 0.75)"><polyline points="20,79 20,39 50,67 80,39 80,79" fill="none" stroke="#1D1D1F" stroke-width="21" stroke-linecap="round" stroke-linejoin="round"/><circle cx="50" cy="20" r="11" fill="#FF6A1A"/></g></svg><div class="eyebrow">Mnet / Operator</div><h1>Network dashboard</h1></div>
<div><button class="ghost hidden" id="refresh">Refresh</button> <button class="ghost hidden" id="logout">Sign out</button></div>
</header>
<div id="login" class="hidden">
<div class="eyebrow">Operator access</div><h2>Sign in to manage networks</h2><p class="muted">Use your assigned operator password. Member access is separate.</p><label for="pw">Admin password</label>
<input id="pw" type="password" placeholder="Admin password" autocomplete="current-password" aria-describedby="loginMsg">
<button id="loginBtn">Sign in</button>
<div class="muted" id="loginMsg" role="status" aria-live="polite"></div>
</div>
<section id="stateReadFeedback" class="hidden" style="position:sticky;top:0;z-index:10;max-width:1160px;margin:12px auto;background:#fff" aria-label="Dashboard refresh status"><p id="stateReadMsg" role="status" aria-live="polite"></p><button id="stateReadRetry" type="button">Retry dashboard refresh</button></section>
<main id="app" class="hidden"><fieldset id="dashboardControls" style="border:0;min-width:0;padding:0;margin:0" aria-label="Dashboard controls">
<div class="overview"><p class="eyebrow">Control service</p><h2 id="overviewHeading" tabindex="-1">Network overview</h2><p class="muted">Inspect and manage this Mnet control service. Device approval, account membership and current connectivity are separate.</p></div>
<nav class="admin-nav" aria-label="Dashboard sections"><a href="#networkSection">Networks</a><a href="#approvalSection">Approvals</a><a href="#nodeSection">Devices</a><a href="#keySection">Auth keys</a><a href="#membershipSection">Account access</a><a href="#policySection">ACL policy</a></nav>
<div id="rowActionPanel" class="hidden" style="padding:16px 0"><p id="rowActionMsg" class="muted" role="status" aria-live="polite" tabindex="-1"></p><button id="rowActionInspect" class="ghost hidden" type="button">Inspect action result</button></div>
<section id="networkSection">
<h2>Networks</h2>
<table id="nets" tabindex="0" aria-label="Networks, scroll horizontally for all columns"><thead><tr><th>Name</th><th>ID</th><th>Created</th></tr></thead><tbody></tbody></table>
<form class="inline" id="newNet" style="margin-top:.7rem">
<label for="netName">Network name<input aria-describedby="newNetMsg" id="netName" placeholder="new network name" required></label>
<button>Create network</button>
</form><p id="newNetMsg" tabindex="-1" class="muted" role="status" aria-live="polite"></p><button id="newNetInspect" class="ghost hidden" type="button">Inspect creation result</button>
</section>
<section id="approvalSection">
<h2>Pending approvals</h2>
<table id="pending" tabindex="0" role="table" aria-label="Pending device approvals"><thead><tr><th>Name</th><th>IP</th><th>Key</th><th>Asked</th><th></th></tr></thead><tbody></tbody></table>
<div class="muted" id="noPending">Nothing waiting.</div>
</section>
<section id="nodeSection">
<h2>Devices</h2>
<table id="nodes" tabindex="0" role="table" aria-label="Devices"><thead><tr><th>Name</th><th>Network</th><th>IP</th><th>User</th><th>Relay connection</th><th>Endpoints</th><th>Last seen</th><th>Key expiry</th><th></th></tr></thead><tbody></tbody></table>
</section>
<section id="keySection">
<h2>Auth keys</h2>
<table id="keys" tabindex="0" role="table" aria-label="Auth keys"><thead><tr><th>Key</th><th>Network</th><th>Note</th><th>Reusable</th><th>Uses</th><th>Expires</th><th></th></tr></thead><tbody></tbody></table>
<form class="inline" id="newKey" style="margin-top:.7rem">
<label for="keyNet">Network<select id="keyNet" aria-describedby="newKeyMsg"></select></label>
<label for="keyNote">Note (optional)<input id="keyNote" placeholder="note (optional)"></label>
<label class="muted"><input id="keyReusable" type="checkbox"> reusable</label>
<label for="keyTTL">Lifetime, hours (0: never)<input id="keyTTL" type="number" min="0" placeholder="hours" style="width:6rem" title="Lifetime in hours, 0 for never"></label>
<button>Create key</button>
</form><p id="newKeyMsg" tabindex="-1" class="muted" role="status" aria-live="polite"></p><button id="newKeyInspect" class="ghost hidden" type="button">Inspect creation result</button>
</section>
<section id="membershipSection">
<h2>Account membership grants</h2>
<p class="muted">Inspect explicit account access for one network. Private issuer/subject identifiers are not email addresses. This does not list device approval provenance. Removing membership closes account queries, not VPN device connections.</p>
<p><label>Network <select id="memberNet"></select></label></p>
<details id="grantDetails"><summary>Add explicit account membership</summary>
<p class="muted">Use the exact issuer and subject from your trusted identity-provider records. Do not guess from an email address, name or device approval. This grants read-only account access to that principal's approved devices in the selected network, not operator privileges or device enrollment.</p>
<form id="memberGrant" class="inline">
<label>Issuer <input id="grantIssuer" autocomplete="off" required maxlength="2048"></label>
<label>Subject <input id="grantSubject" autocomplete="off" required maxlength="1024"></label>
<label class="muted"><input id="grantVerified" type="checkbox" required> I checked this exact principal in trusted identity-provider records.</label>
<button type="submit">Review membership grant</button>
</form></details>
<p id="memberMsg" class="muted" role="status" aria-live="polite" tabindex="-1"></p><button id="memberRead" class="ghost" type="button">Inspect grants</button>
<table id="members" tabindex="0" aria-label="Account membership grants, scroll horizontally for all columns"><thead><tr><th>Verified issuer</th><th>Subject</th><th></th></tr></thead><tbody></tbody></table>
</section>
<section class="card" id="policySection">
<h2>ACL policy</h2>
<p class="muted">JSON rules: <code>[{"src":"*","dst":"*"}]</code>. Empty means allow all. With any rule present the default is deny. src is a node name or *; dst is a node name, *, or a subnet CIDR. Exit-node selection and internet routing are unsupported; an ACL does not enable them.</p>
<p><label for="aclNet">Network <select id="aclNet"></select></label></p>
<label for="aclText">Rules (JSON)</label><textarea id="aclText" aria-describedby="aclMsg" disabled rows="8" style="width:100%;font-family:monospace"></textarea>
<div class="policy-actions" style="display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin:20px 0 0"><button id="aclSave" disabled>Save policy</button> <button id="aclReload" class="ghost" type="button">Reload policy</button> <span id="aclMsg" class="muted" style="flex-basis:100%;min-width:0;overflow-wrap:anywhere" role="status"></span></div>
</section>
</fieldset></main>
<footer>Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au.</footer>
<script>
const $ = (s) => document.querySelector(s);
const api = async (path, body) => {
  const r = await fetch(path, body === undefined ? {} : {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  if (r.status === 401) { showLogin(); throw new Error('auth'); }
  const j = await r.json().catch(()=>({}));
  if (!r.ok) {const error=new Error(j.error || r.statusText);error.responseReceived=r.status>=400 && r.status<500;throw error;}
  return j;
};
let viewGeneration=0, stateRequest=0, signingOut=false, memberRequest=0;
function clearGrant(){ $('#grantIssuer').value=''; $('#grantSubject').value=''; $('#grantVerified').checked=false; $('#grantDetails').open=false; }
let memberOperation=null;
function lockMembership(){
  const locked=!!memberOperation;
  $('#memberGrant').setAttribute('aria-busy',String(!!memberOperation?.busy));
  $('#memberNet').disabled=locked;$('#memberRead').disabled=!!memberOperation?.busy;
  for(const field of $('#memberGrant').elements)field.disabled=locked;
  for(const button of document.querySelectorAll('#members button'))button.disabled=locked;
}
function resetMembership(){memberOperation=null;lockMembership();}
function clearMembers(){ memberRequest++; $('#members tbody').replaceChildren(); $('#memberMsg').textContent=''; }
function showLogin(){ viewGeneration++; clearRowAction(); clearCreateForms(); clearACL(); resetMembership(); clearMembers(); clearGrant(); state=null;stateFresh=false;$('#dashboardControls').disabled=false;$('#stateReadFeedback').classList.add('hidden');$('#stateReadMsg').textContent=''; $('#pw').value=''; $('#login').classList.remove('hidden'); $('#app').classList.add('hidden'); $('#refresh').classList.add('hidden'); $('#logout').classList.add('hidden'); for(const selector of ['#nets tbody','#nodes tbody','#keys tbody','#pending tbody']) $(selector).innerHTML=''; $('#aclText').value=''; for(const selector of ['#keyNet','#aclNet','#memberNet']) $(selector).innerHTML=''; $('#pw').focus(); }

function showApp(){ const entering=$('#app').classList.contains('hidden'); $('#login').classList.add('hidden'); $('#app').classList.remove('hidden'); $('#refresh').classList.remove('hidden'); $('#logout').classList.remove('hidden'); if(entering) $('#overviewHeading').focus(); }
function esc(s){ const d=document.createElement('div'); d.textContent=s==null?'':String(s); return d.innerHTML.replaceAll('"','&quot;').replaceAll("'",'&#39;'); }
function expires(t,now=Date.now()){
  if(!t) return 'never';
  const deadline=new Date(t).getTime();
  if(!Number.isFinite(deadline)) return 'unknown';
  let seconds=Math.floor((deadline-now)/1000);
  if(seconds<=0) return 'expired';
  if(seconds<60) return seconds+'s left';
  if(seconds<3600) return Math.floor(seconds/60)+'m left';
  if(seconds<86400) return Math.floor(seconds/3600)+'h left';
  return Math.floor(seconds/86400)+'d left';
}
function ago(t){ if(!t || new Date(t).getUTCFullYear()===1) return 'Not recorded'; const s=(Date.now()-new Date(t).getTime())/1000;if(!Number.isFinite(s))return 'Unknown'; if(s<60) return Math.floor(s)+'s ago'; if(s<3600) return Math.floor(s/60)+'m ago'; if(s<86400) return Math.floor(s/3600)+'h ago'; return Math.floor(s/86400)+'d ago'; }
let state=null,stateFresh=false;
function validDashboard(data){
  if(!data || typeof data!=='object' || Array.isArray(data))return false;
  const strings=(item,fields)=>item && fields.every(field=>typeof item[field]==='string');
  const collection=(name,valid)=>Object.hasOwn(data,name) && (data[name]===null || Array.isArray(data[name])&&data[name].every(valid));
  return collection('networks',item=>strings(item,['id','name'])&&item.id.length>0) &&
    collection('nodes',item=>strings(item,['id','network_id','name','pubkey','ipv4'])&&item.id.length>0&&typeof item.approved==='boolean'&&(item.endpoints===null||Array.isArray(item.endpoints)&&item.endpoints.every(value=>typeof value==='string'))) &&
    collection('auth_keys',item=>strings(item,['id','network_id','key','note'])&&item.id.length>0&&typeof item.reusable==='boolean'&&Number.isSafeInteger(item.uses)&&item.uses>=0) &&
    data.online_by_node && typeof data.online_by_node==='object' && !Array.isArray(data.online_by_node) && Object.values(data.online_by_node).every(value=>typeof value==='boolean');
}
async function load(){
  if(signingOut) return;
  const generation=viewGeneration, request=++stateRequest;
  let next;
  try {next=await api('/admin/api/state');if(!validDashboard(next))throw Error('invalid state');}
  catch(e){
    if(generation===viewGeneration && request===stateRequest && !signingOut){
      stateFresh=false;$('#dashboardControls').disabled=true;$('#stateReadFeedback').classList.remove('hidden');
      $('#stateReadMsg').textContent=state?'Could not refresh the dashboard. Displayed data may be out of date. Changes are paused until refresh succeeds.':'Could not check the dashboard. This does not confirm sign-out. Retry, or use Sign out.';
      $('#refresh').classList.remove('hidden');$('#logout').classList.remove('hidden');
    }
    return false;
  }
  if(generation!==viewGeneration || request!==stateRequest) return;
  const retryFocused=document.activeElement.id==='stateReadRetry';state=next;stateFresh=true;$('#dashboardControls').disabled=false;$('#stateReadFeedback').classList.add('hidden');$('#stateReadMsg').textContent='';showApp();render();if(retryFocused)$('#overviewHeading').focus();return true;
}
$('#stateReadRetry').addEventListener('click',load);
function netName(id){ const n=(state.networks||[]).find(n=>n.id===id); return n?n.name:id; }
function render(){
  const focusedAction=document.activeElement?.dataset?.mnetAction;
  const focusedTable=focusedAction?document.activeElement.closest('table'):null;
  const nets = state.networks||[], nodes=state.nodes||[], keys=state.auth_keys||[], online=state.online_by_node||{};
  $('#nets tbody').innerHTML = nets.map(n=>'<tr><td>'+esc(n.name)+'</td><td><code>'+esc(n.id)+'</code></td><td>'+ago(n.created_at)+'</td></tr>').join('') || '<tr><td colspan="3" class="muted">No networks yet.</td></tr>';
  const pending = nodes.filter(n=>!n.approved);
  $('#noPending').style.display = pending.length? 'none':'block';
  $('#pending tbody').innerHTML = pending.map(n=>'<tr><td>'+esc(n.name)+'</td><td><code>'+esc(n.ipv4)+'</code></td><td><code>'+esc(n.pubkey.slice(0,12))+'...</code></td><td>'+ago(n.created_at)+'</td><td class="row-actions"><button>Approve</button> <button class="danger">Deny</button></td></tr>').join('');
  const approved = nodes.filter(n=>n.approved);
  $('#nodes tbody').innerHTML = approved.map(n=>'<tr><td>'+esc(n.name)+'</td><td>'+esc(netName(n.network_id))+'</td><td><code>'+esc(n.ipv4)+'</code></td><td>'+esc(n.user||'Not recorded')+'</td><td>'+(online[n.id]===true?'<span class="badge on">connected</span>':'<span class="badge off">not connected</span>')+'</td><td class="muted">'+esc((n.endpoints||[]).join(', ')||'None reported')+'</td><td>'+ago(n.last_seen)+'</td><td>'+expires(n.key_expiry)+'</td><td class="row-actions"><button>Expire key</button> <button class="danger">Remove</button></td></tr>').join('') || '<tr><td colspan="9" class="muted">No approved nodes.</td></tr>';
  $('#keys tbody').innerHTML = keys.map(k=>'<tr><td class="key"><code>'+esc(k.key)+'</code></td><td>'+esc(netName(k.network_id))+'</td><td>'+esc(k.note)+'</td><td>'+(k.reusable?'yes':'no')+'</td><td>'+k.uses+'</td><td>'+expires(k.expires_at)+'</td><td class="row-actions"><button class="danger">Delete</button></td></tr>').join('') || '<tr><td colspan="7" class="muted">No keys.</td></tr>';
  // Set contextual names through DOM attributes, never interpolate names into HTML.
  for(const [selector,items,label,actions] of [
    ['#pending tbody tr',pending,n=>n.name+', '+n.ipv4,['approve','remove']],
    ['#nodes tbody tr',approved,n=>n.name+', '+n.ipv4,['expire','remove']],
    ['#keys tbody tr',keys,k=>(k.note||'auth key')+', network '+netName(k.network_id),['deleteKey']]
  ]) document.querySelectorAll(selector).forEach((row,index)=>{
    if(items[index]){
      if(selector==='#pending tbody tr')row.id='pending-'+items[index].id;
      row.querySelectorAll('button').forEach((button,buttonIndex)=>{
        button.setAttribute('aria-label',button.textContent+' '+label(items[index]));button.dataset.mnetAction=selector+':'+items[index].id+':'+button.textContent;
        button.addEventListener('click',()=>runRowAction(actions[buttonIndex],items[index].id));
      });
    }
  });
  for(const table of document.querySelectorAll('#pending,#nodes,#keys')){
    table.tHead.setAttribute('role','rowgroup');table.tBodies[0].setAttribute('role','rowgroup');
    const headers=Array.from(table.tHead.rows[0].cells,cell=>cell.textContent||'Actions');
    for(const cell of table.tHead.rows[0].cells){if(!cell.textContent)cell.textContent='Actions';cell.setAttribute('scope','col');cell.setAttribute('role','columnheader');}
    for(const row of table.rows){row.setAttribute('role','row');}
    for(const row of table.tBodies[0].rows){for(const [index,cell] of Array.from(row.cells).entries()){
      cell.setAttribute('role','cell');
      if(cell.colSpan>1 || cell.classList.contains('row-actions'))continue;
      const label=document.createElement('span');label.className='record-label';label.setAttribute('aria-hidden','true');label.textContent=headers[index];cell.prepend(label);
    }}
  }
  const selectedKey=$('#keyNet').value;
  $('#keyNet').replaceChildren(...nets.map(n=>new Option(n.name,n.id)));
  if(nets.some(n=>n.id===selectedKey)) $('#keyNet').value=selectedKey;
  const selectedACL=$('#aclNet').value;
  $('#aclNet').innerHTML = $('#keyNet').innerHTML;
  if(nets.some(n=>n.id===selectedACL)) $('#aclNet').value=selectedACL;
  const selectedMember=$('#memberNet').value;
  $('#memberNet').innerHTML = $('#keyNet').innerHTML;
  if(nets.some(n=>n.id===selectedMember)) $('#memberNet').value=selectedMember;
  if(memberOperation){
    if(!nets.some(n=>n.id===memberOperation.network))$('#memberNet').add(new Option(memberOperation.network,memberOperation.network));
    $('#memberNet').value=memberOperation.network;
  }else if($('#memberNet').value!==selectedMember){ clearMembers(); clearGrant(); }
  lockMembership();
  if($('#aclNet').value!==aclReadFor) loadACL();
  lockRowActions();
  if(focusedAction){
    const replacement=Array.from(document.querySelectorAll('[data-mnet-action]')).find(button=>button.dataset.mnetAction===focusedAction);
    (replacement||focusedTable)?.focus({preventScroll:true});
  }
}
async function loadMembers(){
  if(signingOut || !state || memberOperation?.busy) return;
  const inspecting=memberOperation;
  clearMembers();
  const request=memberRequest, generation=viewGeneration, id=$('#memberNet').value;
  if(!id){ $('#memberMsg').textContent='Choose a network first.'; return; }
  $('#memberMsg').textContent='Loading grants...';
  try {
    const r=await fetch('/admin/api/memberships?network_id='+encodeURIComponent(id));
    if(generation!==viewGeneration || request!==memberRequest || signingOut || $('#memberNet').value!==id) return;
    if(r.status===401){ showLogin(); return; }
    if(!r.ok) throw new Error('read failed');
    const j=await r.json();
    if(generation!==viewGeneration || request!==memberRequest || signingOut || $('#memberNet').value!==id) return;
    if(!j || !Array.isArray(j.memberships) || !j.memberships.every(member=>member && member.network_id===id && typeof member.issuer==='string' && member.issuer.length>0 && typeof member.subject==='string' && member.subject.length>0)) throw new Error('invalid response');
    const rows=j.memberships.map(member=>{
      const row=document.createElement('tr');
      for(const value of [member.issuer,member.subject]){
        const cell=document.createElement('td'); cell.style.overflowWrap='anywhere'; cell.textContent=value; row.append(cell);
      }
      const action=document.createElement('td'), remove=document.createElement('button');
      remove.className='danger'; remove.textContent='Remove membership';
      remove.setAttribute('aria-label','Remove membership for '+member.subject+', issuer '+member.issuer);
      remove.onclick=()=>revokeMember(id,member.issuer,member.subject,generation,request);
      action.append(remove); row.append(action);
      return row;
    });
    $('#members tbody').replaceChildren(...rows);
    if(inspecting && memberOperation===inspecting){resetMembership();if(inspecting.allow)clearGrant();}
    $('#memberMsg').textContent=rows.length ? rows.length+' explicit grant(s).' : 'No explicit grants for this network.';
    if(inspecting)$('#memberMsg').focus({preventScroll:true});
  } catch(e){
    if(generation===viewGeneration && request===memberRequest && !signingOut) $('#memberMsg').textContent=inspecting?'Could not inspect grants. The change remains unconfirmed. Inspect grants again before retrying.':'Could not read grants. Try again.';
  }
}
async function changeMembership(network,issuer,subject,allow,generation,request){
  if(memberOperation)return;
  if(allow)$('#grantDetails').open=true;
  clearMembers();
  const operation={network,issuer,subject,allow,busy:true};memberOperation=operation;lockMembership();
  $('#memberMsg').textContent=allow?'Adding membership...':'Removing membership...';
  try{
    const response=await fetch('/admin/api/memberships',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({network_id:network,issuer,subject,allow})});
    if(generation!==viewGeneration || memberOperation!==operation || signingOut)return;
    if(response.status===401){showLogin();return;}
    if(!response.ok){const error=new Error('membership failed');error.rejected=response.status>=400&&response.status<500;throw error;}
    resetMembership();if(allow)clearGrant();await loadMembers();
  }catch(error){
    if(generation!==viewGeneration || memberOperation!==operation || signingOut)return;
    operation.busy=false;
    if(error.rejected){
      resetMembership();$('#grantVerified').checked=false;
      $('#memberMsg').textContent=allow?'The grant was not accepted. Your issuer and subject are kept. Check them and verify the exact principal again before retrying.':'The removal was not accepted. Inspect grants before reviewing another removal.';
    }else{
      lockMembership();$('#memberMsg').textContent=allow?'Could not confirm grant. Inspect grants again before retrying.':'Could not confirm removal. Inspect grants again before retrying. VPN device connections are separate.';
    }
    $('#memberMsg').focus({preventScroll:true});
  }
}
async function revokeMember(network,issuer,subject,generation,request){
  const current=()=>!memberOperation && generation===viewGeneration && request===memberRequest && !signingOut && state && stateFresh && $('#memberNet').value===network;
  if(!current())return;
  if(!confirm('Remove account membership for network '+network+'?\nIssuer: '+issuer+'\nSubject: '+subject+'\nThis closes account queries. It does NOT disconnect VPN devices or revoke their credentials.'))return;
  if(current())await changeMembership(network,issuer,subject,false,generation,request);
}
$('#memberNet').onchange=()=>{if(memberOperation){$('#memberNet').value=memberOperation.network;return;}clearMembers();clearGrant();};
$('#memberGrant').onsubmit=async event=>{
  event.preventDefault();
  const generation=viewGeneration, request=memberRequest, network=$('#memberNet').value;
  const issuer=$('#grantIssuer').value, subject=$('#grantSubject').value;
  const current=()=>!memberOperation && generation===viewGeneration && request===memberRequest && !signingOut && state && stateFresh && $('#memberNet').value===network;
  if(!current() || !network || !issuer || !subject || !$('#grantVerified').checked)return;
  if(!confirm('Grant account membership?\nNetwork: '+network+'\nIssuer: '+issuer+'\nSubject: '+subject+'\nOnly continue if this exact principal was verified in trusted identity-provider records. This is not enrollment, operator access or VPN connectivity.'))return;
  if(current())await changeMembership(network,issuer,subject,true,generation,request);
};
$('#memberRead').onclick=loadMembers;
let aclRequest=0, aclTarget='', aclReadFor='', aclDirty=false, aclSaving=false;
function clearACL(){aclRequest++;aclTarget='';aclReadFor='';aclDirty=false;aclSaving=false;$('#aclText').value='';$('#aclText').disabled=true;$('#aclText').removeAttribute('aria-invalid');$('#aclSave').disabled=true;$('#aclReload').disabled=false;$('#aclNet').disabled=false;$('#aclMsg').textContent='';}
async function loadACL(){
  if(signingOut || !state || aclSaving)return;
  const generation=viewGeneration, request=++aclRequest, id=$('#aclNet').value;
  aclReadFor=id;aclTarget='';aclDirty=false;$('#aclText').value='';$('#aclText').disabled=true;$('#aclText').removeAttribute('aria-invalid');$('#aclSave').disabled=true;
  if(!id){$('#aclMsg').textContent='Choose a network first.';return;}
  $('#aclMsg').textContent='Loading policy...';
  const current=()=>generation===viewGeneration && request===aclRequest && !signingOut && state && $('#aclNet').value===id;
  try{
    const r=await fetch('/admin/api/acl?network_id='+encodeURIComponent(id));
    if(!current())return;
    if(r.status===401){showLogin();return;}
    if(!r.ok)throw Error();
    const j=await r.json();
    if(!current())return;
    if(!j || !Object.prototype.hasOwnProperty.call(j,'rules') || (j.rules!==null && (!Array.isArray(j.rules) || !j.rules.every(rule=>rule && typeof rule.src==='string' && typeof rule.dst==='string'))))throw Error();
    $('#aclText').value=JSON.stringify(j.rules||[],null,2);aclTarget=id;
    $('#aclText').disabled=false;$('#aclSave').disabled=false;
    $('#aclMsg').textContent='Policy loaded for network '+id+'.';
  }catch{if(current())$('#aclMsg').textContent='Could not read this policy. Reload before editing or saving.';}
}
$('#aclNet').onchange=()=>{
  // Network selection itself never writes a policy. Clear stale text at once.
  if(aclDirty && aclTarget && !confirm('Discard unsaved policy edits and switch networks?')){$('#aclNet').value=aclTarget;return;}
  loadACL();
};
$('#aclReload').onclick=()=>{if(aclDirty && !confirm('Discard unsaved policy edits and reload this network?'))return;loadACL();};
$('#aclText').addEventListener('input',()=>{aclDirty=true;$('#aclText').removeAttribute('aria-invalid');$('#aclMsg').textContent='Unsaved edits for network '+aclTarget+'.';});
$('#aclSave').onclick=async ()=>{
  const id=aclTarget;
  if(aclSaving || signingOut || !state || !stateFresh || !id || $('#aclNet').value!==id || $('#aclText').disabled)return;
  let rules;
  try{rules=JSON.parse($('#aclText').value||'[]');}
  catch{ $('#aclMsg').textContent='Invalid JSON. Fix the rules before saving.';$('#aclText').setAttribute('aria-invalid','true');$('#aclText').focus();return; }
  const generation=viewGeneration,request=++aclRequest;
  const current=()=>generation===viewGeneration && request===aclRequest && !signingOut && state && $('#aclNet').value===id;
  aclSaving=true;$('#aclSave').disabled=true;$('#aclReload').disabled=true;$('#aclNet').disabled=true;$('#aclText').disabled=true;$('#aclMsg').textContent='Saving policy for network '+id+'...';
  try{
    const r=await fetch('/admin/api/acl',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({network_id:id,rules})});
    if(!current())return;
    if(r.status===401){showLogin();return;}
    const j=await r.json().catch(()=>({}));
    if(!current())return;
    if(!r.ok){$('#aclMsg').textContent='Policy was not saved: '+(j.error||r.statusText);$('#aclText').disabled=false;$('#aclSave').disabled=false;return;}
    aclDirty=false;$('#aclText').disabled=false;$('#aclSave').disabled=false;$('#aclMsg').textContent='Saved policy for network '+id+'.';
  }catch{
    if(current()){$('#aclMsg').textContent='Could not confirm policy save. Reload to inspect the stored policy before retrying.';aclTarget='';}
  }finally{
    if(current()){aclSaving=false;$('#aclNet').disabled=false;$('#aclReload').disabled=false;}
  }
};
let rowAction=null;
function clearRowAction(){rowAction=null;$('#rowActionPanel').classList.add('hidden');$('#rowActionMsg').textContent='';$('#rowActionInspect').classList.add('hidden');$('#rowActionInspect').disabled=false;}
function lockRowActions(){document.querySelectorAll('#pending button,#nodes button,#keys button').forEach(button=>button.disabled=!!rowAction);}
function rowActionStatus(message,inspect=false){$('#rowActionMsg').textContent=message;$('#rowActionInspect').classList.toggle('hidden',!inspect);$('#rowActionMsg').focus();}
async function runRowAction(kind,id){
  if(rowAction || signingOut || !state || !stateFresh)return;
  const key=kind==='deleteKey', list=()=>key?state?.auth_keys:state?.nodes;
  const item=list()?.find(item=>item.id===id);if(!item)return;
  const generation=viewGeneration, identity=JSON.stringify(item);
  const table=key?'keys':item.approved?'nodes':'pending';
  const label=key?(item.note||'Auth key')+', network '+netName(item.network_id)+', key ID '+item.id:item.name+', '+item.ipv4+', network '+netName(item.network_id);
  const verb={approve:'Approve device',remove:'Remove device',expire:'Expire device key',deleteKey:'Delete auth key'}[kind];
  const consequence={approve:'This approves this VPN device, not account membership.',remove:'This removes this device from the network.',expire:'The device must rejoin.',deleteKey:'This prevents future joins with this auth key. Existing device credentials are separate.'}[kind];
  if(!confirm(verb+'?\n'+label+'\n'+consequence))return;
  if(generation!==viewGeneration || signingOut || !stateFresh || rowAction || JSON.stringify(list()?.find(item=>item.id===id))!==identity)return;
  const operation={kind,id,label};rowAction=operation;lockRowActions();
  const target=$('#'+table);target.parentNode.insertBefore($('#rowActionPanel'),target);$('#rowActionPanel').classList.remove('hidden');$('#rowActionInspect').classList.add('hidden');$('#rowActionMsg').textContent=verb+' pending: '+label+'.';
  const current=()=>rowAction===operation && generation===viewGeneration && !signingOut;
  try{
    const path={approve:'/admin/api/nodes/approve',remove:'/admin/api/nodes/delete',expire:'/admin/api/nodes/expire',deleteKey:'/admin/api/authkeys/delete'}[kind];
    await api(path,key?{id}:{node_id:id});if(!current())return;
    const refreshed=await load();if(!current())return;
    if(!refreshed){rowActionStatus(verb+' request accepted, but the list could not be refreshed. Inspect the result before another action. '+label+'.',true);return;}
    rowAction=null;lockRowActions();rowActionStatus(verb+' request completed. List refreshed. '+label+'.');
  }catch(error){
    if(!current())return;
    if(error.responseReceived){rowAction=null;lockRowActions();rowActionStatus(verb+' was rejected. No retry was sent. '+label+'.');}
    else rowActionStatus('Could not confirm '+verb.toLowerCase()+'. Inspect the result before another action. No retry was sent. '+label+'.',true);
  }finally{
    if(rowAction===operation && generation!==viewGeneration)rowActionStatus('Action outcome needs inspection after the sign-out attempt. No action was repeated.',true);
  }
}
$('#rowActionInspect').addEventListener('click',async()=>{
  const operation=rowAction,generation=viewGeneration;if(!operation)return;
  $('#rowActionInspect').disabled=true;
  const refreshed=await load();if(rowAction!==operation||generation!==viewGeneration||signingOut)return;
  $('#rowActionInspect').disabled=false;
  if(!refreshed){rowActionStatus('Could not refresh the list. Action outcome is still unconfirmed. No retry was sent. '+operation.label+'.',true);return;}
  rowAction=null;lockRowActions();rowActionStatus('List refreshed for inspection. Check '+operation.label+' before choosing another action. No action was repeated.');
});
async function approve(id){await runRowAction('approve',id);}
async function delNode(id){await runRowAction('remove',id);}
async function delKey(id){await runRowAction('deleteKey',id);}
async function expireNode(id){await runRowAction('expire',id);}
let signingIn=false;
$('#loginBtn').onclick = async ()=>{
  if(signingIn)return;
  if(!$('#pw').value){ $('#loginMsg').textContent='Enter your operator password.'; $('#pw').setAttribute('aria-invalid','true'); $('#pw').focus(); return; }
  signingIn=true; $('#pw').removeAttribute('aria-invalid'); $('#loginBtn').disabled=true; $('#loginMsg').textContent='Signing in...';
  try{ await api('/admin/api/login',{password:$('#pw').value}); $('#pw').value=''; $('#pw').removeAttribute('aria-invalid'); $('#loginMsg').textContent=''; await load(); }
  catch(e){ $('#loginMsg').textContent=e.message==='auth'?'Password was not accepted. Try again.':'Could not sign in. Check your connection and try again.'; if(e.message==='auth')$('#pw').setAttribute('aria-invalid','true'); $('#pw').focus(); }
  finally{ signingIn=false; $('#loginBtn').disabled=false; }
};
$('#pw').addEventListener('input',()=>$('#pw').removeAttribute('aria-invalid'));
$('#pw').addEventListener('keydown', e=>{ if(e.key==='Enter') $('#loginBtn').click(); });
$('#refresh').onclick = load;
$('#logout').onclick = async ()=>{
  viewGeneration++; signingOut=true; if(memberOperation)memberOperation={network:memberOperation.network,allow:memberOperation.allow,busy:false}; clearMembers(); clearGrant();lockMembership();
  $('#logout').disabled=true;
  try { await api('/admin/api/logout',{}); showLogin(); $('#loginMsg').textContent='Signed out.'; }
  catch(e){ if(e.message!=='auth') alert('Sign out failed: '+e.message); }
  finally { signingOut=false; $('#logout').disabled=false;if(memberOperation){$('#memberMsg').textContent='Sign-out was not confirmed. The membership change is still unconfirmed. Inspect grants before making another change.';lockMembership();} }
};
const createStates={newNet:{busy:false,uncertain:false},newKey:{busy:false,uncertain:false}};
function enableCreateForm(id,enabled){document.querySelectorAll('#'+id+' input,#'+id+' select,#'+id+' button').forEach(control=>control.disabled=!enabled);}
function clearCreateForms(){for(const id of ['newNet','newKey']){createStates[id].busy=false;createStates[id].uncertain=false;enableCreateForm(id,true);$('#'+id).reset();$('#'+id+'Msg').textContent='';$('#'+id+'Inspect').classList.add('hidden');$('#'+id+'Inspect').disabled=false;}}
async function createFromForm(id,path,body){
  const operation=createStates[id];
  if(operation.busy || operation.uncertain || signingOut || !state || !stateFresh)return;
  const generation=viewGeneration;operation.busy=true;enableCreateForm(id,false);$('#'+id+'Msg').textContent='Creating...';
  try{
    await api(path,body);
    if(generation!==viewGeneration || signingOut)return;
    if(id==='newNet')$('#netName').value='';
    else{$('#keyNote').value='';$('#keyTTL').value='';}
    $('#'+id+'Msg').textContent=id==='newNet'?'Network created. Inspect the Networks table.':'Auth key created. Inspect the Auth keys table and keep it private.';
    await load();
  }catch(error){
    if(generation!==viewGeneration || signingOut)return;
    if(error.responseReceived){$('#'+id+'Msg').textContent='Not created: '+error.message;}
    else{operation.uncertain=true;$('#'+id+'Inspect').classList.remove('hidden');$('#'+id+'Msg').textContent='Could not confirm creation. Inspect the result before retrying. Reload this page after checking.';}
  }finally{
    if(generation===viewGeneration && !signingOut){operation.busy=false;if(!operation.uncertain)enableCreateForm(id,true);if(document.activeElement===document.body)$('#'+id+'Msg').focus({preventScroll:true});}
  }
}
for(const id of ['newNet','newKey'])$('#'+id+'Inspect').onclick=async()=>{
  const generation=viewGeneration;$('#'+id+'Inspect').disabled=true;
  const loaded=await load();
  if(generation!==viewGeneration || signingOut)return;
  $('#'+id+'Inspect').disabled=false;
  if(!loaded){$('#'+id+'Msg').textContent='Could not inspect the result. Creation remains unconfirmed. Try inspection again.';return;}
  const table=$(id==='newNet'?'#nets':'#keys');table.focus();table.scrollIntoView({block:'center'});
};
$('#newNet').onsubmit=e=>{
  e.preventDefault();
  const name=$('#netName').value.trim();
  if(!name){$('#newNetMsg').textContent='Enter a network name.';$('#netName').focus();return;}
  createFromForm('newNet','/admin/api/networks',{name});
};
$('#newKey').onsubmit=e=>{
  e.preventDefault();
  const network=$('#keyNet').value,ttl=$('#keyTTL').value===''?0:Number($('#keyTTL').value);
  if(!network){$('#newKeyMsg').textContent='Choose a network first.';$('#keyNet').focus();return;}
  if(!Number.isSafeInteger(ttl) || ttl<0){$('#newKeyMsg').textContent='Enter a whole number of hours,0 or more.';$('#keyTTL').focus();return;}
  createFromForm('newKey','/admin/api/authkeys',{network_id:network,note:$('#keyNote').value,reusable:$('#keyReusable').checked,ttl_hours:ttl});
};
load();
setInterval(()=>{ if(!$('#app').classList.contains('hidden')) load(); }, 10000);
</script>
</body>
</html>`
