package main

import (
	"context"
	"io"

	"fintex.au/mnet/internal/session"
)

// Safety is acquired before capture. The local owner arbitrates cooperating
// Mnet input tools and reserves a focus-independent live-process stop path.
// It does not isolate arbitrary X11 applications or survive process/X death.
type attendedSafety interface {
	io.Closer
	Stopped() <-chan struct{}
}

func bindAttendedSafety(ctx context.Context, m *session.Mux, safety attendedSafety, sessionDone <-chan struct{}, failed func(error)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-safety.Stopped():
			m.Close()
		case <-ctx.Done():
			m.Close()
		case <-sessionDone:
		}
		<-sessionDone
		if err := safety.Close(); err != nil && failed != nil {
			failed(err)
		}
	}()
	return done
}

// Compose local-owner liveness with peer authorization, never use the owner as
// a grant. The input layer also checks this inside its serialized dispatch.
func attendedSafetyAuthorized(safety attendedSafety, peer func() bool) func() bool {
	return func() bool {
		select {
		case <-safety.Stopped():
			return false
		default:
		}
		return peer != nil && peer()
	}
}
