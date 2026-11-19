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
            Fixture(name: "error-390-accessibility-request", width: 390, expanded: true, connected: false, busy: false, status: "VPN configuration not saved. Try again from this device.", type: .accessibility3),
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
            // ImageRenderer cannot draw AppKit-backed TextField/SecureField.
            // Host the SAME layout and cache its actual native view hierarchy.
            let host = NSHostingView(rootView: panel)
            host.frame = NSRect(x: 0, y: 0, width: fixture.width, height: 1)
            host.layoutSubtreeIfNeeded()
            let size = host.fittingSize
            host.frame = NSRect(x: 0, y: 0, width: fixture.width, height: size.height)
            let window = NSWindow(contentRect: host.frame, styleMask: [.borderless], backing: .buffered, defer: false)
            window.contentView = host
            window.orderFrontRegardless()
            host.layoutSubtreeIfNeeded()
            RunLoop.current.run(until: Date(timeIntervalSinceNow: 0.15))
            guard let bitmap = host.bitmapImageRepForCachingDisplay(in: host.bounds) else { fatalError("Native bitmap failed") }
            host.cacheDisplay(in: host.bounds, to: bitmap)
            // Do not accept a successful render with an unconstrained width.
            let expectedWidth = Int((fixture.width * window.backingScaleFactor).rounded())
            guard bitmap.pixelsWide == expectedWidth, bitmap.pixelsHigh > 0 else {
                fatalError("Native bitmap dimensions do not match the requested width")
            }
            guard let png = bitmap.representation(using: .png, properties: [:]) else { fatalError("PNG encoding failed") }
            try png.write(to: directory.appendingPathComponent(fixture.name + ".png"))
            print("Rendered \(fixture.name): \(bitmap.pixelsWide)x\(bitmap.pixelsHigh) native macOS hosted SwiftUI pixels, synthetic state")
            window.orderOut(nil)
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
