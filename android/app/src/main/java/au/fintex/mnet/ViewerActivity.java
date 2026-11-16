package au.fintex.mnet;

import android.app.Activity;
import android.graphics.BitmapFactory;
import android.os.Bundle;
import android.view.MotionEvent;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.EditText;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;

import org.json.JSONObject;

import mobile.Node;
import mobile.SessionViewer;
import mobile.ViewerCallbacks;

// ViewerActivity is the viewing side of a session: it renders the
// peer's screen, turns touches into pointer events, and carries chat.
public class ViewerActivity extends Activity {

    private ImageView screen;
    private TextView chat;
    private EditText chatIn;
    private final ViewerGate<SessionViewer> viewers = new ViewerGate<>();
    private final ViewerTap taps = new ViewerTap();
    private volatile int frameW, frameH;

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        peerIp = getIntent().getStringExtra("peer_ip");
        if (peerIp == null) {
            finish();
            return;
        }

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);

        screen = new ImageView(this);
        screen.setScaleType(ImageView.ScaleType.FIT_CENTER);
        LinearLayout.LayoutParams sp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f);
        root.addView(screen, sp);

        chat = new TextView(this);
        chat.setTypeface(android.graphics.Typeface.MONOSPACE);
        chat.setTextSize(11);
        chat.setMaxLines(4);
        root.addView(chat);

        LinearLayout bar = new LinearLayout(this);
        bar.setOrientation(LinearLayout.HORIZONTAL);
        chatIn = new EditText(this);
        LinearLayout.LayoutParams ep = new LinearLayout.LayoutParams(0,
                ViewGroup.LayoutParams.WRAP_CONTENT, 1f);
        bar.addView(chatIn, ep);
        Button send = new Button(this);
        send.setText("Send");
        send.setOnClickListener(v -> {
            String t = chatIn.getText().toString().trim();
            SessionViewer viewer = viewers.current();
            if (!t.isEmpty() && viewer != null) {
                new Thread(() -> {
                    try {
                        viewer.sendChat(t);
                    } catch (Exception ignored) {
                    }
                }).start();
                chatIn.setText("");
            }
        });
        bar.addView(send);
        root.addView(bar);
        setContentView(root);

        screen.setOnTouchListener((v, ev) -> {
            SessionViewer viewer = viewers.current();
            if (viewer == null || frameW <= 0 || frameH <= 0) { taps.cancel(); return true; }
            float slop = android.view.ViewConfiguration.get(this).getScaledTouchSlop();
            if (ev.getPointerCount() != 1) { taps.cancel(); return true; }
            // Map view coordinates into frame coordinates. FIT_CENTER
            // letterboxes the frame, so scale by the fitted rect, not
            // the raw view.
            float vw = v.getWidth(), vh = v.getHeight();
            float fit = Math.min(vw / frameW, vh / frameH);
            if (!(fit > 0) || Float.isInfinite(fit)) { taps.cancel(); return true; }
            float offX = (vw - frameW * fit) / 2f;
            float offY = (vh - frameH * fit) / 2f;
            float sx = (ev.getX() - offX) / fit;
            float sy = (ev.getY() - offY) / fit;
            if (sx < 0 || sy < 0 || sx >= frameW || sy >= frameH) {
                taps.cancel();
                return true; // touch in the letterbox margin
            }
            switch (ev.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    taps.down(ev.getX(), ev.getY());
                    return true;
                case MotionEvent.ACTION_MOVE:
                    taps.move(ev.getX(), ev.getY(), slop);
                    return true;
                case MotionEvent.ACTION_UP:
                    if (!taps.up(ev.getX(), ev.getY(), slop)) return true;
                    break;
                default:
                    taps.cancel();
                    return true;
            }
            // Only a completed tap is supported; never inject on finger-down.
            String action = "click";
            String json = String.format(java.util.Locale.US,
                    "{\"kind\":\"pointer\",\"x\":%d,\"y\":%d,\"button\":\"left\",\"action\":\"%s\"}",
                    (int) sx, (int) sy, action);
            new Thread(() -> {
                try {
                    viewer.sendInputJSON(json);
                } catch (Exception ignored) {
                }
            }).start();
            return true;
        });

        connectWithRetry();
    }

    // Reconnect state: a dropped session redials with capped backoff
    // until the user leaves; the host's fresh keyframe restores the
    // picture without any viewer state.
    private String peerIp;
    private final android.os.Handler reconnects = new android.os.Handler(android.os.Looper.getMainLooper());
    private volatile boolean userClosed;
    private int retryMs = 1000;

    private boolean retryPending;
    private final Runnable retryTask = () -> {
        retryPending = false;
        connectWithRetry();
    };

    // State transitions and queued callbacks run on the main thread.
    private void connectWithRetry() {
        if (userClosed || isFinishing()) return;
        ViewerGate.Ticket attempt = viewers.begin();
        if (attempt == null) return;
        new Thread(() -> {
            Node node = MnetVpnService.currentNode();
            if (node == null) {
                reconnects.post(() -> failed(attempt, "connect the mesh first"));
                return;
            }
            try {
                SessionViewer connected = node.connectSession(peerIp, callbacks(attempt));
                reconnects.post(() -> {
                    if (!viewers.publish(attempt, connected)) {
                        closeViewer(connected);
                        return;
                    }
                    retryMs = 1000;
                    appendChat("connected to " + peerIp);
                });
            } catch (Exception e) {
                reconnects.post(() -> failed(attempt, "connection failed"));
            }
        }, "session-connect").start();
    }

    private void failed(ViewerGate.Ticket attempt, String message) {
        if (!viewers.active(attempt)) return;
        taps.cancel();
        frameW = frameH = 0;
        screen.setImageDrawable(null);
        closeViewer(viewers.end(attempt));
        appendChat(message);
        scheduleReconnect();
    }

    private void scheduleReconnect() {
        if (userClosed || isFinishing() || retryPending) return;
        retryPending = true;
        appendChat("reconnecting in " + (retryMs / 1000) + "s...");
        reconnects.postDelayed(retryTask, retryMs);
        retryMs = Math.min(retryMs * 2, 15000);
    }

    private ViewerCallbacks callbacks(ViewerGate.Ticket attempt) {
        return new ViewerCallbacks() {
            @Override
            public void onFrame(byte[] jpeg) {
                if (!viewers.active(attempt) || jpeg == null || jpeg.length == 0) return;
                android.graphics.Bitmap bmp = BitmapFactory.decodeByteArray(jpeg, 0, jpeg.length);
                if (bmp == null) return;
                reconnects.post(() -> {
                    if (!viewers.active(attempt)) return;
                    if (frameW != bmp.getWidth() || frameH != bmp.getHeight()) taps.cancel();
                    frameW = bmp.getWidth();
                    frameH = bmp.getHeight();
                    screen.setImageBitmap(bmp);
                });
            }
            @Override
            public void onChat(String json) {
                reconnects.post(() -> {
                    if (!viewers.active(attempt)) return;
                    try {
                        JSONObject m = new JSONObject(json);
                        appendChat(m.optString("from") + ": " + m.optString("text"));
                    } catch (Exception ignored) { }
                });
            }
            @Override
            public void onClosed() {
                reconnects.post(() -> failed(attempt, "connection lost"));
            }
        };
    }

    private static void closeViewer(SessionViewer viewer) {
        if (viewer != null) {
            try { viewer.close(); } catch (Exception ignored) { }
        }
    }

    private void appendChat(String s) {
        runOnUiThread(() -> { if (!userClosed) chat.append(s + "\n"); });
    }

    @Override
    protected void onDestroy() {
        userClosed = true;
        taps.cancel();
        reconnects.removeCallbacks(retryTask);
        closeViewer(viewers.close());
        super.onDestroy();
    }
}
