package control

import (
	"fintex.au/mnet/internal/version"
	"fmt"
	"html"
	"net/http"
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
	id := html.EscapeString(r.URL.Query().Get("node_id"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, approveHTML, id, id)
}

const landingHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mnet</title>
<style>
:root{font:100% system-ui,-apple-system,sans-serif;color:#162b33;background:#f4f7f6;color-scheme:light}*{box-sizing:border-box}h1,h2,p,label{overflow-wrap:anywhere}input,select{min-width:0}body{margin:0;padding:32px 24px}.shell{max-width:1040px;margin:auto}header{display:flex;flex-wrap:wrap;gap:16px;align-items:center;justify-content:space-between;padding-bottom:28px;border-bottom:1px solid #d5e1dd}.brand{font-size:1.625rem;font-weight:700;letter-spacing:-.03em}header span{overflow-wrap:anywhere;color:#4d6269;font-size:0.875rem}.hero{padding:64px 0 36px;max-width:760px}.eyebrow{font-size:0.75rem;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:#086963}h1{font-size:3.25rem;line-height:1.08;letter-spacing:-.045em;margin:16px 0 24px}h2{font-size:1.562rem;letter-spacing:-.025em;margin:8px 0 16px}p{line-height:1.65;color:#4d6269}a{color:#086963;text-underline-offset:4px}.intro{font-size:1.188rem;max-width:660px}.routes{display:grid;grid-template-columns:1fr 1fr;gap:20px}.card{padding:28px;background:white;border:1px solid #d5e1dd;border-radius:20px}.button{display:inline-block;padding:14px 20px;background:#086963;color:white;border-radius:12px;text-decoration:none;font-weight:600;margin:8px 0;min-height:50px}.button.secondary{background:#eaf3ef;color:#086963}.note{font-size:0.875rem}.technical{padding:24px 0}.technical code{background:#eaf3ef;padding:3px 8px;border-radius:6px;color:#162b33;overflow-wrap:anywhere}footer{border-top:1px solid #d5e1dd;margin-top:32px;padding-top:24px;color:#4d6269;font-size:0.875rem;line-height:1.65}:focus-visible{outline:3px solid #b66022;outline-offset:4px}@media(max-width:600px){body{padding:24px 18px}.hero{padding:40px 0 24px}h1{font-size:2rem}h2{font-size:1.375rem}.routes{grid-template-columns:1fr}.card{padding:24px}.button{display:block;text-align:center}header span{max-width:100%;text-align:left}}
</style>
</head>
<body><div class="shell">
<header><div class="brand">Mnet</div><span>Private-network development build</span></header>
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

const approveHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Approve device - Mnet</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;margin:0;background:#0b0f14;color:#e6edf3;display:flex;min-height:100vh;align-items:center;justify-content:center}
.card{max-width:560px;padding:2rem}
a{color:#58a6ff}
code{background:#161b22;padding:.15rem .4rem;border-radius:6px}
</style>
</head>
<body>
<div class="card">
<h1>A device is asking to join</h1>
<p>Node <code>%s</code> is waiting for approval. Sign in to the <a href="/admin#pending-%s">admin dashboard</a> to approve or deny it.</p>
</div>
</body>
</html>`

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mnet admin</title>
<style>
:root{color-scheme:light;font:100% system-ui,-apple-system,sans-serif;color:#162b33;background:#f4f7f6}*{box-sizing:border-box}h1,h2,p,label{overflow-wrap:anywhere}input,select{min-width:0}body{margin:0}header{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:24px max(24px,calc((100vw - 1160px)/2));border-bottom:1px solid #d5e1dd;background:white}h1{font-size:1.438rem;letter-spacing:-.03em;margin:0}main{padding:28px;max-width:1216px;margin:auto}section{margin-bottom:24px;background:white;border:1px solid #d5e1dd;border-radius:20px;padding:24px;min-width:0}h2{font-size:1.438rem;margin:0 0 16px;letter-spacing:-.025em}p{line-height:1.65}a{color:#086963;text-underline-offset:4px}table{display:block;width:100%;max-width:100%;overflow-x:auto;border-collapse:collapse;font-size:0.875rem;margin:20px 0;scrollbar-color:#9aafaa #f4f7f6}th,td{text-align:left;padding:14px 12px;border-bottom:1px solid #d5e1dd;vertical-align:middle;min-width:90px;max-width:320px;overflow-wrap:anywhere}th{color:#4d6269;font-size:0.8125rem;font-weight:650;white-space:nowrap}code{background:#eaf3ef;padding:3px 6px;border-radius:6px;font-size:0.8125rem;overflow-wrap:anywhere}button{background:#086963;border:0;color:white;padding:12px 16px;border-radius:12px;cursor:pointer;font:600 .875rem system-ui;min-height:46px}button:hover{background:#07564f}button.danger{background:#a32e2e}button.ghost{background:#eaf3ef;color:#086963}button:disabled{background:#d5e1dd;color:#4d6269;cursor:wait}input,select,textarea{background:white;border:1px solid #9aafaa;color:#162b33;padding:12px;border-radius:10px;font:inherit;min-height:46px;max-width:100%}input[type=checkbox]{min-height:0;accent-color:#086963;width:18px;height:18px}form.inline{display:flex;gap:12px;flex-wrap:wrap;align-items:center}form.inline>label{line-height:1.6;font-size:.875rem}form.inline>label>input:not([type=checkbox]),form.inline>label>select{display:block;width:100%;margin-top:6px}textarea{resize:vertical}.badge{display:inline-block;padding:4px 10px;border-radius:999px;font-size:0.75rem;white-space:nowrap}.badge.on{background:#dcf1e5;color:#175436}.badge.off{background:#edf1ef;color:#4d6269}.badge.wait{background:#fff0d2;color:#79500e}.hidden,#login.hidden{display:none!important}#login{max-width:440px;margin:60px auto;padding:28px;border:1px solid #d5e1dd;border-radius:20px;background:white;display:flex;flex-direction:column;gap:14px}#login label{font-size:0.875rem;font-weight:650}.key{font-family:ui-monospace,monospace;word-break:break-all}.muted{color:#4d6269;font-size:0.875rem;line-height:1.65}footer{padding:24px 28px;color:#4d6269;font-size:0.875rem;line-height:1.65;max-width:1216px;margin:auto}details{background:#f4f7f6;border-radius:12px;padding:16px}summary{cursor:pointer;font-weight:600;color:#086963;min-height:28px}.admin-nav{display:flex;flex-wrap:wrap;gap:10px;margin-bottom:24px}.admin-nav a{background:#eaf3ef;padding:10px 14px;border-radius:10px;text-decoration:none}.overview{padding:4px 0 20px}.overview h2{font-size:1.875rem}.overview p{max-width:760px;margin:8px 0}.eyebrow{font-size:0.75rem;letter-spacing:.08em;text-transform:uppercase;color:#086963;font-weight:700}:focus-visible{outline:3px solid #b66022;outline-offset:4px}@media(max-width:600px){header{padding:18px;align-items:flex-start;flex-wrap:wrap}main{padding:20px 14px}section{padding:20px 16px}h2{font-size:1.375rem}#login{margin:30px 18px}.overview h2{font-size:1.75rem}form.inline{align-items:stretch}form.inline>input:not([type=checkbox]),form.inline>select,form.inline>button,form.inline>label{width:100%}th,td{padding:12px 10px}.admin-nav a{font-size:0.875rem}footer{padding:20px 18px}}
</style>
</head>
<body>
<header>
<div><div class="eyebrow">Mnet / Operator</div><h1>Network dashboard</h1></div>
<div><button class="ghost hidden" id="refresh">Refresh</button> <button class="ghost hidden" id="logout">Sign out</button></div>
</header>
<div id="login" class="hidden">
<div class="eyebrow">Operator access</div><h2>Sign in to manage networks</h2><p class="muted">Use your assigned operator password. Member access is separate.</p><label for="pw">Admin password</label>
<input id="pw" type="password" placeholder="Admin password" autocomplete="current-password">
<button id="loginBtn">Sign in</button>
<div class="muted" id="loginMsg"></div>
</div>
<main id="app" class="hidden">
<div class="overview"><p class="eyebrow">Control service</p><h2>Network overview</h2><p class="muted">Inspect and manage this Mnet control service. Device approval, account membership and current connectivity are separate.</p></div>
<nav class="admin-nav" aria-label="Dashboard sections"><a href="#networkSection">Networks</a><a href="#approvalSection">Approvals</a><a href="#nodeSection">Devices</a><a href="#keySection">Auth keys</a><a href="#membershipSection">Account access</a><a href="#policySection">ACL policy</a></nav>
<section id="networkSection">
<h2>Networks</h2>
<table id="nets" tabindex="0" aria-label="Networks, scroll horizontally for all columns"><thead><tr><th>Name</th><th>ID</th><th>Created</th></tr></thead><tbody></tbody></table>
<form class="inline" id="newNet" style="margin-top:.7rem">
<label for="netName">Network name<input id="netName" placeholder="new network name" required></label>
<button>Create network</button>
</form>
</section>
<section id="approvalSection">
<h2>Pending approvals</h2>
<table id="pending" tabindex="0" aria-label="Pending device approvals, scroll horizontally for all columns"><thead><tr><th>Name</th><th>IP</th><th>Key</th><th>Asked</th><th></th></tr></thead><tbody></tbody></table>
<div class="muted" id="noPending">Nothing waiting.</div>
</section>
<section id="nodeSection">
<h2>Devices</h2>
<table id="nodes" tabindex="0" aria-label="Devices, scroll horizontally for all columns"><thead><tr><th>Name</th><th>Network</th><th>IP</th><th>User</th><th>Relay connection</th><th>Endpoints</th><th>Last seen</th><th>Key expiry</th><th></th></tr></thead><tbody></tbody></table>
</section>
<section id="keySection">
<h2>Auth keys</h2>
<table id="keys" tabindex="0" aria-label="Auth keys, scroll horizontally for all columns"><thead><tr><th>Key</th><th>Network</th><th>Note</th><th>Reusable</th><th>Uses</th><th>Expires</th><th></th></tr></thead><tbody></tbody></table>
<form class="inline" id="newKey" style="margin-top:.7rem">
<label for="keyNet">Network<select id="keyNet"></select></label>
<label for="keyNote">Note (optional)<input id="keyNote" placeholder="note (optional)"></label>
<label class="muted"><input id="keyReusable" type="checkbox"> reusable</label>
<label for="keyTTL">Lifetime, hours (0: never)<input id="keyTTL" type="number" min="0" placeholder="hours" style="width:6rem" title="Lifetime in hours, 0 for never"></label>
<button>Create key</button>
</form>
</section>
<section id="membershipSection">
<h2>Account membership grants</h2>
<p class="muted">Inspect explicit account access for one network. Private issuer/subject identifiers are not email addresses. This does not list device approval provenance. Removing membership closes account queries, not VPN device connections.</p>
<p><label>Network <select id="memberNet"></select></label> <button id="memberRead" type="button">Inspect grants</button></p>
<details id="grantDetails"><summary>Add explicit account membership</summary>
<p class="muted">Use the exact issuer and subject from your trusted identity-provider records. Do not guess from an email address, name or device approval. This grants read-only account access to that principal's approved devices in the selected network, not operator privileges or device enrollment.</p>
<form id="memberGrant" class="inline">
<label>Issuer <input id="grantIssuer" autocomplete="off" required maxlength="2048"></label>
<label>Subject <input id="grantSubject" autocomplete="off" required maxlength="1024"></label>
<label class="muted"><input id="grantVerified" type="checkbox" required> I checked this exact principal in trusted identity-provider records.</label>
<button type="submit">Review membership grant</button>
</form></details>
<p id="memberMsg" class="muted" role="status"></p>
<table id="members" tabindex="0" aria-label="Account membership grants, scroll horizontally for all columns"><thead><tr><th>Verified issuer</th><th>Subject</th><th></th></tr></thead><tbody></tbody></table>
</section>
<section class="card" id="policySection">
<h2>ACL policy</h2>
<p class="muted">JSON rules: <code>[{"src":"*","dst":"*"}]</code>. Empty means allow all. With any rule present the default is deny. src is a node name or *; dst is a node name, *, or a subnet CIDR. Exit-node selection and internet routing are unsupported; an ACL does not enable them.</p>
<p><label for="aclNet">Network <select id="aclNet"></select></label></p>
<label for="aclText">Rules (JSON)</label><textarea id="aclText" rows="8" style="width:100%;font-family:monospace"></textarea>
<p style="margin:20px 0 0"><button id="aclSave">Save policy</button> <span id="aclMsg" class="muted" role="status"></span></p>
</section>
</main>
<footer>Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au.</footer>
<script>
const $ = (s) => document.querySelector(s);
const api = async (path, body) => {
  const r = await fetch(path, body === undefined ? {} : {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  if (r.status === 401) { showLogin(); throw new Error('auth'); }
  const j = await r.json().catch(()=>({}));
  if (!r.ok) throw new Error(j.error || r.statusText);
  return j;
};
let viewGeneration=0, signingOut=false, memberRequest=0;
function clearGrant(){ $('#grantIssuer').value=''; $('#grantSubject').value=''; $('#grantVerified').checked=false; $('#grantDetails').open=false; }
function clearMembers(){ memberRequest++; $('#members tbody').replaceChildren(); $('#memberMsg').textContent=''; }
function showLogin(){ viewGeneration++; clearMembers(); clearGrant(); state=null; $('#pw').value=''; $('#login').classList.remove('hidden'); $('#app').classList.add('hidden'); $('#refresh').classList.add('hidden'); $('#logout').classList.add('hidden'); for(const selector of ['#nets tbody','#nodes tbody','#keys tbody','#pending tbody']) $(selector).innerHTML=''; $('#aclText').value=''; for(const selector of ['#keyNet','#aclNet','#memberNet']) $(selector).innerHTML=''; }

function showApp(){ $('#login').classList.add('hidden'); $('#app').classList.remove('hidden'); $('#refresh').classList.remove('hidden'); $('#logout').classList.remove('hidden'); }
function esc(s){ const d=document.createElement('div'); d.textContent=s==null?'':String(s); return d.innerHTML; }
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
function ago(t){ if(!t) return ''; const s=(Date.now()-new Date(t).getTime())/1000; if(s<60) return Math.floor(s)+'s ago'; if(s<3600) return Math.floor(s/60)+'m ago'; if(s<86400) return Math.floor(s/3600)+'h ago'; return Math.floor(s/86400)+'d ago'; }
let state=null;
async function load(){
  if(signingOut) return;
  const generation=viewGeneration;
  let next;
  try { next = await api('/admin/api/state'); } catch(e){ return; }
  if(generation!==viewGeneration) return;
  state=next; showApp(); render();
}
function netName(id){ const n=(state.networks||[]).find(n=>n.id===id); return n?n.name:id; }
function render(){
  const nets = state.networks||[], nodes=state.nodes||[], keys=state.auth_keys||[], online=state.online_by_node||{};
  $('#nets tbody').innerHTML = nets.map(n=>'<tr><td>'+esc(n.name)+'</td><td><code>'+esc(n.id)+'</code></td><td>'+ago(n.created_at)+'</td></tr>').join('') || '<tr><td colspan="3" class="muted">No networks yet.</td></tr>';
  const pending = nodes.filter(n=>!n.approved);
  $('#noPending').style.display = pending.length? 'none':'block';
  $('#pending tbody').innerHTML = pending.map(n=>'<tr id="pending-'+esc(n.id)+'"><td>'+esc(n.name)+'</td><td><code>'+esc(n.ipv4)+'</code></td><td><code>'+esc(n.pubkey.slice(0,12))+'...</code></td><td>'+ago(n.created_at)+'</td><td><button onclick="approve(\''+esc(n.id)+'\')">Approve</button> <button class="danger" onclick="delNode(\''+esc(n.id)+'\')">Deny</button></td></tr>').join('');
  const approved = nodes.filter(n=>n.approved);
  $('#nodes tbody').innerHTML = approved.map(n=>'<tr><td>'+esc(n.name)+'</td><td>'+esc(netName(n.network_id))+'</td><td><code>'+esc(n.ipv4)+'</code></td><td>'+esc(n.user||'')+'</td><td>'+(online[n.id]?'<span class="badge on">connected</span>':'<span class="badge off">not connected</span>')+'</td><td class="muted">'+esc((n.endpoints||[]).join(', '))+'</td><td>'+ago(n.last_seen)+'</td><td>'+expires(n.key_expiry)+'</td><td><button onclick="expireNode(\''+esc(n.id)+'\')">Expire key</button> <button class="danger" onclick="delNode(\''+esc(n.id)+'\')">Remove</button></td></tr>').join('') || '<tr><td colspan="10" class="muted">No approved nodes.</td></tr>';
  $('#keys tbody').innerHTML = keys.map(k=>'<tr><td class="key"><code>'+esc(k.key)+'</code></td><td>'+esc(netName(k.network_id))+'</td><td>'+esc(k.note)+'</td><td>'+(k.reusable?'yes':'no')+'</td><td>'+k.uses+'</td><td>'+expires(k.expires_at)+'</td><td><button class="danger" onclick="delKey(\''+esc(k.id)+'\')">Delete</button></td></tr>').join('') || '<tr><td colspan="7" class="muted">No keys.</td></tr>';
  $('#keyNet').innerHTML = nets.map(n=>'<option value="'+esc(n.id)+'">'+esc(n.name)+'</option>').join('');
  $('#aclNet').innerHTML = $('#keyNet').innerHTML;
  const selectedMember=$('#memberNet').value;
  $('#memberNet').innerHTML = $('#keyNet').innerHTML;
  if(nets.some(n=>n.id===selectedMember)) $('#memberNet').value=selectedMember;
  if($('#memberNet').value!==selectedMember){ clearMembers(); clearGrant(); }
  loadACL();
}
async function loadMembers(){
  if(signingOut || !state) return;
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
      remove.onclick=()=>revokeMember(id,member.issuer,member.subject,generation,request);
      action.append(remove); row.append(action);
      return row;
    });
    $('#members tbody').replaceChildren(...rows);
    $('#memberMsg').textContent=rows.length ? rows.length+' explicit grant(s).' : 'No explicit grants for this network.';
  } catch(e){
    if(generation===viewGeneration && request===memberRequest && !signingOut) $('#memberMsg').textContent='Could not read grants. Try again.';
  }
}
async function revokeMember(network,issuer,subject,generation,request){
  const current=()=>generation===viewGeneration && request===memberRequest && !signingOut && state && $('#memberNet').value===network;
  if(!current()) return;
  if(!confirm('Remove account membership for network '+network+'?\nIssuer: '+issuer+'\nSubject: '+subject+'\nThis closes account queries. It does NOT disconnect VPN devices or revoke their credentials.')) return;
  if(!current()) return;
  clearMembers();
  const operation=memberRequest;
  $('#memberMsg').textContent='Removing membership...';
  try {
    const response=await fetch('/admin/api/memberships',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({network_id:network,issuer,subject,allow:false})});
    if(generation!==viewGeneration || operation!==memberRequest || signingOut || $('#memberNet').value!==network) return;
    if(response.status===401){ showLogin(); return; }
    if(!response.ok) throw new Error('revoke failed');
    await loadMembers();
  } catch(e){
    if(generation===viewGeneration && operation===memberRequest && !signingOut) $('#memberMsg').textContent='Could not confirm removal. Inspect grants again before retrying. VPN device connections are separate.';
  }
}
$('#memberNet').onchange=()=>{clearMembers();clearGrant();};
$('#memberGrant').onsubmit=async event=>{
  event.preventDefault();
  const generation=viewGeneration, request=memberRequest, network=$('#memberNet').value;
  const issuer=$('#grantIssuer').value, subject=$('#grantSubject').value;
  const current=()=>generation===viewGeneration && request===memberRequest && !signingOut && state && $('#memberNet').value===network;
  if(!current() || !network || !issuer || !subject || !$('#grantVerified').checked) return;
  if(!confirm('Grant account membership?\nNetwork: '+network+'\nIssuer: '+issuer+'\nSubject: '+subject+'\nOnly continue if this exact principal was verified in trusted identity-provider records. This is not enrollment, operator access or VPN connectivity.')) return;
  if(!current()) return;
  clearMembers(); clearGrant(); const operation=memberRequest;
  $('#memberMsg').textContent='Adding membership...';
  try{
    const response=await fetch('/admin/api/memberships',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({network_id:network,issuer,subject,allow:true})});
    if(generation!==viewGeneration || operation!==memberRequest || signingOut || $('#memberNet').value!==network) return;
    if(response.status===401){showLogin();return;}
    if(!response.ok) throw new Error('grant failed');
    await loadMembers();
  }catch{
    if(generation===viewGeneration && operation===memberRequest && !signingOut) $('#memberMsg').textContent='Could not confirm grant. Inspect grants again before retrying.';
  }
};
$('#memberRead').onclick=loadMembers;
async function loadACL(){
  const generation=viewGeneration;
  const id=$('#aclNet').value;
  if(!id){ $('#aclText').value=''; return; }
  const r=await fetch('/admin/api/acl?network_id='+encodeURIComponent(id));
  if(!r.ok) return;
  const j=await r.json();
  if(generation!==viewGeneration || $('#aclNet').value!==id) return;
  $('#aclText').value=JSON.stringify(j.rules||[],null,2);
}
$('#aclNet').onchange=loadACL;
$('#aclSave').onclick=async ()=>{
  try{
    const rules=JSON.parse($('#aclText').value||'[]');
    const r=await fetch('/admin/api/acl',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({network_id:$('#aclNet').value,rules})});
    const j=await r.json().catch(()=>({}));
    $('#aclMsg').textContent=r.ok?'Saved.':('Error: '+(j.error||r.statusText));
  }catch(e){ $('#aclMsg').textContent='Bad JSON: '+e.message; }
};
async function approve(id){ await api('/admin/api/nodes/approve',{node_id:id}); load(); }
async function delNode(id){ if(!confirm('Remove this node?')) return; await api('/admin/api/nodes/delete',{node_id:id}); load(); }
async function delKey(id){ await api('/admin/api/authkeys/delete',{id}); load(); }
async function expireNode(id){ if(!confirm('Expire this node key now? The device must rejoin.')) return; await api('/admin/api/nodes/expire',{node_id:id}); load(); }
$('#loginBtn').onclick = async ()=>{ try{ await api('/admin/api/login',{password:$('#pw').value}); $('#pw').value=''; $('#loginMsg').textContent=''; load(); }catch(e){ $('#loginMsg').textContent='Wrong password.'; } };
$('#pw').addEventListener('keydown', e=>{ if(e.key==='Enter') $('#loginBtn').click(); });
$('#refresh').onclick = load;
$('#logout').onclick = async ()=>{
  viewGeneration++; signingOut=true; clearMembers(); clearGrant();
  $('#logout').disabled=true;
  try { await api('/admin/api/logout',{}); showLogin(); $('#loginMsg').textContent='Signed out.'; }
  catch(e){ if(e.message!=='auth') alert('Sign out failed: '+e.message); }
  finally { signingOut=false; $('#logout').disabled=false; }
};
$('#newNet').onsubmit = async (e)=>{ e.preventDefault(); try{ await api('/admin/api/networks',{name:$('#netName').value}); $('#netName').value=''; load(); }catch(err){ alert(err.message); } };
$('#newKey').onsubmit = async (e)=>{ e.preventDefault(); try{ const k=await api('/admin/api/authkeys',{network_id:$('#keyNet').value,note:$('#keyNote').value,reusable:$('#keyReusable').checked,ttl_hours:parseInt($('#keyTTL').value||'0')}); $('#keyNote').value=''; $('#keyTTL').value=''; load(); prompt('New auth key (also visible in the admin key table):', k.key); }catch(err){ alert(err.message); } };
load();
setInterval(()=>{ if(!$('#app').classList.contains('hidden')) load(); }, 10000);
</script>
</body>
</html>`
