package au.fintex.mnet;

import android.app.Activity;
import android.widget.Button;
import android.widget.LinearLayout;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.robolectric.Robolectric;
import org.robolectric.RobolectricTestRunner;
import org.robolectric.annotation.Config;
import static org.junit.Assert.*;

/** Drawable state only, not installed keyboard/TalkBack acceptance. */
@RunWith(RobolectricTestRunner.class)
@Config(manifest=Config.NONE,sdk={24,25,26,33})
public class AppUiFocusTest {
 @Test public void bothButtonStylesHaveDistinctFocusWithoutClicking(){
  try(var controller=Robolectric.buildActivity(Activity.class).setup()){
   Activity activity=controller.get();LinearLayout root=AppUi.column(activity);activity.setContentView(root);
   for(boolean primary:new boolean[]{true,false}){
    Button button=AppUi.button(root,"Fixture action",primary);
    java.util.concurrent.atomic.AtomicInteger clicks=new java.util.concurrent.atomic.AtomicInteger();
    button.setOnClickListener(v->clicks.incrementAndGet());
    android.graphics.drawable.StateListDrawable background=(android.graphics.drawable.StateListDrawable)button.getBackground();
    background.setState(new int[]{android.R.attr.state_enabled});android.graphics.drawable.Drawable normal=background.getCurrent();
    background.setState(new int[]{android.R.attr.state_enabled,android.R.attr.state_focused});
    assertNotSame(normal,background.getCurrent());
    button.setFocusableInTouchMode(true);button.requestFocus();assertEquals(0,clicks.get());
    button.setEnabled(false);background.setState(new int[]{android.R.attr.state_focused});
    assertNotSame(normal,background.getCurrent());assertFalse(button.isEnabled());assertEquals(0,clicks.get());
   }
  }
 }
}
