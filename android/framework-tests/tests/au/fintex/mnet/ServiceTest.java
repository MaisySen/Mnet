package au.fintex.mnet;
import android.content.*;import android.app.*;import org.junit.*;import org.junit.runner.RunWith;import org.robolectric.*;import org.robolectric.android.controller.ServiceController;import org.robolectric.annotation.Config;import static org.junit.Assert.*;import static org.robolectric.Shadows.shadowOf;
@RunWith(RobolectricTestRunner.class) @Config(manifest=Config.NONE,sdk={24,25,26,33,34})
public class ServiceTest{
 @Test public void notificationAndPrivateReceiver(){
  android.app.Application app=RuntimeEnvironment.getApplication();
  Notification n=VpnPlatform.notification(app,"mesh","Mnet").setSmallIcon(android.R.drawable.ic_dialog_info).setContentTitle("fixture").build();assertNotNull(n);
  if(android.os.Build.VERSION.SDK_INT>=26)assertNotNull(app.getSystemService(NotificationManager.class).getNotificationChannel("mesh"));
  BroadcastReceiver r=new BroadcastReceiver(){public void onReceive(Context c,Intent i){}};
  VpnPlatform.registerPrivateReceiver(app,r,new IntentFilter("fixture"));app.unregisterReceiver(r);
 }
 @Test public void missingConsentNeverStartsHost(){
  ServiceController<HostSessionService> c=Robolectric.buildService(HostSessionService.class).create();HostSessionService s=c.get();
  MnetVpnService.node=new mobile.Node();
  assertEquals(Service.START_NOT_STICKY,s.onStartCommand(new Intent().putExtra("resultCode",0),0,1));
  assertTrue(shadowOf(s).isStoppedBySelf());assertFalse(MnetAccessibilityService.hosting);c.destroy();MnetVpnService.node=null;
 }
 @Test public void stopCancelsActualServiceConsentWait() throws Exception {
  ServiceController<HostSessionService> c=Robolectric.buildService(HostSessionService.class).create();HostSessionService s=c.get();
  java.lang.reflect.Field running=HostSessionService.class.getDeclaredField("running");running.setAccessible(true);running.set(s,true);
  java.util.concurrent.ExecutorService worker=java.util.concurrent.Executors.newSingleThreadExecutor();
  try {
   java.util.concurrent.Future<Boolean> answer=worker.submit(()->s.acceptSession("fixture-peer"));
   long deadline=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);boolean asked=false;
   while(System.nanoTime()<deadline&&!asked){
    for(Intent intent:shadowOf(RuntimeEnvironment.getApplication()).getBroadcastIntents()) if(HostSessionService.ACTION_ASK.equals(intent.getAction())) asked=true;
    if(!asked)Thread.sleep(1);
   }
   assertTrue("service never asked for consent",asked);
   s.onStartCommand(new Intent().setAction("stop"),0,1);
   assertFalse(answer.get(1,java.util.concurrent.TimeUnit.SECONDS));
   assertFalse(MnetAccessibilityService.hosting);
  } finally {worker.shutdownNow();c.destroy();}
 }
 @Test public void stopAndProcessRestartStayNonSticky(){
  ServiceController<HostSessionService> c=Robolectric.buildService(HostSessionService.class).create();HostSessionService s=c.get();
  assertEquals(Service.START_NOT_STICKY,s.onStartCommand(null,0,1));
  assertEquals(Service.START_NOT_STICKY,s.onStartCommand(new Intent().setAction("stop"),0,2));
  assertTrue(shadowOf(s).isStoppedBySelf());assertFalse(s.acceptSession("unapproved"));c.destroy();
 }
}
