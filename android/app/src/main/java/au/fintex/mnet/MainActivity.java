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
    private TextView status;
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

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        int pad = dp(20);
        root.setPadding(pad, pad, pad, pad);

        TextView title = new TextView(this);
        title.setText(R.string.app_name);
        title.setTextSize(28);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        title.setTextColor(Color.rgb(0x0B, 0x3D, 0x91));
        root.addView(title);

        server = field(root, R.string.server_hint);
        server.setId(R.id.server_url);
        network = field(root, R.string.network_hint);
        network.setId(R.id.network_name);
        authKey = field(root, R.string.authkey_hint);
        authKey.setId(R.id.auth_key);
        name = field(root, R.string.name_hint);
        name.setId(R.id.device_name);
        exitNode = field(root, R.string.exitnode_hint);
        exitNode.setId(R.id.exit_node);

        SharedPreferences prefs = getSharedPreferences("mnet", MODE_PRIVATE);
        server.setText(prefs.getString("server", getString(R.string.server_default)));
        network.setText(prefs.getString("network", ""));
        authKey.setText(prefs.getString("authkey", ""));
        name.setText(prefs.getString("name", android.os.Build.MODEL));
        exitNode.setText(prefs.getString("exitnode", ""));

        toggle = new Button(this);
        toggle.setId(R.id.connect_button);
        toggle.setText(R.string.connect);
        toggle.setOnClickListener(v -> onToggle());
        root.addView(toggle);

        // Session layer: host this phone, or view a peer.
        LinearLayout sessionRow = new LinearLayout(this);
        sessionRow.setOrientation(LinearLayout.HORIZONTAL);
        Button hostBtn = new Button(this);
        hostBtn.setId(R.id.host_session_button);
        hostBtn.setText("Host a session");
        hostBtn.setOnClickListener(v ->
                startActivity(new Intent(this, HostingActivity.class)));
        sessionRow.addView(hostBtn, new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));
        Button viewBtn = new Button(this);
        viewBtn.setId(R.id.view_peer_button);
        viewBtn.setText("View a peer");
        viewBtn.setOnClickListener(v -> {
            android.widget.EditText in = new EditText(this);
            in.setHint("peer mesh IP");
            new android.app.AlertDialog.Builder(this)
                    .setTitle("View a peer")
                    .setView(in)
                    .setPositiveButton("Connect", (d, w) -> {
                        String ip = in.getText().toString().trim();
                        if (!ip.isEmpty()) {
                            startActivity(new Intent(this, ViewerActivity.class)
                                    .putExtra("peer_ip", ip));
                        }
                    })
                    .setNegativeButton("Cancel", null)
                    .show();
        });
        sessionRow.addView(viewBtn, new LinearLayout.LayoutParams(
                0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));
        root.addView(sessionRow);

        Button privacy = new Button(this);
        privacy.setId(R.id.privacy_support_button);
        privacy.setText("Privacy and support");
        privacy.setOnClickListener(v -> showPrivacySupport());
        root.addView(privacy);

        ScrollView scroll = new ScrollView(this);
        status = new TextView(this);
        status.setTypeface(Typeface.MONOSPACE);
        status.setTextSize(12);
        status.setPadding(0, pad, 0, 0);
        status.setText("Disconnected. Enter your control server and network, then Connect.");
        scroll.addView(status);
        LinearLayout.LayoutParams sp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f);
        root.addView(scroll, sp);

        ContentInsets.install(root);
        setContentView(root);
    }

    // Development notice, not a replacement for the final hosted policy.
    private void showPrivacySupport() {
        new android.app.AlertDialog.Builder(this)
                .setTitle("Privacy and support")
                .setMessage("Mnet is operated by Fintex Australia Pty Ltd. " +
                        "For support or data-deletion requests, contact contact@fintex.au. " +
                        "The inbox is monitored. Never send passwords, enrollment keys or private node keys.\n\n" +
                        "Mnet stores enrollment settings and device identity on this phone. " +
                        "The configured control service processes device and connection metadata. " +
                        "Screen sharing sends screen images to an approved viewer; optional accessibility access permits remote gestures. " +
                        "Disconnecting does not delete saved identity or server records. " +
                        "Other endpoints may retain content you send.\n\n" +
                        "This is a development build. The full privacy notice and complete retention/deletion handling are still being finalized. " +
                        "Opening an email app does not send a request or delete data.")
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

    private EditText field(LinearLayout root, int hint) {
        EditText e = new EditText(this);
        e.setHint(hint);
        e.setSingleLine(true);
        root.addView(e);
        return e;
    }

    private void onToggle() {
        if (connected) {
            if (alwaysOn) return;
            startService(new Intent(this, MnetVpnService.class).setAction("disconnect"));
            connected = false;
            toggle.setText(R.string.connect);
            return;
        }
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
        if (req == 1 && res == RESULT_OK) {
            connected = true;
            toggle.setText(R.string.disconnect);
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
            toggle.setText(connected ? R.string.disconnect : R.string.connect);
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
