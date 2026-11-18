package au.fintex.mnet.fixture;

import android.app.Activity;
import android.app.Instrumentation;
import android.content.Intent;
import android.graphics.Bitmap;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.accessibility.AccessibilityNodeInfo;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;
import java.io.*;
import java.net.*;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.atomic.AtomicReference;
import org.json.JSONObject;
import au.fintex.mnet.MainActivity;
import au.fintex.mnet.HostingActivity;
import au.fintex.mnet.MnetVpnService;
import au.fintex.mnet.R;

// Manual-only control journey acceptance. No DNS, bulk or tracing calls.
public class ControlAcceptance extends Instrumentation {
 private final org.json.JSONArray passedStages=new org.json.JSONArray();
 private volatile Activity resumedActivity;
 private Bundle options;private Activity activity;private String stage="created";private boolean consentClicked,entireScreenSelected;
 private interface Probe{boolean ready()throws Exception;}
 @Override public void onCreate(Bundle args){super.onCreate(args);options=args;start();}
    private void checkpoint(String label) throws Exception {
        stage=label;
        try(FileOutputStream file=new FileOutputStream(new File(getTargetContext().getFilesDir(),"fixture-stage"))){file.write(label.getBytes("UTF-8"));}
    }
    private void await(Probe probe, String label) throws Exception {
        checkpoint(label);
        long deadline = SystemClock.elapsedRealtime() + 60000;
        while (SystemClock.elapsedRealtime() < deadline) {
            if (probe.ready()){passedStages.put(label);return;}
            SystemClock.sleep(150);
        }
        throw new IllegalStateException("bounded fixture condition not met");
    }
    private void click(int id) { runOnMainSync(() -> ((Button) activity.findViewById(id)).performClick()); }
    private void set(int id, String value) { runOnMainSync(() -> ((EditText) activity.findViewById(id)).setText(value)); }
    private void vpnDisclosure(boolean agree, boolean capture) throws Exception {
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                    || root.findAccessibilityNodeInfosByText("VPN data disclosure").isEmpty()) return false;

            return clickText(root, agree ? "Agree" : "Not now");
        }, agree ? "vpn-disclosure-agree" : "vpn-disclosure-decline");
        if (agree) {
            await(() -> {
                if (getTargetContext().getSharedPreferences("mnet", 0).getBoolean("vpn_disclosure_v1", false)) return true;
                // An accessibility click acknowledgement is not proof that
                // the app ran its listener. Retry only while still unaccepted.
                AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
                if (root != null && "au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())
                        && !root.findAccessibilityNodeInfosByText("VPN data disclosure").isEmpty()) clickText(root, "Agree");
                return false;
            }, "vpn-disclosure-accepted");
        }
    }
    private void vpnConsent() throws Exception {
        await(() -> {
            if (MnetVpnService.currentNode() != null) return true;
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null || !"com.android.vpndialogs".contentEquals(root.getPackageName() == null ? "" : root.getPackageName())) return false;
            // Click the actual OS consent button, never a setting that disables
            // Play Protect or a production-phone policy. English emulator UI.
            List<AccessibilityNodeInfo> buttons = root.findAccessibilityNodeInfosByViewId("android:id/button1");
            for (AccessibilityNodeInfo button : buttons) {
                if ("OK".equals(String.valueOf(button.getText()))) {
                    consentClicked = button.performAction(AccessibilityNodeInfo.ACTION_CLICK);
                    return consentClicked;
                }
            }
            return false;
        }, "vpn-consent");
    }
    private boolean up() {
        if (MnetVpnService.currentNode() == null) return false;
        try { return "up".equals(new JSONObject(MnetVpnService.currentNode().status()).optString("status")); }
        catch (Exception unavailable) { return false; }
    }
    private void osVpnReady() throws Exception {
        // Node.up is engine state. Android publishes the VPN network separately;
        // opening a socket before that can cache an underlay source address.
        await(() -> {
            android.net.ConnectivityManager cm = getTargetContext().getSystemService(android.net.ConnectivityManager.class);
            android.net.Network network = cm.getActiveNetwork();
            android.net.NetworkCapabilities caps = cm.getNetworkCapabilities(network);
            android.net.LinkProperties props = cm.getLinkProperties(network);
            if (caps == null || props == null || !caps.hasTransport(android.net.NetworkCapabilities.TRANSPORT_VPN)) return false;
            String self = MnetVpnService.currentNode().meshIPv4();
            for (android.net.LinkAddress address : props.getLinkAddresses()) {
                if (self.equals(address.getAddress().getHostAddress())) return true;
            }
            return false;
        }, "os-vpn-published");

    }
    private boolean clickText(AccessibilityNodeInfo root, String text) {
        if (root == null) return false;
        if (text.equalsIgnoreCase(String.valueOf(root.getText()))) {
            AccessibilityNodeInfo candidate = root;
            for (int i=0; candidate != null && i<4; i++, candidate=candidate.getParent()) {
                if (candidate.isClickable() && candidate.isEnabled()) return candidate.performAction(AccessibilityNodeInfo.ACTION_CLICK);
            }
        }
        for (int i=0;i<root.getChildCount();i++) if (clickText(root.getChild(i), text)) return true;
        return false;
    }
    private void enableRemoteControl() throws Exception {
        stage = "accessibility-disclosure";
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Open accessibility settings"), "accessibility-open-disclosure");
        SystemClock.sleep(3500);
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Not now"), "accessibility-decline");
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            return root != null && "au.fintex.mnet".contentEquals(root.getPackageName()) && !au.fintex.mnet.MnetAccessibilityService.available();
        }, "accessibility-decline-stays-in-app");
        SystemClock.sleep(1500);
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Open accessibility settings"), "accessibility-reopen-disclosure");
        SystemClock.sleep(3500);
        await(() -> clickText(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow(), "Agree"), "accessibility-agree");
        await(() -> {
            if (au.fintex.mnet.MnetAccessibilityService.available()) return true;
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            if (root == null) return false;
            String pkg = String.valueOf(root.getPackageName());
            if (!pkg.equals("com.android.settings") && !pkg.equals("com.android.permissioncontroller")) return false;
            if (clickText(root, "Allow")) return false;
            if (clickText(root, "Use Mnet")) return false;
            if (clickText(root, "Mnet")) return false;
            if (clickText(root, "Downloaded apps") || clickText(root, "Installed apps")) return false;
            return false;
        }, "accessibility-system-enable");
        // Return to the already-created host page, without replaying consent.
        runOnMainSync(() -> activity.startActivity(new Intent(activity, HostingActivity.class).addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT)));
        await(() -> {
            AccessibilityNodeInfo root = getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();
            return root != null && "au.fintex.mnet".contentEquals(root.getPackageName() == null ? "" : root.getPackageName());
        }, "accessibility-return-host");
    }
 private JSONObject command(String command)throws Exception{
  HttpURLConnection c=(HttpURLConnection)new URL("http://127.0.0.1:8081/command").openConnection();c.setRequestMethod("POST");c.setDoOutput(true);c.setConnectTimeout(25000);c.setReadTimeout(25000);
  byte[] body=new JSONObject().put("command",command).toString().getBytes("UTF-8");c.getOutputStream().write(body);
  try(InputStream in=c.getInputStream();ByteArrayOutputStream out=new ByteArrayOutputStream()){byte[] b=new byte[1024];int n;while((n=in.read(b))>0)out.write(b,0,n);return new JSONObject(out.toString("UTF-8"));}finally{c.disconnect();}
 }
 private void require(boolean value){if(!value)throw new IllegalStateException("acceptance condition");}
 private JSONObject wire()throws Exception{return new JSONObject(MnetVpnService.currentNode().sessionControlStatus());}
 private void hostState(String want)throws Exception{await(()->want.equals(wire().optString("state")),"host-state-"+want);}
 private JSONObject peer()throws Exception{return command("status");}
 private void peerState(String want)throws Exception{await(()->want.equals(peer().getJSONObject("control").optString("state")),"peer-state-"+want);}
 private AccessibilityNodeInfo root(){return getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getRootInActiveWindow();}
 private void button(String text)throws Exception{await(()->clickText(root(),text),"button-"+text.replace(' ','-'));waitForIdleSync();}
 private void shot(String name)throws Exception{
  // All calls are on dedicated fixture host/viewer/consent screens, never MainActivity.
  SystemClock.sleep(800);Bitmap image=getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).takeScreenshot();require(image!=null);
  try(FileOutputStream out=new FileOutputStream(new File(getTargetContext().getFilesDir(),"control-"+name+".png"))){require(image.compress(Bitmap.CompressFormat.PNG,100,out));}image.recycle();
 }
 private void projectionConsent()throws Exception{
  await(()->{AccessibilityNodeInfo r=root();if(r==null||!"com.android.systemui".contentEquals(r.getPackageName()))return false;
   if(!entireScreenSelected){if(clickText(r,"Share entire screen")||clickText(r,"Entire screen")){entireScreenSelected=true;return false;}if(clickText(r,"Share one app"))return false;}
   return clickText(r,"Start now")||clickText(r,"Start")||clickText(r,"Share screen");},"projection-consent");
 }
 private static Object field(Object object,String name)throws Exception{java.lang.reflect.Field f=object.getClass().getDeclaredField(name);f.setAccessible(true);return f.get(object);}
 private void viewerState(String expected)throws Exception{await(()->{java.util.concurrent.atomic.AtomicBoolean ok=new java.util.concurrent.atomic.AtomicBoolean();runOnMainSync(()->{try{String text=((TextView)field(activity,"controlStatus")).getText().toString();ok.set(text.contains(expected));}catch(Exception e){}});return ok.get();},"viewer-"+expected.replace(' ','-'));}
 private void screenTap()throws Exception{
  int[] point=new int[2];runOnMainSync(()->{try{android.view.View v=(android.view.View)field(activity,"screen");v.getLocationOnScreen(point);point[0]+=v.getWidth()/2;point[1]+=v.getHeight()/2;}catch(Exception e){throw new RuntimeException(e);}});
  long now=SystemClock.uptimeMillis();android.view.MotionEvent down=android.view.MotionEvent.obtain(now,now,0,point[0],point[1],0),up=android.view.MotionEvent.obtain(now,now+50,1,point[0],point[1],0);
  require(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).injectInputEvent(down,true));require(getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).injectInputEvent(up,true));down.recycle();up.recycle();SystemClock.sleep(400);
 }
 private boolean hostReady()throws Exception{
  java.lang.reflect.Field f=au.fintex.mnet.HostSessionService.class.getDeclaredField("diagnosticInstance");f.setAccessible(true);
  java.lang.ref.WeakReference<?> ref=(java.lang.ref.WeakReference<?>)f.get(null);Object host=ref==null?null:ref.get();if(host==null)return false;
  synchronized(field(host,"hostLifecycle")){return (boolean)field(host,"running")&&(boolean)field(host,"hostStarted");}
 }
 private boolean consentPending()throws Exception{
  java.lang.reflect.Field f=au.fintex.mnet.HostSessionService.class.getDeclaredField("CONSENT");f.setAccessible(true);Object consent=f.get(null);
  synchronized(consent){Object request=field(consent,"current");return request!=null&&!(boolean)field(request,"resolved");}
 }
 private void admissionPending()throws Exception{
  await(()->{String outcome=peer().optString("admission");
   if(ControlAcceptanceState.admissionFailed(outcome)){stage="prerequisite-admission-"+outcome;throw new IllegalStateException("fixed admission outcome");}
   return consentPending();},"prerequisite-admission-request-pending");
 }
 private boolean viewerBounds(Activity current)throws Exception{
  android.view.View status=(android.view.View)field(current,"controlStatus"),button=(android.view.View)field(current,"controlRequest");
  android.graphics.Rect a=new android.graphics.Rect(),b=new android.graphics.Rect();
  return status.isShown()&&button.isShown()&&status.getGlobalVisibleRect(a)&&button.getGlobalVisibleRect(b)
   &&a.height()==status.getHeight()&&a.width()==status.getWidth()&&b.height()==button.getHeight()&&b.width()==button.getWidth();
 }
 private boolean fontRecovered(Activity before)throws Exception{
  Activity current=resumedActivity;if(!(current instanceof au.fintex.mnet.ViewerActivity)||current==before)return false;
  java.util.concurrent.atomic.AtomicBoolean ready=new java.util.concurrent.atomic.AtomicBoolean();
  int inputs=peer().getInt("inputs");
  runOnMainSync(()->{try{
   Object gate=field(current,"viewers");java.lang.reflect.Method m=gate.getClass().getDeclaredMethod("current");m.setAccessible(true);mobile.SessionViewer viewer=(mobile.SessionViewer)m.invoke(gate);
   String state=viewer==null?"":new JSONObject(viewer.controlStatus()).optString("state");
   ready.set(ControlAcceptanceState.fontViewingReady(before,current,current.getResources().getConfiguration().fontScale,(int)field(current,"frameW"),state,inputs,viewerBounds(current)));
  }catch(Exception unavailable){}});
  if(ready.get())activity=current;return ready.get();
 }
 private void hostLeg()throws Exception{
  activity=startActivitySync(new Intent(getTargetContext(),HostingActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));enableRemoteControl();
  java.util.concurrent.atomic.AtomicInteger taps=new java.util.concurrent.atomic.AtomicInteger();final Button[] target=new Button[1];
  runOnMainSync(()->{target[0]=new Button(activity);target[0].setText("Disposable control target");target[0].setOnClickListener(v->taps.incrementAndGet());android.view.ViewGroup root=activity.findViewById(android.R.id.content);((android.view.ViewGroup)root.getChildAt(0)).addView(target[0],1);});waitForIdleSync();SystemClock.sleep(600);
  JSONObject point=new JSONObject();runOnMainSync(()->{try{int[] xy=new int[2];target[0].getLocationOnScreen(xy);android.util.DisplayMetrics dm=new android.util.DisplayMetrics();activity.getWindowManager().getDefaultDisplay().getRealMetrics(dm);point.put("x",xy[0]+target[0].getWidth()/2).put("y",xy[1]+target[0].getHeight()/2).put("width",dm.widthPixels).put("height",dm.heightPixels);}catch(Exception e){throw new RuntimeException(e);}});
  try(FileOutputStream out=new FileOutputStream(new File(getTargetContext().getFilesDir(),"control-target.json"))){out.write(point.toString().getBytes("UTF-8"));}
  // Animate synthetic host target so frames after revoke prove continued capture.
  android.os.Handler h=new android.os.Handler(android.os.Looper.getMainLooper());java.util.concurrent.atomic.AtomicBoolean stopped=new java.util.concurrent.atomic.AtomicBoolean();h.post(new Runnable(){int n;public void run(){if(stopped.get())return;target[0].setText("Disposable control target "+(++n));h.postDelayed(this,500);}});
  click(R.id.start_hosting_button);projectionConsent();
  // Runner mirrors only the validated synthetic coordinate before this marker.
  new File(getTargetContext().getFilesDir(),"control-host-ready").createNewFile();
  await(()->new File(getTargetContext().getFilesDir(),"control-target-ready").exists(),"target-mirrored");
  stage="prerequisite-host-session";await(()->hostReady()&&resumedActivity==activity,"prerequisite-host-listener-resumed");
  await(()->peer().getBoolean("authorized_peer"),"prerequisite-current-authorized-peer");
  require(command("viewer").getBoolean("ok"));admissionPending();button("Allow viewing");peerState("available");await(()->peer().getInt("frames")>=2,"host-decoded-frames");
  require(!command("tap").getBoolean("ok"));require(taps.get()==0);shot("host-view-only");
  require(command("request").getBoolean("ok"));hostState("pending");JSONObject canceled=wire();require(command("cancel").getBoolean("ok"));hostState("declined");
  boolean refused=false;try{MnetVpnService.currentNode().decideSessionControl(canceled.getString("session"),canceled.getString("request"),true);}catch(Exception e){refused=true;}require(refused);
  require(command("request").getBoolean("ok"));hostState("pending");button("Remote control");shot("host-decline");button("View only");peerState("declined");require(taps.get()==0);
  require(command("request").getBoolean("ok"));hostState("pending");button("Remote control");shot("host-allow");button("Allow control");peerState("allowed");
  int before=taps.get();require(command("tap").getBoolean("ok"));await(()->taps.get()>before,"actual-accessibility-gesture");
  button("Remote control");peerState("paused");require(!command("tap").getBoolean("ok"));shot("host-paused");button("Keep control");peerState("allowed");
  button("Remote control");button("Revoke control");peerState("revoked");int after=taps.get(),frames=peer().getInt("frames");require(!command("tap").getBoolean("ok"));require(command("rawtap").getBoolean("ok"));SystemClock.sleep(800);require(taps.get()==after);await(()->peer().getInt("frames")>frames,"viewing-after-revoke");shot("host-revoked");
  require(command("request").getBoolean("ok"));hostState("pending");button("Remote control");button("Allow control");peerState("allowed");
  // Remove only the disposable emulator's optional service permission through OS.
  try(android.os.ParcelFileDescriptor fd=getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).executeShellCommand("settings put secure enabled_accessibility_services ''");java.io.InputStream in=new android.os.ParcelFileDescriptor.AutoCloseInputStream(fd)){while(in.read()!=-1){}}
  await(()->!au.fintex.mnet.MnetAccessibilityService.available(),"accessibility-removed");peerState("revoked");require(!command("tap").getBoolean("ok"));
  button("Stop hosting");await(()->peer().getBoolean("closed"),"host-stop-closes-peer");
  stopped.set(true);command("close");runOnMainSync(()->activity.finish());waitForIdleSync();
 }
 private void viewerLeg()throws Exception{
  require(command("host").getBoolean("ok"));activity=startActivitySync(new Intent(getTargetContext(),au.fintex.mnet.ViewerActivity.class).putExtra("peer_ip",options.getString("peer")).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
  viewerState("Ask the host");await(()->(int)field(activity,"frameW")>0,"viewer-decoded-frame");shot("viewer-available");screenTap();require(peer().getInt("inputs")==0);
  button("Request control");peerState("pending");viewerState("Waiting for");shot("viewer-pending");button("Cancel request");peerState("declined");viewerState("declined or canceled");
  button("Request control");peerState("pending");require(command("decline").getBoolean("ok"));viewerState("declined or canceled");shot("viewer-declined");
  button("Request control");peerState("pending");require(command("stale").getBoolean("ok"));require(peer().getBoolean("stale_refused"));require(command("allow").getBoolean("ok"));viewerState("Control allowed");shot("viewer-allowed");screenTap();await(()->peer().getInt("inputs")==1,"viewer-bound-input-received");
  require(command("pause").getBoolean("ok"));viewerState("Control paused");shot("viewer-paused");screenTap();require(peer().getInt("inputs")==1);require(command("keep").getBoolean("ok"));viewerState("Control allowed");
  require(command("revoke").getBoolean("ok"));viewerState("Control revoked");shot("viewer-revoked");screenTap();require(peer().getInt("inputs")==1);
  // Same peer reconnect creates a different host session and no carried grant.
  JSONObject old=peer().getJSONObject("control");command("close");require(command("host").getBoolean("ok"));viewerState("Ask the host");require(command("stale").getBoolean("ok"));require(peer().getBoolean("stale_refused"));require(!old.getString("session").equals(peer().getJSONObject("control").getString("session")));screenTap();require(peer().getInt("inputs")==0);
  runOnMainSync(()->activity.finish());waitForIdleSync();command("close");require(command("unsupported").getBoolean("ok"));activity=startActivitySync(new Intent(getTargetContext(),au.fintex.mnet.ViewerActivity.class).putExtra("peer_ip",options.getString("peer")).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
  await(()->(int)field(activity,"frameW")>0,"unsupported-viewing-frame");viewerState("unavailable");shot("viewer-unsupported");screenTap();require(peer().getInt("inputs")==0);
  Activity beforeFont=activity;
  // The font configuration recreates the viewer and closes its old connection.
  // Arm exactly one fresh unsupported session on the retained listener first.
  require(command("accept-next-unsupported").getBoolean("ok"));
  new File(getTargetContext().getFilesDir(),"control-font-ready").createNewFile();await(()->new File(getTargetContext().getFilesDir(),"control-font-applied").exists(),"font-scale-applied");
  await(()->fontRecovered(beforeFont),"large-font-recreated-viewing-ready");shot("viewer-large-font");screenTap();require(peer().getInt("inputs")==0);
  runOnMainSync(()->activity.finish());command("close");
 }
 @Override public void onStart(){Bundle result=new Bundle();
 android.app.Application application=(android.app.Application)getTargetContext().getApplicationContext();
 android.app.Application.ActivityLifecycleCallbacks lifecycle=new android.app.Application.ActivityLifecycleCallbacks(){
  public void onActivityResumed(Activity value){resumedActivity=value;}
  public void onActivityPaused(Activity value){if(resumedActivity==value)resumedActivity=null;}
  public void onActivityCreated(Activity a,Bundle b){}public void onActivityStarted(Activity a){}public void onActivityStopped(Activity a){}public void onActivitySaveInstanceState(Activity a,Bundle b){}public void onActivityDestroyed(Activity a){}
 };application.registerActivityLifecycleCallbacks(lifecycle);
 try{
  android.accessibilityservice.AccessibilityServiceInfo info=getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).getServiceInfo();info.flags|=android.accessibilityservice.AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS;getUiAutomation(android.app.UiAutomation.FLAG_DONT_SUPPRESS_ACCESSIBILITY_SERVICES).setServiceInfo(info);
  activity=startActivitySync(new Intent(getTargetContext(),MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));set(R.id.server_url,options.getString("server"));set(R.id.network_name,"control-acceptance");set(R.id.auth_key,options.getString("key"));set(R.id.device_name,"android-emulator");set(R.id.exit_node,"");click(R.id.connect_button);vpnDisclosure(true,false);vpnConsent();await(this::up,"prerequisite-vpn-up");osVpnReady();
  hostLeg();viewerLeg();stage="complete";result.putString("checks",passedStages.toString());result.putString("acceptance","passed");finish(Activity.RESULT_OK,result);
 }catch(Throwable e){result.putString("checks",passedStages.toString());result.putString("acceptance","failed");result.putString("stage",stage);finish(Activity.RESULT_CANCELED,result);}finally{application.unregisterActivityLifecycleCallbacks(lifecycle);}}
}
