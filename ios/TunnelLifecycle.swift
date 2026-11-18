import Foundation

// State is confined to queue. Potentially blocking Go prepare/start/close work
// runs elsewhere. A retiring run stays owned until both work and Close finish,
// so an old settings callback cannot race a replacement run's settings.
final class TunnelLifecycle<Resource: AnyObject, Settings> {
    enum LifecycleError: Error { case busy, stopped }
    private final class Run {
        let resource: Resource
        var completion: ((Error?) -> Void)?
        var inFlight = true
        var settingsPending = false
        var terminal = false
        var closeStarted = false
        var closed = false
        var stopCompletions: [() -> Void] = []
        init(_ resource: Resource, _ completion: @escaping (Error?) -> Void) {
            self.resource = resource; self.completion = completion
        }
        func finish(_ error: Error?) {
            let callback = completion; completion = nil; callback?(error)
        }
    }
    private let queue = DispatchQueue(label: "au.fintex.mnet.tunnel-lifecycle")
    private var current: Run?
    private let close: (Resource) -> Void

    init(close: @escaping (Resource) -> Void) { self.close = close }

    func start(make: @escaping () throws -> Resource,
               prepare: @escaping (Resource) throws -> Settings,
               apply: @escaping (Settings, @escaping (Error?) -> Void) -> Void,
               activate: @escaping (Resource) throws -> Void,
               completion: @escaping (Error?) -> Void) {
        queue.async {
            guard self.current == nil else { completion(LifecycleError.busy); return }
            let resource: Resource
            do { resource = try make() }
            catch { completion(error); return }
            let run = Run(resource, completion)
            self.current = run
            DispatchQueue.global(qos: .userInitiated).async {
                let prepared = Result { try prepare(resource) }
                self.queue.async {
                    guard !run.terminal else { self.drained(run); return }
                    switch prepared {
                    case .failure(let error):
                        run.inFlight = false
                        self.end(run, error)
                    case .success(let settings):
                        // Marked inFlight until the platform settings callback
                        // returns, including if Stop arrives while it is pending.
                        run.settingsPending = true
                        apply(settings) { error in
                            self.queue.async {
                                guard self.current === run, run.settingsPending else { return }
                                run.settingsPending = false
                                if run.terminal { self.drained(run); return }
                                if let error = error {
                                    run.inFlight = false; self.end(run, error); return
                                }
                                DispatchQueue.global(qos: .userInitiated).async {
                                    let started = Result { try activate(resource) }
                                    self.queue.async {
                                        run.inFlight = false
                                        if run.terminal { self.retire(run); return }
                                        switch started {
                                        case .success: run.finish(nil)
                                        case .failure(let error): self.end(run, error)
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    func stop(completion: @escaping () -> Void) {
        queue.async {
            guard let run = self.current else { completion(); return }
            run.stopCompletions.append(completion)
            self.end(run, LifecycleError.stopped)
        }
    }

    private func drained(_ run: Run) { run.inFlight = false; retire(run) }

    private func end(_ run: Run, _ error: Error) {
        run.terminal = true
        run.finish(error)
        if !run.closeStarted {
            run.closeStarted = true
            DispatchQueue.global(qos: .userInitiated).async {
                self.close(run.resource)
                self.queue.async { run.closed = true; self.retire(run) }
            }
        }
        retire(run)
    }

    private func retire(_ run: Run) {
        guard run.terminal && run.closed && !run.inFlight else { return }
        if current === run { current = nil }
        let completions = run.stopCompletions
        run.stopCompletions.removeAll()
        completions.forEach { $0() }
    }
}
