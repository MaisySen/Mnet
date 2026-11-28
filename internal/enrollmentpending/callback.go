package enrollmentpending

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	config "fintex.au/mnet/internal/enrollmentconfig"
	contract "fintex.au/mnet/internal/enrollmentcontract"
	"fintex.au/mnet/internal/handoffview"
)

// Callback owns a fresh literal loopback socket for ONE saved intent. It neither
// opens a browser nor exchanges a code or installs a credential. The caller must
// independently own the actual durable native key and installed/current policy.
// Copies share the private lifecycle. The socket is never recovered from disk.
type Callback struct{ *callback }
type callback struct {
	journal   *Journal
	listener  net.Listener
	server    *http.Server
	launch    contract.SourceLaunch
	clock     func() time.Time
	initial   time.Time
	started   time.Time
	done      chan struct{}
	once      sync.Once
	admission func() error
}

func (Callback) String() string   { return "[private enrollment callback owner]" }
func (Callback) GoString() string { return "[private enrollment callback owner]" }

// PrepareCallback owns a socket BEFORE Begin durably pins its exact address.
// It returns no launch on a failed save or lifecycle recheck. Existing intent
// is never reset, and an intent saved before a later failure stays suspended.
// Explicit expiry is bounded to 15 minutes; no lifetime default is selected.
func (j *Journal) PrepareCallback(publicKey, platform string, policy *config.LocalPolicyV2, clock func() time.Time, expires time.Time) (*Callback, contract.SourceLaunch, error) {
	return j.PrepareCheckedCallback(publicKey, platform, policy, clock, expires, nil)
}

// PrepareCheckedCallback additionally requires the caller's own admission check
// before recording callback DATA. This hook is NOT an authority grant. It runs
// outside the journal mutex; the caller must own its lifecycle/config/key checks.
func (j *Journal) PrepareCheckedCallback(publicKey, platform string, policy *config.LocalPolicyV2, clock func() time.Time, expires time.Time, admission func() error) (*Callback, contract.SourceLaunch, error) {

	if clock == nil {
		return nil, contract.SourceLaunch{}, ErrInvalid
	}
	started := time.Now()
	now := clock()
	if now.IsZero() || !expires.After(now) || expires.Sub(now) > 15*time.Minute {
		return nil, contract.SourceLaunch{}, ErrInvalid
	}
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, contract.SourceLaunch{}, err
	}
	ok := false
	defer func() {
		if !ok {
			l.Close()
		}
	}()
	address, valid := l.Addr().(*net.TCPAddr)
	if !valid || !address.IP.Equal(net.IPv4(127, 0, 0, 1)) || address.Port < 1024 {
		return nil, contract.SourceLaunch{}, ErrInvalid
	}
	uri := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(address.Port)) + "/mnet/enrollment/callback"
	launch, err := j.Begin(publicKey, platform, uri, policy, now, expires)
	if err != nil {
		return nil, contract.SourceLaunch{}, err
	}
	c := &Callback{&callback{journal: j, listener: &boundedCallbackListener{Listener: l}, launch: launch, clock: clock, initial: now, started: started, admission: admission, done: make(chan struct{})}}
	c.server = &http.Server{Handler: c, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192}
	c.server.SetKeepAlivesEnabled(false)
	postNow := clock()
	elapsed := now.Add(time.Since(started))
	if postNow.Before(now) {
		c.Close()
		return nil, contract.SourceLaunch{}, ErrInvalid
	}
	if postNow.Before(elapsed) {
		postNow = elapsed
	}
	j.mu.Lock()
	if err = j.health(); err == nil {
		if j.callback != nil || j.data == nil || j.data.Phase != "prepared" || j.data.Launch != launch || !live(*j.data, postNow) {
			err = ErrInvalid
		} else {
			j.callback = c
		}
	}
	j.mu.Unlock()
	if err != nil {
		c.Close()
		return nil, contract.SourceLaunch{}, err
	}
	ok = true
	go func() {
		err := c.server.Serve(c.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.finish()
		}
	}()
	go func() {
		remaining := expires.Sub(now) - time.Since(started)
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-timer.C:
			c.Close()
		case <-c.done:
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if c.server.Shutdown(ctx) != nil {
				c.server.Close()
			}
		}
	}()
	return c, launch, nil
}
func (c *Callback) finish()      { c.once.Do(func() { close(c.done); c.listener.Close() }) }
func (c *Callback) Close() error { c.finish(); return c.server.Close() }
func (c *Callback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Connection", "close")
	r.Close = true
	fail := func() { http.Error(w, "Mnet callback unavailable.", http.StatusBadRequest) }
	select {
	case <-c.done:
		fail()
		return
	default:
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host != "127.0.0.1" || r.Method != http.MethodGet || r.Host != c.listener.Addr().String() || r.URL.IsAbs() || r.URL.Path != "/mnet/enrollment/callback" || r.URL.RawPath != "" || r.URL.Fragment != "" || len(r.URL.RawQuery) > 1024 || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || len(r.Header.Values("Origin")) != 0 {
		fail()
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 3 {
		fail()
		return
	}
	for _, values := range query {
		if len(values) != 1 {
			fail()
			return
		}
	}
	if query.Get("state") != c.launch.State || query.Get("attemptId") != c.launch.AttemptID {
		fail()
		return
	}
	if c.admission != nil && c.admission() != nil {
		fail()
		return
	}
	now := c.clock()
	if now.Before(c.initial) {
		fail()
		return
	}
	elapsed := c.initial.Add(time.Since(c.started))
	if now.Before(elapsed) {
		now = elapsed
	}
	if code, ok := query["code"]; ok {
		if !token(code[0]) || c.journal.RecordCallback(c.launch.AttemptID, c.launch.State, code[0], now) != nil {
			fail()
			return
		}
		c.finish()
		handoffview.Write(w, "Approval returned to Mnet.", "Return to Mnet to check whether this computer has joined the network. Screen sharing and remote control require separate consent.")
		return
	}
	if query.Get("error") != "access_denied" || c.journal.recordDecline(c.launch.AttemptID, c.launch.State, now) != nil {
		fail()
		return
	}
	c.finish()
	handoffview.Write(w, "Computer not approved.", "You can start a new joining request from Mnet.")
}
func (j *Journal) recordDecline(attempt, state string, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.health(); err != nil {
		return err
	}
	if j.data == nil || j.data.Phase != "prepared" || j.reopened || (j.callback != nil && !j.callback.active()) || !live(*j.data, now) || attempt != j.data.Launch.AttemptID || state != j.data.Launch.State {
		return ErrInvalid
	}
	r := *j.data
	r.Phase = "declined-data"
	return j.commit(r)
}

// Bound live accepted sockets independently of handler/query admission.
type boundedCallbackListener struct {
	net.Listener
	active atomic.Int32
}

func (l *boundedCallbackListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if l.active.Add(1) > 8 {
			l.active.Add(-1)
			conn.Close()
			continue
		}
		return &callbackConn{Conn: conn, owner: l}, nil
	}
}

type callbackConn struct {
	net.Conn
	owner *boundedCallbackListener
	once  sync.Once
}

func (c *callbackConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.owner.active.Add(-1) })
	return err
}

func (c *Callback) active() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}
