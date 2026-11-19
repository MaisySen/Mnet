package au.fintex.mnet;

import android.app.Activity;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.VpnService;
import android.os.Bundle;
import android.view.Gravity;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

// MainActivity collects the join details, asks for VPN consent, and
// shows live mesh status pushed by MnetVpnService.
public class MainActivity extends Activity {

    private EditText server, network, authKey, name, exitNode;
    private Button toggle;
    private TextView status, connectionTitle, connectionHint;
    private LinearLayout setupFields;
    private Button setupToggle;
    private android.widget.ScrollView page;
    private boolean connected;
    private boolean alwaysOn;

    private final BroadcastReceiver statusReceiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context ctx, Intent intent) {
            String json = intent.getStringExtra(MnetVpnService.EXTRA_STATUS);
            if (json != null) {
                alwaysOn = intent.getBooleanExtra(MnetVpnService.EXTRA_ALWAYS_ON, false);
                render(json);
            }
        }
    };

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);

        LinearLayout root = AppUi.screen(this);
        page = (ScrollView) root.getParent();
        AppUi.text(root, "Mnet", 30, AppUi.INK, true);
        AppUi.text(root, "Your devices. Within reach.", 16, AppUi.MUTED, false);
        AppUi.gap(root, 24);

        LinearLayout connection = AppUi.card(root);
        AppUi.text(connection, "THIS DEVICE", 12, AppUi.ACCENT, true);
        AppUi.gap(connection, 8);
        connectionTitle = AppUi.text(connection, "Not connected", 25, AppUi.INK, true);
        connectionHint = AppUi.text(connection, "Set up a private-network connection to get started.", 15, AppUi.MUTED, false);
        toggle = AppUi.button(connection, "Connect", true);
        toggle.setId(R.id.connect_button);
        toggle.setOnClickListener(v -> onToggle());
        AppUi.gap(root, 24);

        AppUi.text(root, "Screen sessions", 21, AppUi.INK, true);
        AppUi.text(root, "Connect to the same network before starting a session.", 14, AppUi.MUTED, false);
        Button viewBtn = AppUi.button(root, "View another device", false);
        viewBtn.setId(R.id.view_peer_button);
        viewBtn.setOnClickListener(v -> showPeerDialog());
        Button hostBtn = AppUi.button(root, "Share this screen", false);
        hostBtn.setId(R.id.host_session_button);
        hostBtn.setOnClickListener(v -> startActivity(new Intent(this, HostingActivity.class)));
        AppUi.gap(root, 24);

        LinearLayout setup = AppUi.card(root);
        AppUi.text(setup, "Connection settings", 20, AppUi.INK, true);
        AppUi.text(setup, "Manual setup for this development build. Multies sign-in is not available yet.", 14, AppUi.MUTED, false);
        setupToggle = AppUi.button(setup, "Edit connection settings", false);
        setupFields = AppUi.column(this);
        setup.addView(setupFields);
        setupFields.setVisibility(android.view.View.GONE);
        setupToggle.setOnClickListener(v -> setSetupOpen(setupFields.getVisibility() != android.view.View.VISIBLE));

        server = field(setupFields, "Control server URL", R.string.server_hint);
        server.setId(R.id.server_url);
        server.setInputType(android.text.InputType.TYPE_CLASS_TEXT | android.text.InputType.TYPE_TEXT_VARIATION_URI);
        network = field(setupFields, "Network name", R.string.network_hint);
        network.setId(R.id.network_name);
        authKey = field(setupFields, "Auth key (first join)", R.string.authkey_hint);
        authKey.setId(R.id.auth_key);
        authKey.setInputType(android.text.InputType.TYPE_CLASS_TEXT | android.text.InputType.TYPE_TEXT_VARIATION_PASSWORD);
        authKey.setTypeface(Typeface.create("sans-serif", Typeface.NORMAL));
        name = field(setupFields, "This device name", R.string.name_hint);
        name.setId(R.id.device_name);
        exitNode = field(setupFields, "Exit node (optional)", R.string.exitnode_hint);
        exitNode.setId(R.id.exit_node);
        AppUi.text(setupFields, "Use details from your network administrator. An empty exit node keeps the existing default routing.", 13, AppUi.MUTED, false);

        SharedPreferences prefs = getSharedPreferences("mnet", MODE_PRIVATE);
        server.setText(prefs.getString("server", getString(R.string.server_default)));
        network.setText(prefs.getString("network", ""));
        authKey.setText(prefs.getString("authkey", ""));
        name.setText(prefs.getString("name", android.os.Build.MODEL));
        exitNode.setText(prefs.getString("exitnode", ""));
        if (network.getText().toString().trim().isEmpty() || server.getText().toString().trim().isEmpty()) {
            toggle.setText("Set up connection");
        }
        AppUi.button(setupFields, "Review & connect", true).setOnClickListener(v -> onToggle());

        AppUi.gap(root, 16);
        LinearLayout details = AppUi.card(root);
        AppUi.text(details, "Connection details", 18, AppUi.INK, true);
        status = AppUi.text(details, "No active connection. Your connection details will appear here.", 14, AppUi.MUTED, false);
        status.setTextIsSelectable(true);
        Button privacy = AppUi.button(root, "Privacy and support", false);
        privacy.setId(R.id.privacy_support_button);
        privacy.setOnClickListener(v -> showPrivacySupport());
    }

    private void setSetupOpen(boolean open) {
        setupFields.setVisibility(open ? android.view.View.VISIBLE : android.view.View.GONE);
        setupToggle.setText(open ? "Hide connection settings" : "Edit connection settings");
    }

    private void showPeerDialog() {
        LinearLayout form = AppUi.column(this);
        int pad = dp(24); form.setPadding(pad, dp(8), pad, 0);
        AppUi.text(form, "Enter the other device's mesh IP address. It must be on your network and ready to share.", 15, AppUi.MUTED, false);
        EditText in = new EditText(this);
        in.setHint("Mesh IP address");
        in.setSingleLine(true);
        in.setInputType(android.text.InputType.TYPE_CLASS_TEXT | android.text.InputType.TYPE_TEXT_VARIATION_URI);
        in.setMinimumHeight(dp(52));
        form.addView(in);
        android.app.AlertDialog dialog = new android.app.AlertDialog.Builder(this)
                .setTitle("View another device").setView(form)
                .setPositiveButton("Connect", null).setNegativeButton("Cancel", null).create();
        dialog.setOnShowListener(ignored -> dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).setOnClickListener(v -> {
            String ip = in.getText().toString().trim();
            if (ip.isEmpty()) { in.setError("Enter a mesh IP address"); return; }
            startActivity(new Intent(this, ViewerActivity.class).putExtra("peer_ip", ip));
            dialog.dismiss();
        }));
        dialog.show();
    }

    // Development notice, not a replacement for the final hosted policy.
    private void showPrivacySupport() {
        new android.app.AlertDialog.Builder(this)
                .setTitle("Privacy and support")
                .setMessage("Mnet is operated by Fintex Australia Pty Ltd. Support/deletion: contact@fintex.au (monitored). Never email passwords or keys.\n\n" +
                        "Mnet saves enrollment and identity on this phone. The control service processes device/connection data. " +
                        "Google and Cloudflare STUN see your source IP address and port; upstream DNS sees queried names.\n\n" +
                        "Sharing and remote gestures require consent. Peers may keep content. Disconnecting does not delete data.\n\n" +
                        "Read the policy for retention and manual-request limits. Opening email does not send or delete data.")
                .setNeutralButton("Privacy policy", (d, w) -> {
                    Intent policy = new Intent(Intent.ACTION_VIEW, android.net.Uri.parse("https://maisysen.github.io/Mnet/"));
                    try { startActivity(policy); }
                    catch (android.content.ActivityNotFoundException unavailable) {
                        android.widget.Toast.makeText(this, "Open https://maisysen.github.io/Mnet/ in a browser.", android.widget.Toast.LENGTH_LONG).show();
                    }
                })
                .setPositiveButton("Open email app", (d, w) -> {
                    Intent email = new Intent(Intent.ACTION_SENDTO, android.net.Uri.parse("mailto:contact@fintex.au"));
                    try { startActivity(email); }
                    catch (android.content.ActivityNotFoundException unavailable) {
                        android.widget.Toast.makeText(this, "No email app found. Contact contact@fintex.au.", android.widget.Toast.LENGTH_LONG).show();
                    }
                })
                .setNegativeButton("Close", null)
                .show();
    }

    private EditText field(LinearLayout root, String label, int hint) {
        AppUi.gap(root, 16);
        TextView title = AppUi.text(root, label, 14, AppUi.INK, true);
        EditText e = new EditText(this);
        e.setHint(hint); e.setTextSize(16); e.setTextColor(AppUi.INK);
        e.setHintTextColor(AppUi.MUTED);
        e.setSingleLine(true); e.setMinimumHeight(dp(52));
        e.setBackgroundTintList(android.content.res.ColorStateList.valueOf(AppUi.ACCENT));
        root.addView(e, new LinearLayout.LayoutParams(-1, -2));
        // Bind the persistent label after callers assign the resource ID.
        e.post(() -> title.setLabelFor(e.getId()));
        return e;
    }

    private void onToggle() {
        if (connected) {
            if (alwaysOn) return;
            startService(new Intent(this, MnetVpnService.class).setAction("disconnect"));
            connected = false;
            toggle.setText("Connect");
            connectionTitle.setText("Disconnecting");
            connectionHint.setText("Waiting for the network service to stop.");
            return;
        }
        if (server.getText().toString().trim().isEmpty() || network.getText().toString().trim().isEmpty()) {
            setSetupOpen(true);
            EditText missing = server.getText().toString().trim().isEmpty() ? server : network;
            missing.setError(missing == server ? "Enter your control server URL" : "Enter your network name");
            // Reveal the real setup form, without saving or starting a service.
            page.post(() -> {
                android.graphics.Rect bounds = new android.graphics.Rect();
                missing.getDrawingRect(bounds);
                page.offsetDescendantRectToMyCoords(missing, bounds);
                page.smoothScrollTo(0, Math.max(0, bounds.top - dp(72)));
            });
            return;
        }
        if (!getSharedPreferences("mnet", MODE_PRIVATE).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY, false)) {
            new android.app.AlertDialog.Builder(this)
                    .setTitle("VPN data disclosure")
                    .setMessage("Mnet uses Android's VPN service for encrypted private-mesh traffic and remote access. " +
                            "It processes mesh traffic and DNS queries. " +
                            "The control service and approved peers receive public device identity and connection metadata to establish and secure connections. " +
                            "Google and Cloudflare STUN receive source IP/port metadata for NAT discovery. " +
                            "DNS names outside .mnet, including queries from other apps, go to the configured resolver or Cloudflare 1.1.1.1, which sees those names and connection metadata. " +
                            "Mnet does not tunnel all internet traffic or use traffic for advertising. " +
                            "Agree continues to Android's separate VPN permission. Not now leaves the VPN off without saving join details.")
                    .setPositiveButton("Agree", (d, w) -> {
                        getSharedPreferences("mnet", MODE_PRIVATE).edit().putBoolean(MnetVpnService.VPN_DISCLOSURE_KEY, true).apply();
                        connectAfterDisclosure();
                    })
                    .setNegativeButton("Not now", null)
                    .show();
            return;
        }
        connectAfterDisclosure();
    }

    private void connectAfterDisclosure() {
        SharedPreferences.Editor ed = getSharedPreferences("mnet", MODE_PRIVATE).edit();
        ed.putString("server", server.getText().toString().trim());
        ed.putString("network", network.getText().toString().trim());
        ed.putString("authkey", authKey.getText().toString().trim());
        ed.putString("name", name.getText().toString().trim());
        ed.putString("exitnode", exitNode.getText().toString().trim());
        ed.apply();

        Intent consent = VpnService.prepare(this);
        if (consent != null) {
            startActivityForResult(consent, 1);
        } else {
            onActivityResult(1, RESULT_OK, null);
        }
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req == 1 && res == RESULT_OK && getSharedPreferences("mnet", MODE_PRIVATE).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY, false)) {
            connected = true;
            toggle.setText("Disconnect");
            connectionTitle.setText("Connecting");
            connectionHint.setText("Joining your private network...");
            status.setText("Joining...");
            startService(new Intent(this, MnetVpnService.class));
        }
    }

    private void render(String json) {
        try {
            JSONObject st = new JSONObject(json);
            String state = st.optString("status");
            StringBuilder sb = new StringBuilder();
            sb.append("State: ").append(state).append('\n');
            if (alwaysOn) sb.append("Always-on VPN: Android Settings controls disconnect.\n");
            if (!st.optString("ipv4").isEmpty()) {
                sb.append("Mesh IP: ").append(st.optString("ipv4")).append('\n');
            }
            if (!st.optString("approval_url").isEmpty()) {
                sb.append("Approve this device:\n").append(st.optString("approval_url")).append('\n');
            }
            if (!st.optString("last_error").isEmpty()) {
                sb.append("Error: ").append(st.optString("last_error")).append('\n');
            }
            JSONArray peers = st.optJSONArray("peers");
            if (peers != null && peers.length() > 0) {
                sb.append("\nPeers:\n");
                for (int i = 0; i < peers.length(); i++) {
                    JSONObject p = peers.getJSONObject(i);
                    String path = p.optBoolean("relay") ? "relay" : "direct";
                    sb.append("  ").append(p.optString("name"))
                            .append("  ").append(p.optString("ipv4"))
                            .append("  ").append(path)
                            .append(p.optBoolean("online") ? "" : "  (offline)")
                            .append('\n');
                }
            }
            status.setText(sb.toString());
            connected = "up".equals(state) || "joining".equals(state) || "waiting-approval".equals(state);
            toggle.setText(connected ? "Disconnect" : "Connect");
            connectionTitle.setText("up".equals(state) ? "Connected" : "joining".equals(state) ? "Connecting" : "waiting-approval".equals(state) ? "Approval needed" : "Not connected");
            connectionHint.setText(alwaysOn && connected ? "Android Settings controls this always-on connection." :
                    "up".equals(state) ? "This device is on your private network." :
                    "joining".equals(state) ? "Joining your private network..." :
                    "waiting-approval".equals(state) ? "See connection details below to approve this device." :
                    st.optString("last_error").isEmpty() ? "Connect when you are ready." : "Connection needs attention. See details below.");
            toggle.setEnabled(!connected || !alwaysOn);
        } catch (Exception e) {
            status.setText(json);
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        IntentFilter f = new IntentFilter(MnetVpnService.ACTION_STATUS);
        VpnPlatform.registerPrivateReceiver(this, statusReceiver, f);
    }

    @Override
    protected void onPause() {
        unregisterReceiver(statusReceiver);
        super.onPause();
    }

    private int dp(int v) {
        return (int) (v * getResources().getDisplayMetrics().density + 0.5f);
    }
}
