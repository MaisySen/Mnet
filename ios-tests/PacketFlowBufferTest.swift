import Foundation

@main
struct PacketFlowBufferTest {
    static func main() throws {
        let buffer = PacketFlowBuffer()
        var requests = 0
        let request: (@escaping ([Data]) -> Void) -> Void = { complete in
            requests += 1
            complete([Data([1]), Data([2]), Data([3])])
        }
        for byte: UInt8 in [1, 2, 3] {
            let packet = try buffer.read(request: request)
            precondition(packet == Data([byte]), "batch lost or reordered")
        }
        precondition(requests == 1, "requested again before draining batch")
        buffer.cancel()
        do { _ = try buffer.read(request: request); preconditionFailure("closed read succeeded") }
        catch PacketFlowBuffer.FlowError.closed {}
        do { try buffer.checkOpen(); preconditionFailure("closed write preflight succeeded") }
        catch PacketFlowBuffer.FlowError.closed {}
        precondition(requests == 1, "cancel started another OS read")

        // A platform callback may never arrive. The blocked consumer must still
        // exit on Cancel, then ignore that same callback if it eventually runs.
        for _ in 0..<100 {
            let pending = PacketFlowBuffer()
            let entered = DispatchSemaphore(value: 0)
            let finished = DispatchSemaphore(value: 0)
            let callbackLock = NSLock()
            var late: (([Data]) -> Void)?
            DispatchQueue.global().async {
                do {
                    _ = try pending.read { receive in
                        callbackLock.lock(); late = receive; callbackLock.unlock()
                        entered.signal()
                    }
                    preconditionFailure("canceled waiter returned data")
                } catch PacketFlowBuffer.FlowError.closed {
                } catch { preconditionFailure("wrong read error") }
                finished.signal()
            }
            precondition(entered.wait(timeout: .now() + 2) == .success, "read not entered")
            pending.cancel(); pending.cancel()
            precondition(finished.wait(timeout: .now() + 2) == .success, "cancel left waiter blocked")
            callbackLock.lock(); let callback = late; callbackLock.unlock()
            callback?([Data([9])])
            do { _ = try pending.read(request: request); preconditionFailure("late callback revived flow") }
            catch PacketFlowBuffer.FlowError.closed {}
        }
        for batch in [[], [Data()]] as [[Data]] {
            let ended = PacketFlowBuffer()
            do { _ = try ended.read { $0(batch) }; preconditionFailure("empty platform read accepted") }
            catch PacketFlowBuffer.FlowError.closed {}
        }
        print("PASS Swift packet batch/order,100blocked-read cancellations,late callback refusal,empty reads and terminal write checks")
    }
}
