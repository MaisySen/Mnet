import NetworkExtension
import Mnet

// The Go TUN owns this flow. Its Close calls Cancel, which wakes a blocked Go
// reader even when NetworkExtension has not delivered the pending callback.
final class Flow: NSObject, MobilePacketFlowProtocol {
    private let flow: NEPacketTunnelFlow
    private let buffer = PacketFlowBuffer()

    init(_ flow: NEPacketTunnelFlow) { self.flow = flow }

    func readPacket() throws -> Data {
        try buffer.read { receive in
            self.flow.readPackets { packets, _ in receive(packets) }
        }
    }

    func cancel() { buffer.cancel() }

    func writePacket(_ pkt: Data?) throws {
        try buffer.checkOpen()
        guard let pkt = pkt, let first = pkt.first,
              first >> 4 == 4 || first >> 4 == 6 else {
            throw NSError(domain: "au.fintex.mnet", code: 2,
                          userInfo: [NSLocalizedDescriptionKey: "invalid IP packet"])
        }
        let proto = (first >> 4) == 6 ? AF_INET6 : AF_INET
        guard flow.writePackets([pkt], withProtocols: [NSNumber(value: proto)]) else {
            throw NSError(domain: "au.fintex.mnet", code: 3,
                          userInfo: [NSLocalizedDescriptionKey: "packet delivery failed"])
        }
    }
}
