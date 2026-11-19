package daemon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"fintex.au/mnet/internal/localapi"
)

type downBeforeAcknowledgement struct {
	*httptest.ResponseRecorder
	stop func()
}

func (w *downBeforeAcknowledgement) Write(p []byte) (int, error) {
	w.stop()
	return w.ResponseRecorder.Write(p)
}

// Single-P execution places a completed Down between scheduling the Up worker
// and letting it execute. The response-write boundary is still the actual
// authenticated HTTP handler, not a replacement startup implementation.
func TestLocalAPIQueuedUpCannotRunAfterCompletedDown(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	for _, body := range []string{`{}`, `{"reset_routing":true}`, `{"advertise_routes":["10.8.0.0/24"]}`} {
		t.Run(body, func(t *testing.T) {
			d, err := New(Config{StateDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			handler := d.localAPIHandler("fixture-capability")
			req := httptest.NewRequest(http.MethodPost, "http://mnetd/localapi/up", strings.NewReader(body))
			req.Header.Set(localapi.TokenHeader, "fixture-capability")
			var stopped bool
			writer := &downBeforeAcknowledgement{ResponseRecorder: httptest.NewRecorder(), stop: func() { d.Down(); stopped = true }}
			handler.ServeHTTP(writer, req)
			if writer.Code != http.StatusOK || !stopped {
				t.Fatal("did not complete Down at Up acknowledgement")
			}
			time.Sleep(50 * time.Millisecond)
			state := d.State()
			if state.PrivateKey != "" || len(state.AdvertiseRoutes) != 0 || d.Status().LastError != "" || d.Status().Status != "down" {
				t.Fatal("queued startup acted after completed Down")
			}
		})
	}
}

func TestLocalAPIRetiredWorkerCannotOverwriteFreshAttempt(t *testing.T) {
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	stale := d.stopGeneration
	d.Down()
	// The current generation still starts normally and exposes the existing
	// missing-target error. A late older request must not reset that result.
	d.runLocalUp(localUpRequest{}, d.stopGeneration)
	before := d.State()
	message := d.Status().LastError
	if before.PrivateKey == "" || !strings.Contains(message, "need --server") {
		t.Fatal("fresh generation did not reach ordinary startup")
	}
	d.runLocalUp(localUpRequest{AdvertiseRoutes: []string{"10.8.0.0/24"}}, stale)
	if !reflect.DeepEqual(before, d.State()) || d.Status().LastError != message {
		t.Fatal("retired worker changed fresh attempt configuration/error")
	}
}

func TestLocalAPIDownCancelsAlreadyAdmittedStartup(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer control.Close()
	defer close(release)
	d, err := New(Config{StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	api := httptest.NewServer(d.localAPIHandler("fixture-capability"))
	defer api.Close()
	post := func(path, body string) error {
		req, err := http.NewRequest(http.MethodPost, api.URL+path, strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set(localapi.TokenHeader, "fixture-capability")
		response, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("local API status%d", response.StatusCode)
		}
		return nil
	}
	if err := post("/localapi/up", fmt.Sprintf(`{"server":%q,"network":"fixture"}`, control.URL)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("startup did not reach control fixture")
	}
	identity := d.State()
	if identity.PrivateKey == "" || d.Status().Status != "joining" {
		t.Fatal("startup not admitted before Down")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- post("/localapi/down", "") }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Down waited for blocked enrollment")
	}
	if d.Status().Status != "down" || d.State().PrivateKey != identity.PrivateKey {
		t.Fatal("Down did not preserve stopped identity")
	}
}
