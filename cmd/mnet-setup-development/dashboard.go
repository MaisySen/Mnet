//go:build mnet_enrollment_development

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"fintex.au/mnet/internal/daemon"
)

// One finite run, literal loopback only. No registrar credentials enter this UI.
type developmentDashboard struct {
	handlers                                    sync.WaitGroup
	handlerMu                                   sync.Mutex
	closing                                     bool
	mu                                          sync.Mutex
	server                                      *http.Server
	listener                                    net.Listener
	origin, bootstrap, cookie, csrf, cookieName string
	used, ended, finishing, stopStarted         bool
	reason                                      string
	session                                     *daemon.DevelopmentNetworkSession
	snapshot                                    func() []daemon.DevelopmentNetworkPeer
	done                                        <-chan struct{}
	stop                                        func()
	stopFinished                                chan struct{}
	exited                                      chan struct{}
}
type dashboardView struct {
	State  string                          `json:"state"`
	Reason string                          `json:"reason"`
	Peers  []daemon.DevelopmentNetworkPeer `json:"peers"`
}

func dashboardSecret() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func newDevelopmentDashboard(s *daemon.DevelopmentNetworkSession) (*developmentDashboard, error) {
	return startDevelopmentDashboard(s, s.Peers, s.Done(), s.Close)
}
func startDevelopmentDashboard(s *daemon.DevelopmentNetworkSession, snapshot func() []daemon.DevelopmentNetworkPeer, done <-chan struct{}, stop func()) (*developmentDashboard, error) {
	d := &developmentDashboard{session: s, snapshot: snapshot, done: done, stop: stop, exited: make(chan struct{}), stopFinished: make(chan struct{})}
	var err error
	for _, target := range []*string{&d.bootstrap, &d.cookie, &d.csrf} {
		*target, err = dashboardSecret()
		if err != nil {
			return nil, err
		}
	}
	d.listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	d.cookieName = "mnet_run_" + d.csrf[:16]
	d.origin = "http://" + d.listener.Addr().String()
	d.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.handlerMu.Lock()
		if d.closing {
			d.handlerMu.Unlock()
			http.Error(w, "Unavailable", 503)
			return
		}
		d.handlers.Add(1)
		d.handlerMu.Unlock()
		defer d.handlers.Done()
		d.serve(w, r)
	}), ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192}
	go func() { defer close(d.exited); _ = d.server.Serve(d.listener) }()
	return d, nil
}
func dashboardEqual(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (d *developmentDashboard) URL() string { return d.origin + "/open/" + d.bootstrap }
func (d *developmentDashboard) view() dashboardView {
	d.mu.Lock()
	ended, reason := d.ended, d.reason
	d.mu.Unlock()
	if ended {
		return dashboardView{State: "ended", Reason: reason, Peers: []daemon.DevelopmentNetworkPeer{}}
	}
	select {
	case <-d.done:
		return dashboardView{State: "stopping", Reason: "Waiting for this run's owners to finish.", Peers: []daemon.DevelopmentNetworkPeer{}}
	default:
	}
	peers := d.snapshot()
	// Recheck retirement after the snapshot, never render stale active peers.
	select {
	case <-d.done:
		return dashboardView{State: "stopping", Peers: []daemon.DevelopmentNetworkPeer{}}
	default:
	}
	return dashboardView{State: "active", Peers: peers}
}
func (d *developmentDashboard) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+d.csrf+"'; style-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || r.Host != strings.TrimPrefix(d.origin, "http://") {
		http.Error(w, "Unavailable", 403)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/open/") && r.Method == http.MethodGet {
		d.mu.Lock()
		valid := !d.used && dashboardEqual(strings.TrimPrefix(r.URL.Path, "/open/"), d.bootstrap)
		if valid {
			d.used = true
		}
		d.mu.Unlock()
		if !valid {
			http.Error(w, "Unavailable", 403)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: d.cookieName, Value: d.cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	c, err := r.Cookie(d.cookieName)
	if err != nil || !dashboardEqual(c.Value, d.cookie) {
		http.Error(w, "Unavailable", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != d.origin {
		http.Error(w, "Unavailable", 403)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, dashboardHTML, d.csrf, d.csrf)
	case r.Method == http.MethodGet && r.URL.Path == "/state":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(d.view())
	case r.Method == http.MethodPost && r.URL.Path == "/stop":
		if r.Header.Get("Origin") != d.origin || r.Header.Get("Content-Type") != "application/json" || !dashboardEqual(r.Header.Get("X-Mnet-Run"), d.csrf) || r.ContentLength != 2 {
			http.Error(w, "Unavailable", 403)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 3))
		if err != nil || string(body) != "{}" {
			http.Error(w, "Unavailable", 403)
			return
		}
		d.mu.Lock()
		if !d.finishing && !d.ended && !d.stopStarted {
			d.stopStarted = true
			go func() { defer close(d.stopFinished); d.stop() }()
		}
		d.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(w, r)
	}
}

// Called only AFTER the owning enrollment/network function has returned/joined.
func (d *developmentDashboard) Finish(reason string) {
	d.mu.Lock()
	d.finishing = true
	started := d.stopStarted
	d.mu.Unlock()
	if started {
		<-d.stopFinished
	}
	d.mu.Lock()
	d.ended = true
	d.reason = reason
	d.mu.Unlock()
}
func (d *developmentDashboard) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = d.server.Shutdown(ctx)
	_ = d.listener.Close()
	_ = d.server.Close()
	<-d.exited
	d.handlerMu.Lock()
	d.closing = true
	d.handlerMu.Unlock()
	d.handlers.Wait()
}

const dashboardHTML = `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Mnet disposable network</title><style>body{font:17px system-ui;background:#f4f5f7;color:#202124;padding:48px 20px}main{max-width:680px;margin:auto;background:white;border:1px solid #ddd;border-radius:18px;padding:30px}p,li{line-height:1.6}button{font:inherit;padding:12px 24px;min-height:48px}small{color:#596171}</style><main><small>DISPOSABLE DEVELOPMENT NETWORK</small><h1 id="title">Checking this run</h1><p>Joining saved. This is separate from the finite network session below.</p><p id="status">Waiting for the native owner.</p><ul id="peers"></ul><button id="stop" disabled>Stop network</button><p>Screen, keyboard and mouse access are unavailable. No OS interface, service or automatic reconnect is started.</p><small id="foot">Only this run can control this page.</small></main><script nonce="%s">const token="%s";let ended=false;const title=document.getElementById('title'),status=document.getElementById('status'),peers=document.getElementById('peers'),stop=document.getElementById('stop');async function update(){if(ended)return;try{const r=await fetch('/state',{cache:'no-store'});if(!r.ok)throw 0;const s=await r.json();peers.replaceChildren();for(const p of s.peers){const li=document.createElement('li');li.textContent=p.Name+' · '+p.IPv4;peers.append(li)}stop.disabled=s.state!=='active';title.textContent=s.state==='active'?'Network active':s.state==='ended'?'Network ended':'Stopping network';status.textContent=s.state==='active'?'Fresh encrypted TCP was verified. These peers are from the current validated native snapshot.':s.state==='ended'?s.reason:'Waiting for this run\'s owners to finish.';if(s.state==='ended'){ended=true;document.getElementById('foot').textContent='Historical result. No live network remains in this run.'}}catch{stop.disabled=true;peers.replaceChildren();title.textContent='Live status unavailable';status.textContent='Do not assume this computer is connected or stopped. Check the native run.'}}stop.onclick=async()=>{stop.disabled=true;try{const r=await fetch('/stop',{method:'POST',headers:{'Content-Type':'application/json','X-Mnet-Run':token},body:'{}'});if(!r.ok)throw 0;await update()}catch{status.textContent='Stop was not confirmed. Check the native run.'}};update();setInterval(update,100);</script>`
