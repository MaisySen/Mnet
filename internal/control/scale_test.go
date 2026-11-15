package control

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

// Seed synthetic state directly: these profiles measure map construction,
// authenticated HTTP serving and persisted endpoint updates, NOT registration.
func scaleStore(tb testing.TB, count int) *Store {
	tb.Helper()
	s, err := Open(filepath.Join(tb.TempDir(), "state.json"))
	if err != nil {
		tb.Fatal(err)
	}
	s.data.Networks["scale"] = &netmap.Network{ID: "scale", Name: "scale"}
	for n := 0; n < count; n++ {
		id := fmt.Sprintf("node-%06d", n)
		s.data.Nodes[id] = &netmap.Node{ID: id, NetworkID: "scale", Name: id, PubKey: fmt.Sprintf("synthetic-%06d", n), IPv4: fmt.Sprintf("100.64.%d.%d", n/254, n%254+1), Approved: true, Endpoints: []string{"192.0.2.1:12345"}}
		s.data.NodeTokens[id] = "synthetic-benchmark-token"
	}
	return s
}

func TestCancelledFanoutReleasesWatchers(t *testing.T) {
	s := scaleStore(t, 1)
	const clients = 1000
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{}, clients)
	for i := 0; i < clients; i++ {
		go func() { s.WaitForChangeContext(ctx, "scale", s.Version(), time.Minute); finished <- struct{}{} }()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		registered := len(s.watchers["scale"])
		s.mu.Unlock()
		if registered == clients {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d watchers registered", registered)
		}
		runtime.Gosched()
	}
	cancel()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for i := 0; i < clients; i++ {
		select {
		case <-finished:
		case <-timeout.C:
			t.Fatal("watchers did not exit after cancellation")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.watchers["scale"]); n != 0 {
		t.Fatalf("%d watchers leaked", n)
	}
}

func BenchmarkNetworkMapScale(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := scaleStore(b, size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m, err := s.Map("node-000000")
				if err != nil || len(m.Peers) != size-1 {
					b.Fatalf("map: peers=%d err=%v", len(m.Peers), err)
				}
			}
		})
	}
}

func BenchmarkMapHTTPScale(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := scaleStore(b, size)
			app, _, err := NewServer(s, relay.NewHub(), "http://localhost", "synthetic-admin")
			if err != nil {
				b.Fatal(err)
			}
			server := httptest.NewServer(app.Handler())
			defer server.Close()
			client := &http.Client{Timeout: 10 * time.Second}
			defer client.CloseIdleConnections()
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/map?node_id=node-000000", nil)
					req.Header.Set("X-Node-ID", "node-000000")
					req.Header.Set("X-Node-Token", "synthetic-benchmark-token")
					resp, err := client.Do(req)
					if err != nil {
						b.Error(err)
						return
					}
					_, copyErr := io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK || copyErr != nil {
						b.Errorf("status=%d copy=%v", resp.StatusCode, copyErr)
						return
					}
				}
			})
		})
	}
}

func BenchmarkEndpointPersistenceScale(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := scaleStore(b, size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := s.SetEndpoints("node-000000", []string{fmt.Sprintf("192.0.2.1:%d", 10000+i%50000)}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Separate no-op profile: a periodic discovery report that changes nothing
// must not be compared against the persisted-changing-endpoint baseline.
func BenchmarkUnchangedEndpoints(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := scaleStore(b, size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := s.SetEndpoints("node-000000", []string{"192.0.2.1:12345"}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
