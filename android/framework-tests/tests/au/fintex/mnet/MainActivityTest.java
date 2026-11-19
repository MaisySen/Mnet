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
 @Before public void create(){controller=Robolectric.buildActivity(MainActivity.class).create().start().resume();activity=controller.get();((EditText)activity.findViewById(R.id.server_url)).setText("https://fixture.invalid");((EditText)activity.findViewById(R.id.network_name)).setText("fixture-network");}
 @After public void destroy(){controller.pause().stop().destroy();}
 private Object field(String name)throws Exception{java.lang.reflect.Field f=MainActivity.class.getDeclaredField(name);f.setAccessible(true);return f.get(activity);}
 private void status(String state,boolean always)throws Exception{
  BroadcastReceiver receiver=(BroadcastReceiver)field("statusReceiver");
  receiver.onReceive(activity,new Intent(MnetVpnService.ACTION_STATUS).putExtra(MnetVpnService.EXTRA_STATUS,"{\"status\":\""+state+"\"}").putExtra(MnetVpnService.EXTRA_ALWAYS_ON,always));
 }
 @Test public void statusDetailsActionOnlyFocusesExistingDetails()throws Exception{
  Button action=(Button)field("detailsAction");assertEquals(android.view.View.GONE,action.getVisibility());
  status("waiting-approval",false);assertEquals("View approval details",action.getText().toString());
  action.performClick();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
  assertTrue(((TextView)field("detailsHeading")).hasFocus());
  assertTrue((Boolean)field("connected"));
  assertNull(Shadows.shadowOf(activity).getNextStartedService());
  assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
  BroadcastReceiver receiver=(BroadcastReceiver)field("statusReceiver");
  receiver.onReceive(activity,new Intent(MnetVpnService.ACTION_STATUS).putExtra(MnetVpnService.EXTRA_STATUS,"{\"status\":\"down\",\"last_error\":\"Fixture unavailable\"}"));
  assertEquals("View error details",action.getText().toString());
  assertFalse((Boolean)field("connected"));
  status("up",false);assertEquals("View connection details",action.getText().toString());
 }
 @Test public void emptySetupRevealsFormWithoutConsentSavingOrService()throws Exception{
  ((EditText)activity.findViewById(R.id.network_name)).setText("");
  ((Button)activity.findViewById(R.id.connect_button)).performClick();
  assertEquals(android.view.View.VISIBLE,((android.view.View)field("setupFields")).getVisibility());
  assertNotNull(((EditText)activity.findViewById(R.id.network_name)).getError());
  assertFalse(activity.getSharedPreferences("mnet",0).contains("network"));
  assertFalse(activity.getSharedPreferences("mnet",0).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY,false));
  assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
 @Test public void keyboardOrderIsExplicitAndDoneNeverConnects()throws Exception{
  int[] ids={R.id.server_url,R.id.network_name,R.id.auth_key,R.id.device_name,R.id.exit_node};
  for(int i=0;i<ids.length-1;i++){
   EditText input=activity.findViewById(ids[i]);
   assertEquals(ids[i+1],input.getNextFocusForwardId());
   assertEquals(android.view.inputmethod.EditorInfo.IME_ACTION_NEXT,input.getImeOptions()&android.view.inputmethod.EditorInfo.IME_MASK_ACTION);
  }
  EditText last=activity.findViewById(R.id.exit_node);
  last.onEditorAction(android.view.inputmethod.EditorInfo.IME_ACTION_DONE);
  assertNull(Shadows.shadowOf(activity).getNextStartedService());
  assertFalse(activity.getSharedPreferences("mnet",0).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY,false));
  assertFalse(activity.getSharedPreferences("mnet",0).contains("network"));
 }
 @Test public void incompleteSetupFocusesMissingFieldWithoutConnecting()throws Exception{
  EditText missing=activity.findViewById(R.id.network_name);missing.setText("");
  activity.findViewById(R.id.connect_button).performClick();
  Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
  assertTrue(missing.hasFocus());assertNotNull(missing.getError());
  assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
 @Test public void peerDialogNeedsAddressAndDoesNotStartUntilExplicitConnect(){
  activity.findViewById(R.id.view_peer_button).performClick();
  Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
  android.app.AlertDialog dialog=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();
  assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
  dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).performClick();
  assertTrue(dialog.isShowing());assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
  android.widget.EditText input=findInput(dialog.getWindow().getDecorView());assertNotNull(input);input.setText("100.64.0.3");
  dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).performClick();
  Intent view=Shadows.shadowOf(activity).getNextStartedActivity();assertEquals("100.64.0.3",view.getStringExtra("peer_ip"));
  assertEquals(ViewerActivity.class.getName(),view.getComponent().getClassName());
  assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
 private android.widget.EditText findInput(android.view.View v){
  if(v instanceof android.widget.EditText)return (android.widget.EditText)v;
  if(v instanceof android.view.ViewGroup){android.view.ViewGroup g=(android.view.ViewGroup)v;for(int i=0;i<g.getChildCount();i++){android.widget.EditText input=findInput(g.getChildAt(i));if(input!=null)return input;}}
  return null;
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
 @Test public void vpnDisclosureDeclineAndCancelDoNotSaveOrStart() {
  ((EditText)activity.findViewById(R.id.auth_key)).setText("fixture-not-to-save");
  Button connect=activity.findViewById(R.id.connect_button);connect.performClick();
  android.app.AlertDialog first=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();
  String text=Shadows.shadowOf(first).getMessage().toString();
  assertTrue(text.contains("DNS names outside .mnet"));assertTrue(text.contains("queries from other apps"));assertTrue(text.contains("Google and Cloudflare STUN"));
  assertFalse(activity.getSharedPreferences("mnet",0).contains("authkey"));
  first.getButton(android.app.AlertDialog.BUTTON_NEGATIVE).performClick();
  assertFalse(activity.getSharedPreferences("mnet",0).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY,false));
  assertNull(Shadows.shadowOf(activity).getNextStartedService());assertNull(Shadows.shadowOf(activity).getNextStartedActivity());
  connect.performClick();android.app.AlertDialog reopened=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();assertNotSame(first,reopened);reopened.cancel();
  assertFalse(activity.getSharedPreferences("mnet",0).contains("authkey"));assertNull(Shadows.shadowOf(activity).getNextStartedService());
 }
 @Test public void vpnDisclosureAgreePersistsOnlyAfterExplicitAction() {
  ((Button)activity.findViewById(R.id.connect_button)).performClick();
  assertFalse(activity.getSharedPreferences("mnet",0).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY,false));
  org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog().getButton(android.app.AlertDialog.BUTTON_POSITIVE).performClick();
  Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
  assertTrue(activity.getSharedPreferences("mnet",0).getBoolean(MnetVpnService.VPN_DISCLOSURE_KEY,false));
  assertTrue(activity.getSharedPreferences("mnet",0).contains("server"));
 }
 @Test public void unsolicitedVpnPermissionResultCannotStartWithoutDisclosure() {
  activity.onActivityResult(1,android.app.Activity.RESULT_OK,null);
  assertNull(Shadows.shadowOf(activity).getNextStartedService());
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
