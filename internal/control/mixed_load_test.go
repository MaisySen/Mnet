package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fintex.au/mnet/internal/netmap"
	"fintex.au/mnet/internal/relay"
)

type loadOperation struct {
	Requests    int     `json:"requests"`
	OK          int     `json:"ok"`
	RateLimited int     `json:"rate_limited"`
	Cancelled   int     `json:"expected_poll_cancellations"`
	Errors      int     `json:"errors"`
	P50MS       float64 `json:"p50_ms"`
	P95MS       float64 `json:"p95_ms"`
	P99MS       float64 `json:"p99_ms"`
	durations   []float64
}
type loadSummary struct {
	InitialNodes      int                       `json:"initial_seeded_nodes"`
	FinalNodes        int                       `json:"final_nodes"`
	Workers           int                       `json:"workers"`
	GoProcs           int                       `json:"gomaxprocs"`
	Seconds           float64                   `json:"elapsed_seconds_including_drain"`
	RequestsPerSecond float64                   `json:"completed_requests_per_second"`
	AllocatedMB       float64                   `json:"allocated_mb_during_run"`
	EndHeapMB         float64                   `json:"end_heap_inuse_mb_not_peak"`
	GoroutinesBefore  int                       `json:"goroutines_before"`
	GoroutinesAfter   int                       `json:"goroutines_after_worker_drain"`
	Operations        map[string]*loadOperation `json:"operations"`
}

// Opt-in local HTTP mixed load. All effects stay in an httptest server and
// a temporary synthetic store. No credentials or real service is contacted.
// Run separately from -race for timing; a short race run validates the harness.
func TestControlMixedLoad(t *testing.T) {
	if os.Getenv("MNET_MIXED_LOAD") != "1" {
		t.Skip("opt-in mixed-load measurement")
	}
	size := 1000
	if raw := os.Getenv("MNET_LOAD_NODES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100000 {
			t.Fatal("invalid MNET_LOAD_NODES")
		}
		size = n
	}
	duration := 10 * time.Second
	if raw := os.Getenv("MNET_LOAD_DURATION"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Second || d > time.Minute {
			t.Fatal("invalid MNET_LOAD_DURATION")
		}
		duration = d
	}
	const workers = 16
	store := scaleStore(t, size)
	key, err := store.CreateAuthKey("scale", "synthetic mixed-load key", true, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := NewServer(store, relay.NewHub(), "http://localhost", "synthetic-admin")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	transport := &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: workers}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	summary := loadSummary{InitialNodes: size, Workers: workers, GoProcs: runtime.GOMAXPROCS(0), Operations: map[string]*loadOperation{}}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	summary.GoroutinesBefore = runtime.NumGoroutine()
	var sequence atomic.Uint64
	var mu sync.Mutex
	var wg sync.WaitGroup
	started := time.Now()
	runCtx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for runCtx.Err() == nil {
				seq := sequence.Add(1)
				op := "map"
				method := http.MethodGet
				path := "/api/map"
				var body []byte
				timeout := 2 * time.Second
				switch seq % 20 {
				case 0:
					op = "join"
					method = http.MethodPost
					path = "/api/join"
					pub := sha256.Sum256([]byte(fmt.Sprintf("synthetic-load-%d", seq)))
					body, _ = json.Marshal(netmap.JoinRequest{Network: "scale", Name: fmt.Sprintf("joined-%d", seq), PubKey: base64.StdEncoding.EncodeToString(pub[:]), AuthKey: key.Key})
				case 1:
					op = "poll"
					timeout = 100 * time.Millisecond
					path = fmt.Sprintf("/api/map?wait=1&since=%d", store.Version())
				case 2, 3, 4, 5:
					op = "endpoints"
					method = http.MethodPost
					path = "/api/endpoints"
					body, _ = json.Marshal(netmap.EndpointsUpdate{Endpoints: []string{fmt.Sprintf("192.0.2.1:%d", 10000+seq%50000)}})
				}
				reqCtx, stop := context.WithTimeout(context.Background(), timeout)
				req, _ := http.NewRequestWithContext(reqCtx, method, server.URL+path, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Node-ID", fmt.Sprintf("node-%06d", worker%size))
				req.Header.Set("X-Node-Token", "synthetic-benchmark-token")
				req.Close = seq%100 == 0 // a small repeatable connection-churn component
				begin := time.Now()
				resp, reqErr := client.Do(req)
				status := 0
				if reqErr == nil {
					status = resp.StatusCode
					_, reqErr = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
				latency := float64(time.Since(begin)) / float64(time.Millisecond)
				expectedCancel := op == "poll" && reqCtx.Err() == context.DeadlineExceeded
				stop()
				mu.Lock()
				result := summary.Operations[op]
				if result == nil {
					result = &loadOperation{}
					summary.Operations[op] = result
				}
				result.Requests++
				result.durations = append(result.durations, latency)
				switch {
				case expectedCancel:
					result.Cancelled++
				case reqErr != nil:
					result.Errors++
				case status == http.StatusOK:
					result.OK++
				case op == "join" && status == http.StatusTooManyRequests:
					result.RateLimited++
				default:
					result.Errors++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	summary.Seconds = time.Since(started).Seconds()
	runtime.ReadMemStats(&after)
	summary.AllocatedMB = float64(after.TotalAlloc-before.TotalAlloc) / 1e6
	summary.EndHeapMB = float64(after.HeapInuse) / 1e6
	summary.GoroutinesAfter = runtime.NumGoroutine()
	store.mu.Lock()
	summary.FinalNodes = len(store.data.Nodes)
	watchers := len(store.watchers["scale"])
	store.mu.Unlock()
	if watchers != 0 {
		t.Errorf("%d map watchers leaked after worker drain", watchers)
	}
	requests := 0
	for name, result := range summary.Operations {
		sort.Float64s(result.durations)
		percentile := func(p float64) float64 {
			index := int(math.Ceil(float64(len(result.durations))*p)) - 1
			if index < 0 {
				return 0
			}
			return result.durations[index]
		}
		result.P50MS = percentile(.5)
		result.P95MS = percentile(.95)
		result.P99MS = percentile(.99)
		requests += result.Requests
		if result.Errors > 0 {
			t.Errorf("%s had %d unexpected errors", name, result.Errors)
		}
	}
	summary.RequestsPerSecond = float64(requests) / summary.Seconds
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("MNET_LOAD_OUTPUT"); path != "" {
		if err = os.WriteFile(path, append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("LOCAL SYNTHETIC MIXED LOAD, not production capacity:\n%s", encoded)
}
