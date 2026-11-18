# Incoming file completion and terminal session ownership

The incoming file receiver previously checked current authorization at storage boundaries but did not include the mux's terminal state. A closed session with no extra authorization gate could still create its destination directory and a temporary file. A session ending during the final authorization callback could still hard-link the verified temporary file into place and call OnDone.

The storage guard now checks the owning mux before invoking authorization and again after it returns. Existing checks cover entry, directory setup, each data write, publication and completion notification. A new check before Sync also refuses newly dispatched persistence work after a known terminal session. A late true authorization result cannot revive that session. The private temporary file is still closed and removed on return.

These are checks at work boundaries, not an atomic filesystem transaction with session closure. An OS write, Sync or Link already dispatched is not interrupted or rolled back. Closure racing after a successful pre-Link check can still leave a verified destination file. A destination already published before closure is preserved; completion notification is skipped when closure is observed at its guard. A callback already dispatched can still finish. No deletion of completed user files or forced erasure is added. The receiver's destination, authorization, overwrite, size, digest, idle timeout and retention rules otherwise remain unchanged.

## Evidence

Two focused test families pass 30 fresh race repetitions locally. They cover closed-before-entry with and without an extra authorization callback, and deterministic closure during the directory, data-write, pre-Sync, publication and notification guards. Before publication, no destination remains and temporary files are cleaned. At the post-publication notification guard, the verified file remains but OnDone is not called. These tests use net.Pipe framed sessions and disposable fixture directories, not user files.

Restoring the old authorization-only guard in an overlay reproduces unwanted directory creation, work after closure and completion notification. Full uncached repository race tests and vet pass locally. Desktop CI includes 30 focused repetitions on Windows and macOS; its exact-revision results must be collected before claiming those platforms passed.

This does not establish user consent flows, production/live transfer, physical mobile filesystem behavior, hard cancellation latency or application release readiness. No Multies policy, account/device, signing, payment, closed Android networking experiment or release action is included.
