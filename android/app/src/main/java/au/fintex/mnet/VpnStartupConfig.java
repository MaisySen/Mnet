package au.fintex.mnet;

// Immutable snapshot validated before constructing a node or OS VPN interface.
// This is not VPN consent, account authentication or always-on readiness.
final class VpnStartupConfig {
    final String server;
    final String network;
    final String authKey;
    final String name;

    VpnStartupConfig(String server, String network, String authKey, String name, String exitNode) {
        if (server == null || server.trim().isEmpty() || network == null || network.trim().isEmpty()) {
            throw new IllegalArgumentException("Open Mnet and configure a control server and network before connecting.");
        }
        if (exitNode != null && !exitNode.trim().isEmpty()) {
            throw new IllegalArgumentException("Exit routing is unsupported. Clear the saved exit-node field and reconnect for mesh-only access.");
        }
        this.server = server.trim();
        this.network = network.trim();
        this.authKey = authKey == null ? "" : authKey;
        this.name = name == null ? "" : name;
    }
}
