package enrollmentpending

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func callbackFixture(t *testing.T) (*Journal, *Callback, contract.SourceLaunch, time.Time) {
	t.Helper()
	j, err := Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	now := time.Now()
	mapping := contract.SourceMapping{WorkspaceID: "org", Issuer: "https://issuer.test", RegistrarOrigin: "https://registrar.test", Audience: "fixture", MappingRevision: "r1", NetworkID: "net"}
	c, l, err := j.PrepareCallback(code(), "linux", fixturePolicy(t, "https://source.test", mapping), func() time.Time { return now }, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return j, c, l, now
}
func callbackURL(l contract.SourceLaunch, decision string) string {
	q := url.Values{"state": {l.State}, "attemptId": {l.AttemptID}}
	if decision == "allow" {
		q.Set("code", code())
	} else {
		q.Set("error", "access_denied")
	}
	return l.RedirectURI + "?" + q.Encode()
}
func callbackHTTP(t *testing.T, method, target, host, origin string) (int, string) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	r, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	if r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("private callback cache/referrer headers absent")
	}
	return r.StatusCode, string(body)
}
func TestOwnedCallbackExactSuccessAndDeclinePersistBeforeReply(t *testing.T) {
	for _, kind := range []string{"allow", "decline"} {
		t.Run(kind, func(t *testing.T) {
			j, c, l, _ := callbackFixture(t)
			status, body := callbackHTTP(t, "GET", callbackURL(l, kind), "", "")
			if status != 200 || strings.Contains(body, l.State) || strings.Contains(body, code()) {
				t.Fatal("callback failed or private data echoed", status)
			}
			raw, err := os.ReadFile(j.path)
			if err != nil {
				t.Fatal(err)
			}
			var saved record
			if !strictRecord(raw, &saved) || !validRecord(saved) {
				t.Fatal("reply preceded durable valid record")
			}
			phase := "callback-recorded"
			if kind == "decline" {
				phase = "declined-data"
			}
			if saved.Phase != phase {
				t.Fatal("wrong durable phase", saved.Phase)
			}
			select {
			case <-c.done:
			case <-time.After(time.Second):
				t.Fatal("callback didn't retire socket")
			}
			if _, err := net.DialTimeout("tcp", c.listener.Addr().String(), time.Second); err == nil {
				t.Fatal("retired socket accepts")
			}
			dir := j.owner.Parent()
			j.Close()
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if state, _ := reopened.Status(); state != phase {
				t.Fatal("restart lost decision")
			}
		})
	}
}
func TestOwnedCallbackRejectsWrongHTTPWithoutConsumingPreparedIntent(t *testing.T) {
	for _, kind := range []string{"method", "host", "origin", "path", "encoded-path", "duplicate", "extra", "state", "attempt", "bad-code", "bad-error", "code-and-error", "bad-query"} {
		t.Run(kind, func(t *testing.T) {
			j, _, l, _ := callbackFixture(t)
			target := callbackURL(l, "allow")
			method := "GET"
			host, origin := "", ""
			switch kind {
			case "method":
				method = "POST"
			case "host":
				host = "localhost:1234"
			case "origin":
				origin = "https://source.test"
			case "path":
				target = strings.Replace(target, "/mnet/enrollment/callback", "/other", 1)
			case "encoded-path":
				target = strings.Replace(target, "/mnet/enrollment/callback", "/mnet/enrollment/%63allback", 1)
			case "duplicate":
				target += "&state=" + l.State
			case "extra":
				target += "&other=value"
			case "state":
				target = strings.Replace(target, l.State, code(), 1)
			case "attempt":
				target = strings.Replace(target, l.AttemptID, code(), 1)
			case "bad-code":
				target = strings.Replace(target, "code="+code(), "code=bad", 1)
			case "bad-error":
				target = strings.Replace(callbackURL(l, "decline"), "access_denied", "other", 1)
			case "code-and-error":
				target += "&error=access_denied"
			case "bad-query":
				target += "&broken=%xx"
			}
			status, _ := callbackHTTP(t, method, target, host, origin)
			if status != 400 {
				t.Fatal("invalid callback accepted", status)
			}
			if phase, _ := j.Status(); phase != "prepared" {
				t.Fatal("invalid callback consumed intent")
			}
			status, _ = callbackHTTP(t, "GET", callbackURL(l, "allow"), "", "")
			if status != 200 {
				t.Fatal("invalid request retired valid intent")
			}
		})
	}
}
func TestOwnedCallbackCloseAndSaveFailureNeverAdvanceOrReplaceIntent(t *testing.T) {
	j, c, l, now := callbackFixture(t)
	original, _ := os.ReadFile(j.path)
	j.fs.rename = func(string, string) error { return injected }
	status, _ := callbackHTTP(t, "GET", callbackURL(l, "allow"), "", "")
	if status != 400 {
		t.Fatal("failed save returned success")
	}
	after, _ := os.ReadFile(j.path)
	if !bytes.Equal(after, original) {
		t.Fatal("failed callback changed saved intent")
	}
	j.Close()
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("journal Close left callback open")
	}
	dir := filepath.Dir(j.path)
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.RecordCallback(l.AttemptID, l.State, code(), now); !errors.Is(err, ErrInvalid) {
		t.Fatal("restart adopted old socket", err)
	}
}
func TestOwnedCallbackPreparationFailureReturnsNoLaunchAndReleasesSocket(t *testing.T) {
	j, _, _, _ := prepared(t)
	snapshot := savedPolicy(t, j)
	now := time.Now()
	c, l, err := j.PrepareCallback(code(), "linux", snapshot, time.Now, now.Add(time.Minute))
	if !errors.Is(err, ErrPending) || c != nil || l != (contract.SourceLaunch{}) {
		t.Fatal("existing intent replaced", err)
	}
}
func TestOwnedCallbackExplicitCloseRetainsPreparedRecoveryState(t *testing.T) {
	j, c, _, _ := callbackFixture(t)
	copyOwner := *c
	copyOwner.Close()
	if phase, _ := j.Status(); phase != "prepared" {
		t.Fatal("closing listener fabricated consent")
	}
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("copied callback diverged lifecycle")
	}
}

func TestOwnedCallbackDeadlineClosesSocketWithoutCallbackOrExchange(t *testing.T) {
	j, err := Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	now := time.Now()
	mapping := contract.SourceMapping{WorkspaceID: "org", Issuer: "https://issuer.test", RegistrarOrigin: "https://registrar.test", Audience: "fixture", MappingRevision: "r1", NetworkID: "net"}
	c, l, err := j.PrepareCallback(code(), "linux", fixturePolicy(t, "https://source.test", mapping), func() time.Time { return now }, now.Add(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("deadline left listener open")
	}
	if phase, _ := j.Status(); phase != "prepared" {
		t.Fatal("timeout fabricated response")
	}
	if err := j.RecordCallback(l.AttemptID, l.State, code(), now); err == nil {
		t.Fatal("closed owned callback accepted late response")
	}
}
func TestOwnedCallbackSocketBudgetRejectsNinthIdleConnection(t *testing.T) {
	_, c, _, _ := callbackFixture(t)
	var conns []net.Conn
	defer func() {
		for _, conn := range conns {
			conn.Close()
		}
	}()
	for i := 0; i < 8; i++ {
		conn, err := net.DialTimeout("tcp", c.listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, conn)
	}
	listener := c.listener.(*boundedCallbackListener)
	deadline := time.Now().Add(time.Second)
	for listener.active.Load() < 8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if listener.active.Load() != 8 {
		t.Fatal("test connections not accepted")
	}
	ninth, err := net.DialTimeout("tcp", c.listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer ninth.Close()
	ninth.SetReadDeadline(time.Now().Add(time.Second))
	one := make([]byte, 1)
	_, err = ninth.Read(one)
	if err == nil {
		t.Fatal("ninth connection remained open")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("budget didn't close excess connection")
	}
}
