package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileApprovalRequiredWithoutGateHasNoStorage(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	dir := filepath.Join(t.TempDir(), "not-created")
	done := make(chan struct{})
	host.OnStream(func(s Stream) { defer close(done); FileReceiver{Mux: host, Dir: dir, RequireApproval: true}.Handle(s) })
	s, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var a FileAccept
	if json.Unmarshal(f.Payload, &a) != nil || a.OK {
		t.Fatal("accepted without local gate")
	}
	<-done
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unapproved storage created", err)
	}
}

func TestFileApprovalBoundDecisionAndExactBytes(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	dir := filepath.Join(t.TempDir(), "chosen")
	host.OnStream(func(s Stream) { FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s) })
	source := filepath.Join(t.TempDir(), "safe.bin")
	os.WriteFile(source, []byte("exact bytes"), 0600)
	sent := make(chan error, 1)
	go func() { sent <- SendFile(client, source) }()
	req := approvalRequest(t, gate)
	if req.Offer.Name != "safe.bin" || req.Offer.Size != 11 || req.Offer.SHA256 == "" {
		t.Fatal("wrong immutable offer", req.Offer)
	}
	for _, mutate := range []func(*FileApprovalRequest){func(r *FileApprovalRequest) { r.ID = "wrong" }, func(r *FileApprovalRequest) { r.Session = "wrong" }, func(r *FileApprovalRequest) { r.StreamID++ }, func(r *FileApprovalRequest) { r.Offer.Name = "other.bin" }, func(r *FileApprovalRequest) { r.Offer.Size++ }, func(r *FileApprovalRequest) { r.Offer.SHA256 = "" }} {
		bad := req
		mutate(&bad)
		if gate.Decide(bad, true, dir) == nil {
			t.Fatal("changed binding accepted")
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("storage before approval")
	}
	if gate.Decide(req, true, "relative") == nil {
		t.Fatal("relative consumer destination accepted")
	}
	if err := gate.Decide(req, true, dir); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "safe.bin"))
	if err != nil || string(got) != "exact bytes" {
		t.Fatal(string(got), err)
	}
	if gate.Decide(req, true, dir) == nil {
		t.Fatal("decision replay accepted")
	}
	select {
	case <-req.Done:
	default:
		t.Fatal("prompt remains active")
	}
}

func approvalRequest(t *testing.T, g *FileApprovalGate) FileApprovalRequest {
	t.Helper()
	select {
	case r := <-g.Requests():
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("approval request missing")
		return FileApprovalRequest{}
	}
}

func TestFileApprovalRetiresWithoutStorage(t *testing.T) {
	for _, mode := range []string{"decline", "timeout", "end", "reset", "early-data", "authorization", "gate-close"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			timeout := maxFileApprovalWait
			if mode == "timeout" {
				timeout = 20 * time.Millisecond
			}
			gate := NewFileApprovalGate(timeout)
			defer gate.Close()
			dir := filepath.Join(t.TempDir(), "not-created")
			var authorized atomic.Bool
			authorized.Store(true)
			done := make(chan struct{})
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, Dir: dir, RequireApproval: true, Approval: gate, Authorized: authorized.Load}.Handle(s)
			})
			s, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 0})
			if err != nil {
				t.Fatal(err)
			}
			req := approvalRequest(t, gate)
			switch mode {
			case "decline":
				if err := gate.Decide(req, false, ""); err != nil {
					t.Fatal(err)
				}
			case "end":
				host.Close()
			case "reset":
				client.tryReset(s.ID)
			case "early-data":
				client.SendBytes(s.ID, PriBulk, []byte("unapproved"))
			case "authorization":
				authorized.Store(false)
			case "gate-close":
				gate.Close()
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("receiver failed to retire")
			}
			if gate.Decide(req, true, dir) == nil {
				t.Fatal("late answer accepted")
			}
			select {
			case <-req.Done:
			default:
				t.Fatal("request not retired")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("rejected storage exists", err)
			}
		})
	}
}

func TestFileApprovalSessionIsolationCapacityAndReplacement(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	receiver := func(s Stream) { FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s) }
	host.OnStream(receiver)
	var requests []FileApprovalRequest
	for i := 0; i < maxPendingFileApprovals; i++ {
		if _, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0}); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, approvalRequest(t, gate))
	}
	s, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var a FileAccept
	if json.Unmarshal(f.Payload, &a) != nil || a.OK {
		t.Fatal("capacity did not deny")
	}
	for _, r := range requests {
		if err := gate.Decide(r, false, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0}); err != nil {
		t.Fatal(err)
	}
	replacement := approvalRequest(t, gate)
	for _, r := range requests {
		if gate.Decide(r, true, t.TempDir()) == nil {
			t.Fatal("old answer approved replacement")
		}
	}
	if err := gate.Decide(replacement, false, ""); err != nil {
		t.Fatal(err)
	}
	c2, h2 := pipe(t)
	defer c2.Close()
	defer h2.Close()
	h2.OnStream(func(s Stream) { FileReceiver{Mux: h2, RequireApproval: true, Approval: gate}.Handle(s) })
	s2, err := c2.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	f, err = s2.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(f.Payload, &a) != nil || a.OK {
		t.Fatal("gate crossed session")
	}
}

func TestFileApprovalConcurrentIdenticalOffersNeedSeparateAnswers(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	host.OnStream(func(s Stream) { FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s) })
	streams := make(map[uint32]Stream)
	for i := 0; i < 2; i++ {
		s, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 0})
		if err != nil {
			t.Fatal(err)
		}
		streams[s.ID] = s
	}
	r1, r2 := approvalRequest(t, gate), approvalRequest(t, gate)
	if r1.ID == r2.ID || r1.StreamID == r2.StreamID {
		t.Fatal("request bindings reused")
	}
	dir := t.TempDir()
	if err := gate.Decide(r1, true, dir); err != nil {
		t.Fatal(err)
	}
	s := streams[r1.StreamID]
	f, err := s.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var a FileAccept
	if json.Unmarshal(f.Payload, &a) != nil || !a.OK {
		t.Fatal("approved request refused")
	}
	select {
	case <-r2.Done:
		t.Fatal("unanswered sibling retired")
	default:
	}
	if err := gate.Decide(r2, false, ""); err != nil {
		t.Fatal(err)
	}
	client.send(s.ID, msgFin, PriBulk, nil)
	f, err = s.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var ack FileAck
	if json.Unmarshal(f.Payload, &ack) != nil || !ack.OK {
		t.Fatal("empty file not published")
	}
	other := streams[r2.StreamID]
	f, err = other.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(f.Payload, &a) != nil || a.OK {
		t.Fatal("declined sibling accepted")
	}
}

func TestFileApprovalHostOptionsGateRealReceiver(t *testing.T) {
	for _, required := range []bool{false, true} {
		t.Run(fmt.Sprint(required), func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			legacy := t.TempDir()
			chosen := t.TempDir()
			h := ServeHostWithOptions(host, &authorizationCapturer{}, nil, 8, HostOptions{FileDir: legacy, FileApprovalRequired: required, FileApproval: gate})
			defer h.Stop()
			source := filepath.Join(t.TempDir(), "host.bin")
			if err := os.WriteFile(source, []byte("host-option bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			sent := make(chan error, 1)
			go func() { sent <- SendFile(client, source) }()
			if required {
				r := approvalRequest(t, gate)
				if err := gate.Decide(r, true, chosen); err != nil {
					t.Fatal(err)
				}
			}
			if err := <-sent; err != nil {
				t.Fatal(err)
			}
			used, unused := legacy, chosen
			if required {
				used, unused = chosen, legacy
			}
			b, err := os.ReadFile(filepath.Join(used, "host.bin"))
			if err != nil || string(b) != "host-option bytes" {
				t.Fatal("host option not enforced", err)
			}
			if _, err = os.Stat(filepath.Join(unused, "host.bin")); !os.IsNotExist(err) {
				t.Fatal("wrong destination written")
			}
			if !required {
				select {
				case <-gate.Requests():
					t.Fatal("legacy default changed")
				default:
				}
			}
		})
	}
}

func TestFileApprovalInvalidOffersNeverPrompt(t *testing.T) {
	for _, offer := range []FileOffer{{Name: "../bad", Size: 0}, {Name: "bad", Size: -1}, {Name: "bad", Size: 11}, {Name: "bad", SHA256: "not-a-digest"}} {
		t.Run(fmt.Sprint(offer), func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			host.OnStream(func(s Stream) { FileReceiver{Mux: host, MaxBytes: 10, RequireApproval: true, Approval: gate}.Handle(s) })
			s, err := client.Open(PriBulk, offer)
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.Recv(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var a FileAccept
			if json.Unmarshal(f.Payload, &a) != nil || a.OK {
				t.Fatal("invalid offer accepted")
			}
			select {
			case <-gate.Requests():
				t.Fatal("invalid metadata reached local decision")
			default:
			}
		})
	}
}

func TestFileApprovalKeepsStorageSafety(t *testing.T) {
	for _, mode := range []string{"collision", "digest", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			gate := NewFileApprovalGate(0)
			defer gate.Close()
			dir := t.TempDir()
			target := filepath.Join(dir, "safe.bin")
			if mode == "collision" {
				if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan struct{})
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
			})
			offer := FileOffer{Name: "safe.bin", Size: 3}
			if mode == "digest" {
				offer.SHA256 = strings.Repeat("0", 64)
			}
			s, err := client.Open(PriBulk, offer)
			if err != nil {
				t.Fatal(err)
			}
			r := approvalRequest(t, gate)
			if err = gate.Decide(r, true, dir); err != nil {
				t.Fatal(err)
			}
			f, err := s.Recv(time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var a FileAccept
			if json.Unmarshal(f.Payload, &a) != nil {
				t.Fatal("invalid accept")
			}
			if mode == "collision" {
				if a.OK {
					t.Fatal("collision accepted")
				}
			} else {
				if !a.OK {
					t.Fatal("approved offer refused")
				}
				payload := []byte("abc")
				if mode == "overflow" {
					payload = []byte("abcd")
				}
				if err = client.SendBytes(s.ID, PriBulk, payload); err != nil {
					t.Fatal(err)
				}
				if mode != "overflow" {
					if err = client.send(s.ID, msgFin, PriBulk, nil); err != nil {
						t.Fatal(err)
					}
				}
				f, err = s.Recv(time.Second)
				if err != nil {
					t.Fatal(err)
				}
				var ack FileAck
				if json.Unmarshal(f.Payload, &ack) != nil || ack.OK {
					t.Fatal("unsafe bytes published")
				}
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("receiver stuck")
			}
			b, err := os.ReadFile(target)
			if mode == "collision" {
				if err != nil || string(b) != "old" {
					t.Fatal("existing bytes overwritten")
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("rejected target published")
			}
			temps, err := filepath.Glob(filepath.Join(dir, ".mnet-transfer-*"))
			if err != nil || len(temps) != 0 {
				t.Fatal("temp leak", temps, err)
			}
		})
	}
}

func TestFileApprovalHostStopRetiresPrompt(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	h := ServeHostWithOptions(host, &authorizationCapturer{}, nil, 8, HostOptions{FileApprovalRequired: true, FileApproval: gate})
	if _, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 0}); err != nil {
		t.Fatal(err)
	}
	req := approvalRequest(t, gate)
	stopped := make(chan struct{})
	go func() { h.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("host stop blocked by pending approval")
	}
	if gate.Decide(req, true, t.TempDir()) == nil {
		t.Fatal("stopped host accepted late answer")
	}
	select {
	case <-req.Done:
	default:
		t.Fatal("stopped host prompt active")
	}
}

func TestFileApprovalZeroGateAndRedaction(t *testing.T) {
	var zero FileApprovalGate
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	host.OnStream(func(s Stream) { FileReceiver{Mux: host, RequireApproval: true, Approval: &zero}.Handle(s) })
	s, err := client.Open(PriBulk, FileOffer{Name: "private-name", Size: 0})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var a FileAccept
	if json.Unmarshal(f.Payload, &a) != nil || a.OK {
		t.Fatal("zero gate accepted")
	}
	req := FileApprovalRequest{ID: "private-id", Session: "private-session", Offer: FileOffer{Name: "private-name"}}
	for _, text := range []string{fmt.Sprint(req), fmt.Sprintf("%+v", req), fmt.Sprintf("%#v", req)} {
		if strings.Contains(text, "private") {
			t.Fatal("local request formatting leaked metadata")
		}
	}
	b, err := json.Marshal(req)
	if err != nil || string(b) != "{}" {
		t.Fatal("request became wire authority", string(b), err)
	}
	zero.Close()
	zero.Close()
}
