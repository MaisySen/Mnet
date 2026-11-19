// macOS-only render harness. It links the SAME production layout used by iOS,
// not ContentView, the app entry point, preferences or NetworkExtension.
import AppKit
import SwiftUI

struct MnetLayoutRender {
    @MainActor static func main() throws {
        _ = NSApplication.shared
        guard let path = ProcessInfo.processInfo.environment["MNET_SWIFTUI_OUTPUT"] else {
            fatalError("MNET_SWIFTUI_OUTPUT is required")
        }
        let directory = URL(fileURLWithPath: path, isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        for fixture in [
            Fixture(name: "setup-390", width: 390, expanded: true, connected: false, busy: false, status: "Disconnected", type: .large),
            Fixture(name: "settings-320", width: 320, expanded: true, connected: false, busy: false, status: "Disconnected", type: .large),
            Fixture(name: "connected-390", width: 390, expanded: false, connected: true, busy: false, status: "VPN connected", type: .large),
            Fixture(name: "error-390-large", width: 390, expanded: true, connected: false, busy: false, status: "VPN configuration not saved. Try again from this device.", type: .accessibility3),
            Fixture(name: "busy-640", width: 640, expanded: false, connected: false, busy: true, status: "Connecting", type: .large)
        ] {
            let panel = MnetConnectionPanel(
                server: .constant(fixture.expanded ? "https://control.example" : ""),
                network: .constant(fixture.expanded ? "office-network" : ""),
                authKey: .constant(""), nodeName: .constant("My iPhone"),
                exitNode: .constant(""), showSettings: .constant(fixture.expanded),
                statusText: fixture.status, connected: fixture.connected, busy: fixture.busy,
                action: { fatalError("Render-only fixture must never invoke an action") })
                .environment(\.colorScheme, .light)
                .environment(\.dynamicTypeSize, fixture.type)
                .frame(width: fixture.width)
                .fixedSize(horizontal: false, vertical: true)
            let renderer = ImageRenderer(content: panel)
            renderer.proposedSize = ProposedViewSize(width: fixture.width, height: nil)
            renderer.scale = 2
            guard let image = renderer.cgImage else { fatalError("SwiftUI render failed: \(fixture.name)") }
            let bitmap = NSBitmapImageRep(cgImage: image)
            guard let png = bitmap.representation(using: .png, properties: [:]) else { fatalError("PNG encoding failed") }
            try png.write(to: directory.appendingPathComponent(fixture.name + ".png"))
            print("Rendered \(fixture.name): \(image.width)x\(image.height) native macOS SwiftUI pixels, synthetic state")
        }
    }

    struct Fixture {
        let name: String
        let width: CGFloat
        let expanded: Bool
        let connected: Bool
        let busy: Bool
        let status: String
        let type: DynamicTypeSize
    }
}
