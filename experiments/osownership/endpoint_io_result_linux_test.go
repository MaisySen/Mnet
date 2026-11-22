//go:build linux && mnet_enrollment_development

package osownership

import (
	"errors"
	"golang.org/x/sys/unix"
	"testing"
)

func TestEndpointIOResultClassification(t *testing.T) {
	cases := []struct {
		name      string
		write     bool
		n, want   int
		err       error
		wantClass endpointIODisposition
	}{
		{"read-full", false, 28, 65536, nil, endpointIOComplete},
		{"read-empty", false, -1, 65536, unix.EAGAIN, endpointIOYield},
		{"read-interrupted", false, -1, 65536, unix.EINTR, endpointIOYield},
		{"write-empty-drop", true, -1, 28, unix.EAGAIN, endpointIODrop},
		{"write-interrupted-drop", true, -1, 28, unix.EINTR, endpointIODrop},
		{"write-allocation-drop", true, -1, 28, unix.ENOBUFS, endpointIODrop},
		{"read-unexpected-allocation", false, -1, 65536, unix.ENOBUFS, endpointIOFatal},
		{"write-memory-unexpected", true, -1, 28, unix.ENOMEM, endpointIOFatal},
		{"write-down", true, -1, 28, unix.EIO, endpointIOFatal},
		{"write-detached", true, -1, 28, unix.EBADFD, endpointIOFatal},
		{"write-shape", true, -1, 28, unix.EINVAL, endpointIOFatal},
		{"write-fault", true, -1, 28, unix.EFAULT, endpointIOFatal},
		{"write-shutdown", true, -1, 28, unix.EPIPE, endpointIOFatal},
		{"write-ambiguous-positive", true, 1, 28, unix.EINTR, endpointIOFatal},
		{"write-partial", true, 27, 28, nil, endpointIOFatal},
		{"write-zero", true, 0, 28, nil, endpointIOFatal},
		{"read-zero", false, 0, 65536, nil, endpointIOFatal},
		{"read-overflow", false, 65537, 65536, nil, endpointIOFatal},
		{"read-custody-plus-yield", false, -1, 65536, errors.Join(unix.EAGAIN, ErrConflict), endpointIOFatal},
		{"write-complete", true, 28, 28, nil, endpointIOComplete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyEndpointIO(c.write, c.n, c.want, c.err); got != c.wantClass {
				t.Fatalf("got %v want %v", got, c.wantClass)
			}
		})
	}
}
