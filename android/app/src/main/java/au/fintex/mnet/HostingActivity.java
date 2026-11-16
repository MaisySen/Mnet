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

    private final BroadcastReceiver receiver = new BroadcastReceiver() {
        @Override
        public void onReceive(Context ctx, Intent intent) {
            if (HostSessionService.ACTION_LOG.equals(intent.getAction())) {
                append(intent.getStringExtra(HostSessionService.EXTRA_LOG));
            } else if (HostSessionService.ACTION_ASK.equals(intent.getAction())) {
                String peer = intent.getStringExtra(HostSessionService.EXTRA_PEER);
                new AlertDialog.Builder(HostingActivity.this)
                        .setTitle("Session request")
                        .setMessage(peer + " wants to view and control this phone. Allow?")
                        .setPositiveButton("Allow", (d, w) -> HostSessionService.answerSession(true))
                        .setNegativeButton("Refuse", (d, w) -> HostSessionService.answerSession(false))
                        .setOnCancelListener(d -> HostSessionService.answerSession(false))
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
        hint.setText("Sharing your screen lets a connected viewer see and control this phone over the mesh. " +
                "For control, also enable Mnet in Settings > Accessibility.");
        hint.setPadding(0, pad / 2, 0, pad / 2);
        root.addView(hint);

        Button accessibility = new Button(this);
        accessibility.setText("Open accessibility settings");
        accessibility.setOnClickListener(v ->
                startActivity(new Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)));
        root.addView(accessibility);

        autoAccept = new Switch(this);
        autoAccept.setText("Accept sessions automatically");
        autoAccept.setOnCheckedChangeListener((b, on) -> HostSessionService.autoAccept.set(on));
        root.addView(autoAccept);

        Button start = new Button(this);
        start.setId(R.id.start_hosting_button);
        start.setText("Start hosting");
        start.setOnClickListener(v -> {
            HostSessionService.autoAccept.set(autoAccept.isChecked());
            MediaProjectionManager mpm = getSystemService(MediaProjectionManager.class);
            startActivityForResult(mpm.createScreenCaptureIntent(), 42);
        });
        root.addView(start);

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

        setContentView(root);
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req == 42 && res == RESULT_OK && data != null) {
            Intent svc = new Intent(this, HostSessionService.class);
            svc.putExtra("resultCode", res);
            svc.putExtra("data", data);
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
