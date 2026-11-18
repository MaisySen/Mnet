import Foundation

@main
struct StatusReadGateTest {
    static func main() {
        let gate = StatusReadGate()
        let first = gate.begin()
        precondition(gate.isCurrent(first))
        let second = gate.begin()
        precondition(!gate.isCurrent(first) && gate.isCurrent(second), "old read won")
        gate.invalidate()
        precondition(!gate.isCurrent(second), "explicit action did not invalidate read")
        let afterAction = gate.begin()
        precondition(gate.isCurrent(afterAction))
        gate.invalidate()
        precondition(!gate.isCurrent(afterAction), "disappearance did not invalidate read")
        // Exercise the lock using simultaneous status notifications. Only the
        // latest ticket may commit after every notification has returned.
        let group = DispatchGroup(), lock = NSLock()
        var tickets: [UInt64] = []
        for _ in 0..<100 {
            group.enter()
            DispatchQueue.global().async {
                let ticket = gate.begin()
                lock.lock(); tickets.append(ticket); lock.unlock()
                group.leave()
            }
        }
        precondition(group.wait(timeout: .now() + 3) == .success)
        precondition(Set(tickets).count == 100)
        precondition(tickets.filter { gate.isCurrent($0) }.count == 1)
        gate.invalidate()
        precondition(tickets.allSatisfy { !gate.isCurrent($0) })
        print("PASS Swift latest-status-read ownership,explicit action/disappearance invalidation and100concurrent notifications; no NetworkExtension access")
    }
}
