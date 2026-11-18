import Foundation

@main
struct EnrollmentPreferencesTest {
    static func main() {
        let suite = "au.fintex.mnet.fixture.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        defer { defaults.removePersistentDomain(forName: suite) }
        // Synthetic test values only. No standard preferences or real key.
        defaults.set("synthetic-enrollment-fixture", forKey: "authKey")
        defaults.set("untouched", forKey: "unrelated")
        EnrollmentPreferences.removeLegacyKey(from: defaults)
        precondition(defaults.object(forKey: "authKey") == nil)
        precondition(defaults.string(forKey: "unrelated") == "untouched")
        EnrollmentPreferences.removeLegacyKey(from: defaults)
        defaults.set("synthetic-old-copy", forKey: "authKey")
        EnrollmentPreferences.saveNonSecretSettings(to: defaults, server: "fixture.invalid",
                                                   network: "fixture-network", nodeName: "fixture-node",
                                                   exitNode: "")
        precondition(defaults.object(forKey: "authKey") == nil)
        precondition(defaults.string(forKey: "server") == "fixture.invalid")
        precondition(defaults.string(forKey: "network") == "fixture-network")
        precondition(defaults.string(forKey: "nodeName") == "fixture-node")
        precondition(defaults.string(forKey: "exitNode") == "")
        precondition(defaults.string(forKey: "unrelated") == "untouched")
        print("PASS Swift isolated-preferences legacy enrollment-copy removal,non-secret settings preservation and idempotence; no real key/preferences accessed")
    }
}
