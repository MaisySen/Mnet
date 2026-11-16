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
body{font-family:system-ui,-apple-system,sans-serif;margin:0;background:#0b0f14;color:#e6edf3;display:flex;min-height:100vh;align-items:center;justify-content:center}
.card{max-width:560px;padding:2rem}
h1{font-size:2rem;margin:0 0 .5rem}
p{line-height:1.55;color:#9da7b1}
a{color:#58a6ff}
code{background:#161b22;padding:.15rem .4rem;border-radius:6px}
</style>
</head>
<body>
<div class="card">
<h1>Mnet</h1>
<p>Your devices, one private mesh. WireGuard encrypted, peer to peer when possible, relayed when it has to be.</p>
<p>Get the client, then run <code>mnet up</code> and point it at this server.</p>
<p>Admins: <a href="/admin">open the dashboard</a>.</p>
<p style="margin-top:2rem;font-size:.85rem">Mnet is provided by <a href="https://fintex.au">Fintex Australia Pty Ltd</a> (ABN 60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au.</p>
</div>
</body>
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
:root{color-scheme:dark}
body{font-family:system-ui,-apple-system,sans-serif;margin:0;background:#0b0f14;color:#e6edf3}
header{display:flex;align-items:center;justify-content:space-between;padding:1rem 1.5rem;border-bottom:1px solid #21262d}
h1{font-size:1.1rem;margin:0}
main{padding:1.5rem;max-width:1100px;margin:0 auto}
section{margin-bottom:2rem}
h2{font-size:1rem;margin:0 0 .75rem;color:#9da7b1;text-transform:uppercase;letter-spacing:.05em}
table{width:100%;border-collapse:collapse;font-size:.9rem}
th,td{text-align:left;padding:.5rem .6rem;border-bottom:1px solid #21262d;vertical-align:middle}
th{color:#9da7b1;font-weight:600}
code{background:#161b22;padding:.1rem .35rem;border-radius:6px;font-size:.82rem}
button{background:#238636;border:0;color:#fff;padding:.4rem .8rem;border-radius:6px;cursor:pointer;font-size:.85rem}
button.danger{background:#da3633}
button.ghost{background:#21262d}
input,select{background:#0d1117;border:1px solid #30363d;color:#e6edf3;padding:.45rem .6rem;border-radius:6px;font-size:.9rem}
form.inline{display:flex;gap:.5rem;flex-wrap:wrap;align-items:center}
.badge{display:inline-block;padding:.12rem .5rem;border-radius:999px;font-size:.75rem}
.badge.on{background:#1a7f37}
.badge.off{background:#30363d;color:#9da7b1}
.badge.wait{background:#9e6a03}
.hidden{display:none}
#login.hidden{display:none}
#login{max-width:340px;margin:15vh auto;display:flex;flex-direction:column;gap:.7rem}
.key{font-family:ui-monospace,monospace;word-break:break-all}
.muted{color:#9da7b1;font-size:.85rem}
footer{padding:1.5rem;color:#9da7b1;font-size:.8rem}
</style>
</head>
<body>
<header>
<h1>Mnet admin</h1>
<div><button class="ghost hidden" id="refresh">Refresh</button> <button class="ghost hidden" id="logout">Sign out</button></div>
</header>
<div id="login" class="hidden">
<h2>Sign in</h2>
<input id="pw" type="password" placeholder="Admin password" autocomplete="current-password">
<button id="loginBtn">Sign in</button>
<div class="muted" id="loginMsg"></div>
</div>
<main id="app" class="hidden">
<section>
<h2>Networks</h2>
<table id="nets"><thead><tr><th>Name</th><th>ID</th><th>Created</th></tr></thead><tbody></tbody></table>
<form class="inline" id="newNet" style="margin-top:.7rem">
<input id="netName" placeholder="new network name" required>
<button>Create network</button>
</form>
</section>
<section>
<h2>Pending approvals</h2>
<table id="pending"><thead><tr><th>Name</th><th>IP</th><th>Key</th><th>Asked</th><th></th></tr></thead><tbody></tbody></table>
<div class="muted" id="noPending">Nothing waiting.</div>
</section>
<section>
<h2>Nodes</h2>
<table id="nodes"><thead><tr><th>Name</th><th>Network</th><th>IP</th><th>User</th><th>Status</th><th>Endpoints</th><th>Last seen</th><th>Key expiry</th><th></th></tr></thead><tbody></tbody></table>
</section>
<section>
<h2>Auth keys</h2>
<table id="keys"><thead><tr><th>Key</th><th>Network</th><th>Note</th><th>Reusable</th><th>Uses</th><th>Expires</th><th></th></tr></thead><tbody></tbody></table>
<form class="inline" id="newKey" style="margin-top:.7rem">
<select id="keyNet"></select>
<input id="keyNote" placeholder="note (optional)">
<label class="muted"><input id="keyReusable" type="checkbox"> reusable</label>
<input id="keyTTL" type="number" min="0" placeholder="hours" style="width:6rem" title="Lifetime in hours, 0 for never">
<button>Create key</button>
</form>
</section>
<section class="card">
<h2>ACL policy</h2>
<p class="muted">JSON rules: <code>[{"src":"*","dst":"*"}]</code>. Empty means allow all. With any rule present the default is deny. src is a node name or *; dst is a node name, *, or a subnet CIDR. Exit-node selection and internet routing are unsupported; an ACL does not enable them.</p>
<p><select id="aclNet"></select> <button id="aclSave">Save policy</button> <span id="aclMsg" class="muted"></span></p>
<textarea id="aclText" rows="8" style="width:100%;font-family:monospace"></textarea>
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
let viewGeneration=0, signingOut=false;
function showLogin(){ viewGeneration++; state=null; $('#pw').value=''; $('#login').classList.remove('hidden'); $('#app').classList.add('hidden'); $('#refresh').classList.add('hidden'); $('#logout').classList.add('hidden'); for(const selector of ['#nets tbody','#nodes tbody','#keys tbody','#pending tbody']) $(selector).innerHTML=''; $('#aclText').value=''; for(const selector of ['#keyNet','#aclNet']) $(selector).innerHTML=''; }

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
  const nets = state.networks||[], nodes=state.nodes||[], keys=state.auth_keys||[], online=state.online||{};
  $('#nets tbody').innerHTML = nets.map(n=>'<tr><td>'+esc(n.name)+'</td><td><code>'+esc(n.id)+'</code></td><td>'+ago(n.created_at)+'</td></tr>').join('') || '<tr><td colspan="3" class="muted">No networks yet.</td></tr>';
  const pending = nodes.filter(n=>!n.approved);
  $('#noPending').style.display = pending.length? 'none':'block';
  $('#pending tbody').innerHTML = pending.map(n=>'<tr id="pending-'+esc(n.id)+'"><td>'+esc(n.name)+'</td><td><code>'+esc(n.ipv4)+'</code></td><td><code>'+esc(n.pubkey.slice(0,12))+'...</code></td><td>'+ago(n.created_at)+'</td><td><button onclick="approve(\''+esc(n.id)+'\')">Approve</button> <button class="danger" onclick="delNode(\''+esc(n.id)+'\')">Deny</button></td></tr>').join('');
  const approved = nodes.filter(n=>n.approved);
  $('#nodes tbody').innerHTML = approved.map(n=>'<tr><td>'+esc(n.name)+'</td><td>'+esc(netName(n.network_id))+'</td><td><code>'+esc(n.ipv4)+'</code></td><td>'+esc(n.user||'')+'</td><td>'+(online[n.pubkey]?'<span class="badge on">online</span>':'<span class="badge off">offline</span>')+'</td><td class="muted">'+esc((n.endpoints||[]).join(', '))+'</td><td>'+ago(n.last_seen)+'</td><td>'+expires(n.key_expiry)+'</td><td><button onclick="expireNode(\''+esc(n.id)+'\')">Expire key</button> <button class="danger" onclick="delNode(\''+esc(n.id)+'\')">Remove</button></td></tr>').join('') || '<tr><td colspan="10" class="muted">No approved nodes.</td></tr>';
  $('#keys tbody').innerHTML = keys.map(k=>'<tr><td class="key"><code>'+esc(k.key)+'</code></td><td>'+esc(netName(k.network_id))+'</td><td>'+esc(k.note)+'</td><td>'+(k.reusable?'yes':'no')+'</td><td>'+k.uses+'</td><td>'+expires(k.expires_at)+'</td><td><button class="danger" onclick="delKey(\''+esc(k.id)+'\')">Delete</button></td></tr>').join('') || '<tr><td colspan="7" class="muted">No keys.</td></tr>';
  $('#keyNet').innerHTML = nets.map(n=>'<option value="'+esc(n.id)+'">'+esc(n.name)+'</option>').join('');
  $('#aclNet').innerHTML = $('#keyNet').innerHTML;
  loadACL();
}
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
  viewGeneration++; signingOut=true;
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
