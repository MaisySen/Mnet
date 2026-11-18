package au.fintex.mnet.baseline;
import android.app.*;
import android.content.*;
import android.net.*;
import android.os.*;
import android.view.accessibility.*;
import java.io.*;
import java.util.*;
import org.json.*;

public final class OwnerInstrumentation extends Instrumentation {
 private interface Check{boolean ready()throws Exception;}
 private void await(Check check)throws Exception{long end=SystemClock.elapsedRealtime()+60000;while(SystemClock.elapsedRealtime()<end){if(check.ready())return;SystemClock.sleep(150);}throw new IOException("bounded baseline condition");}
 private File file(String name){return new File(getTargetContext().getFilesDir(),name);}
 private void save(JSONArray rows)throws Exception{try(FileOutputStream f=new FileOutputStream(file("baseline-result.json"))){f.write(rows.toString().getBytes("UTF-8"));}}
 @Override public void onCreate(Bundle args){super.onCreate(args);start();}
 @Override public void onStart(){
  JSONArray rows=new JSONArray();Bundle result=new Bundle();
  try{
   Activity activity=startActivitySync(new Intent(getTargetContext(),ConsentActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
   await(()->{
    if(DirectVpn.current!=null)return true;
    AccessibilityNodeInfo root=getUiAutomation().getRootInActiveWindow();
    if(root!=null&&"com.android.vpndialogs".contentEquals(root.getPackageName()))for(AccessibilityNodeInfo b:root.findAccessibilityNodeInfosByViewId("android:id/button1"))if("OK".equals(String.valueOf(b.getText())))b.performAction(AccessibilityNodeInfo.ACTION_CLICK);
    return false;
   });
   for(int phase=0;phase<2;phase++){
    if(phase==1){getTargetContext().stopService(new Intent(getTargetContext(),DirectVpn.class));await(()->DirectVpn.current==null&&!DirectVpn.workerAlive);getTargetContext().startForegroundService(new Intent(getTargetContext(),DirectVpn.class));}
    await(()->{
     if(DirectVpn.current==null)return false;
     ConnectivityManager cm=getTargetContext().getSystemService(ConnectivityManager.class);Network n=cm.getActiveNetwork();NetworkCapabilities c=cm.getNetworkCapabilities(n);LinkProperties p=cm.getLinkProperties(n);
     if(c==null||p==null||!c.hasTransport(NetworkCapabilities.TRANSPORT_VPN))return false;
     for(LinkAddress address:p.getLinkAddresses())if("100.64.0.1".equals(address.getAddress().getHostAddress()))return true;return false;
    });
    JSONObject row=new JSONObject();row.put("phase",phase==0?"initial":"reconnect");row.put("role","owner");
    JSONObject original=Probe.udp(getTargetContext(),true);row.put("original_udp",original);
    if(!original.optBoolean("passed"))row.put("fresh_connected_contrast",Probe.udp(getTargetContext(),true));
    row.put("os_dns",Probe.system(getTargetContext(),true));row.put("fixture_queries_seen",DirectVpn.fixtureQueries);row.put("fixture_replies_written",DirectVpn.fixtureReplies);rows.put(row);save(rows);
    file("ready-"+phase).createNewFile();final int currentPhase=phase;await(()->file("done-"+currentPhase).exists());
   }
   getTargetContext().stopService(new Intent(getTargetContext(),DirectVpn.class));await(()->DirectVpn.current==null&&!DirectVpn.workerAlive);
   runOnMainSync(()->activity.finish());result.putString("summary","COMPLETE minimal baseline only; original Mnet readiness unchanged");finish(Activity.RESULT_OK,result);
  }catch(Throwable error){try{save(rows);}catch(Exception ignored){}result.putString("summary","INCONCLUSIVE baseline orchestration "+error.getClass().getSimpleName());finish(Activity.RESULT_CANCELED,result);}
 }
}
