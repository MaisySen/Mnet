package au.fintex.mnet;

import android.app.Activity;
import android.graphics.Rect;
import android.view.View;
import android.view.WindowInsets;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.robolectric.Robolectric;
import org.robolectric.RobolectricTestRunner;
import org.robolectric.annotation.Config;
import static org.junit.Assert.*;

@RunWith(RobolectricTestRunner.class)
@Config(sdk = {24, 25, 26, 33}, manifest = Config.NONE)
public class ContentInsetsTest {
    @Test public void preservesBaseAndDoesNotAccumulate() {
        Activity activity = Robolectric.buildActivity(Activity.class).setup().get();
        View root = new View(activity);
        root.setPadding(7, 11, 13, 17);
        ContentInsets.install(root);
        assertTrue((root.getSystemUiVisibility() & View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR) != 0);
        if (android.os.Build.VERSION.SDK_INT >= 26) assertTrue((root.getSystemUiVisibility() & View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR) != 0);
        WindowInsets insets = new WindowInsets(new Rect(2, 24, 3, 40));
        assertSame(insets, root.dispatchApplyWindowInsets(insets));
        assertEquals(9, root.getPaddingLeft());
        assertEquals(35, root.getPaddingTop());
        assertEquals(16, root.getPaddingRight());
        assertEquals(57, root.getPaddingBottom());
        root.dispatchApplyWindowInsets(insets);
        assertEquals(35, root.getPaddingTop());
        assertEquals(57, root.getPaddingBottom());
        root.dispatchApplyWindowInsets(new WindowInsets(new Rect()));
        assertEquals(11, root.getPaddingTop());
        assertEquals(17, root.getPaddingBottom());
    }
    @Test public void actualMainActivityInstallsInsets() {
        MainActivity activity = Robolectric.buildActivity(MainActivity.class).setup().get();
        View root = ((android.view.ViewGroup) activity.findViewById(android.R.id.content)).getChildAt(0);
        int base = root.getPaddingTop();
        root.dispatchApplyWindowInsets(new WindowInsets(new Rect(0, 24, 0, 40)));
        assertEquals(base + 24, root.getPaddingTop());
    }
    @Test public void actualHostingActivityInstallsInsets() {
        HostingActivity activity = Robolectric.buildActivity(HostingActivity.class).setup().get();
        View root = ((android.view.ViewGroup) activity.findViewById(android.R.id.content)).getChildAt(0);
        int base = root.getPaddingBottom();
        root.dispatchApplyWindowInsets(new WindowInsets(new Rect(0, 24, 0, 40)));
        assertEquals(base + 40, root.getPaddingBottom());
    }
    @Test @Config(sdk = 33) public void keyboardUsesMaxRatherThanAddingNavigation() {
        Activity activity = Robolectric.buildActivity(Activity.class).setup().get();
        View root = new View(activity);
        root.setPadding(5, 6, 7, 8);
        ContentInsets.install(root);
        assertTrue((root.getSystemUiVisibility() & View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR) != 0);
        if (android.os.Build.VERSION.SDK_INT >= 26) assertTrue((root.getSystemUiVisibility() & View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR) != 0);
        WindowInsets insets = new WindowInsets.Builder()
            .setInsets(WindowInsets.Type.systemBars(), android.graphics.Insets.of(0, 24, 0, 30))
            .setInsets(WindowInsets.Type.ime(), android.graphics.Insets.of(0, 0, 0, 300))
            .setInsets(WindowInsets.Type.displayCutout(), android.graphics.Insets.of(10, 40, 0, 0))
            .build();
        root.dispatchApplyWindowInsets(insets);
        assertEquals(15, root.getPaddingLeft());
        assertEquals(46, root.getPaddingTop());
        assertEquals(308, root.getPaddingBottom());
    }
}
