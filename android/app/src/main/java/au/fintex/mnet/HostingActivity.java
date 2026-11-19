package au.fintex.mnet;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.media.projection.MediaProjectionManager;
import android.os.Bundle;
import android.provider.Settings;
import android.text.method.ScrollingMovementMethod;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.Switch;
import android.widget.TextView;

// HostingActivity starts and supervises session hosting: MediaProjection
// consent, the auto-accept switch, the per-session consent dialog, and
// a log of what the session is doing.
public class HostingActivity extends Activity {

    private TextView log, hostStatus, sharingHint;
    private Button start, stop;
    private boolean starting;
    private Switch autoAccept;
    private boolean capturePending;
    private boolean pendingAutoAccept;

    private final BroadcastReceiver receiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context ctx, Intent intent) {
            if (HostSessionService.ACTION_STATE.equals(intent.getAction()) || HostSessionService.ACTION_LOG.equals(intent.getAction())) {
                starting = false;
                refreshHostingState();
            }
            if (HostSessionService.ACTION_LOG.equals(intent.getAction())) {
                append(intent.getStringExtra(HostSessionService.EXTRA_LOG));
            } else if (HostSessionService.ACTION_ASK.equals(intent.getAction())) {
                String peer = intent.getStringExtra(HostSessionService.EXTRA_PEER);
                String requestId = intent.getStringExtra(HostSessionService.EXTRA_REQUEST);
                if (!HostSessionService.consentPending(requestId)) return;
                new AlertDialog.Builder(HostingActivity.this)
                        .setTitle("Session request")
                        .setMessage(peer + " wants to view this phone. Remote control stays off unless you approve it separately.")
                        .setPositiveButton("Allow viewing", (d, w) -> HostSessionService.answerSession(requestId, true))
                        .setNegativeButton("Refuse", (d, w) -> HostSessionService.answerSession(requestId, false))
                        .setOnCancelListener(d -> HostSessionService.answerSession(requestId, false))
                        .show();
            }
        }
    };

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        LinearLayout footer = AppUi.column(this);
        LinearLayout root = AppUi.screen(this, footer);
        Button back = AppUi.button(root, "Back to Mnet", false);
        back.setOnClickListener(v -> finish());
        AppUi.gap(root, 20);
        AppUi.text(root, "Share this screen", 28, AppUi.INK, true);
        AppUi.text(root, "You choose who can view and control this phone.", 16, AppUi.MUTED, false);
        AppUi.gap(root, 24);

        LinearLayout sharing = AppUi.card(root);
        hostStatus = AppUi.text(sharing, "Not sharing", 20, AppUi.INK, true);
        AppUi.gap(sharing, 8);
        sharingHint = AppUi.text(sharing, "Android will ask which screen to share. Viewers still need your approval. Avoid opening sensitive content while sharing.", 14, AppUi.MUTED, false);
        autoAccept = new Switch(this);
        autoAccept.setText("Auto-accept viewing on the next start only");
        autoAccept.setTextSize(15); autoAccept.setTextColor(AppUi.INK);
        autoAccept.setMinimumHeight(AppUi.dp(this, 56));
        autoAccept.setPadding(0, AppUi.dp(this,12), 0, AppUi.dp(this,12));
        sharing.addView(autoAccept, new LinearLayout.LayoutParams(-1,-2));
        AppUi.text(sharing, "Optional. Remote control always needs separate approval.", 13, AppUi.MUTED, false);
        start = AppUi.button(sharing, "Start hosting", true);
        start.setId(R.id.start_hosting_button);
        start.setOnClickListener(v -> {
            if (capturePending) return;
            capturePending = true;
            refreshHostingState();
            pendingAutoAccept = autoAccept.isChecked();
            MediaProjectionManager mpm = getSystemService(MediaProjectionManager.class);
            startActivityForResult(mpm.createScreenCaptureIntent(), 42);
        });
        stop = AppUi.button(footer, "Stop hosting", false);
        stop.setTextColor(new android.content.res.ColorStateList(new int[][]{new int[]{-android.R.attr.state_enabled},new int[]{}},
                new int[]{AppUi.MUTED,android.graphics.Color.rgb(176,0,23)}));
        stop.setOnClickListener(v -> {
            stop.setEnabled(false);
            hostStatus.setText("Stopping screen sharing...");
            startService(new Intent(this, HostSessionService.class).setAction("stop"));
        });
        AppUi.gap(root, 16);

        LinearLayout controlCard = AppUi.card(root);
        AppUi.text(controlCard, "Remote control (optional)", 20, AppUi.INK, true);
        AppUi.text(controlCard, "A viewer requests control after connecting. Review that request here. You can pause or revoke control without stopping viewing.", 14, AppUi.MUTED, false);
        Button control = AppUi.button(controlCard, "Review remote control", false);
        control.setOnClickListener(v -> showControlConsent());
        AppUi.gap(controlCard, 12);
        AppUi.text(controlCard, "To allow taps and scrolling, Android also needs Mnet Accessibility enabled. Opening settings is a separate choice.", 14, AppUi.MUTED, false);
        Button accessibility = AppUi.button(controlCard, "Open accessibility settings", false);
        accessibility.setOnClickListener(v -> showAccessibilityDisclosure());
        AppUi.gap(root, 16);

        LinearLayout activity = AppUi.card(root);
        AppUi.text(activity, "Session activity", 18, AppUi.INK, true);
        log = AppUi.text(activity, "", 14, AppUi.MUTED, false);
        log.setHint("No session activity yet.");
        log.setHintTextColor(AppUi.MUTED);
        log.setMinHeight(AppUi.dp(this, 52));
        log.setTextIsSelectable(true);
        refreshHostingState();
    }

    private void refreshHostingState() {
        boolean active = HostSessionService.isHosting();
        hostStatus.setText(active ? "Screen sharing active" : capturePending ? "Waiting for screen permission" : starting ? "Starting screen sharing..." : "Not sharing");
        sharingHint.setText(active ? "Approved viewers can see your screen. Stop hosting before opening sensitive content." : "Android will ask which screen to share. Viewers still need your approval. Avoid opening sensitive content while sharing.");
        start.setEnabled(!active && !capturePending && !starting);
        stop.setEnabled(active || starting);
        autoAccept.setEnabled(!active && !capturePending && !starting);
    }

    void showControlConsent(){
        SessionControl.Decision decision=HostSessionService.openControlDecision();
        if(decision==null){new AlertDialog.Builder(this).setTitle("No control request").setMessage("The viewer must request control first using a compatible app. Viewing alone never enables input.").setPositiveButton("OK",null).show();return;}
        String session=decision.id;
        boolean allowed=decision.wasAllowed;
        // Pause before showing host controls: remote gestures must not dismiss
        // or operate the dialog that manages their own authority.
        AlertDialog dialog=new AlertDialog.Builder(this)
            .setTitle(allowed?"Revoke remote control?":"Allow remote control?")
            .setMessage(allowed?"Remote control is paused while this dialog is open. Revoke it or choose Keep control to resume. Screen viewing continues.":"Let this viewer tap and scroll on this phone for this session. Android Accessibility must also be enabled. Viewing continues if you decline. Approval ends when the session ends; you can revoke it here at any time.")
            .setPositiveButton(allowed?"Revoke control":"Allow control",(d,w)->HostSessionService.answerControl(session,!allowed))
            .setNegativeButton(allowed?"Keep control":"View only",(d,w)->{
                HostSessionService.answerControl(session,allowed);
            })
            .show();
        dialog.setOnCancelListener(d->HostSessionService.answerControl(session,false));
        dialog.setCanceledOnTouchOutside(false);
    }

    // Separate, just-in-time consent for gesture access. Declining leaves VPN,
    // viewing and screen-sharing flows available; it never opens Settings.
    void showAccessibilityDisclosure() {
        new AlertDialog.Builder(this)
                .setTitle("Remote-control accessibility access")
                .setMessage("Mnet uses Android Accessibility to let a connected viewer you approve " +
                        "tap and scroll on this phone while you are hosting a session. " +
                        "The viewer's pointer commands are sent over the mesh to control the phone. " +
                        "This accessibility service does not read text or window contents, collect " +
                        "accessibility events, or type text. Screen sharing is a separate permission " +
                        "and shows your screen to the viewer. Do not share sensitive screens. " +
                        "You can stop hosting at any time and disable Mnet in Accessibility Settings. " +
                        "If you decline, remote taps and scrolling will not work; VPN and viewing " +
                        "remain available. Agree to open Android Settings? You must still choose " +
                        "whether to enable Mnet there.")
                .setPositiveButton("Agree", (d, w) ->
                        startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)))
                .setNegativeButton("Not now", (d, w) -> { })
                .show();
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        // A result is owned by one explicit pending Start. A duplicate or late
        // result after decline must not reuse a projection token or start anew.
        if (req != 42 || !capturePending) return;
        boolean autoAcceptThisHost = capturePending && pendingAutoAccept;
        capturePending = false;
        pendingAutoAccept = false;
        autoAccept.setChecked(false);
        if (res == RESULT_OK && data != null) {
            Intent svc = new Intent(this, HostSessionService.class);
            svc.putExtra("resultCode", res);
            svc.putExtra("data", data);
            svc.putExtra(HostSessionService.EXTRA_AUTO_ACCEPT, autoAcceptThisHost);
            if (android.os.Build.VERSION.SDK_INT >= 26) startForegroundService(svc);
            else startService(svc);
            starting = true;
            refreshHostingState();
            append("starting screen capture");
        } else {
            starting = false;
            refreshHostingState();
            append("screen capture permission denied");
        }
    }

    private void append(String s) {
        if (s == null) {
            return;
        }
        runOnUiThread(() -> log.append(s + "\n"));
    }

    @Override
    protected void onResume() {
        super.onResume();
        IntentFilter f = new IntentFilter();
        f.addAction(HostSessionService.ACTION_LOG);
        f.addAction(HostSessionService.ACTION_STATE);
        f.addAction(HostSessionService.ACTION_ASK);
        VpnPlatform.registerPrivateReceiver(this, receiver, f);
        starting = false;
        refreshHostingState();
    }

    @Override
    protected void onPause() {
        unregisterReceiver(receiver);
        super.onPause();
    }
}
