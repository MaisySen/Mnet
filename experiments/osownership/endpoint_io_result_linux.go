//go:build linux && mnet_enrollment_development

package osownership

import "golang.org/x/sys/unix"

type endpointIODisposition byte

const (
	endpointIOComplete endpointIODisposition = iota
	endpointIOYield
	endpointIODrop
	endpointIOFatal
)

// Pinned Linux6.17 tun.c: ring empty+noblock => EAGAIN before consumption;
// sock_alloc_send_pskb noblock => EAGAIN, allocation => ENOBUFS before enqueue.
// unix.Read/Write v0.50 call one raw syscall, no implicit EINTR retry. A negative
// EINTR means no reported bytes, not permission to retry the same datagram here.
// Write no-progress/allocation refusal deliberately drops that one input, while
// read no-progress yields. A later turn is a new read, never replay of a write.
// EIO(down), EBADFD(detached), EINVAL(unexpected under exact gate), EFAULT,
// EPIPE, socket-specific/unknown errors, zero/positive short writes and any
// positive count+error fail closed. A joined custody error never matches errno.
func classifyEndpointIO(write bool, n, want int, e error) endpointIODisposition {
	if n <= 0 && (e == unix.EAGAIN || e == unix.EINTR) {
		if write {
			return endpointIODrop
		}
		return endpointIOYield
	}
	if write && n <= 0 && e == unix.ENOBUFS {
		return endpointIODrop
	}
	if e != nil || want <= 0 || n <= 0 || n > want || (write && n != want) {
		return endpointIOFatal
	}
	return endpointIOComplete
}
