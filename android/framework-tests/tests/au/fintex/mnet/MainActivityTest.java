package au.fintex.mnet;
import android.content.*;
import android.widget.*;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.android.controller.ActivityController;
import org.robolectric.annotation.Config;
import static org.junit.Assert.*;

// Actual Activity code, simulated framework, resource placeholders and fake VPN endpoint.
@RunWith(RobolectricTestRunner.class) @Config(manifest=Config.NONE,sdk={24,25,26,33})
public class MainActivityTest {
 private ActivityController<MainActivity> controller;
 private MainActivity activity;
 @Before public void create(){controller=Robolectric.buildActivity(MainActivity.class).create().start().resume();activity=controller.get();}
 @After public void destroy(){controller.pause().stop().destroy();}
 private Object field(String name)throws Exception{java.lang.reflect.Field f=MainActivity.class.getDeclaredField(name);f.setAccessible(true);return f.get(activity);}
 private void status(String state,boolean always)throws Exception{
  BroadcastReceiver receiver=(BroadcastReceiver)field("statusReceiver");
  receiver.onReceive(activity,new Intent(MnetVpnService.ACTION_STATUS).putExtra(MnetVpnService.EXTRA_STATUS,"{\"status\":\""+state+"\"}").putExtra(MnetVpnService.EXTRA_ALWAYS_ON,always));
 }
 @Test public void managedStatesDisableDisconnectAndExplainSettings()throws Exception{
  Button toggle=activity.findViewById(R.id.connect_button);
  for(String state:new String[]{"up","joining","waiting-approval"}){
   status(state,true);assertFalse(toggle.isEnabled());assertTrue(((TextView)field("status")).getText().toString().contains("Android Settings controls disconnect"));
   // Invoke the handler as a stale click, independent of disabled View enforcement.
   java.lang.reflect.Method m=MainActivity.class.getDeclaredMethod("onToggle");m.setAccessible(true);m.invoke(activity);
   assertNull(Shadows.shadowOf(activity).getNextStartedService());assertTrue((Boolean)field("connected"));
  }
 }
 @Test public void modeRefreshRestoresManualDisconnect()throws Exception{
  status("up",true);status("up",false);Button toggle=activity.findViewById(R.id.connect_button);assertTrue(toggle.isEnabled());
  assertFalse(((TextView)field("status")).getText().toString().contains("Android Settings controls disconnect"));
  toggle.performClick();Intent stop=Shadows.shadowOf(activity).getNextStartedService();assertNotNull(stop);assertEquals("disconnect",stop.getAction());assertFalse((Boolean)field("connected"));
 }
 @Test public void downStateLeavesConnectAvailableEvenWhenManaged()throws Exception{
  status("up",true);status("down",true);assertTrue(((Button)activity.findViewById(R.id.connect_button)).isEnabled());assertFalse((Boolean)field("connected"));assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
 @Test public void privacySupportRequiresUserActionAndDoesNotAttachSecrets() {
  ((Button)activity.findViewById(R.id.privacy_support_button)).performClick();
  android.app.AlertDialog dialog=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();
  String message=Shadows.shadowOf(dialog).getMessage().toString();
  assertTrue(message.contains("contact@fintex.au"));assertTrue(message.contains("Disconnecting does not delete"));
  assertTrue(message.contains("Google and Cloudflare STUN"));assertTrue(message.contains("source IP address and port"));
  assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
  dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).performClick();
  Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
  Intent email=Shadows.shadowOf(activity).getNextStartedActivity();
  assertEquals(Intent.ACTION_SENDTO,email.getAction());assertEquals("mailto:contact@fintex.au",email.getDataString());
  assertNull(email.getExtras());
 }
 @Test public void privacyPolicyOpensOnlyPublicNoticeWithoutPrivateExtras() {
  ((Button)activity.findViewById(R.id.privacy_support_button)).performClick();
  android.app.AlertDialog dialog=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();
  assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
  dialog.getButton(android.app.AlertDialog.BUTTON_NEUTRAL).performClick();
  Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
  Intent policy=Shadows.shadowOf(activity).getNextStartedActivity();
  assertEquals(Intent.ACTION_VIEW,policy.getAction());assertEquals("https://maisysen.github.io/Mnet/",policy.getDataString());
  assertNull(policy.getExtras());assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
 @Test public void closingPrivacySupportDoesNotSendOrDisconnect() {
  ((Button)activity.findViewById(R.id.privacy_support_button)).performClick();
  org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog().getButton(android.app.AlertDialog.BUTTON_NEGATIVE).performClick();
  assertNull(Shadows.shadowOf(activity).getNextStartedActivity());assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
}
