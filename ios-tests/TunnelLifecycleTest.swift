import Foundation

@main
struct TunnelLifecycleTest {
    enum Failure: Error { case expected }
    final class Node {
        let lock = NSLock()
        var closes = 0
        func close() { lock.lock(); closes += 1; lock.unlock() }
        var closeCount: Int { lock.lock(); defer { lock.unlock() }; return closes }
    }
    static func wait(_ sem: DispatchSemaphore, _ label: String) {
        precondition(sem.wait(timeout: .now() + 3) == .success, label)
    }
    static func main() {
        // Successful startup stays owned; Stop permanently closes once, and
        // only after retirement may a fresh node start.
        do {
            let node = Node()
            let owner = TunnelLifecycle<Node, Int>(close: { $0.close() })
            let started = DispatchSemaphore(value: 0), stopped = DispatchSemaphore(value: 0)
            owner.start(make: { node }, prepare: { _ in 1 }, apply: { _, done in done(nil); done(nil) },
                        activate: { _ in }, completion: { error in precondition(error == nil); started.signal() })
            wait(started, "start completion missing")
            owner.stop { stopped.signal() }; wait(stopped, "stop completion missing")
            precondition(node.closeCount == 1)
            let replacement = Node()
            owner.start(make: { replacement }, prepare: { _ in 2 }, apply: { _, done in done(nil) },
                        activate: { _ in }, completion: { error in precondition(error == nil); started.signal() })
            wait(started, "replacement rejected after retirement")
            owner.stop { stopped.signal() }; wait(stopped, "replacement stop missing")
            precondition(replacement.closeCount == 1)
        }
        // Stop while prepare is held: Close is issued before waiting for the
        // prepare worker to return; neither settings nor activate may happen.
        do {
            let node = Node(), owner = TunnelLifecycle<Node, Int>(close: { $0.close() })
            let entered = DispatchSemaphore(value: 0), release = DispatchSemaphore(value: 0)
            let canceled = DispatchSemaphore(value: 0), stopped = DispatchSemaphore(value: 0)
            owner.start(make: { node }, prepare: { _ in entered.signal(); wait(release, "prepare release"); return 1 },
                        apply: { _, _ in preconditionFailure("late settings") },
                        activate: { _ in preconditionFailure("late activate") },
                        completion: { error in precondition(error != nil); canceled.signal() })
            wait(entered, "prepare missing")
            owner.stop { stopped.signal() }; wait(canceled, "pending start not canceled")
            precondition(stopped.wait(timeout: .now() + 0.02) == .timedOut, "stop completed before worker drain")
            release.signal(); wait(stopped, "prepare retirement missing")
            precondition(node.closeCount == 1)
        }
        // Outstanding OS settings must drain before stop completes or another
        // run may begin. A delayed/duplicated callback cannot activate old work.
        do {
            let node = Node(), owner = TunnelLifecycle<Node, Int>(close: { $0.close() })
            let entered = DispatchSemaphore(value: 0), canceled = DispatchSemaphore(value: 0)
            let stopped = DispatchSemaphore(value: 0), rejected = DispatchSemaphore(value: 0)
            let lock = NSLock(); var callback: ((Error?) -> Void)?
            owner.start(make: { node }, prepare: { _ in 1 }, apply: { _, done in
                lock.lock(); callback = done; lock.unlock(); entered.signal()
            }, activate: { _ in preconditionFailure("settings callback revived stopped run") },
                        completion: { error in precondition(error != nil); canceled.signal() })
            wait(entered, "settings missing"); owner.stop { stopped.signal() }; wait(canceled, "settings start not canceled")
            owner.start(make: { preconditionFailure("created replacement while old settings pending") }, prepare: { _ in 1 },
                        apply: { _, _ in }, activate: { _ in }, completion: { error in
                precondition(error != nil); rejected.signal()
            })
            wait(rejected, "overlap not rejected")
            precondition(stopped.wait(timeout: .now() + 0.02) == .timedOut, "stop ignored settings drain")
            lock.lock(); let done = callback!; lock.unlock(); done(nil); done(nil)
            wait(stopped, "settings callback did not retire run"); precondition(node.closeCount == 1)
        }
        // Every failure after construction closes its own node once. A make
        // failure has no resource to close. Completion never means leaked state.
        for stage in 0..<4 {
            let node = Node(), owner = TunnelLifecycle<Node, Int>(close: { $0.close() })
            let failed = DispatchSemaphore(value: 0), stopped = DispatchSemaphore(value: 0)
            owner.start(make: { if stage == 0 { throw Failure.expected }; return node },
                        prepare: { _ in if stage == 1 { throw Failure.expected }; return 1 },
                        apply: { _, done in done(stage == 2 ? Failure.expected : nil) },
                        activate: { _ in if stage == 3 { throw Failure.expected } },
                        completion: { error in precondition(error != nil); failed.signal() })
            wait(failed, "failure completion missing")
            owner.stop { stopped.signal() }; wait(stopped, "failure close did not drain")
            precondition(node.closeCount == (stage == 0 ? 0 : 1), "wrong failed node close count")
        }
        // A blocked activation already dispatched before Stop must drain too.
        do {
            let node = Node(), owner = TunnelLifecycle<Node, Int>(close: { $0.close() })
            let entered = DispatchSemaphore(value: 0), release = DispatchSemaphore(value: 0)
            let canceled = DispatchSemaphore(value: 0), stopped = DispatchSemaphore(value: 0)
            owner.start(make: { node }, prepare: { _ in 1 }, apply: { _, done in done(nil) }, activate: { _ in
                entered.signal(); wait(release, "activation release")
            }, completion: { error in precondition(error != nil); canceled.signal() })
            wait(entered, "activation missing"); owner.stop { stopped.signal() }; wait(canceled, "activation cancellation missing")
            release.signal(); wait(stopped, "activation drain missing"); precondition(node.closeCount == 1)
        }
        print("PASS Swift provider ownership:stop during prepare/settings/activate,overlap refusal,duplicate callback,4failure stages,replacement and permanent close")
    }
}
