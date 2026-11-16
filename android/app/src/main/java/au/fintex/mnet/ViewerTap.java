package au.fintex.mnet;

// One completed stationary finger gesture produces one click. Drag/multitouch
// forwarding is not implemented, so those gestures must not become taps.
final class ViewerTap {
    private boolean pending;
    private float startX, startY;
    void down(float x, float y) { pending = true; startX = x; startY = y; }
    void move(float x, float y, float slop) {
        if (Math.abs(x - startX) > slop || Math.abs(y - startY) > slop) pending = false;
    }
    boolean up(float x, float y, float slop) {
        move(x, y, slop);
        boolean click = pending;
        pending = false;
        return click;
    }
    void cancel() { pending = false; }
}
