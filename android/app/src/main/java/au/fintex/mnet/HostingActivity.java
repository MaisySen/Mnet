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

    private TextView log;
    private Switch autoAccept;
    private boolean capturePending;
    private boolean pendingAutoAccept;

    private final BroadcastReceiver receiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context ctx, Intent intent) {
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
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        int pad = (int) (20 * getResources().getDisplayMetrics().density);
        root.setPadding(pad, pad, pad, pad);

        TextView title = new TextView(this);
        title.setText("Host a session");
        title.setTextSize(22);
        root.addView(title);

        TextView hint = new TextView(this);
        hint.setText("Sharing your screen lets an approved viewer see this phone over the mesh. Remote control needs separate approval for each viewer. " +
                "For control, also enable Mnet in Settings > Accessibility.");
        hint.setPadding(0, pad / 2, 0, pad / 2);
        root.addView(hint);

        Button accessibility = new Button(this);
        accessibility.setText("Open accessibility settings");
        accessibility.setOnClickListener(v -> showAccessibilityDisclosure());
        root.addView(accessibility);

        autoAccept = new Switch(this);
        autoAccept.setText("Auto-accept viewing only on next hosting start");
        root.addView(autoAccept);

        Button start = new Button(this);
        start.setId(R.id.start_hosting_button);
        start.setText("Start hosting");
        start.setOnClickListener(v -> {
            if (capturePending) return;
            capturePending = true;
            pendingAutoAccept = autoAccept.isChecked();
            MediaProjectionManager mpm = getSystemService(MediaProjectionManager.class);
            startActivityForResult(mpm.createScreenCaptureIntent(), 42);
        });
        root.addView(start);

        Button control = new Button(this);
        control.setText("Remote control");
        control.setOnClickListener(v -> showControlConsent());
        root.addView(control);

        Button stop = new Button(this);
        stop.setText("Stop hosting");
        stop.setOnClickListener(v ->
                startService(new Intent(this, HostSessionService.class).setAction("stop")));
        root.addView(stop);

        log = new TextView(this);
        log.setTypeface(android.graphics.Typeface.MONOSPACE);
        log.setTextSize(12);
        log.setMovementMethod(new ScrollingMovementMethod());
        log.setPadding(0, pad, 0, 0);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f);
        root.addView(log, lp);

        ContentInsets.install(root);
        setContentView(root);
    }

    void showControlConsent(){
        SessionControl.Decision decision=HostSessionService.openControlDecision();
        if(decision==null){new AlertDialog.Builder(this).setTitle("No approved viewer").setMessage("Approve a viewing request first. Remote control is off.").setPositiveButton("OK",null).show();return;}
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
        if (req != 42) return;
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
            append("starting screen capture");
        } else {
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
        f.addAction(HostSessionService.ACTION_ASK);
        VpnPlatform.registerPrivateReceiver(this, receiver, f);
    }

    @Override
    protected void onPause() {
        unregisterReceiver(receiver);
        super.onPause();
    }
}
