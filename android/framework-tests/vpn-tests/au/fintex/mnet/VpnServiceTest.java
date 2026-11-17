package au.fintex.mnet;
import android.app.*;
import android.content.*;
import android.net.VpnService;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.annotation.*;
import org.robolectric.shadows.ShadowVpnService;
import static org.junit.Assert.*;
@RunWith(RobolectricTestRunner.class)
@Config(manifest=Config.NONE,sdk={24,25,26,29,33,34},shadows=VpnServiceTest.Mode.class)
public class VpnServiceTest {
 @Implements(VpnService.class) public static class Mode extends ShadowVpnService {
  static boolean managed;
  @Implementation(minSdk=29) protected boolean isAlwaysOn(){return managed;}
 }
 private MnetVpnService service;
 @Before public void create(){Mode.managed=false;service=Robolectric.buildService(MnetVpnService.class).create().get();}
 @After public void destroy(){service.onDestroy();Mode.managed=false;}
 private void node(mobile.Node value)throws Exception{
  for(String name:new String[]{"node","sharedNode"}){java.lang.reflect.Field f=MnetVpnService.class.getDeclaredField(name);f.setAccessible(true);f.set(service,value);}
 }
 private Notification notification()throws Exception{java.lang.reflect.Method m=MnetVpnService.class.getDeclaredMethod("buildNotification",String.class);m.setAccessible(true);return(Notification)m.invoke(service,"test");}
 @Test public void manualNotificationHasDisconnectAndActionStops()throws Exception{
  Notification n=notification();assertNotNull(n.actions);assertEquals(1,n.actions.length);
  Intent action=Shadows.shadowOf(n.actions[0].actionIntent).getSavedIntent();assertEquals("disconnect",action.getAction());
  mobile.Node node=new mobile.Node();node(node);
  assertEquals(Service.START_NOT_STICKY,service.onStartCommand(action,0,1));assertTrue(node.closed);assertNull(MnetVpnService.currentNode());assertTrue(Shadows.shadowOf(service).isStoppedBySelf());
 }
 @Test public void managedNotificationAndStaleActionFollowApiGuard()throws Exception{
  Mode.managed=true;boolean supported=android.os.Build.VERSION.SDK_INT>=29;
  Notification n=notification();assertEquals(supported?0:1,n.actions==null?0:n.actions.length);
  mobile.Node node=new mobile.Node();node(node);
  assertEquals(supported?Service.START_STICKY:Service.START_NOT_STICKY,service.onStartCommand(new Intent().setAction("disconnect"),0,1));
  assertEquals(!supported,node.closed);assertEquals(!supported,Shadows.shadowOf(service).isStoppedBySelf());
  if(supported){assertSame(node,MnetVpnService.currentNode());assertEquals(1,node.statusReads);Intent b=Shadows.shadowOf((Application)service.getApplicationContext()).getBroadcastIntents().get(0);assertTrue(b.getBooleanExtra(MnetVpnService.EXTRA_ALWAYS_ON,false));}
 }
 @Test public void managedStaleActionWithoutNodeDoesNotCrash()throws Exception{
  Mode.managed=true;assertEquals(android.os.Build.VERSION.SDK_INT>=29?Service.START_STICKY:Service.START_NOT_STICKY,service.onStartCommand(new Intent().setAction("disconnect"),0,1));assertNull(MnetVpnService.currentNode());
 }
 @Test public void managedSystemRevokeStillCleansUp()throws Exception{
  Mode.managed=true;mobile.Node node=new mobile.Node();node(node);service.onRevoke();assertTrue(node.closed);assertNull(MnetVpnService.currentNode());assertTrue(Shadows.shadowOf(service).isStoppedBySelf());
 }
}
