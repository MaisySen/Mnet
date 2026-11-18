import Foundation

@main
struct TunnelManagerPolicyTest {
    static func main() throws {
        let own = TunnelManagerPolicy.providerBundleIdentifier
        func expect(_ values: [String?], _ index: Int?, _ label: String) throws {
            let actual = try TunnelManagerPolicy.selectedIndex(in: values)
            precondition(actual == index, label)
        }
        try expect([], nil, "absent configuration")
        try expect([nil], nil, "non-tunnel protocol")
        try expect(["different.extension"], nil, "different provider")
        try expect([own], 0, "one exact provider")
        try expect([nil, "different.extension", own], 2, "target is not first")
        try expect([own, nil, "different.extension"], 0, "target before other records")
        try expect([own + ".suffix", "AU.FINTEX.MNET.TUNNEL", " " + own], nil, "no fuzzy match")
        for providers in [[own, own], ["different.extension", own, own]] {
            do {
                _ = try TunnelManagerPolicy.selectedIndex(in: providers.map { Optional($0) })
                preconditionFailure("ambiguous matching profiles selected")
            } catch TunnelManagerPolicy.SelectionError.duplicateProfiles {
                // No first/last profile guess and no automatic destructive cleanup.
            }
        }
        print("PASS 9 Swift exact-provider/absence/ambiguity checks; no preferences or VPN operation performed")
    }
}
