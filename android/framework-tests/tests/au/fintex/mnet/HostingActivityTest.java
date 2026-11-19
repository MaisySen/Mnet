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
 private AlertDialog disclosure(){Button access=accessButton(activity.findViewById(android.R.id.content));assertNotNull(access);access.performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();AlertDialog dialog=ShadowAlertDialog.getLatestAlertDialog();assertNotNull(dialog);assertTrue(dialog.isShowing());assertNull(Shadows.shadowOf(activity).getNextStartedActivity());String text=Shadows.shadowOf(dialog).getMessage().toString();assertTrue(text.contains("tap and scroll"));assertTrue(text.contains("does not read text or window contents"));assertTrue(text.contains("VPN and viewing remain available"));return dialog;}
 @Test public void hostingStateIsReadFromServiceAndStopRemainsIndependent()throws Exception{
  android.widget.Button start=(android.widget.Button)activity.findViewById(R.id.start_hosting_button);
  java.lang.reflect.Field stopField=HostingActivity.class.getDeclaredField("stop");stopField.setAccessible(true);
  android.widget.Button stop=(android.widget.Button)stopField.get(activity);
  assertTrue(start.isEnabled());assertFalse(stop.isEnabled());
  org.robolectric.android.controller.ServiceController<HostSessionService> service=Robolectric.buildService(HostSessionService.class).create();
  java.lang.reflect.Field running=HostSessionService.class.getDeclaredField("running");running.setAccessible(true);
  java.lang.reflect.Method refresh=HostingActivity.class.getDeclaredMethod("refreshHostingState");refresh.setAccessible(true);
  try{
   running.set(service.get(),true);refresh.invoke(activity);
   assertFalse(start.isEnabled());assertTrue(stop.isEnabled());
   stop.performClick();Intent request=Shadows.shadowOf(activity).getNextStartedService();
   assertEquals("stop",request.getAction());assertFalse(stop.isEnabled());
   // It is a request, not confirmation. Service observation drives the final state.
   running.set(service.get(),false);refresh.invoke(activity);
   assertTrue(start.isEnabled());assertFalse(stop.isEnabled());
  }finally{service.destroy();}
 }
 @Test public void affirmativeConsentAloneOpensSettings(){AlertDialog dialog=disclosure();dialog.getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Intent intent=Shadows.shadowOf(activity).getNextStartedActivity();assertNotNull(intent);assertEquals(Settings.ACTION_ACCESSIBILITY_SETTINGS,intent.getAction());assertNull(Shadows.shadowOf(activity).getNextStartedService());}
 @Test public void declineLeavesOtherFeaturesAndCanAskLater(){AlertDialog dialog=disclosure();dialog.getButton(AlertDialog.BUTTON_NEGATIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();assertNull(Shadows.shadowOf(activity).getNextStartedActivity());assertTrue(activity.findViewById(R.id.start_hosting_button).isEnabled());assertNull(Shadows.shadowOf(activity).getNextStartedService());disclosure();}
 @Test public void cancelNeverOpensSettings(){disclosure().cancel();assertNull(Shadows.shadowOf(activity).getNextStartedActivity());assertNull(Shadows.shadowOf(activity).getNextStartedService());}
 @Test public void controlWithoutViewerCannotGrantOrOpenSettings(){
  activity.showControlConsent();AlertDialog dialog=ShadowAlertDialog.getLatestAlertDialog();
  assertEquals("No control request",Shadows.shadowOf(dialog).getTitle().toString());
  assertFalse(HostSessionService.controlAllowed());assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
 }
 @Test public void olderSameViewerDialogCannotRestoreControlAfterNewerDecision()throws Exception{
  org.robolectric.android.controller.ServiceController<HostSessionService> service=Robolectric.buildService(HostSessionService.class).create();
  MnetAccessibilityService.current=new MnetAccessibilityService();
  try{
   for(String name:new String[]{"running","autoAccept"}){java.lang.reflect.Field field=HostSessionService.class.getDeclaredField(name);field.setAccessible(true);field.set(service.get(),true);}
   assertTrue(service.get().acceptSession("viewer"));ServiceTest.requestControl(service.get());
   activity.showControlConsent();AlertDialog stale=ShadowAlertDialog.getLatestAlertDialog();
   activity.showControlConsent();AlertDialog current=ShadowAlertDialog.getLatestAlertDialog();
   current.getButton(AlertDialog.BUTTON_NEGATIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   stale.getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   assertFalse("old same-viewer approval restored control",HostSessionService.controlAllowed());
   ServiceTest.requestControl(service.get());activity.showControlConsent();ShadowAlertDialog.getLatestAlertDialog().getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   assertTrue(HostSessionService.controlAllowed());
   activity.showControlConsent();AlertDialog oldKeep=ShadowAlertDialog.getLatestAlertDialog();
   assertFalse(HostSessionService.controlAllowed());
   activity.showControlConsent();AlertDialog newer=ShadowAlertDialog.getLatestAlertDialog();
   newer.getButton(AlertDialog.BUTTON_NEGATIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();oldKeep.getButton(AlertDialog.BUTTON_NEGATIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   assertFalse("old Keep restored control",HostSessionService.controlAllowed());
  }finally{service.destroy();MnetAccessibilityService.current=null;}
 }
 @Test public void separateRequestedControlDeclineApprovePauseRevokeAndOldSession()throws Exception{
  org.robolectric.android.controller.ServiceController<HostSessionService> service=Robolectric.buildService(HostSessionService.class).create();
  MnetAccessibilityService.current=new MnetAccessibilityService();
  try{
   for(String name:new String[]{"running","autoAccept"}){java.lang.reflect.Field field=HostSessionService.class.getDeclaredField(name);field.setAccessible(true);field.set(service.get(),true);}
   assertTrue(service.get().acceptSession("viewer"));
   activity.showControlConsent();assertEquals("No control request",Shadows.shadowOf(ShadowAlertDialog.getLatestAlertDialog()).getTitle().toString());
   ServiceTest.requestControl(service.get());activity.showControlConsent();AlertDialog first=ShadowAlertDialog.getLatestAlertDialog();
   assertTrue(Shadows.shadowOf(first).getMessage().toString().contains("Viewing continues"));first.getButton(AlertDialog.BUTTON_NEGATIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();assertFalse(HostSessionService.controlAllowed());
   ServiceTest.requestControl(service.get());activity.showControlConsent();ShadowAlertDialog.getLatestAlertDialog().getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();assertTrue(HostSessionService.controlAllowed());
   activity.showControlConsent();assertFalse(HostSessionService.controlAllowed());ShadowAlertDialog.getLatestAlertDialog().getButton(AlertDialog.BUTTON_NEGATIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();assertTrue(HostSessionService.controlAllowed());
   activity.showControlConsent();assertFalse(HostSessionService.controlAllowed());ShadowAlertDialog.getLatestAlertDialog().getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();assertFalse(HostSessionService.controlAllowed());
   ServiceTest.requestControl(service.get());activity.showControlConsent();AlertDialog stale=ShadowAlertDialog.getLatestAlertDialog();
   service.get().onSessionEnd();assertTrue(service.get().acceptSession("next"));ServiceTest.requestControl(service.get());stale.getButton(AlertDialog.BUTTON_POSITIVE).performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();assertFalse(HostSessionService.controlAllowed());
  }finally{service.destroy();MnetAccessibilityService.current=null;}
 }
}
