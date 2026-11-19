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

    private let ink = Color(red: 0.114, green: 0.114, blue: 0.122)
    private let accent = Color(red: 0, green: 0.4, blue: 0.8)
    private let muted = Color(red: 95.0/255, green: 95.0/255, blue: 100.0/255)
    private let border = Color.black.opacity(0.09)

    var body: some View {
        VStack(alignment: .leading, spacing: 24) {
            MultiesBrandMark().frame(width: 40, height: 40).accessibilityLabel("Multies")
            VStack(alignment: .leading, spacing: 8) {
                Text("Mnet").font(.largeTitle.bold())
                Text("Your devices. Within reach.")
                    .font(.title3).foregroundStyle(muted)
            }
            .accessibilityElement(children: .combine)

            card {
                Label("Connection", systemImage: connected ? "network" : "wifi.slash")
                    .font(.headline).foregroundStyle(accent)
                Text(statusText)
                    .font(.title2.bold()).fixedSize(horizontal: false, vertical: true)
                Text("This status comes from the VPN configuration on this device. Device approval and account membership are separate.")
                    .font(.subheadline).foregroundStyle(muted)
                Button(action: action) {
                    Text(busy ? "Working..." : (connected ? "Disconnect" : "Connect"))
                        .font(.headline).frame(maxWidth: .infinity, minHeight: 28)
                        .padding(.vertical, 10)
                }
                .buttonStyle(.borderedProminent).tint(accent)
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
                .tint(accent)
                Text("Connect may ask for iOS permission to add a VPN configuration. It does not start screen sharing or remote control.")
                    .font(.footnote).foregroundStyle(muted)
            }
        }
        // Fixed light surfaces must not inherit dark native fields/controls.
        // Full Multies appearance preferences are not integrated in Mnet yet.
        .preferredColorScheme(.light)
        .foregroundStyle(ink)
        .padding(20)
        .frame(maxWidth: 760)
        .frame(maxWidth: .infinity)
        .background(Color(red: 242.0/255, green: 242.0/255, blue: 247.0/255))
    }

    private func card<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 16, content: content)
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(.white)
            .clipShape(RoundedRectangle(cornerRadius: 18))
            .overlay(RoundedRectangle(cornerRadius: 18).stroke(border, lineWidth: 1))
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

// Exact Multies logo-mark geometry from brand/logo-mark.svg, not a new Mnet mark.
private struct MultiesBrandMark: View {
    var body: some View {
        Canvas { context, size in
            let scale = size.width / 100
            context.scaleBy(x: scale, y: scale)
            context.translateBy(x: 0, y: 0.75)
            var m = Path()
            m.move(to: CGPoint(x: 20, y: 79))
            for point in [CGPoint(x: 20, y: 39), CGPoint(x: 50, y: 67), CGPoint(x: 80, y: 39), CGPoint(x: 80, y: 79)] { m.addLine(to: point) }
            context.stroke(m, with: .color(Color(red: 29.0/255, green: 29.0/255, blue: 31.0/255)), style: StrokeStyle(lineWidth: 21, lineCap: .round, lineJoin: .round))
            context.fill(Path(ellipseIn: CGRect(x: 39, y: 9, width: 22, height: 22)), with: .color(Color(red: 1, green: 106.0/255, blue: 26.0/255)))
        }
    }
}
