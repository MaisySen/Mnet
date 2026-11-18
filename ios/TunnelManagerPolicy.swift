import Foundation

// Selection policy only. A provider belonging to another app is never ours,
// and multiple matching profiles require an explicit user cleanup decision.
enum TunnelManagerPolicy {
    static let providerBundleIdentifier = "au.fintex.mnet.tunnel"

    enum SelectionError: LocalizedError {
        case duplicateProfiles
        var errorDescription: String? {
            "More than one Mnet VPN configuration exists. Review them in Settings before connecting."
        }
    }

    static func selectedIndex(in providerIDs: [String?]) throws -> Int? {
        let matches = providerIDs.indices.filter {
            providerIDs[$0] == providerBundleIdentifier
        }
        guard matches.count <= 1 else { throw SelectionError.duplicateProfiles }
        return matches.first
    }
}
