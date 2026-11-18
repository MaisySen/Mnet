import Foundation

// One outstanding platform read, preserving every packet in its returned batch.
// Cancel releases a blocked consumer even if that platform callback never runs.
final class PacketFlowBuffer {
    enum FlowError: Error { case closed }
    private let condition = NSCondition()
    private var packets: [Data] = []
    private var reading = false
    private var closed = false

    func read(request: (@escaping ([Data]) -> Void) -> Void) throws -> Data {
        condition.lock()
        defer { condition.unlock() }
        while !closed && packets.isEmpty {
            if !reading {
                reading = true
                condition.unlock()
                request { [weak self] batch in self?.receive(batch) }
                condition.lock()
            } else {
                condition.wait()
            }
        }
        guard !closed else { throw FlowError.closed }
        return packets.removeFirst()
    }

    private func receive(_ batch: [Data]) {
        condition.lock()
        defer { condition.unlock() }
        guard !closed else { return }
        reading = false
        // A completed empty read means the platform ended the flow. Empty
        // packets cannot be valid IP packets and must not index pkt[0].
        if batch.isEmpty || batch.contains(where: { $0.isEmpty }) {
            closed = true
            packets.removeAll()
        } else {
            packets.append(contentsOf: batch)
        }
        condition.broadcast()
    }

    func cancel() {
        condition.lock()
        closed = true
        packets.removeAll()
        condition.broadcast()
        condition.unlock()
    }

    func checkOpen() throws {
        condition.lock()
        defer { condition.unlock() }
        guard !closed else { throw FlowError.closed }
    }
}
