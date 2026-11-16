package au.fintex.mnet;

public final class VpnStartupConfigTest {
    public static void main(String[] args) {
        VpnStartupConfig valid = new VpnStartupConfig(" https://fixture.invalid ", " home ", "", "phone", "");
        if (!valid.server.equals("https://fixture.invalid") || !valid.network.equals("home") || !valid.authKey.isEmpty()) {
            throw new AssertionError("configuration snapshot changed");
        }
        for (String[] values : new String[][] {
                {null, "home", ""}, {"", "home", ""}, {"  ", "home", ""},
                {"https://fixture.invalid", null, ""}, {"https://fixture.invalid", "  ", ""},
                {"https://fixture.invalid", "home", "legacy-private-exit"}
        }) {
            try {
                new VpnStartupConfig(values[0], values[1], "private-fixture-key", "phone", values[2]);
                throw new AssertionError("invalid configuration accepted");
            } catch (IllegalArgumentException expected) {
                if (expected.getMessage().contains("private-fixture-key") || expected.getMessage().contains("legacy-private-exit")) {
                    throw new AssertionError("configuration error exposed private values");
                }
            }
        }
        VpnStartupConfig noKey = new VpnStartupConfig("https://fixture.invalid", "home", null, null, null);
        if (!noKey.authKey.isEmpty() || !noKey.name.isEmpty()) throw new AssertionError("optional values not normalized");
        System.out.println("PASS Android immutable startup config, missing configuration/unsupported exit refusal and safe errors; JVM only");
    }
}
