package au.fintex.mnet;

import android.app.Activity;
import android.content.Context;
import android.content.res.ColorStateList;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

/** Shared native screen furniture. No session, transport or permission decisions. */
final class AppUi {
    static final int INK = Color.rgb(22, 43, 51);
    static final int MUTED = Color.rgb(77, 98, 105);
    static final int ACCENT = Color.rgb(8, 105, 99);
    static final int CANVAS = Color.rgb(244, 247, 246);
    static final int LINE = Color.rgb(213, 225, 221);
    private AppUi() {}

    static int dp(Context c, int n) { return Math.round(n * c.getResources().getDisplayMetrics().density); }
    static LinearLayout column(Context c) {
        LinearLayout v = new LinearLayout(c);
        v.setOrientation(LinearLayout.VERTICAL);
        return v;
    }
    static LinearLayout screen(Activity a) { return screen(a, null); }
    static LinearLayout screen(Activity a, LinearLayout footer) {
        LinearLayout shell = column(a);
        shell.setBackgroundColor(CANVAS);
        shell.setFocusableInTouchMode(true);
        shell.requestFocus();
        ScrollView scroll = new ScrollView(a);
        scroll.setFillViewport(true);
        scroll.setClipToPadding(false);
        LinearLayout body = column(a);
        int pad = dp(a, 20);
        body.setPadding(pad, pad, pad, pad);
        scroll.addView(body, new ScrollView.LayoutParams(-1, -2));
        shell.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
        if (footer != null) {
            footer.setPadding(pad, 0, pad, dp(a, 12));
            shell.addView(footer, new LinearLayout.LayoutParams(-1, -2));
        }
        ContentInsets.install(shell);
        a.setContentView(shell);
        return body;
    }
    static TextView text(LinearLayout parent, String text, int sp, int color, boolean bold) {
        TextView v = new TextView(parent.getContext());
        v.setText(text); v.setTextSize(sp); v.setTextColor(color);
        v.setFontFeatureSettings("kern");
        if (bold) v.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        v.setLineSpacing(dp(v.getContext(), 3), 1f);
        parent.addView(v, new LinearLayout.LayoutParams(-1, -2));
        return v;
    }
    static void gap(LinearLayout parent, int dp) {
        View v = new View(parent.getContext());
        v.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        parent.addView(v, new LinearLayout.LayoutParams(1, dp(parent.getContext(), dp)));
    }
    static GradientDrawable shape(Context c, int fill, int border) {
        GradientDrawable g = new GradientDrawable(); g.setColor(fill); g.setCornerRadius(dp(c, 18));
        if (border != 0) g.setStroke(dp(c, 1), border);
        return g;
    }
    static LinearLayout card(LinearLayout parent) {
        LinearLayout v = column(parent.getContext());
        int p = dp(v.getContext(), 18); v.setPadding(p,p,p,p);
        v.setBackground(shape(v.getContext(), Color.WHITE, LINE));
        parent.addView(v, new LinearLayout.LayoutParams(-1, -2));
        return v;
    }
    static Button button(LinearLayout parent, String label, boolean primary) {
        Button b = new Button(parent.getContext()); b.setText(label); b.setAllCaps(false);
        b.setTextSize(16); b.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        b.setTextColor(new ColorStateList(new int[][]{new int[]{-android.R.attr.state_enabled},new int[]{}},
                new int[]{MUTED,primary ? Color.WHITE : ACCENT}));
        b.setBackgroundTintList(null);
        android.graphics.drawable.StateListDrawable bg = new android.graphics.drawable.StateListDrawable();
        bg.addState(new int[]{-android.R.attr.state_enabled}, shape(b.getContext(), LINE, 0));
        bg.addState(new int[]{android.R.attr.state_pressed}, shape(b.getContext(), primary ? Color.rgb(5,78,74) : LINE, 0));
        bg.addState(new int[]{}, shape(b.getContext(), primary ? ACCENT : Color.rgb(234,243,239), 0));
        b.setBackground(bg); b.setMinimumHeight(dp(b.getContext(),52));
        b.setPadding(dp(b.getContext(),16),dp(b.getContext(),12),dp(b.getContext(),16),dp(b.getContext(),12));
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(-1, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.topMargin=dp(b.getContext(),10); parent.addView(b,lp);
        return b;
    }
}
