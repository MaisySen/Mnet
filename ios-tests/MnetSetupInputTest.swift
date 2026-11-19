import SwiftUI

@main struct MnetSetupInputTest {
    static func main() {
        precondition(MnetSetupInput.missing(server: "", network: "") == .server)
        precondition(MnetSetupInput.missing(server: " \n\t", network: "fixture") == .server)
        precondition(MnetSetupInput.missing(server: "https://control.example", network: " \n") == .network)
        precondition(MnetSetupInput.missing(server: "https://control.example", network: "fixture") == nil)
        // Only emptiness is classified; no spelling/identity normalization or
        // server-trust promise is introduced by this helper.
        precondition(MnetSetupInput.missing(server: "custom", network: " exact ") == nil)
        precondition(MnetSetupInput.Field.server.message.contains("Nothing was saved"))
        precondition(MnetSetupInput.Field.network.message.contains("Nothing was saved"))
        print("PASS Swift required-input policy, whitespace, exact-value preservation and fixed no-effect messages; no preferences or VPN calls")
    }
}
