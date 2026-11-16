package control

import "net/http"

func (s *Server) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/account/" {
		http.NotFound(w, r)
		return
	}
	if !accountMethod(w, r, "GET") {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	w.Write([]byte(accountPage))
}

// Browser callbacks keep a useful recovery path without echoing provider
// errors, principal identifiers, codes, state or membership diagnostics.
func accountLoginFailure(w http.ResponseWriter, status int) {
	accountPrivate(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	w.WriteHeader(status)
	w.Write([]byte(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Mnet - sign-in not completed</title><style>body{font:17px/1.6 system-ui,sans-serif;color:#183040;background:#f3f6f8;max-width:650px;margin:64px auto;padding:0 24px}main{background:white;border:1px solid #d7e1e8;border-radius:16px;padding:28px}h1{font-size:30px;line-height:1.25}a{color:#125970}footer{font-size:14px;margin-top:30px}</style><main><h1>Sign-in was not completed</h1><p>This login may have expired, been refused or opened in a different browser. Your identity must already have access to the selected network.</p><p><a href="/account/">Return to account sign-in</a> and start again in this browser. If access is still refused, ask your network operator to check membership and login setup.</p><p>Retrying does not enroll a device or grant membership. An existing session may still be active; use Sign out from the account page if needed.</p></main><footer>Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia. Contact: <a href="mailto:contact@fintex.au">contact@fintex.au</a>.</footer></html>`))
}

const accountPage = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Mnet - your devices</title><style>
:root{font-family:system-ui,sans-serif;color:#183040;background:#f3f6f8}body{max-width:760px;margin:48px auto;padding:0 22px}header{display:flex;align-items:baseline;justify-content:space-between;gap:20px}h1{font-size:32px;margin:0 0 12px}p{line-height:1.6}main{background:white;border:1px solid #d7e1e8;border-radius:16px;padding:28px;margin-top:24px}label{display:block;font-weight:600;margin:18px 0 8px}input{box-sizing:border-box;width:100%;border:1px solid #9fb0bb;border-radius:8px;padding:12px;font:inherit}button{background:#125970;color:white;border:0;border-radius:8px;padding:12px 18px;font:inherit;cursor:pointer;margin-top:16px}button:disabled{opacity:.6;cursor:wait}.secondary{background:#e2ebef;color:#183040}table{width:100%;border-collapse:collapse;text-align:left;margin:20px 0}th,td{padding:12px 8px;border-bottom:1px solid #d7e1e8;overflow-wrap:anywhere}td:first-child{max-width:300px}.hint,footer{font-size:14px;color:#526b7b}#message{min-height:26px;color:#365a69}footer{margin:32px 0;line-height:1.6}[hidden]{display:none!important}@media(max-width:500px){body{margin:24px auto}main{padding:18px}h1{font-size:27px}}
</style><header><div><h1>Your Mnet devices</h1><span class="hint">Private member view</span></div><a href="/">Mnet</a></header>
<main><div id="message" role="status" aria-live="polite">Checking your session...</div>
<section id="login" hidden><h2>Sign in to your network</h2><p>Use the network ID provided by your operator. Your identity must already be a member of that network.</p>
<form id="loginForm"><label for="network">Network ID</label><input id="network" name="network_id" autocomplete="off" maxlength="256" required><button id="signIn" type="submit">Continue to sign in</button></form>
<p class="hint">Signing in does not enroll a device or grant membership. Ask your operator if access has not been set up.</p></section>
<section id="devices" hidden><h2>Your approved devices</h2><p>Only devices approved with your verified identity in this network appear here. This list is not a live online-status check.</p>
<table><thead><tr><th scope="col">Device</th><th scope="col">Mesh address</th></tr></thead><tbody id="deviceRows"></tbody></table><p id="empty" hidden>No approved, unexpired devices were found for your identity.</p>
<button class="secondary" id="refresh" type="button">Refresh</button> <button class="secondary" id="logout" type="button">Sign out</button>
<p class="hint">Signing out ends this browser session. It does not disconnect VPN devices.</p></section></main>
<footer>Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia. Contact: <a href="mailto:contact@fintex.au">contact@fintex.au</a>.</footer>
<script>
const el=id=>document.getElementById(id);let generation=0,signingOut=false;
function clearPrivate(){el('deviceRows').replaceChildren();el('empty').hidden=true;el('devices').hidden=true;el('network').value='';}
function signedOut(message){clearPrivate();el('login').hidden=false;el('message').textContent=message;}
async function load(){if(signingOut)return;const current=++generation;try{const r=await fetch('/account/api/devices',{cache:'no-store'});if(current!==generation||signingOut)return;if(!r.ok){signedOut('Sign in with an existing network membership.');return;}const data=await r.json();if(current!==generation||signingOut)return;el('deviceRows').replaceChildren();for(const device of data.devices){const row=document.createElement('tr');for(const value of [device.Name,device.IPv4]){const cell=document.createElement('td');cell.textContent=value;row.append(cell);}el('deviceRows').append(row);}el('empty').hidden=data.devices.length!==0;el('login').hidden=true;el('devices').hidden=false;el('message').textContent='Signed in. Device approval is separate from current connectivity.';}catch{if(current===generation&&!signingOut){clearPrivate();el('login').hidden=false;el('message').textContent='Could not read your devices. Try again.';}}}
el('loginForm').addEventListener('submit',async e=>{e.preventDefault();el('signIn').disabled=true;el('message').textContent='Opening your identity provider...';try{const r=await fetch('/account/api/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({network_id:el('network').value.trim()})});if(!r.ok)throw Error();const data=await r.json();const target=new URL(data.authorization_url);if(!['https:','http:'].includes(target.protocol))throw Error();location.assign(target.href);}catch{el('message').textContent='Could not start sign-in. Check the network ID and ask your operator to confirm login setup.';el('signIn').disabled=false;}});
el('refresh').addEventListener('click',load);
el('logout').addEventListener('click',async()=>{if(signingOut)return;signingOut=true;++generation;clearPrivate();el('login').hidden=true;el('message').textContent='Signing out...';try{const r=await fetch('/account/api/logout',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});if(!r.ok)throw Error();signedOut('Signed out.');}catch{el('login').hidden=true;el('message').textContent='Could not confirm sign-out. Retry before leaving this browser.';el('devices').hidden=false;}finally{signingOut=false;}});
load();
</script></html>`
