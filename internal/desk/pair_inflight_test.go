package desk

import (
	"errors"
	"sync/atomic"
	"testing"
)

// Channels order the in-flight native completion after capture disable. No
// sleep or scheduler order is used as evidence that Stop has taken effect.
type inflightPairBackend struct {
	ownedFixture
	entered chan struct{}
	allow   chan struct{}
}

func (b *inflightPairBackend) Apply(ev Input) error {
	if ev.Kind == "key" {
		close(b.entered)
		<-b.allow
	}
	return b.ownedFixture.Apply(ev)
}

func TestPairStopDuringNativePressJoinsExactCleanup(t *testing.T) {
	for _, unconfirmed := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "unconfirmed"}[unconfirmed], func(t *testing.T) {
			layout, binding := pairFixture(t)
			var disabled atomic.Int32
			captureStopped := make(chan struct{})
			board, err := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { disabled.Add(1); close(captureStopped) })
			if err != nil {
				t.Fatal(err)
			}
			sentinel := errors.New("fixture release unconfirmed")
			backend := &inflightPairBackend{entered: make(chan struct{}), allow: make(chan struct{})}
			if unconfirmed {
				backend.releaseErr = sentinel
			}
			if err = board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
				t.Fatal(err)
			}
			applied := make(chan error, 1)
			go func() { applied <- board.Apply(Input{Sequence: 2, Kind: "key", Action: "down", Code: 38}) }()
			<-backend.entered
			stopped := make(chan error, 1)
			go func() { stopped <- board.Stop() }()
			<-captureStopped
			queued := make(chan error, 1)
			go func() { queued <- board.Apply(Input{Sequence: 3, Kind: "key", Action: "up", Code: 38}) }()
			close(backend.allow)
			for name, result := range map[string]<-chan error{"inflight": applied, "stop": stopped, "queued": queued} {
				got := <-result
				if name != "stop" && got == nil {
					t.Fatal(name, "accepted after disable")
				}
				if unconfirmed && !errors.Is(got, sentinel) {
					t.Fatal(name, "lost cleanup uncertainty", got)
				}
				if !unconfirmed && name == "stop" && got != nil {
					t.Fatal(got)
				}
			}
			if disabled.Load() != 1 || backend.released != 1 || len(backend.events) != 2 || backend.events[1].Action != "down" {
				t.Fatal("late queued input or repeated cleanup", disabled.Load(), backend.released, backend.events)
			}
			if err = board.Stop(); unconfirmed != errors.Is(err, sentinel) {
				t.Fatal("repeat stop lost cleanup result", err)
			}
			if err = board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { t.Fatal("stopped board invoked successor"); return nil, nil }); err == nil {
				t.Fatal("stopped board restarted")
			}
		})
	}
}
