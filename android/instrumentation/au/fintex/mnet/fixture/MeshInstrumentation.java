package au.fintex.mnet.fixture;

import android.app.Activity;
import android.app.Instrumentation;
import android.content.Intent;
import android.graphics.Bitmap;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.accessibility.AccessibilityNodeInfo;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;
import java.io.*;
import java.net.*;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.atomic.AtomicReference;
import org.json.JSONObject;
import au.fintex.mnet.MainActivity;
import au.fintex.mnet.MnetVpnService;
import au.fintex.mnet.R;

// Disposable emulator-only instrumentation. Uses real app/JNI/VPN and an
// isolated real Go mesh peer; no production account, backend or user data.
public class MeshInstrumentation extends Instrumentation {
    private Bundle options;
    private Activity activity;
    private String stage = "created";
    private boolean consentClicked;
    @Override public void onCreate(Bundle args) { super.onCreate(args); options = args; start(); }
    private interface Probe { boolean ready() throws Exception; }
    private void await(Probe probe, String label) throws Exception {
        stage = label;
        long deadline = SystemClock.elapsedRealtime() + 60000;
        while (SystemClock.elapsedRealtime() < deadline) {
            if (probe.ready()) return;
            SystemClock.sleep(150);
        }
        throw new IllegalStateException("bounded fixture condition not met");
    }
    private void click(int id) { runOnMainSync(() -> ((Button) activity.findViewById(id)).performClick()); }
    private void set(int id, String value) { runOnMainSync(() -> ((EditText) activity.findViewById(id)).setText(value)); }
    private void vpnConsent() throws Exception {
        await(() -> {
            if (MnetVpnService.currentNode() != null) return true;
            AccessibilityNodeInfo root = getUiAutomation().getRootInActiveWindow();
            if (root == null || !"com.android.vpndialogs".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())) return false;
            // Click the actual OS consent button, never a setting that disables
            // Play Protect or a production-phone policy. English emulator UI.
            List<AccessibilityNodeInfo> buttons = root.findAccessibilityNodeInfosByViewId("android:id/button1");
            for (AccessibilityNodeInfo button : buttons) {
                if ("OK".equals(String.valueOf(button.getText()))) {
                    consentClicked = button.performAction(AccessibilityNodeInfo.ACTION_CLICK);
                    return true;
                }
            }
            return false;
        }, "vpn-consent");
    }
    private boolean up() {
        if (MnetVpnService.currentNode() == null) return false;
        try { return "up".equals(new JSONObject(MnetVpnService.currentNode().status()).optString("status")); }
        catch (Exception unavailable) { return false; }
    }
    private void hashRoundTrip() throws Exception {
        stage = "mesh-hash-round-trip";
        // This address has no host TCP listener. Reaching it requires the real
        // Android VPN fd, WireGuard and the Go peer's userspace network stack.
        byte[] sent = new byte[4 << 20];
        byte[] unit = "kernel-peer-proof".getBytes("UTF-8");
        for (int i = 0; i < sent.length; i++) sent[i] = unit[i % unit.length];
        try (Socket socket = new Socket()) {
            stage = "mesh-tcp-connect";
            socket.connect(new InetSocketAddress(options.getString("peer"), 45993), 45000);
            socket.setSoTimeout(45000);
            stage = "mesh-outbound-send";
            socket.getOutputStream().write(sent);
            DataInputStream input = new DataInputStream(socket.getInputStream());
            stage = "mesh-outbound-hash-read";
            byte[] hash = new byte[32]; input.readFully(hash);
            if (!Arrays.equals(hash, MessageDigest.getInstance("SHA-256").digest(sent))) throw new IOException("outbound digest mismatch");
            stage = "mesh-reverse-read";
            byte[] reverse = new byte[4 << 20]; input.readFully(reverse);
            byte[] expectedUnit = "reverse-native-peer-proof".getBytes("UTF-8");
            for (int i = 0; i < reverse.length; i++) if (reverse[i] != expectedUnit[i % expectedUnit.length]) throw new IOException("reverse bytes mismatch");
            stage = "mesh-reverse-hash-send";
            socket.getOutputStream().write(MessageDigest.getInstance("SHA-256").digest(reverse));
            if (input.readByte() != 'O' || input.readByte() != 'K') throw new IOException("reverse acknowledgment mismatch");
        }
    }
    @Override public void onStart() {
        Bundle result = new Bundle();
        try {
            android.accessibilityservice.AccessibilityServiceInfo info = getUiAutomation().getServiceInfo();
            info.flags |= android.accessibilityservice.AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS;
            getUiAutomation().setServiceInfo(info);
            stage = "actual-apk-launch";
            Intent launch = new Intent(getTargetContext(), MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            activity = startActivitySync(launch);
            // Capture before fixture keys enter the UI. Never export app state,
            // enrollment keys, full status or logcat as public CI artifacts.
            Bitmap launchImage = getUiAutomation().takeScreenshot();
            try (FileOutputStream file = new FileOutputStream(new File(getTargetContext().getFilesDir(), "fixture-launch.png"))) {
                if (launchImage == null || !launchImage.compress(Bitmap.CompressFormat.PNG, 100, file)) throw new IOException("screenshot unavailable");
            }
            set(R.id.server_url, options.getString("server"));
            set(R.id.network_name, "android-emulator");
            set(R.id.auth_key, options.getString("key"));
            set(R.id.device_name, "android-emulator");
            set(R.id.exit_node, "");
            click(R.id.connect_button);
            vpnConsent();
            await(this::up, "initial-vpn-up");
            hashRoundTrip();
            click(R.id.connect_button);
            await(() -> MnetVpnService.currentNode() == null, "manual-stop");
            // A new actual worker/Node must recover saved identity after stop.
            await(() -> { for (Thread thread : Thread.getAllStackTraces().keySet()) if ("mnet-vpn".equals(thread.getName()) && thread.isAlive()) return false; return true; }, "worker-exit-before-reconnect");
            waitForIdleSync();
            click(R.id.connect_button);
            vpnConsent();
            await(this::up, "reconnect-vpn-up");
            hashRoundTrip();
            click(R.id.connect_button);
            await(() -> MnetVpnService.currentNode() == null, "final-stop");
            if (!consentClicked) throw new IllegalStateException("no actual initial VPN consent click observed");
            stage = "complete";
            result.putString("summary", "PASS actual APK/JNI/VPN consent, bidirectional 4MiB hash, manual stop/reconnect and repeat hash; emulator only, no physical reboot/capture/control proof");
            finish(Activity.RESULT_OK, result);
        } catch (Throwable failure) {
            // Deliberately omit exception messages, app state and arguments.
            String diagnostics = "";
            try {
                if (MnetVpnService.currentNode() != null) {
                    JSONObject state = new JSONObject(MnetVpnService.currentNode().status());
                    org.json.JSONArray peers = state.optJSONArray("peers");
                    boolean handshake = false, received = false, transmitted = false, relay = false;
                    if (peers != null) for (int i = 0; i < peers.length(); i++) {
                        JSONObject peer = peers.getJSONObject(i);
                        handshake |= peer.optBoolean("online");
                        received |= peer.optLong("rx_bytes") > 0;
                        transmitted |= peer.optLong("tx_bytes") > 0;
                        relay |= peer.optBoolean("relay");
                    }
                    diagnostics = " peersPresent=" + (peers != null && peers.length() > 0)
                        + " handshake=" + handshake + " received=" + received + " transmitted=" + transmitted + " relay=" + relay;
                }
            } catch (Exception ignored) { diagnostics = " diagnosticsUnavailable=true"; }
            result.putString("summary", "FAIL stage=" + stage + " type=" + failure.getClass().getSimpleName() + diagnostics);
            finish(Activity.RESULT_CANCELED, result);
        }
    }
}
