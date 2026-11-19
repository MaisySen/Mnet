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
    private TextView chat, chatFeedback;
    private boolean chatSending;
    private int chatSendVersion;
    private EditText chatIn;
    private TextView controlStatus, sessionStatus, emptyScreen;
    private Button sendChat;
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

        LinearLayout footer = AppUi.column(this);
        LinearLayout root = AppUi.screen(this, footer);
        AppUi.text(root, "Screen session", 26, AppUi.INK, true);
        TextView peer = AppUi.text(root, peerIp, 14, AppUi.MUTED, false);
        peer.setMaxLines(1);
        peer.setEllipsize(android.text.TextUtils.TruncateAt.MIDDLE);
        AppUi.gap(root, 8);
        sessionStatus = AppUi.text(root, "Connecting...", 16, AppUi.ACCENT, true);
        sessionStatus.setAccessibilityLiveRegion(android.view.View.ACCESSIBILITY_LIVE_REGION_POLITE);
        AppUi.gap(root, 16);

        android.widget.FrameLayout picture = new android.widget.FrameLayout(this);
        picture.setBackground(AppUi.shape(this, android.graphics.Color.rgb(29,29,31), 0));
        picture.setClipToOutline(true);
        int screenHeight = getResources().getConfiguration().screenHeightDp;
        int pictureHeight = AppUi.dp(this, Math.max(200, Math.min(480, Math.round(screenHeight * .52f))));
        root.addView(picture, new LinearLayout.LayoutParams(-1, pictureHeight));
        screen = new ImageView(this);
        screen.setScaleType(ImageView.ScaleType.FIT_CENTER);
        screen.setContentDescription("Remote screen. Input requires the host's separate approval.");
        picture.addView(screen, new android.widget.FrameLayout.LayoutParams(-1,-1));
        emptyScreen = new TextView(this);
        emptyScreen.setText("Waiting for a screen\nThe host may need to approve viewing.");
        emptyScreen.setTextColor(android.graphics.Color.WHITE);
        emptyScreen.setTextSize(16);
        emptyScreen.setGravity(android.view.Gravity.CENTER);
        int inset=AppUi.dp(this,24);emptyScreen.setPadding(inset,inset,inset,inset);
        picture.addView(emptyScreen,new android.widget.FrameLayout.LayoutParams(-1,-1));
        AppUi.gap(root, 16);

        LinearLayout controls = AppUi.card(root);
        AppUi.text(controls, "Remote control", 20, AppUi.INK, true);
        controlStatus = AppUi.text(controls, "View only. Control is unavailable until a compatible host responds.", 14, AppUi.MUTED, false);
        controlStatus.setAccessibilityLiveRegion(android.view.View.ACCESSIBILITY_LIVE_REGION_POLITE);
        controlRequest = AppUi.button(controls, "Request control", false);
        controlRequest.setEnabled(false);
        controlRequest.setOnClickListener(v->{
            SessionViewer viewer=viewers.current();if(viewer==null)return;
            boolean cancel=controlPending;
            controlAllowed=false;taps.cancel();controlRequest.setEnabled(false);
            new Thread(()->{
                try{if(cancel)viewer.cancelControlRequest();else viewer.requestControl();}
                catch(Exception unavailable){reconnects.post(()->{if(viewers.current()==viewer)applyControl("unavailable");});}
            },"session-control").start();
        });
        AppUi.gap(root, 16);

        LinearLayout messages = AppUi.card(root);
        Button chatToggle = AppUi.button(messages, "Open session chat", false);
        LinearLayout chatPanel = AppUi.column(this);
        messages.addView(chatPanel, new LinearLayout.LayoutParams(-1,-2));
        chatPanel.setVisibility(android.view.View.GONE);
        chatToggle.setOnClickListener(v -> {
            boolean open = chatPanel.getVisibility() != android.view.View.VISIBLE;
            chatPanel.setVisibility(open ? android.view.View.VISIBLE : android.view.View.GONE);
            chatToggle.setText(open ? "Hide session chat" : "Open session chat");
        });
        AppUi.gap(chatPanel, 12);
        chat = AppUi.text(chatPanel, "", 14, AppUi.MUTED, false);
        chat.setMaxLines(6);
        chat.setTextIsSelectable(true);
        chat.setMovementMethod(new android.text.method.ScrollingMovementMethod());
        TextView chatLabel = AppUi.text(chatPanel, "Message", 14, AppUi.INK, true);
        chatIn = new EditText(this);
        chatIn.setId(android.view.View.generateViewId());
        chatLabel.setLabelFor(chatIn.getId());
        chatIn.setHint("Message the other device");
        AppUi.input(chatIn);
        chatIn.setTextSize(16);chatIn.setTextColor(AppUi.INK);chatIn.setHintTextColor(AppUi.MUTED);
        chatIn.setMinHeight(AppUi.dp(this,52));
        chatIn.setMaxLines(4);
        chatIn.setInputType(android.text.InputType.TYPE_CLASS_TEXT | android.text.InputType.TYPE_TEXT_FLAG_CAP_SENTENCES | android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE);
        chatPanel.addView(chatIn, new LinearLayout.LayoutParams(-1,-2));
        sendChat = AppUi.button(chatPanel, "Send message", false);
        sendChat.setEnabled(false);
        chatFeedback = AppUi.text(chatPanel, "Messages are available while this session is connected.", 13, AppUi.MUTED, false);
        chatFeedback.setAccessibilityLiveRegion(android.view.View.ACCESSIBILITY_LIVE_REGION_POLITE);
        sendChat.setOnClickListener(v -> {
            if (chatSending || userClosed) return;
            final String draft = chatIn.getText().toString();
            final String text = draft.trim();
            final SessionViewer viewer = viewers.current();
            if (text.isEmpty()) { chatIn.setError("Enter a message"); chatIn.requestFocus(); return; }
            if (viewer == null) { chatFeedback.setText("Session disconnected. Your draft is kept."); return; }
            chatSending=true;sendChat.setEnabled(false);chatFeedback.setText("Sending message...");
            final int version=++chatSendVersion;
            new Thread(() -> {
                boolean sent=false;
                try { viewer.sendChat(text);sent=true; } catch (Exception ignored) { }
                final boolean completed=sent;
                runOnUiThread(() -> {
                    if(userClosed || version!=chatSendVersion || viewer!=viewers.current())return;
                    chatSending=false;sendChat.setEnabled(true);
                    if(completed){
                        if(chatIn.getText().toString().equals(draft))chatIn.setText("");
                        chatFeedback.setText("Message queued. Delivery is not confirmed.");
                    }else{
                        chatFeedback.setText("Could not confirm send. Draft kept. Check chat before retrying.");
                    }
                });
            },"session-chat-send").start();
        });
        endViewing = AppUi.button(footer, "End viewing", false);
        endViewing.setOnClickListener(v->{stopViewing();finish();});

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
        sessionStatus.setText("Connecting...");
        emptyScreen.setVisibility(android.view.View.VISIBLE);
        emptyScreen.setText("Waiting for a screen\nThe host may need to approve viewing.");
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
                    sessionStatus.setText(frameW > 0 ? "Viewing screen" : "Connected. Waiting for screen...");
                    sendChat.setEnabled(!chatSending);
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
        chatSendVersion++;
        if(chatSending)chatFeedback.setText("Connection interrupted. Draft kept; send is unconfirmed.");
        chatSending=false;
        sendChat.setEnabled(false);
        sessionStatus.setText(message.equals("connect the mesh first") ? "Network connection needed" : "Connection interrupted");
        emptyScreen.setText(message.equals("connect the mesh first") ? "Connect Mnet to your network first.\nEnd viewing to return to setup." : "No screen to display.\nCheck that the host is ready to share.");
        emptyScreen.setVisibility(android.view.View.VISIBLE);
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
                    emptyScreen.setVisibility(android.view.View.GONE);
                    sessionStatus.setText("Viewing screen");
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
        chatSendVersion++;chatSending=false;
        taps.cancel();
        reconnects.removeCallbacks(retryTask);
        retryPending=false;
        if(pendingConnect!=null){pendingConnect.cancel();pendingConnect=null;}
        closeViewer(viewers.close());
        frameW = frameH = 0;
        if(screen!=null)screen.setImageDrawable(null);
        if(controlRequest!=null)applyControl("unavailable");
        if(endViewing!=null)endViewing.setEnabled(false);
        if(sendChat!=null)sendChat.setEnabled(false);
        if(sessionStatus!=null)sessionStatus.setText("Viewing ended");
        if(emptyScreen!=null){emptyScreen.setVisibility(android.view.View.VISIBLE);emptyScreen.setText("Viewing ended");}
    }

    @Override
    protected void onDestroy() {
        stopViewing();
        super.onDestroy();
    }
}
