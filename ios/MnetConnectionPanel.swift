import SwiftUI

// Shared production layout. No preferences, account access or VPN APIs live here.
// ContentView supplies the live bindings and owns the explicit connection action.
struct MnetConnectionPanel: View {
    @Binding var server: String
    @Binding var network: String
    @Binding var authKey: String
    @Binding var nodeName: String
    @Binding var exitNode: String
    @Binding var showSettings: Bool
    let statusText: String
    let connected: Bool
    let busy: Bool
    let action: () -> Void

    private let ink = Color(red: 0.086, green: 0.169, blue: 0.20)
    private let teal = Color(red: 0.031, green: 0.412, blue: 0.388)
    private let muted = Color(red: 0.302, green: 0.384, blue: 0.412)
    private let border = Color(red: 0.835, green: 0.882, blue: 0.867)

    var body: some View {
        VStack(alignment: .leading, spacing: 24) {
            VStack(alignment: .leading, spacing: 8) {
                Text("Mnet").font(.largeTitle.bold())
                Text("Your devices. Within reach.")
                    .font(.title3).foregroundStyle(muted)
            }
            .accessibilityElement(children: .combine)

            card {
                Label("Connection", systemImage: connected ? "network" : "wifi.slash")
                    .font(.headline).foregroundStyle(teal)
                Text(statusText)
                    .font(.title2.bold()).fixedSize(horizontal: false, vertical: true)
                Text("This status comes from the VPN configuration on this device. Device approval and account membership are separate.")
                    .font(.subheadline).foregroundStyle(muted)
                Button(action: action) {
                    Text(busy ? "Working..." : (connected ? "Disconnect" : "Connect"))
                        .font(.headline).frame(maxWidth: .infinity, minHeight: 28)
                        .padding(.vertical, 10)
                }
                .buttonStyle(.borderedProminent).tint(teal)
                .disabled(busy)
            }

            card {
                Text("Set up this device").font(.title2.bold())
                Text("This development build uses connection details from your network operator. Multies sign-in is not available here yet.")
                    .font(.subheadline).foregroundStyle(muted)
                field("Device name") {
                    TextField("This device", text: $nodeName)
                        .accessibilityLabel("Device name")
                }
                DisclosureGroup(isExpanded: $showSettings) {
                    VStack(alignment: .leading, spacing: 18) {
                        field("Control server URL") {
                            TextField("https://control.example", text: $server)
                                .accessibilityLabel("Control server URL")
                                .mnetIdentifierInput()
                        }
                        field("Network name") {
                            TextField("From your operator", text: $network)
                                .accessibilityLabel("Network name")
                                .mnetIdentifierInput()
                        }
                        field("Auth key, first join") {
                            SecureField("Only when required", text: $authKey)
                                .accessibilityLabel("Auth key, first join")
                                .mnetIdentifierInput()
                        }
                        Text("The auth key is not saved in ordinary app settings.")
                            .font(.footnote).foregroundStyle(muted)
                        field("Exit node, optional") {
                            TextField("Peer name", text: $exitNode)
                                .accessibilityLabel("Exit node, optional peer name")
                                .mnetIdentifierInput()
                        }
                    }
                    .padding(.top, 16)
                } label: {
                    Text("Manual connection settings").font(.headline)
                }
                .tint(teal)
                Text("Connect may ask for iOS permission to add a VPN configuration. It does not start screen sharing or remote control.")
                    .font(.footnote).foregroundStyle(muted)
            }
        }
        .foregroundStyle(ink)
        .padding(20)
        .frame(maxWidth: 760)
        .frame(maxWidth: .infinity)
        .background(Color(red: 0.957, green: 0.969, blue: 0.965))
    }

    private func card<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 16, content: content)
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.white)
            .clipShape(RoundedRectangle(cornerRadius: 20))
            .overlay(RoundedRectangle(cornerRadius: 20).stroke(border, lineWidth: 1))
    }

    private func field<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title).font(.subheadline.weight(.semibold))
            content()
                .textFieldStyle(.roundedBorder)
                .autocorrectionDisabled()
                .font(.body)
                .frame(minHeight: 44)
        }
    }
}

private extension View {
    @ViewBuilder func mnetIdentifierInput() -> some View {
#if os(iOS)
        self.textInputAutocapitalization(.never)
#else
        self
#endif
    }
}
