package au.fintex.mnet;

import android.accessibilityservice.AccessibilityService;
import android.accessibilityservice.GestureDescription;
import android.graphics.Path;
import android.view.accessibility.AccessibilityEvent;

import org.json.JSONObject;

// MnetAccessibilityService injects viewer input into the device while
// this phone hosts a session. The user enables it once in system
// settings; gestures only run while HostSessionService is active.
public class MnetAccessibilityService extends AccessibilityService {

    private static MnetAccessibilityService instance;
    private static volatile boolean hosting;

    public static void setHosting(boolean v) {
        hosting = v;
    }

    public static boolean available() {
        return instance != null;
    }

    public static MnetAccessibilityService instance() {
        return instance;
    }

    @Override
    public void onServiceConnected() {
        instance = this;
    }

    @Override
    public boolean onUnbind(android.content.Intent intent) {
        instance = null;
        return super.onUnbind(intent);
    }

    @Override
    public void onAccessibilityEvent(AccessibilityEvent event) {
    }

    @Override
    public void onInterrupt() {
    }

    // inject applies one session.InputEvent JSON from the Go engine.
    // Coordinates arrive in capture-frame pixels; the caller scales
    // them to device pixels before this call.
    public void inject(String evJson, float scale) {
        if (!hosting || !(scale > 0) || Float.isInfinite(scale)) {
            return;
        }
        try {
            JSONObject ev = new JSONObject(evJson);
            String kind = ev.optString("kind");
            if (!"pointer".equals(kind)) {
                return; // key text stays on the host's own keyboard in v1
            }
            float x = (float) ev.optDouble("x") * scale;
            float y = (float) ev.optDouble("y") * scale;
            if (Float.isNaN(x) || Float.isInfinite(x) || x < 0
                    || Float.isNaN(y) || Float.isInfinite(y) || y < 0) return;
            String action = ev.optString("action");
            Path p = new Path();
            p.moveTo(x, y);
            GestureDescription.Builder b = new GestureDescription.Builder();
            switch (action) {
                case "click":
                    b.addStroke(new GestureDescription.StrokeDescription(p, 0, 50));
                    break;
                case "scroll": {
                    float dy = (float) ev.optDouble("dy", -300);
                    p.lineTo(x, y + (dy < 0 ? -300 : 300));
                    b.addStroke(new GestureDescription.StrokeDescription(p, 0, 300));
                    break;
                }
                default:
                    return; // move needs no gesture
            }
            dispatchGesture(b.build(), null, null);
        } catch (Exception ignored) {
        }
    }
}
