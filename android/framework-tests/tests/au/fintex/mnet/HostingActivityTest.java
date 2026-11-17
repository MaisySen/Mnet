package au.fintex.mnet;
import android.app.AlertDialog;
import android.content.Intent;
import android.provider.Settings;
import android.widget.Button;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.android.controller.ActivityController;
import org.robolectric.annotation.Config;
import org.robolectric.shadows.ShadowAlertDialog;
import static org.junit.Assert.*;
@RunWith(RobolectricTestRunner.class) @Config(manifest=Config.NONE,sdk={24,25,26,33})
public class HostingActivityTest {
 private ActivityController<HostingActivity> controller;private HostingActivity activity;
 @Before public void create(){controller=Robolectric.buildActivity(HostingActivity.class).create().start().resume();activity=controller.get();}
 @After public void destroy(){controller.pause().stop().destroy();}
 private Button accessButton(android.view.View view){if(view instanceof Button && ((Button)view).getText().toString().equals("Open accessibility settings"))return (Button)view;if(view instanceof android.view.ViewGroup){android.view.ViewGroup group=(android.view.ViewGroup)view;for(int i=0;i<group.getChildCount();i++){Button found=accessButton(group.getChildAt(i));if(found!=null)return found;}}return null;}
 private AlertDialog disclosure(){Button access=accessButton(activity.findViewById(android.R.id.content));assertNotNull(access);access.performClick();AlertDialog dialog=ShadowAlertDialog.getLatestAlertDialog();assertNotNull(dialog);assertTrue(dialog.isShowing());assertNull(Shadows.shadowOf(activity).getNextStartedActivity());String text=Shadows.shadowOf(dialog).getMessage().toString();assertTrue(text.contains("tap and scroll"));assertTrue(text.contains("does not read text or window contents"));assertTrue(text.contains("VPN and viewing remain available"));return dialog;}
 @Test public void affirmativeConsentAloneOpensSettings(){AlertDialog dialog=disclosure();dialog.getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Intent intent=Shadows.shadowOf(activity).getNextStartedActivity();assertNotNull(intent);assertEquals(Settings.ACTION_ACCESSIBILITY_SETTINGS,intent.getAction());assertNull(Shadows.shadowOf(activity).getNextStartedService());}
 @Test public void declineLeavesOtherFeaturesAndCanAskLater(){AlertDialog dialog=disclosure();dialog.getButton(AlertDialog.BUTTON_NEGATIVE).performClick();assertNull(Shadows.shadowOf(activity).getNextStartedActivity());assertTrue(activity.findViewById(R.id.start_hosting_button).isEnabled());assertNull(Shadows.shadowOf(activity).getNextStartedService());disclosure();}
 @Test public void cancelNeverOpensSettings(){disclosure().cancel();assertNull(Shadows.shadowOf(activity).getNextStartedActivity());assertNull(Shadows.shadowOf(activity).getNextStartedService());}
}
