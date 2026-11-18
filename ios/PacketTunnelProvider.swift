import NetworkExtension
import UIKit
import Mnet

// Owns exactly one node from creation through prepare/settings/start or terminal
// failure/stop. No callback closes or starts a node belonging to a later run.
final class PacketTunnelProvider: NEPacketTunnelProvider {
    private let lifecycle = TunnelLifecycle<MobileNode, NEPacketTunnelNetworkSettings>(close: { $0.close() })

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        let server = options?["server"] as? String ?? ""
        let network = options?["network"] as? String ?? ""
        let authKey = options?["authKey"] as? String ?? ""
        let nodeName = options?["nodeName"] as? String ?? UIDevice.current.name
        let exitNode = options?["exitNode"] as? String ?? ""
        let dir = FileManager.default
            .urls(for: .libraryDirectory, in: .userDomainMask).first!
            .appendingPathComponent("state").path

        lifecycle.start(make: {
            try FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
            var error: NSError?
            guard let node = MobileNewNode(dir, &error) else {
                throw error ?? NSError(domain: "au.fintex.mnet", code: 4,
                                       userInfo: [NSLocalizedDescriptionKey: "node creation failed"])
            }
            return node
        }, prepare: { node in
            // Always set, including empty, so an old saved exit selection does
            // not silently outlive this request. Failure is terminal.
            try node.setUseExitNode(exitNode)
            let ipv4 = try node.prepare(server, network: network, name: nodeName, authKey: authKey)
            let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
            settings.mtu = 1280
            let v4 = NEIPv4Settings(addresses: [ipv4], subnetMasks: ["255.192.0.0"])
            v4.includedRoutes = [NEIPv4Route(destinationAddress: "100.64.0.0", subnetMask: "255.192.0.0"),
                                 NEIPv4Route(destinationAddress: "198.18.0.53", subnetMask: "255.255.255.255")]
            if !exitNode.isEmpty { v4.includedRoutes?.append(NEIPv4Route.default()) }
            settings.ipv4Settings = v4
            settings.dnsSettings = NEDNSSettings(servers: ["198.18.0.53"])
            settings.dnsSettings?.matchDomains = ["mnet"]
            return settings
        }, apply: { settings, done in
            self.setTunnelNetworkSettings(settings, completionHandler: done)
        }, activate: { node in
            try node.startFlow(Flow(self.packetFlow), server: server, network: network,
                               name: nodeName, authKey: authKey)
        }, completion: completionHandler)
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        lifecycle.stop(completion: completionHandler)
    }
}
