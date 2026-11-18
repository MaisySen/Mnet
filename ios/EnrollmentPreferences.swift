import Foundation

// Enrollment input belongs to this explicit connection, not ordinary app
// preferences. This removes only the redundant legacy app copy. The native
// daemon's separate protected state/rejoin policy is unchanged.
enum EnrollmentPreferences {
    static func removeLegacyKey(from defaults: UserDefaults) {
        defaults.removeObject(forKey: "authKey")
    }

    static func saveNonSecretSettings(to defaults: UserDefaults,
                                      server: String, network: String,
                                      nodeName: String, exitNode: String) {
        removeLegacyKey(from: defaults)
        defaults.set(server, forKey: "server")
        defaults.set(network, forKey: "network")
        defaults.set(nodeName, forKey: "nodeName")
        defaults.set(exitNode, forKey: "exitNode")
    }
}
