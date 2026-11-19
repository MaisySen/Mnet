package desk

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func pairFixture(t *testing.T) (*Compiled, Binding) {
	t.Helper()
	c, err := Compile(Layout{Version: 1, Displays: []Display{{ID: "local", Device: "key-a", Width: 100, Height: 100, PixelWidth: 100, PixelHeight: 100}, {ID: "remote", Device: "key-b", X: 100, Width: 100, Height: 100, PixelWidth: 200, PixelHeight: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	return c, Binding{Run: strings.Repeat("a", 64), Primary: "key-a", Recipient: "key-b", Layout: c.Fingerprint(), Keyboard: strings.Repeat("b", 64), Display: "remote"}
}
func TestPairCrossingHoldGuardAndLocalReturn(t *testing.T) {
	layout, binding := pairFixture(t)
	var disabled atomic.Int32
	board, err := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { disabled.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	backend := &ownedFixture{}
	if err = board.Cross(Right, 50, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
		t.Fatal(err)
	}
	if len(backend.events) != 1 || backend.events[0].X != 0 || backend.events[0].Y != 100 {
		t.Fatal("entry position not mapped")
	}
	if err = board.Apply(Input{Sequence: 2, Kind: "key", Action: "down", Code: 38}); err != nil {
		t.Fatal(err)
	}
	if err = board.Cross(Left, 100, Binding{}, nil); err == nil || backend.released != 0 {
		t.Fatal("held key crossed")
	}
	if err = board.Apply(Input{Sequence: 3, Kind: "key", Action: "up", Code: 38}); err != nil {
		t.Fatal(err)
	}
	if err = board.Cross(Left, 100, Binding{}, nil); err != nil || backend.released != 1 {
		t.Fatal("local return did not join release", err)
	}
	if err = board.Apply(Input{Sequence: 4, Kind: "key", Action: "down", Code: 38}); err == nil {
		t.Fatal("local input forwarded")
	}
	board.Stop()
	if disabled.Load() != 1 {
		t.Fatal("capture not disabled")
	}
	if err = board.Cross(Right, 50, binding, func(Binding) (NativeInput, error) { t.Fatal("stopped board restarted"); return nil, nil }); err == nil {
		t.Fatal("stop not terminal")
	}
}
func TestPairRejectsForeignBindingBeforeNative(t *testing.T) {
	for _, change := range []func(*Binding){func(b *Binding) { b.Primary = "foreign" }, func(b *Binding) { b.Recipient = "foreign" }, func(b *Binding) { b.Run = "" }, func(b *Binding) { b.Layout = strings.Repeat("c", 64) }, func(b *Binding) { b.Keyboard = "" }, func(b *Binding) { b.Display = "local" }} {
		layout, binding := pairFixture(t)
		change(&binding)
		board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() {})
		if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { t.Fatal("invalid binding created native input"); return nil, nil }); err == nil {
			t.Fatal("foreign binding accepted")
		}
	}
}
func TestPairReleaseFailureBlocksReactivation(t *testing.T) {
	layout, binding := pairFixture(t)
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() {})
	backend := &ownedFixture{releaseErr: errors.New("release not confirmed")}
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
		t.Fatal(err)
	}
	if err := board.Cross(Left, 20, Binding{}, nil); err == nil {
		t.Fatal("cleanup failure ignored")
	}
	if err := board.Stop(); err == nil {
		t.Fatal("cleanup outcome lost")
	}
	if err := board.Cross(Left, 20, Binding{}, nil); err == nil {
		t.Fatal("failed cleanup generation restarted")
	}
}
func TestPairStopDisablesCaptureBeforeBlockedRelease(t *testing.T) {
	layout, binding := pairFixture(t)
	disabled := make(chan struct{})
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { close(disabled) })
	blocked := &blockedRelease{entered: make(chan struct{}), allow: make(chan struct{})}
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return blocked, nil }); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { board.Stop(); close(done) }()
	<-blocked.entered
	select {
	case <-disabled:
	default:
		t.Fatal("capture active while waiting remote cleanup")
	}
	close(blocked.allow)
	<-done
}

type blockedRelease struct{ entered, allow chan struct{} }

func (b *blockedRelease) Apply(Input) error { return nil }
func (b *blockedRelease) Release() error    { close(b.entered); <-b.allow; return nil }

func TestPairStopDuringFactoryCannotActivateLateBackend(t *testing.T) {
	layout, binding := pairFixture(t)
	disabled := make(chan struct{})
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { close(disabled) })
	entered := make(chan struct{})
	allow := make(chan struct{})
	crossed := make(chan error, 1)
	backend := &ownedFixture{}
	go func() {
		crossed <- board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { close(entered); <-allow; return backend, nil })
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- board.Stop() }()
	<-disabled
	close(allow)
	if err := <-crossed; err == nil {
		t.Fatal("late backend activated after local stop")
	}
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	if backend.released != 1 || len(backend.events) != 0 {
		t.Fatal("late backend not retired before input")
	}
}

func TestPairPartialFactoryFailureRetainsCleanupFailure(t *testing.T) {
	layout, binding := pairFixture(t)
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() {})
	backend := &ownedFixture{releaseErr: errors.New("native cleanup failed")}
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return backend, errors.New("partial setup failed") }); err == nil {
		t.Fatal("partial setup failure ignored")
	}
	if backend.released != 1 {
		t.Fatal("partially created backend leaked")
	}
	if err := board.Stop(); err == nil {
		t.Fatal("partial setup cleanup failure forgotten")
	}
}

type terminalBackend struct {
	ownedFixture
	done chan struct{}
}

func (b *terminalBackend) Stopped() <-chan struct{} { return b.done }
func TestPairExactBackendTerminalStopsWithoutAnotherInput(t *testing.T) {
	layout, binding := pairFixture(t)
	disabled := make(chan struct{})
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { close(disabled) })
	backend := &terminalBackend{done: make(chan struct{})}
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
		t.Fatal(err)
	}
	close(backend.done)
	select {
	case <-disabled:
	case <-time.After(time.Second):
		t.Fatal("idle backend death left forwarding active")
	}
	if err := board.Stop(); err != nil {
		t.Fatal(err)
	}
	if backend.released != 1 {
		t.Fatal("terminal backend cleanup not joined")
	}
}
func TestPairOldBackendDeathCannotRetireSuccessor(t *testing.T) {
	layout, binding := pairFixture(t)
	var disabled atomic.Int32
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { disabled.Add(1) })
	old := &ownedFixture{}
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return old, nil }); err != nil {
		t.Fatal(err)
	}
	oldRecipient := board.recipient
	if err := board.Cross(Left, 20, Binding{}, nil); err != nil {
		t.Fatal(err)
	}
	next := &ownedFixture{}
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return next, nil }); err != nil {
		t.Fatal(err)
	}
	board.backendRetired(oldRecipient)
	if disabled.Load() != 0 || next.released != 0 {
		t.Fatal("old backend retired successor")
	}
	if err := board.Apply(Input{Sequence: 2, Kind: "pointer", Action: "move", X: 20, Y: 20}); err != nil {
		t.Fatal(err)
	}
	board.Stop()
}

func TestPairAlreadyTerminalBackendRefusedBeforeEntry(t *testing.T) {
	layout, binding := pairFixture(t)
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() {})
	backend := &terminalBackend{done: make(chan struct{})}
	close(backend.done)
	if err := board.Cross(Right, 20, binding, func(Binding) (NativeInput, error) { return backend, nil }); err == nil {
		t.Fatal("terminal backend activated")
	}
	if backend.released != 1 || len(backend.events) != 0 {
		t.Fatal("terminal backend entry touched native input")
	}
	board.Stop()
}

func TestPairReturnLocalEdgeHeldGuardAndTerminalMapping(t *testing.T) {
	layout, binding := pairFixture(t)
	disabled := false
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { disabled = true })
	backend := &ownedFixture{}
	if err := board.Cross(Right, 50, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
		t.Fatal(err)
	}
	if err := board.Apply(Input{Sequence: 2, Kind: "button", Action: "down", Code: 1}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := board.ReturnLocalEdge(Left, 100); err != nil || ok || disabled {
		t.Fatal("held input crossed", err)
	}
	if err := board.Apply(Input{Sequence: 3, Kind: "button", Action: "up", Code: 1}); err != nil {
		t.Fatal(err)
	}
	mapped, ok, err := board.ReturnLocalEdge(Left, 100)
	if err != nil || !ok || mapped.X != 99 || mapped.Y != 50 || !disabled || backend.released != 1 {
		t.Fatal("mapped local return failed", mapped, ok, err)
	}
	if err = board.Apply(Input{Sequence: 4, Kind: "key", Action: "down", Code: 38}); err == nil {
		t.Fatal("returned run forwarded input")
	}
}

func TestPairReturnLocalDisablesBeforeBlockedRelease(t *testing.T) {
	layout, binding := pairFixture(t)
	disabled := make(chan struct{})
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() { close(disabled) })
	backend := &blockedRelease{entered: make(chan struct{}), allow: make(chan struct{})}
	if err := board.Cross(Right, 50, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, ok, err := board.ReturnLocalEdge(Left, 100); err != nil || !ok {
			t.Error("local return failed", err)
		}
	}()
	<-backend.entered
	select {
	case <-disabled:
	default:
		t.Fatal("source forwarding active while edge return waits for recipient release")
	}
	close(backend.allow)
	<-done
}
func TestPairReturnLocalReleaseFailureHasNoMappedActivation(t *testing.T) {
	layout, binding := pairFixture(t)
	board, _ := NewSwitchboard(layout, "key-a", "local", func() bool { return true }, func() {})
	backend := &ownedFixture{releaseErr: errors.New("native release unavailable")}
	if err := board.Cross(Right, 50, binding, func(Binding) (NativeInput, error) { return backend, nil }); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := board.ReturnLocalEdge(Left, 100); err == nil || ok {
		t.Fatal("failed release produced local pointer placement")
	}
	if err := board.Stop(); err == nil {
		t.Fatal("edge return lost cleanup error")
	}
}
