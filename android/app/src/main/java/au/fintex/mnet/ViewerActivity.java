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
    private TextView controlStatus;
    private Button controlRequest;
    private Button endViewing;
    private mobile.SessionConnect pendingConnect;
    private boolean controlAllowed;
    private boolean controlPending;
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

        endViewing=new Button(this);
        endViewing.setText("End viewing");
        endViewing.setMinHeight((int)(48*getResources().getDisplayMetrics().density));
        endViewing.setOnClickListener(v->{stopViewing();finish();});
        root.addView(endViewing);

        screen = new ImageView(this);
        screen.setScaleType(ImageView.ScaleType.FIT_CENTER);
        LinearLayout.LayoutParams sp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f);
        root.addView(screen, sp);

        controlStatus=new TextView(this);
        controlStatus.setTextSize(14);
        controlStatus.setPadding(16,8,16,8);
        controlStatus.setText("View only. Control is unavailable until a compatible host responds.");
        root.addView(controlStatus);
        controlRequest=new Button(this);
        controlRequest.setText("Request control");
        controlRequest.setEnabled(false);
        controlRequest.setMinHeight((int)(48*getResources().getDisplayMetrics().density));
        controlRequest.setOnClickListener(v->{
            SessionViewer viewer=viewers.current();if(viewer==null)return;
            boolean cancel=controlPending;
            controlAllowed=false;taps.cancel();controlRequest.setEnabled(false);
            new Thread(()->{
                try{if(cancel)viewer.cancelControlRequest();else viewer.requestControl();}
                catch(Exception unavailable){reconnects.post(()->{if(viewers.current()==viewer)applyControl("unavailable");});}
            },"session-control").start();
        });
        root.addView(controlRequest);

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
        ContentInsets.install(root);
        setContentView(root);

        screen.setOnTouchListener((v, ev) -> {
            SessionViewer viewer = viewers.current();
            if (viewer == null || frameW <= 0 || frameH <= 0 || !controlAllowed) { taps.cancel(); return true; }
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
        Node node=MnetVpnService.currentNode();
        if(node==null){failed(attempt,"connect the mesh first");return;}
        final mobile.SessionConnect connecting;
        try{connecting=node.newSessionConnect();pendingConnect=connecting;}
        catch(Exception unavailable){failed(attempt,"connection unavailable");return;}
        new Thread(() -> {
            try {
                SessionViewer connected = connecting.connectWithControl(peerIp, callbacks(attempt),stateJson->{
                    reconnects.post(()->{
                        if(!viewers.active(attempt))return;
                        try{applyControl(new JSONObject(stateJson).optString("state"));}
                        catch(Exception invalid){applyControl("unavailable");}
                    });
                });
                reconnects.post(() -> {
                    if(pendingConnect==connecting)pendingConnect=null;
                    if (!viewers.publish(attempt, connected)) {
                        closeViewer(connected);
                        return;
                    }
                    retryMs = 1000;
                    try{applyControl(new JSONObject(connected.controlStatus()).optString("state"));}
                    catch(Exception unavailable){applyControl("unavailable");}
                    appendChat("connected to " + peerIp);
                });
            } catch (Exception e) {
                reconnects.post(() -> failed(attempt, "connection failed"));
            }
        }, "session-connect").start();
    }

    private void failed(ViewerGate.Ticket attempt, String message) {
        if (!viewers.active(attempt)) return;
        if(pendingConnect!=null){pendingConnect.cancel();pendingConnect=null;}
        taps.cancel();
        applyControl("unavailable");
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

    private void applyControl(String state){
        controlAllowed="allowed".equals(state);
        controlPending="pending".equals(state);
        taps.cancel();
        controlRequest.setText(controlPending?"Cancel request":controlAllowed?"Control allowed":"paused".equals(state)?"Control paused":"unavailable".equals(state)?"Control unavailable":"Request control");
        controlRequest.setEnabled("available".equals(state)||controlPending||"declined".equals(state)||"revoked".equals(state));
        String message;
        switch(state){
            case "available":message="View only. Ask the host before controlling this device.";break;
            case "pending":message="Control requested. Waiting for the host's separate approval.";break;
            case "allowed":message="Control allowed for this session. The host can revoke it at any time.";break;
            case "paused":message="Control paused while the host reviews permission. Viewing continues.";break;
            case "declined":message="Control declined or canceled. Viewing continues.";break;
            case "revoked":message="Control revoked by the host. Viewing continues.";break;
            default:message="View only. Control is unavailable with this connection.";
        }
        controlStatus.setText(message);
    }

    private static void closeViewer(SessionViewer viewer) {
        if (viewer != null) {
            try { viewer.close(); } catch (Exception ignored) { }
        }
    }

    private void appendChat(String s) {
        runOnUiThread(() -> { if (!userClosed) chat.append(s + "\n"); });
    }

    // User stop and Activity destruction share the same terminal ownership path.
    // A queued connect/control/frame callback cannot reopen a closed ViewerGate.
    private void stopViewing(){
        userClosed=true;
        taps.cancel();
        reconnects.removeCallbacks(retryTask);
        retryPending=false;
        if(pendingConnect!=null){pendingConnect.cancel();pendingConnect=null;}
        closeViewer(viewers.close());
        frameW = frameH = 0;
        if(screen!=null)screen.setImageDrawable(null);
        if(controlRequest!=null)applyControl("unavailable");
        if(endViewing!=null)endViewing.setEnabled(false);
    }

    @Override
    protected void onDestroy() {
        stopViewing();
        super.onDestroy();
    }
}
