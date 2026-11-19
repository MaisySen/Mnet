import SwiftUI
import UIKit
import NetworkExtension

// Join form plus live mesh status. The tunnel itself runs in the
// PacketTunnel extension; the app talks to it through NETunnelProviderManager.
struct ContentView: View {
    @State private var server = UserDefaults.standard.string(forKey: "server") ?? ""
    @State private var network = UserDefaults.standard.string(forKey: "network") ?? ""
    @State private var authKey = ""
    @State private var nodeName = UIDevice.current.name
    @State private var exitNode = UserDefaults.standard.string(forKey: "exitNode") ?? ""
    @State private var statusText = "Disconnected"
    @State private var connected = false
    @State private var busy = false
    @State private var statusReads = StatusReadGate()
    @State private var showSettings = false
    @Environment(\.scenePhase) private var scenePhase

    var body: some View {
        NavigationStack {
            ScrollView {
                MnetConnectionPanel(server: $server, network: $network,
                                    authKey: $authKey, nodeName: $nodeName,
                                    exitNode: $exitNode, showSettings: $showSettings,
                                    statusText: statusText, connected: connected, busy: busy) {
                    save()
                    connected ? disconnect() : connect()
                }
            }
            .background(Color(red: 242.0/255, green: 242.0/255, blue: 247.0/255))
            .preferredColorScheme(.light)
            .navigationTitle("Mnet")
            .navigationBarTitleDisplayMode(.inline)
            .onAppear {
                EnrollmentPreferences.removeLegacyKey(from: .standard)
                if server.isEmpty || network.isEmpty { showSettings = true }
                refresh()
            }
            .onReceive(NotificationCenter.default.publisher(for: .NEVPNStatusDidChange)) { _ in refresh() }
            .onReceive(NotificationCenter.default.publisher(for: .NEVPNConfigurationChange)) { _ in refresh() }
            .onChange(of: scenePhase) { phase in if phase == .active { refresh() } }
            .onDisappear { statusReads.invalidate() }
        }
    }

    private func save() {
        EnrollmentPreferences.saveNonSecretSettings(to: .standard, server: server,
                                                     network: network, nodeName: nodeName,
                                                     exitNode: exitNode)
    }

    // Loading is read-only. A new unsaved Mnet manager is constructed only for
    // explicit Connect. Never select the first arbitrary VPN configuration.
    private func manager(createIfMissing: Bool,
                         _ cb: @escaping (Result<NETunnelProviderManager?, Error>) -> Void) {
        NETunnelProviderManager.loadAllFromPreferences { managers, error in
            DispatchQueue.main.async {
                if let error = error { cb(.failure(error)); return }
                let managers = managers ?? []
                do {
                    let ids = managers.map {
                        ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier
                    }
                    if let index = try TunnelManagerPolicy.selectedIndex(in: ids) {
                        cb(.success(managers[index]))
                        return
                    }
                    guard createIfMissing else { cb(.success(nil)); return }
                    let manager = NETunnelProviderManager()
                    let proto = NETunnelProviderProtocol()
                    proto.providerBundleIdentifier = TunnelManagerPolicy.providerBundleIdentifier
                    proto.serverAddress = "mnet"
                    manager.protocolConfiguration = proto
                    manager.localizedDescription = "Mnet"
                    cb(.success(manager))
                } catch { cb(.failure(error)) }
            }
        }
    }

    private func connect() {
        statusReads.invalidate()
        busy = true
        // This explicit request owns an immutable snapshot, even if the form
        // changes while preference save/reload callbacks are pending.
        let opts: [String: NSObject] = [
            "server": server as NSObject,
            "network": network as NSObject,
            "authKey": authKey as NSObject,
            "nodeName": nodeName as NSObject,
            "exitNode": exitNode as NSObject,
        ]
        let submittedKey = authKey
        manager(createIfMissing: true) { result in
            switch result {
            case .failure(let error):
                busy = false
                statusText = "Connect unavailable: \(error.localizedDescription)"
            case .success(let manager):
                guard let manager = manager else { busy = false; return }
                // Saving/enabling is allowed only after the explicit Connect
                // action. Reload before start instead of assuming save made it ready.
                manager.isEnabled = true
                manager.saveToPreferences { saveError in
                    if let error = saveError {
                        DispatchQueue.main.async {
                            busy = false
                            statusText = "VPN configuration not saved: \(error.localizedDescription)"
                        }
                        return
                    }
                    manager.loadFromPreferences { loadError in
                        DispatchQueue.main.async {
                            defer { busy = false }
                            if let error = loadError {
                                statusText = "VPN configuration not loaded: \(error.localizedDescription)"
                                return
                            }
                            do {
                                try manager.connection.startVPNTunnel(options: opts)
                                if authKey == submittedKey { authKey = "" }
                                busy = false
                                refresh()
                            } catch {
                                statusText = "Start failed: \(error.localizedDescription)"
                            }
                        }
                    }
                }
            }
        }
    }

    private func disconnect() {
        statusReads.invalidate()
        busy = true
        manager(createIfMissing: false) { result in
            busy = false
            switch result {
            case .failure(let error):
                statusText = "Disconnect unavailable: \(error.localizedDescription)"
            case .success(let manager):
                manager?.connection.stopVPNTunnel()
                refresh()
            }
        }
    }

    private func refresh() {
        // OS notifications during an explicit mutation are reconciled by the
        // final refresh. They must not race its error or selected action.
        guard !busy else { return }
        let ticket = statusReads.begin()
        manager(createIfMissing: false) { result in
            guard statusReads.isCurrent(ticket), !busy else { return }
            switch result {
            case .failure(let error):
                statusText = "VPN configuration unavailable: \(error.localizedDescription)"
            case .success(let manager):
                guard let manager = manager else {
                    connected = false
                    statusText = "Disconnected"
                    return
                }
                connected = manager.connection.status == .connected || manager.connection.status == .connecting
                switch manager.connection.status {
                case .invalid: statusText = "VPN configuration unavailable"
                case .disconnected: statusText = "Disconnected"
                case .connecting: statusText = "Connecting"
                case .connected: statusText = "VPN connected"
                case .reasserting: statusText = "Reconnecting"
                case .disconnecting: statusText = "Disconnecting"
                @unknown default: statusText = "VPN status unavailable"
                }
            }
        }
    }
}
