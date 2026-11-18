import Foundation

// A read result belongs only to the newest requested status read. Explicit
// actions invalidate every outstanding read before changing configuration.
final class StatusReadGate {
    private let lock = NSLock()
    private var generation: UInt64 = 0

    func begin() -> UInt64 {
        lock.lock(); defer { lock.unlock() }
        generation &+= 1
        return generation
    }

    func invalidate() { _ = begin() }

    func isCurrent(_ ticket: UInt64) -> Bool {
        lock.lock(); defer { lock.unlock() }
        return ticket == generation
    }
}
