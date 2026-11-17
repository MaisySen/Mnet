package multies

import (
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"testing"
	"time"
)

func consentFixture(t *testing.T) (*AttendedConsent, SessionBinding, *time.Time) {
	t.Helper()
	intent, _ := intentFixture()
	viewer := sha256.Sum256([]byte("viewer device"))
	b := SessionBinding{WorkspaceID: intent.WorkspaceID, HostUserID: "host-user", ViewerUserID: "viewer-user", HostDeviceKey: intent.DeviceKey, ViewerDeviceKey: base64.RawURLEncoding.EncodeToString(viewer[:])}
	now := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	c, err := NewAttendedConsent(b, now.Add(time.Minute), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return c, c.Binding(), &now
}
func TestAttendedViewNeverImpliesControl(t *testing.T) {
	c, b, _ := consentFixture(t)
	if got := c.Snapshot(); got.CanView || got.CanControl || got.State != WaitingForView {
		t.Fatal(got)
	}
	if _, err := c.RequestControl(b, b.ViewerUserID); err != ErrDenied {
		t.Fatal("control before view")
	}
	if err := c.DecideView(b, b.HostUserID, true); err != nil {
		t.Fatal(err)
	}
	if got := c.Snapshot(); !got.CanView || got.CanControl {
		t.Fatal(got)
	}
	ticket, err := c.RequestControl(b, b.ViewerUserID)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Snapshot(); !got.CanView || got.CanControl || got.State != WaitingForControl {
		t.Fatal(got)
	}
	if err := c.DecideControl(b, b.HostUserID, ticket, true); err != nil {
		t.Fatal(err)
	}
	if got := c.Snapshot(); !got.CanControl || !got.CanView {
		t.Fatal(got)
	}
	if err := c.RevokeControl(b, b.HostUserID); err != nil {
		t.Fatal(err)
	}
	if got := c.Snapshot(); got.CanControl || !got.CanView {
		t.Fatal(got)
	}
}
func TestConsentLateApprovalCannotCrossNewRequest(t *testing.T) {
	c, b, _ := consentFixture(t)
	c.DecideView(b, b.HostUserID, true)
	first, _ := c.RequestControl(b, b.ViewerUserID)
	c.RevokeControl(b, b.HostUserID)
	second, _ := c.RequestControl(b, b.ViewerUserID)
	if first == second {
		t.Fatal("request id reused")
	}
	if err := c.DecideControl(b, b.HostUserID, first, true); err != ErrDenied {
		t.Fatal("stale approval accepted")
	}
	if c.Snapshot().CanControl {
		t.Fatal("stale control")
	}
	if err := c.DecideControl(b, b.HostUserID, second, true); err != nil {
		t.Fatal(err)
	}
}
func TestConsentBindingAndActorCannotBeSubstituted(t *testing.T) {
	for _, field := range []string{"workspace", "host", "viewer", "host-device", "viewer-device", "actor"} {
		t.Run(field, func(t *testing.T) {
			c, b, _ := consentFixture(t)
			wrong := b
			actor := b.HostUserID
			switch field {
			case "workspace":
				wrong.WorkspaceID = "other"
			case "host":
				wrong.HostUserID = "other"
			case "viewer":
				wrong.ViewerUserID = "other"
			case "host-device":
				wrong.HostDeviceKey = b.ViewerDeviceKey
			case "viewer-device":
				wrong.ViewerDeviceKey = b.HostDeviceKey
			case "actor":
				actor = b.ViewerUserID
			}
			if err := c.DecideView(wrong, actor, true); err != ErrDenied {
				t.Fatal("actor/binding mismatch")
			}
			if c.Snapshot().CanView {
				t.Fatal("mismatch granted view")
			}
			if err := c.DecideView(b, b.HostUserID, true); err != nil {
				t.Fatal("wrong attempt consumed consent")
			}
		})
	}
}
func TestConsentEndAndExpiryPermanentlyDenyStaleMessages(t *testing.T) {
	for _, reason := range []string{"end", "expiry", "decline"} {
		t.Run(reason, func(t *testing.T) {
			c, b, now := consentFixture(t)
			var ticket uint64
			if reason == "decline" {
				c.DecideView(b, b.HostUserID, false)
			} else {
				c.DecideView(b, b.HostUserID, true)
				ticket, _ = c.RequestControl(b, b.ViewerUserID)
				if reason == "expiry" {
					*now = now.Add(time.Minute)
				} else {
					c.End()
				}
			}
			if got := c.Snapshot(); got.State != ConsentEnded || got.CanView || got.CanControl {
				t.Fatal(got)
			}
			if c.DecideControl(b, b.HostUserID, ticket, true) != ErrDenied || c.DecideView(b, b.HostUserID, true) != ErrDenied {
				t.Fatal("ended session revived")
			}
			if _, err := c.RequestControl(b, b.ViewerUserID); err != ErrDenied {
				t.Fatal("ended request accepted")
			}
		})
	}
}
func TestConsentConcurrentStopAlwaysWins(t *testing.T) {
	for i := 0; i < 30; i++ {
		c, b, _ := consentFixture(t)
		c.DecideView(b, b.HostUserID, true)
		ticket, _ := c.RequestControl(b, b.ViewerUserID)
		var wg sync.WaitGroup
		for j := 0; j < 20; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); c.DecideControl(b, b.HostUserID, ticket, true); c.Snapshot() }()
		}
		c.End()
		wg.Wait()
		if got := c.Snapshot(); got.State != ConsentEnded || got.CanControl || got.CanView {
			t.Fatal(got)
		}
	}
}
func TestConsentZeroAndInvalidLifetimesDeny(t *testing.T) {
	var zero AttendedConsent
	if got := zero.Snapshot(); got.State != ConsentEnded || got.CanView || got.CanControl {
		t.Fatal(got)
	}
	_, b, now := consentFixture(t)
	b.SessionID = ""
	for _, expiry := range []time.Time{*now, now.Add(-time.Second), now.Add(MaxConsentLifetime + time.Nanosecond)} {
		if _, err := NewAttendedConsent(b, expiry, func() time.Time { return *now }); err != ErrDenied {
			t.Fatal("invalid expiry")
		}
	}
	b.ViewerDeviceKey = b.HostDeviceKey
	if _, err := NewAttendedConsent(b, now.Add(time.Minute), func() time.Time { return *now }); err != ErrDenied {
		t.Fatal("same device session accepted")
	}
}
func FuzzConsentCannotGrantAfterEnd(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Fuzz(func(t *testing.T, events []byte) {
		if len(events) > 1024 {
			return
		}
		c, b, _ := consentFixture(t)
		var ticket uint64
		ended := false
		for _, event := range events {
			switch event % 6 {
			case 0:
				c.DecideView(b, b.HostUserID, true)
			case 1:
				ticket, _ = c.RequestControl(b, b.ViewerUserID)
			case 2:
				c.DecideControl(b, b.HostUserID, ticket, true)
			case 3:
				c.RevokeControl(b, b.HostUserID)
			case 4:
				c.End()
				ended = true
			case 5:
				c.DecideView(b, b.ViewerUserID, true)
			}
			got := c.Snapshot()
			if got.CanControl && !got.CanView || ended && (got.CanView || got.CanControl || got.State != ConsentEnded) {
				t.Fatal("consent invariant")
			}
		}
	})
}

func TestConsentApprovalCannotCrossNewSessionWithSameParticipants(t *testing.T) {
	first, old, _ := consentFixture(t)
	first.DecideView(old, old.HostUserID, true)
	ticket, _ := first.RequestControl(old, old.ViewerUserID)
	second, current, _ := consentFixture(t)
	if old.SessionID == current.SessionID {
		t.Fatal("session correlation reused")
	}
	if second.DecideView(old, old.HostUserID, true) != ErrDenied {
		t.Fatal("old view approval replayed")
	}
	second.DecideView(current, current.HostUserID, true)
	second.RequestControl(current, current.ViewerUserID)
	if second.DecideControl(old, old.HostUserID, ticket, true) != ErrDenied {
		t.Fatal("old control approval replayed")
	}
	if second.Snapshot().CanControl {
		t.Fatal("cross-session control")
	}
}
