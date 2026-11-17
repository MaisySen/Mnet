import NetworkExtension
import Mnet // the gomobile-built framework: class MobileNode, protocol MobilePacketFlow

// Flow adapts NEPacketTunnelFlow to the Go engine's PacketFlow protocol.
// The engine reads and writes raw IP packets, one per call.
final class Flow: NSObject, MobilePacketFlow {
    private let flow: NEPacketTunnelFlow
    private let queue = DispatchQueue(label: "au.fintex.mnet.flow")

    init(_ flow: NEPacketTunnelFlow) {
        self.flow = flow
    }

    // Blocks until the system delivers a packet or the tunnel is cancelled.
    func readPacket() throws -> Data {
        var result: (Data?, Error?) = (nil, nil)
        let sem = DispatchSemaphore(value: 0)
        flow.readPackets { packets, _ in
            if let first = packets.first {
                result = (first, nil)
            }
            sem.signal()
        }
        sem.wait()
        if let data = result.0 {
            return data
        }
        throw NSError(domain: "au.fintex.mnet", code: 1,
                      userInfo: [NSLocalizedDescriptionKey: "tunnel flow ended"])
    }

    func writePacket(_ pkt: Data?) throws {
        guard let pkt = pkt else { return }
        let proto = (pkt[0] >> 4) == 6 ? AF_INET6 : AF_INET
        flow.writePackets([pkt], withProtocols: [NSNumber(value: proto)])
    }
}

// PacketTunnelProvider is the iOS NetworkExtension entry point.
final class PacketTunnelProvider: NEPacketTunnelProvider {
    private var node: MobileNode?

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let server = options?["server"] as? String ?? ""
        let network = options?["network"] as? String ?? ""
        let authKey = options?["authKey"] as? String ?? ""
        let nodeName = options?["nodeName"] as? String ?? UIDevice.current.name
        let exitNode = options?["exitNode"] as? String ?? ""

        // The extension's own container holds the node state. No app group:
        // on an individual Apple account only the Account Holder can register
        // groups, and the Fintex runbook says ship without one (apple.md).
        let dir = FileManager.default
            .urls(for: .libraryDirectory, in: .userDomainMask).first!
            .appendingPathComponent("state").path
        try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)

        var err: NSError?
        guard let node = MobileNewNode(dir, &err) else {
            completionHandler(err)
            return
        }
        self.node = node
        if !exitNode.isEmpty {
            try? node.setUseExitNode(exitNode)
        }

        DispatchQueue.global(qos: .userInitiated).async {
            // Join first: the mesh assigns our address before we configure
            // the interface, exactly like the Android flow.
            let ipv4 = node.prepare(server, network: network, name: nodeName, authKey: authKey, error: &err)
            if let err = err {
                completionHandler(err)
                return
            }

            let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
            settings.mtu = 1280
            let v4 = NEIPv4Settings(addresses: [ipv4], subnetMasks: ["255.192.0.0"])
            v4.includedRoutes = [NEIPv4Route(destinationAddress: "100.64.0.0", subnetMask: "255.192.0.0"),
                                 NEIPv4Route(destinationAddress: "100.64.0.0", subnetMask: "255.255.255.255")]
            if !exitNode.isEmpty {
                // Whole internet into the tunnel; allowed IPs steer it to
                // the exit node.
                v4.includedRoutes?.append(NEIPv4Route.default())
            }
            settings.ipv4Settings = v4
            settings.dnsSettings = NEDNSSettings(servers: ["100.64.0.0"])
            settings.dnsSettings?.matchDomains = ["mnet"]

            self.setTunnelNetworkSettings(settings) { setErr in
                if let setErr = setErr {
                    completionHandler(setErr)
                    return
                }
                var startErr: NSError?
                node.startFlow(Flow(self.packetFlow), server: server, network: network,
                               name: nodeName, authKey: authKey, error: &startErr)
                completionHandler(startErr)
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        node?.stop()
        node = nil
        completionHandler()
    }
}
