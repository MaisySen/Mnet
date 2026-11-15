import SwiftUI
import NetworkExtension

// Join form plus live mesh status. The tunnel itself runs in the
// PacketTunnel extension; the app talks to it through NETunnelProviderManager.
struct ContentView: View {
    @State private var server = UserDefaults.standard.string(forKey: "server") ?? ""
    @State private var network = UserDefaults.standard.string(forKey: "network") ?? ""
    @State private var authKey = UserDefaults.standard.string(forKey: "authKey") ?? ""
    @State private var nodeName = UIDevice.current.name
    @State private var statusText = "Disconnected"
    @State private var connected = false
    @State private var busy = false

    var body: some View {
        NavigationStack {
            Form {
                Section("Control plane") {
                    TextField("Control server URL", text: $server)
                        .textContentType(.URL)
                        .autocapitalization(.none)
                    TextField("Network name", text: $network)
                        .autocapitalization(.none)
                    TextField("Auth key (first join)", text: $authKey)
                        .autocapitalization(.none)
                    TextField("This device name", text: $nodeName)
                }
                Section {
                    Button(connected ? "Disconnect" : (busy ? "Working..." : "Connect")) {
                        save()
                        connected ? disconnect() : connect()
                    }
                    .disabled(busy)
                }
                Section("Status") {
                    Text(statusText)
                        .font(.system(.footnote, design: .monospaced))
                }
            }
            .navigationTitle("Mnet")
            .onAppear(perform: refresh)
        }
    }

    private func save() {
        let d = UserDefaults.standard
        d.set(server, forKey: "server")
        d.set(network, forKey: "network")
        d.set(authKey, forKey: "authKey")
        d.set(nodeName, forKey: "nodeName")
    }

    private func manager(_ cb: @escaping (NETunnelProviderManager?) -> Void) {
        NETunnelProviderManager.loadAllFromPreferences { managers, _ in
            if let m = managers?.first {
                cb(m)
                return
            }
            let m = NETunnelProviderManager()
            let proto = NETunnelProviderProtocol()
            proto.providerBundleIdentifier = "au.fintex.mnet.tunnel"
            proto.serverAddress = "mnet"
            m.protocolConfiguration = proto
            m.localizedDescription = "Mnet"
            m.isEnabled = true
            m.saveToPreferences { _ in cb(m) }
        }
    }

    private func connect() {
        busy = true
        manager { m in
            guard let m = m else { busy = false; return }
            let opts: [String: NSObject] = [
                "server": server as NSObject,
                "network": network as NSObject,
                "authKey": authKey as NSObject,
                "nodeName": nodeName as NSObject,
            ]
            do {
                try m.connection.startVPNTunnel(options: opts)
                connected = true
            } catch {
                statusText = "Start failed: \(error.localizedDescription)"
            }
            busy = false
            refresh()
        }
    }

    private func disconnect() {
        manager { m in
            m?.connection.stopVPNTunnel()
            connected = false
            refresh()
        }
    }

    private func refresh() {
        manager { m in
            guard let m = m else { return }
            connected = m.connection.status == .connected || m.connection.status == .connecting
            statusText = "VPN status: \(m.connection.status.rawValue)"
        }
    }
}
