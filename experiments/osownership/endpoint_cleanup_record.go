//go:build mnet_enrollment_development

package osownership

import "time"

// Private source-owned result, not approval or packet authority. The actual
// inherited endpoint constructor installs it before any controller work.
// Only stopOwnedLocked, under the original owner lane/mutex, writes it.
// An unfinished (including interrupted/panicking) entry cannot be retried or
// interpreted as verified cleanup by a later caller.
type endpointOriginalCleanupResult struct {
	owner                                              *operation1Controller
	attempt                                            *originalAttempt
	started, finished                                  bool
	deadline                                           time.Time
	closeErr                                           error
	err                                                error
	pendingCleanup, absenceConfirmed, absenceAttempted bool
}
