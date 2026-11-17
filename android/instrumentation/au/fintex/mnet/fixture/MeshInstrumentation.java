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
import au.fintex.mnet.HostingActivity;
import au.fintex.mnet.MnetVpnService;
import au.fintex.mnet.R;

// Disposable emulator-only instrumentation. Uses real app/JNI/VPN and an
// isolated real Go mesh peer; no production account, backend or user data.
public class MeshInstrumentation extends Instrumentation {
    private Bundle options;
    private Activity activity;
    private String stage = "created";
    private boolean consentClicked;
    private boolean entireScreenSelected;
    @Override public void onCreate(Bundle args) { super.onCreate(args); options = args; start(); }
    private interface Probe { boolean ready() throws Exception; }
    private void checkpoint(String label) throws Exception {
        stage=label;
        try(FileOutputStream file=new FileOutputStream(new File(getTargetContext().getFilesDir(),"fixture-stage"))){file.write(label.getBytes("UTF-8"));}
    }
    private void await(Probe probe, String label) throws Exception {
        checkpoint(label);
        long deadline = SystemClock.elapsedRealtime() + 60000;
        while (SystemClock.elapsedRealtime() < deadline) {
            if (probe.ready()) return;
            SystemClock.sleep(150);
        }
        throw new IllegalStateException("bounded fixture condition not met");
    }
    private void click(int id) { runOnMainSync(() -> ((Button) activity.findViewById(id)).performClick()); }
    private void set(int id, String value) { runOnMainSync(() -> ((EditText) activity.findViewById(id)).setText(value)); }
    // All callers run before enrollment values enter the UI. Require visible
    // actions and consecutive equal compositor frames, not only a ready tree.
    private AccessibilityNodeInfo captureSettledDialog(String title, String filename, String... actions) throws Exception {
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                    || root.findAccessibilityNodeInfosByText(title).isEmpty()) return false;
            for (String action : actions) {
                boolean visible = false;
                for (AccessibilityNodeInfo node : root.findAccessibilityNodeInfosByText(action)) {
                    android.graphics.Rect bounds = new android.graphics.Rect();
                    node.getBoundsInScreen(bounds);
                    visible |= node.isVisibleToUser() && !bounds.isEmpty();
                }
                if (!visible) return false;
            }
            return true;
        }, "dialog-ready-" + filename);
        waitForIdleSync();
        Bitmap previous = null;
        int equalFrames = 0;
        long deadline = SystemClock.elapsedRealtime() + 15000;
        try {
            while (SystemClock.elapsedRealtime() < deadline) {
                SystemClock.sleep(350);
                Bitmap current = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).takeScreenshot();
                if (current == null) throw new IOException("dialog screenshot unavailable");
                equalFrames = previous != null && previous.sameAs(current) ? equalFrames + 1 : 0;
                if (previous != null) previous.recycle();
                previous = current;
                if (equalFrames >= 3) {
                    try (FileOutputStream file = new FileOutputStream(new File(getTargetContext().getFilesDir(), filename))) {
                        if (!current.compress(Bitmap.CompressFormat.PNG, 100, file)) throw new IOException("dialog screenshot failed");
                    }
                    return getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                }
            }
            throw new IOException("dialog compositor did not settle");
        } finally { if (previous != null) previous.recycle(); }
    }
    private void vpnDisclosure(boolean agree, boolean capture) throws Exception {
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                    || root.findAccessibilityNodeInfosByText("VPN data disclosure").isEmpty()) return false;
            if (capture) {
                root = captureSettledDialog("VPN data disclosure", "fixture-vpn-disclosure.png", "Agree", "Not now");
            }
            return clickText(root, agree ? "Agree" : "Not now");
        }, agree ? "vpn-disclosure-agree" : "vpn-disclosure-decline");
        if (agree) {
            await(() -> {
                if (getTargetContext().getSharedPreferences("mnet", 0).getBoolean("vpn_disclosure_v1", false)) return true;
                // An accessibility click acknowledgement is not proof that
                // the app ran its listener. Retry only while still unaccepted.
                AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                if (root != null && "au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                        && !root.findAccessibilityNodeInfosByText("VPN data disclosure").isEmpty()) clickText(root, "Agree");
                return false;
            }, "vpn-disclosure-accepted");
        }
    }
    private void vpnConsent() throws Exception {
        await(() -> {
            if (MnetVpnService.currentNode() != null) return true;
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"com.android.vpndialogs".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())) return false;
            // Click the actual OS consent button, never a setting that disables
            // Play Protect or a production-phone policy. English emulator UI.
            List<AccessibilityNodeInfo> buttons = root.findAccessibilityNodeInfosByViewId("android:id/button1");
            for (AccessibilityNodeInfo button : buttons) {
                if ("OK".equals(String.valueOf(button.getText()))) {
                    consentClicked = button.performAction(AccessibilityNodeInfo.ACTION_CLICK);
                    return consentClicked;
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
    private void osVpnReady() throws Exception {
        // Node.up is engine state. Android publishes the VPN network separately;
        // opening a socket before that can cache an underlay source address.
        await(() -> {
            android.net.ConnectivityManager cm = getTargetContext().getSystemService(android.net.ConnectivityManager.class);
            android.net.Network network = cm.getActiveNetwork();
            android.net.NetworkCapabilities caps = cm.getNetworkCapabilities(network);
            android.net.LinkProperties props = cm.getLinkProperties(network);
            if (caps == null || props == null || !caps.hasTransport(android.net.NetworkCapabilities.TRANSPORT_VPN)) return false;
            String self = MnetVpnService.currentNode().meshIPv4();
            for (android.net.LinkAddress address : props.getLinkAddresses()) {
                if (self.equals(address.getAddress().getHostAddress())) return true;
            }
            return false;
        }, "os-vpn-published");
    }
    private void hashRoundTrip() throws Exception {
        stage = "mesh-hash-round-trip";
        // This address has no host TCP listener. Reaching it requires the real
        // Android VPN fd, WireGuard and the Go peer's userspace network stack.
        byte[] sent = new byte[4 << 20];
        byte[] unit = "kernel-peer-proof".getBytes("UTF-8");
        for (int i = 0; i < sent.length; i++) sent[i] = unit[i % unit.length];
        try (Socket socket = new Socket()) {
            stage = "mesh-tcp-local-bind";
            // Isolate source-address selection from encrypted transport. This
            // experiment does not prove unbound third-party socket behavior.
            socket.bind(new InetSocketAddress(MnetVpnService.currentNode().meshIPv4(), 0));
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
    private boolean clickText(AccessibilityNodeInfo root, String text) {
        if (root == null) return false;
        if (text.equalsIgnoreCase(String.valueOf(root.getText()))) {
            AccessibilityNodeInfo candidate = root;
            for (int i=0; candidate != null && i<4; i++, candidate=candidate.getParent()) {
                if (candidate.isClickable() && candidate.isEnabled()) return candidate.performAction(AccessibilityNodeInfo.ACTION_CLICK);
            }
        }
        for (int i=0;i<root.getChildCount();i++) if (clickText(root.getChild(i), text)) return true;
        return false;
    }
    private void enableRemoteControl() throws Exception {
        stage = "accessibility-disclosure";
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Open accessibility settings"), "accessibility-open-disclosure");
        SystemClock.sleep(3500);
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Not now"), "accessibility-decline");
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            return root != null && "au.fintex.mnet".contentEquals(root.getPackageName()) && !au.fintex.mnet.MnetAccessibilityService.available();
        }, "accessibility-decline-stays-in-app");
        SystemClock.sleep(1500);
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Open accessibility settings"), "accessibility-reopen-disclosure");
        SystemClock.sleep(3500);
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Agree"), "accessibility-agree");
        await(() -> {
            if (au.fintex.mnet.MnetAccessibilityService.available()) return true;
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null) return false;
            String pkg = String.valueOf(root.getPackageName());
            if (!pkg.equals("com.android.settings") && !pkg.equals("com.android.permissioncontroller")) return false;
            if (clickText(root, "Allow")) return false;
            if (clickText(root, "Use Mnet")) return false;
            if (clickText(root, "Mnet")) return false;
            if (clickText(root, "Downloaded apps") || clickText(root, "Installed apps")) return false;
            return false;
        }, "accessibility-system-enable");
        // Return to the already-created host page, without replaying consent.
        runOnMainSync(() -> activity.startActivity(new Intent(activity, HostingActivity.class).addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT)));
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            return root != null && "au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName());
        }, "accessibility-return-host");
    }
    private void screenShareProof() throws Exception {
        activity = startActivitySync(new Intent(getTargetContext(), HostingActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        new File(getTargetContext().getFilesDir(), "fixture-record-start").createNewFile();
        await(() -> new File(getTargetContext().getFilesDir(), "fixture-record-ready").exists(), "consent-record-ready");
        enableRemoteControl();
        java.util.concurrent.atomic.AtomicBoolean remoteTapped = new java.util.concurrent.atomic.AtomicBoolean();
        final Button[] target = new Button[1];
        runOnMainSync(() -> {
            target[0] = new Button(activity);
            target[0].setText("Disposable remote tap target");
            target[0].setOnClickListener(v -> { remoteTapped.set(true); target[0].setText("Remote mesh tap received"); });
            android.view.ViewGroup content = activity.findViewById(android.R.id.content);
            ((android.view.ViewGroup) content.getChildAt(0)).addView(target[0], 1);
        });
        waitForIdleSync();
        JSONObject point = new JSONObject();
        runOnMainSync(() -> {
            try {
                int[] location = new int[2]; target[0].getLocationOnScreen(location);
                point.put("x", location[0]+target[0].getWidth()/2);
                point.put("y", location[1]+target[0].getHeight()/2);
                android.util.DisplayMetrics metrics = new android.util.DisplayMetrics();
                activity.getWindowManager().getDefaultDisplay().getRealMetrics(metrics);
                point.put("width", metrics.widthPixels); point.put("height", metrics.heightPixels);
            } catch(Exception e) { throw new RuntimeException(e); }
        });
        try (FileOutputStream file = new FileOutputStream(new File(getTargetContext().getFilesDir(), "fixture-input-target.json"))) {
            file.write(point.toString().getBytes("UTF-8"));
        }
        // A static screen is allowed to produce one image only. Animate a
        // disposable marker so the two-decoded-frame proof tests real updates,
        // rather than depending on incidental OS animation timing.
        android.os.Handler pulseHandler = new android.os.Handler(android.os.Looper.getMainLooper());
        pulseHandler.post(new Runnable() {
            int pulse;
            public void run() {
                if (remoteTapped.get() || activity.isFinishing()) return;
                target[0].setText("Disposable remote tap target " + (++pulse));
                pulseHandler.postDelayed(this, 500);
            }
        });
        click(R.id.start_hosting_button);
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"com.android.systemui".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())) return false;
            // API36 defaults to single-app capture and a Next button. The
            // disposable test deliberately selects this empty emulator's screen.
            if (!entireScreenSelected) {
                if (clickText(root, "Share entire screen") || clickText(root, "Entire screen")) { entireScreenSelected = true; return false; }
                if (clickText(root, "Share one app")) return false;
            }
            return clickText(root, "Start now") || clickText(root, "Start") || clickText(root, "Share screen");
        }, "screen-projection-consent");
        new File(getTargetContext().getFilesDir(), "fixture-projection-ready").createNewFile();
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())) return false;
            SystemClock.sleep(1500);
            return clickText(root, "Allow");
        }, "screen-viewer-consent");
        await(() -> new File(getTargetContext().getFilesDir(), "fixture-screen-received").exists(), "screen-decoded-by-peer");
        await(remoteTapped::get, "remote-mesh-gesture-click");
        SystemClock.sleep(2000);
        runOnMainSync(() -> getTargetContext().startService(new Intent(getTargetContext(), au.fintex.mnet.HostSessionService.class).setAction("stop")));
        await(() -> new File(getTargetContext().getFilesDir(), "fixture-screen-closed").exists(), "screen-stop-closed-peer");
        new File(getTargetContext().getFilesDir(), "fixture-record-stop").createNewFile();
        await(() -> new File(getTargetContext().getFilesDir(), "fixture-record-stopped").exists(), "consent-record-stopped");
        checkpoint("hosting-activity-finish");
        runOnMainSync(() -> activity.finish());
    }
    @Override public void onStart() {
        Bundle result = new Bundle();
        try {
            android.accessibilityservice.AccessibilityServiceInfo info = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getServiceInfo();
            info.flags |= android.accessibilityservice.AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS;
            getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).setServiceInfo(info);
            stage = "target-sdk-check";
            if (getTargetContext().getApplicationInfo().targetSdkVersion != 36) throw new IllegalStateException("target not current");
            stage = "actual-apk-launch";
            Intent launch = new Intent(getTargetContext(), MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            activity = startActivitySync(launch);
            click(R.id.privacy_support_button);
            await(() -> {
                AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                if (root == null || root.findAccessibilityNodeInfosByText("Privacy and support").isEmpty()) return false;
                root = captureSettledDialog("Privacy and support", "fixture-privacy.png", "Privacy policy", "Open email app", "Close");
                return clickText(root, "Close");
            }, "privacy-support-close");
            // startActivitySync may return before the first display traversal.
            // Wait for a laid-out field, then let SurfaceFlinger present it.
            await(() -> {
                java.util.concurrent.atomic.AtomicBoolean drawn = new java.util.concurrent.atomic.AtomicBoolean();
                runOnMainSync(() -> {
                    android.view.View field = activity.findViewById(R.id.server_url);
                    drawn.set(field != null && field.isShown() && field.getWidth() > 0 && field.getHeight() > 0);
                });
                return drawn.get();
            }, "first-layout");
            waitForIdleSync();
            await(() -> {
                AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                return root != null && "au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                    && !root.findAccessibilityNodeInfosByViewId("au.fintex.mnet:id/connect_button").isEmpty();
            }, "first-visible-window");
            SystemClock.sleep(1500);
            // Capture before fixture keys enter the UI. Never export app state,
            // enrollment keys, full status or logcat as public CI artifacts.
            Bitmap launchImage = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).takeScreenshot();
            try (FileOutputStream file = new FileOutputStream(new File(getTargetContext().getFilesDir(), "fixture-launch.png"))) {
                if (launchImage == null || !launchImage.compress(Bitmap.CompressFormat.PNG, 100, file)) throw new IOException("screenshot unavailable");
            }
            // Exercise decline before any enrollment values enter the UI.
            click(R.id.connect_button);
            vpnDisclosure(false, true);
            await(() -> {
                AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                return root != null && "au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                        && root.findAccessibilityNodeInfosByText("VPN data disclosure").isEmpty()
                        && MnetVpnService.currentNode() == null
                        && !getTargetContext().getSharedPreferences("mnet", 0).getBoolean("vpn_disclosure_v1", false);
            }, "vpn-disclosure-decline-stays-off");
            set(R.id.server_url, options.getString("server"));
            set(R.id.network_name, "android-emulator");
            set(R.id.auth_key, options.getString("key"));
            set(R.id.device_name, "android-emulator");
            set(R.id.exit_node, "");
            click(R.id.connect_button);
            vpnDisclosure(true, false);
            vpnConsent();
            await(this::up, "initial-vpn-up");
            osVpnReady();
            checkpoint("os-udp-tcp-dns");
            DNSProof.verify(options.getString("peer"));
            hashRoundTrip();
            click(R.id.connect_button);
            await(() -> MnetVpnService.currentNode() == null, "manual-stop");
            // A new actual worker/Node must recover saved identity after stop.
            await(() -> { for (Thread thread : Thread.getAllStackTraces().keySet()) if ("mnet-vpn".equals(thread.getName()) && thread.isAlive()) return false; return true; }, "worker-exit-before-reconnect");
            waitForIdleSync();
            click(R.id.connect_button);
            vpnConsent();
            await(this::up, "reconnect-vpn-up");
            osVpnReady();
            checkpoint("os-udp-tcp-dns");
            DNSProof.verify(options.getString("peer"));
            hashRoundTrip();
            // Keep the VPN-owning instrumentation process alive while another
            // installed app UID performs ordinary unbound TCP transfers.
            new File(getTargetContext().getFilesDir(), "fixture-peer-ready").createNewFile();
            stage = "independent-app-wait";
            long deadline = SystemClock.elapsedRealtime() + 180000;
            while (!new File(getTargetContext().getFilesDir(), "fixture-peer-done").exists()) {
                if (SystemClock.elapsedRealtime() > deadline) throw new IllegalStateException("independent app timeout");
                SystemClock.sleep(150);
            }
            Activity mainActivity = activity;
            screenShareProof();
            activity = mainActivity;
            checkpoint("main-activity-idle-after-hosting");
            waitForIdleSync();
            checkpoint("final-stop-click");
            click(R.id.connect_button);
            await(() -> MnetVpnService.currentNode() == null, "final-stop");
            if (!consentClicked) throw new IllegalStateException("no actual initial VPN consent click observed");
            checkpoint("complete");
            result.putString("summary", "PASS self-bound target36 runtimeAPI=" + android.os.Build.VERSION.SDK_INT + " actual APK/JNI/VPN consent, accessibility/projection/viewer consent, real mesh gesture click and decoded frames/stop, OS/UDP/TCP DNS, bidirectional 4MiB hash, manual stop/reconnect and repeat hash; emulator only, no physical reboot/OEM proof");
            finish(Activity.RESULT_OK, result);
        } catch (Throwable failure) {
            // Only these controlled hosting/OS consent screens contain no
            // enrollment UI or secrets. Never capture a general failure screen.
            if (stage.equals("screen-projection-consent") || stage.equals("screen-viewer-consent") || stage.startsWith("accessibility-")) {
                try {
                    Bitmap image = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).takeScreenshot();
                    try (FileOutputStream file = new FileOutputStream(new File(getTargetContext().getFilesDir(), "fixture-consent.png"))) {
                        if (image != null) image.compress(Bitmap.CompressFormat.PNG, 100, file);
                    }
                } catch (Exception ignored) { }
            }
            // Deliberately omit exception messages, app state and arguments.
            String diagnostics = DNSProof.failureDiagnostics(failure);
            try{
                android.net.ConnectivityManager cm=getTargetContext().getSystemService(android.net.ConnectivityManager.class);
                android.net.LinkProperties lp=cm.getLinkProperties(cm.getActiveNetwork());
                diagnostics+=" resolverConfigured="+(lp!=null&&!lp.getDnsServers().isEmpty());
                if(android.os.Build.VERSION.SDK_INT>=28)diagnostics+=" privateDNSActive="+(lp!=null&&lp.isPrivateDnsActive())+" strictPrivateDNS="+(lp!=null&&lp.getPrivateDnsServerName()!=null);
            }catch(Exception ignored){diagnostics+=" resolverDiagnosticsUnavailable=true";}

            try {
                java.lang.reflect.Field f = MnetVpnService.class.getDeclaredField("dnsFailureCategory");f.setAccessible(true);
                String category=String.valueOf(f.get(null));
                diagnostics += " dnsPermission=" + "permission".equals(category) + " dnsAddress=" + "address".equals(category) + " dnsOccupied=" + "occupied".equals(category) + " dnsOther=" + "other".equals(category);
            } catch(Exception ignored) {diagnostics += " dnsDiagnosticsUnavailable=true";}

            if (stage.startsWith("vpn-")) {
                try {
                    AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                    String pkg = root == null ? "" : String.valueOf(root.getPackageName());
                    diagnostics += " disclosureAccepted=" + getTargetContext().getSharedPreferences("mnet", 0).getBoolean("vpn_disclosure_v1", false)
                        + " rootMnet=" + "au.fintex.mnet".equals(pkg) + " rootVpnDialog=" + "com.android.vpndialogs".equals(pkg)
                        + " rootSystemUI=" + "com.android.systemui".equals(pkg)
                        + " disclosureVisible=" + (root != null && !root.findAccessibilityNodeInfosByText("VPN data disclosure").isEmpty());
                    boolean ok = false, enabled = false, clickable = false;
                    if (root != null) for (AccessibilityNodeInfo button : root.findAccessibilityNodeInfosByViewId("android:id/button1")) {
                        ok |= "OK".equals(String.valueOf(button.getText()));
                        enabled |= button.isEnabled(); clickable |= button.isClickable();
                    }
                    diagnostics += " positiveOK=" + ok + " positiveEnabled=" + enabled + " positiveClickable=" + clickable;
                } catch (Exception ignored) { diagnostics += " vpnDiagnosticsUnavailable=true"; }
            }
            try {
                if (MnetVpnService.currentNode() != null) {
                    JSONObject state = new JSONObject(MnetVpnService.currentNode().status());
                    JSONObject dnsPackets=state.optJSONObject("dns_packets");
                    if(dnsPackets!=null)for(String key:new String[]{"udp_seen","udp_received","udp_sent","udp_checksum_error","udp_malformed","udp_unknown_port","udp_buffer_error","udp_send_error","ip_invalid_source","ip_malformed"})diagnostics+=" dns_"+key+"="+dnsPackets.optBoolean(key);
                    org.json.JSONArray peers = state.optJSONArray("peers");
                    boolean handshake = false, received = false, transmitted = false, relay = false;
                    if (peers != null) for (int i = 0; i < peers.length(); i++) {
                        JSONObject peer = peers.getJSONObject(i);
                        handshake |= peer.optBoolean("online");
                        received |= peer.optLong("rx_bytes") > 0;
                        transmitted |= peer.optLong("tx_bytes") > 0;
                        relay |= peer.optBoolean("relay");
                    }
                    JSONObject packets = new JSONObject(MnetVpnService.currentNode().packetDiagnostics());
                    diagnostics += " tunSourceSelf=" + packets.optBoolean("read_source_self") + " tunDestinationPeer=" + packets.optBoolean("read_destination_peer") + " tunIPv4=" + packets.optBoolean("read_ipv4") + " tunTCP=" + packets.optBoolean("read_tcp") + " tunSYN=" + packets.optBoolean("read_syn") + " tunRead=" + packets.optBoolean("read_packet") + " tunWrite=" + packets.optBoolean("wrote_packet") + " tunReadError=" + packets.optBoolean("read_failed") + " tunWriteError=" + packets.optBoolean("write_failed") + " peersPresent=" + (peers != null && peers.length() > 0)
                        + " handshake=" + handshake + " received=" + received + " transmitted=" + transmitted + " relay=" + relay;
                }
            } catch (Exception ignored) { diagnostics += " diagnosticsUnavailable=true"; }
            try {
                Class<?> type = au.fintex.mnet.HostSessionService.class;
                java.lang.reflect.Field current = type.getDeclaredField("diagnosticInstance"); current.setAccessible(true);
                java.lang.ref.WeakReference<?> reference = (java.lang.ref.WeakReference<?>) current.get(null);
                Object host = reference == null ? null : reference.get();
                if (host != null) {
                    for (String name : new String[]{"running", "captureImageSeen", "captureEncodingFailed"}) {
                        java.lang.reflect.Field field = type.getDeclaredField(name); field.setAccessible(true);
                        diagnostics += " " + name + "=" + field.getBoolean(host);
                    }
                    for (String name : new String[]{"latestSeq", "servedSeq"}) {
                        java.lang.reflect.Field field = type.getDeclaredField(name); field.setAccessible(true);
                        diagnostics += " " + name + "Present=" + (field.getLong(host) > 0);
                    }
                }
            } catch (Exception ignored) { diagnostics += " captureDiagnosticsUnavailable=true"; }
            result.putString("summary", "FAIL stage=" + stage + " type=" + failure.getClass().getSimpleName() + diagnostics);
            finish(Activity.RESULT_CANCELED, result);
        }
    }
}
