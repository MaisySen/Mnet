package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNativeFileDecisionSuccessfulDoneKeepsOutputUntilReceiverRelease(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	source := filepath.Join(t.TempDir(), "safe.bin")
	if err := os.WriteFile(source, []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	sent := make(chan error, 1)
	go func() { sent <- SendFile(client, source) }()
	request := approvalRequest(t, gate)
	decision, err := newNativeFileDecision(gate, request, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer decision.close()
	if !decision.current() {
		t.Fatal("new exact decision not current")
	}
	native := fixtureNativeOutput("finish")
	if err := decision.choose(native); err != nil {
		t.Fatal(err)
	}
	<-request.Done
	<-decision.watched
	if decision.current() {
		t.Fatal("successful choice kept prompt current")
	}
	select {
	case <-native.entered:
	case <-time.After(time.Second):
		t.Fatal("native Finish not reached")
	}
	if native.canceled.Load() != 0 {
		t.Fatal("successful prompt Done canceled its own export")
	}
	// Retire the exact local owner, which must cancel blocked Finish without
	// restoring another prompt or accepting a new native output.
	decision.close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closed owner retained receiver")
	}
	if err := <-sent; err == nil {
		t.Fatal("canceled selection succeeded")
	}
	if native.canceled.Load() != 1 {
		t.Fatal("owner release was not one-use")
	}
	replay := fixtureNativeOutput("success")
	if decision.choose(replay) == nil || replay.canceled.Load() != 1 {
		t.Fatal("replayed decision retained output")
	}
}

func TestNativeFileDecisionRejectsForeignZeroExpiredAndDeclinedRequests(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	if _, err := client.Open(PriBulk, FileOffer{Name: "same.bin"}); err != nil {
		t.Fatal(err)
	}
	request := approvalRequest(t, gate)
	foreign := NewFileApprovalGate(0)
	defer foreign.Close()
	if _, err := newNativeFileDecision(foreign, request, t.TempDir()); err == nil {
		t.Fatal("foreign gate accepted")
	}
	for _, bad := range []FileApprovalRequest{{}, func() FileApprovalRequest { b := request; b.Offer.Name = "changed.bin"; return b }()} {
		if _, err := newNativeFileDecision(gate, bad, t.TempDir()); err == nil {
			t.Fatal("forged exact request accepted")
		}
	}
	decision, err := newNativeFileDecision(gate, request, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := decision.decline(); err != nil {
		t.Fatal(err)
	}
	decision.close()
	<-done
	if _, err := newNativeFileDecision(gate, request, t.TempDir()); err == nil {
		t.Fatal("retired request accepted")
	}
	native := fixtureNativeOutput("success")
	if decision.choose(native) == nil || native.canceled.Load() != 1 || native.opens != 0 {
		t.Fatal("declined request opened output")
	}
	if (*nativeFileDecision)(nil).choose(fixtureNativeOutput("success")) == nil {
		t.Fatal("nil decision accepted")
	}
}

func TestNativeFileDecisionOwnerCloseRetiresPendingPrompt(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	stream, err := client.Open(PriBulk, FileOffer{Name: "safe.bin"})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := newNativeFileDecision(gate, approvalRequest(t, gate), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decision.close()
	decision.close()
	frame, err := stream.Recv(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var accept FileAccept
	if json.Unmarshal(frame.Payload, &accept) != nil || accept.OK {
		t.Fatal("closed owner accepted file")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closed decision retained receiver")
	}
}

func TestNativeFileDecisionDuplicateWrappersCannotAuthorizeTwice(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		client, host := pipe(t)
		gate := NewFileApprovalGate(0)
		done := make(chan struct{})
		host.OnStream(func(s Stream) {
			defer close(done)
			FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
		})
		stream, err := client.Open(PriBulk, FileOffer{Name: "same.bin", Size: 1})
		if err != nil {
			t.Fatal(err)
		}
		request := approvalRequest(t, gate)
		one, err := newNativeFileDecision(gate, request, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		two, err := newNativeFileDecision(gate, request, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		first, second := fixtureNativeOutput("success"), fixtureNativeOutput("success")
		var wg sync.WaitGroup
		wg.Add(2)
		results := make(chan error, 2)
		go func() { defer wg.Done(); results <- one.choose(first) }()
		go func() { defer wg.Done(); results <- two.choose(second) }()
		wg.Wait()
		success := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatal("exact gate did not enforce one selection", success)
		}
		client.tryReset(stream.ID)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("duplicate decision retained receiver")
		}
		one.close()
		two.close()
		if first.canceled.Load() != 1 || second.canceled.Load() != 1 {
			t.Fatal("selected/rejected native lifetime leaked")
		}
		gate.Close()
		client.Close()
		host.Close()
	}
}

func TestNativeFileDecisionGateEndCannotChooseLater(t *testing.T) {
	for _, mode := range []string{"end", "gate-close", "expiry"} {
		t.Run(mode, func(t *testing.T) {
			client, host := pipe(t)
			defer client.Close()
			defer host.Close()
			timeout := maxFileApprovalWait
			if mode == "expiry" {
				timeout = 10 * time.Millisecond
			}
			gate := NewFileApprovalGate(timeout)
			defer gate.Close()
			done := make(chan struct{})
			host.OnStream(func(s Stream) {
				defer close(done)
				FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
			})
			if _, err := client.Open(PriBulk, FileOffer{Name: "same.bin"}); err != nil {
				t.Fatal(err)
			}
			decision, err := newNativeFileDecision(gate, approvalRequest(t, gate), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "end" {
				host.Close()
			}
			if mode == "gate-close" {
				gate.Close()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("terminal request not retired")
			}
			<-decision.watched
			native := fixtureNativeOutput("success")
			if decision.choose(native) == nil || native.canceled.Load() != 1 {
				t.Fatal("terminal request chose native output")
			}
			decision.close()
		})
	}
}

func TestNativeFileDecisionCompletionDoesNotNeedPromptLifetime(t *testing.T) {
	client, host := pipe(t)
	defer client.Close()
	defer host.Close()
	gate := NewFileApprovalGate(0)
	defer gate.Close()
	done := make(chan struct{})
	host.OnStream(func(s Stream) {
		defer close(done)
		FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
	})
	source := filepath.Join(t.TempDir(), "safe.bin")
	if err := os.WriteFile(source, []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	sent := make(chan error, 1)
	go func() { sent <- SendFile(client, source) }()
	decision, err := newNativeFileDecision(gate, approvalRequest(t, gate), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	native := fixtureNativeOutput("success")
	if err := decision.choose(native); err != nil {
		t.Fatal(err)
	}
	if err := <-sent; err != nil {
		t.Fatal("selected native completion failed", err)
	}
	<-done
	decision.close()
	if native.String() != "verified" || native.canceled.Load() != 1 {
		t.Fatal("completed selected output not exactly released")
	}
}

func TestNativeFileDecisionChooseCloseRaceKeepsOneNativeOwner(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		client, host := pipe(t)
		gate := NewFileApprovalGate(0)
		done := make(chan struct{})
		host.OnStream(func(s Stream) {
			defer close(done)
			FileReceiver{Mux: host, RequireApproval: true, Approval: gate}.Handle(s)
		})
		stream, err := client.Open(PriBulk, FileOffer{Name: "safe.bin", Size: 1})
		if err != nil {
			t.Fatal(err)
		}
		decision, err := newNativeFileDecision(gate, approvalRequest(t, gate), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		native := fixtureNativeOutput("success")
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = decision.choose(native) }()
		go func() { defer wg.Done(); decision.close() }()
		wg.Wait()
		client.tryReset(stream.ID)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("choice/close retained receiver")
		}
		decision.close()
		if native.canceled.Load() != 1 {
			t.Fatal("choice/close leaked or duplicated native release")
		}
		gate.Close()
		host.Close()
		client.Close()
	}
}
