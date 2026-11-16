package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/daemon"
)

func TestWaitUntilUpTransitions(t *testing.T) {
	sequence := []daemon.Status{{Status: "down"}, {Status: "joining"}, {Status: "waiting-approval", ApprovalURL: "https://private.example/?token=fixture"}, {Status: "up", IPv4: "100.64.0.1"}}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/localapi/status" {
			t.Error("wait performed a mutation or wrong request")
		}
		i := int(calls.Add(1)) - 1
		if i >= len(sequence) {
			i = len(sequence) - 1
		}
		json.NewEncoder(w).Encode(sequence[i])
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := waitUntilUp(ctx, srv.Client(), srv.URL, time.Millisecond)
	if err != nil || st.IPv4 != "100.64.0.1" || calls.Load() != 4 {
		t.Fatalf("wait did not follow transitions: %+v %v", st, err)
	}
}

func TestWaitUntilUpFailureAndCancellation(t *testing.T) {
	for _, st := range []daemon.Status{{Status: "down", LastError: "private diagnostic"}, {Status: "key-expired"}, {Status: "unexpected"}, {Status: "up"}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(st) }))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := waitUntilUp(ctx, srv.Client(), srv.URL, time.Millisecond)
		cancel()
		srv.Close()
		if err == nil {
			t.Fatal("failure reported success")
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err := waitUntilUp(ctx, srv.Client(), srv.URL, time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked status did not honor timeout: %v", err)
	}
}

func TestWaitTimeoutBounds(t *testing.T) {
	for _, args := range [][]string{{"--timeout", "0s"}, {"--timeout", "-1s"}, {"--timeout", "31m"}, {"--timeout", "invalid"}, {"--timeout"}, {"--other", "1s"}} {
		if _, err := waitTimeout(args); err == nil {
			t.Fatal("bad wait flags accepted")
		}
	}
	for _, args := range [][]string{nil, {"--timeout", "1s"}, {"--timeout", "30m"}} {
		if _, err := waitTimeout(args); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWaitApprovalTimeoutAndStatusErrors(t *testing.T) {
	for _, mode := range []string{"approval", "http", "json"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch mode {
				case "http":
					w.WriteHeader(403)
				case "json":
					w.Write([]byte(`invalid`))
				default:
					json.NewEncoder(w).Encode(daemon.Status{Status: "waiting-approval", ApprovalURL: "https://private.example/?token=fixture"})
				}
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			st, err := waitUntilUp(ctx, srv.Client(), srv.URL, time.Millisecond)
			if err == nil {
				t.Fatal("non-ready status reported success")
			}
			if mode == "approval" && (!errors.Is(err, context.DeadlineExceeded) || st == nil || st.Status != "waiting-approval" || calls.Load() < 2) {
				t.Fatal("approval wait did not keep waiting to its deadline")
			}
		})
	}
}
