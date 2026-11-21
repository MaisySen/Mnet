package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestDesktopHomeNoticeNamesSpecificOutcomes(t *testing.T) {
	cases := []struct {
		viewed bool
		err    error
		want   string
	}{
		{false, nil, homeNoticeClosed},
		{true, nil, homeNoticeViewed},
		{false, errDesktopFirstScreen, homeNoticeNoScreen},
		{true, fmt.Errorf("%w: host or path closed", errDesktopHostEnded), homeNoticeHostEnded},
		{false, fmt.Errorf("session: refused: local viewing approval unavailable, declined or retired"), homeNoticeDeclined},
		{false, fmt.Errorf("%w; reopen and review current identity", errDesktopPeerChanged), homeNoticeChanged},
		{false, errCLISessionIdentity, homeNoticeChanged},
		{true, errors.New("transport broke"), homeNoticeFailed},
		{false, context_CanceledSentinel(), homeNoticeFailed},
	}
	for i, c := range cases {
		if got := desktopHomeNotice(c.viewed, c.err); got != c.want {
			t.Fatal(i, "notice", got, "want", c.want)
		}
	}
}

func context_CanceledSentinel() error { return errors.New("dial tcp 10.0.0.2:5909: i/o timeout") }

func TestHomeNoticeTableCoversEveryConstant(t *testing.T) {
	for i, notice := range homeNotices {
		if homeNoticeIndex(notice) != i {
			t.Fatal("notice index mismatch", i, notice)
		}
		if notice == "" {
			t.Fatal("empty notice", i)
		}
	}
	if homeNoticeIndex("unlisted notice") != -1 {
		t.Fatal("unlisted notice indexed")
	}
	if len(homeNotices) > 10 {
		t.Fatal("single-digit note slug exceeded")
	}
	// Every notice must render within the home presentation bounds.
	for _, notice := range homeNotices {
		if _, err := renderHomeSurface(notice, false, false, -1, false); err != nil {
			t.Fatal("notice does not fit", notice, err)
		}
	}
}

func TestDesktopHostEndedKeepsEstablishedText(t *testing.T) {
	err := fmt.Errorf("%w: host or path closed", errDesktopHostEnded)
	if err.Error() != "desktop viewing ended: host or path closed" {
		t.Fatal("established viewer text changed", err.Error())
	}
	if !errors.Is(err, errDesktopHostEnded) {
		t.Fatal("sentinel lost")
	}
}
