package au.fintex.mnet;

import android.os.Build;
import android.view.View;

/** Preserve app spacing while keeping interactive content clear of bars, cutouts and IME. */
final class ContentInsets {
    private ContentInsets() {}

    static void install(View root) {
        final int left = root.getPaddingLeft(), top = root.getPaddingTop();
        final int right = root.getPaddingRight(), bottom = root.getPaddingBottom();
        root.setOnApplyWindowInsetsListener((view, window) -> {
            int l, t, r, b;
            if (Build.VERSION.SDK_INT >= 30) {
                android.graphics.Insets edges = window.getInsets(
                        android.view.WindowInsets.Type.systemBars()
                        | android.view.WindowInsets.Type.displayCutout()
                        | android.view.WindowInsets.Type.ime());
                l = edges.left; t = edges.top; r = edges.right; b = edges.bottom;
            } else {
                l = window.getSystemWindowInsetLeft();
                t = window.getSystemWindowInsetTop();
                r = window.getSystemWindowInsetRight();
                b = window.getSystemWindowInsetBottom();
            }
            view.setPadding(left + l, top + t, right + r, bottom + b);
            // Leave insets available for framework/child dispatch. Do not accumulate padding.
            return window;
        });
        root.requestApplyInsets();
    }
}
