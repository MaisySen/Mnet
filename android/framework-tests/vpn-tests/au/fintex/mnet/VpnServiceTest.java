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
@Config(manifest=Config.NONE,sdk={24,25,26,29,33,34},shadows={VpnServiceTest.Mode.class,VpnServiceTest.FakeBuilder.class,VpnServiceTest.FakeDescriptor.class})
public class VpnServiceTest {
 @Implements(VpnService.class) public static class Mode extends ShadowVpnService {
  static boolean managed;
  @Implementation(minSdk=29) protected boolean isAlwaysOn(){return managed;}
 }
 @Implements(VpnService.Builder.class) public static class FakeBuilder {
  @RealObject VpnService.Builder real;
  static final java.util.List<String> addresses = new java.util.concurrent.CopyOnWriteArrayList<>();
  static final java.util.List<String> routes = new java.util.concurrent.CopyOnWriteArrayList<>();
  static final java.util.List<String> resolvers = new java.util.concurrent.CopyOnWriteArrayList<>();
  @Implementation protected VpnService.Builder addAddress(String address,int prefix){addresses.add(address+"/"+prefix);return real;}
  @Implementation protected VpnService.Builder addRoute(String address,int prefix){routes.add(address+"/"+prefix);return real;}
  @Implementation protected VpnService.Builder addDnsServer(String address){resolvers.add(address);return real;}
  @Implementation protected android.os.ParcelFileDescriptor establish()throws java.io.IOException{
   return new android.os.ParcelFileDescriptor(new java.io.FileDescriptor());
  }
 }
 @Implements(android.os.ParcelFileDescriptor.class) public static class FakeDescriptor {
  @Implementation protected int detachFd(){return 4242;}
  @Implementation protected void close(){}
 }
 private MnetVpnService service;
 @Before public void create(){FakeBuilder.addresses.clear();FakeBuilder.routes.clear();FakeBuilder.resolvers.clear();Mode.managed=false;service=Robolectric.buildService(MnetVpnService.class).create().get();}
 @After public void destroy()throws Exception{service.onDestroy();awaitWorker();mobile.Mobile.factory=mobile.Node::new;Mode.managed=false;}
 private Thread worker()throws Exception{java.lang.reflect.Field f=MnetVpnService.class.getDeclaredField("worker");f.setAccessible(true);return(Thread)f.get(service);}
 private void awaitWorker()throws Exception{Thread w=worker();if(w!=null){long end=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(5);while(w.isAlive()&&System.nanoTime()<end){Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();w.join(5);}assertFalse("worker must exit",w.isAlive());}}
 private void configure(){service.getSharedPreferences("mnet",0).edit().putBoolean(MnetVpnService.VPN_DISCLOSURE_KEY,true).putString("server","https://test.invalid").putString("network","test").apply();}
 private boolean running()throws Exception{java.lang.reflect.Field f=MnetVpnService.class.getDeclaredField("running");f.setAccessible(true);return f.getBoolean(service);}
 private java.util.List<Intent> broadcasts(){return Shadows.shadowOf((Application)service.getApplicationContext()).getBroadcastIntents();}
 private void errorContains(String text){assertTrue(broadcasts().stream().anyMatch(i->i.getStringExtra(MnetVpnService.EXTRA_STATUS)!=null&&i.getStringExtra(MnetVpnService.EXTRA_STATUS).contains(text)));}
 private void node(mobile.Node value)throws Exception{
  for(String name:new String[]{"node","sharedNode"}){java.lang.reflect.Field f=MnetVpnService.class.getDeclaredField(name);f.setAccessible(true);f.set(service,value);}
 }
 private Notification notification()throws Exception{java.lang.reflect.Method m=MnetVpnService.class.getDeclaredMethod("buildNotification",String.class);m.setAccessible(true);return(Notification)m.invoke(service,"test");}

 @Test public void localResolverHasExplicitRouteAndIsNotInterfaceAddress()throws Exception{
  configure();mobile.Mobile.factory=()->new mobile.Node(){@Override public void start(long fd,String a,String b,String c,String d){throw new IllegalStateException("fixture after builder");}};
  service.onStartCommand(new Intent(),0,1);awaitWorker();
  assertTrue(FakeBuilder.routes.contains("100.64.0.0/10"));assertTrue(FakeBuilder.routes.contains("198.18.0.53/32"));
  assertEquals(java.util.Collections.singletonList("198.18.0.53"),FakeBuilder.resolvers);
  assertTrue(FakeBuilder.addresses.contains("198.18.0.54/32"));assertFalse(FakeBuilder.addresses.contains("198.18.0.53/32"));
 }
 @Test public void savedConfigCannotBypassProminentDisclosure()throws Exception{
  configure();service.getSharedPreferences("mnet",0).edit().remove(MnetVpnService.VPN_DISCLOSURE_KEY).apply();
  java.util.concurrent.atomic.AtomicInteger created=new java.util.concurrent.atomic.AtomicInteger();mobile.Mobile.factory=()->{created.incrementAndGet();return new mobile.Node();};
  assertEquals(Service.START_STICKY,service.onStartCommand(null,0,1));awaitWorker();
  assertEquals(0,created.get());assertFalse(running());assertNull(MnetVpnService.currentNode());errorContains("accept the VPN data disclosure");
 }
 @Test public void missingSavedConfigFailsBeforeNodeCreationAndCanRetry()throws Exception{
  java.util.concurrent.atomic.AtomicInteger created=new java.util.concurrent.atomic.AtomicInteger();mobile.Mobile.factory=()->{created.incrementAndGet();return new mobile.Node();};
  service.getSharedPreferences("mnet",0).edit().clear().apply();
  assertEquals(Service.START_STICKY,service.onStartCommand(null,0,1));awaitWorker();assertEquals(0,created.get());assertFalse(running());assertNull(MnetVpnService.currentNode());errorContains("configure a control server");
  configure();mobile.Node node=new mobile.Node(){@Override public String prepare(String a,String b,String c,String d){throw new IllegalStateException("retry preparation failure");}};
  mobile.Mobile.factory=()->{created.incrementAndGet();return node;};service.onStartCommand(null,0,2);awaitWorker();assertEquals(1,created.get());assertTrue(node.closed);assertFalse(running());assertNull(MnetVpnService.currentNode());errorContains("retry preparation failure");
 }
 @Test public void canceledBlockedWorkerHoldsOwnershipUntilItExits()throws Exception{
  configure();java.util.concurrent.CountDownLatch entered=new java.util.concurrent.CountDownLatch(1),release=new java.util.concurrent.CountDownLatch(1);
  java.util.concurrent.atomic.AtomicInteger created=new java.util.concurrent.atomic.AtomicInteger();
  mobile.Node node=new mobile.Node(){@Override public String prepare(String a,String b,String c,String d){entered.countDown();try{if(!release.await(5,java.util.concurrent.TimeUnit.SECONDS))throw new AssertionError("release timed out");}catch(InterruptedException e){throw new AssertionError(e);}return "100.64.0.2";}@Override public void start(long fd,String a,String b,String c,String d){throw new AssertionError("canceled worker must not start tunnel");}};
  mobile.Mobile.factory=()->{created.incrementAndGet();return node;};
  try{
   service.onStartCommand(new Intent(),0,1);assertTrue(entered.await(5,java.util.concurrent.TimeUnit.SECONDS));
   // Duplicate active start must not create a competing worker.
   service.onStartCommand(new Intent(),0,2);assertEquals(1,created.get());
   assertEquals(Service.START_NOT_STICKY,service.onStartCommand(new Intent().setAction("disconnect"),0,3));assertTrue(node.closed);assertNull(MnetVpnService.currentNode());
   assertEquals(Service.START_NOT_STICKY,service.onStartCommand(new Intent(),0,4));assertEquals(1,created.get());errorContains("Previous VPN worker is still stopping");
  }finally{release.countDown();awaitWorker();}
  assertFalse(running());assertNull(MnetVpnService.currentNode());
  mobile.Node next=new mobile.Node(){@Override public String prepare(String a,String b,String c,String d){throw new IllegalStateException("next worker accepted");}};
  mobile.Mobile.factory=()->{created.incrementAndGet();return next;};assertEquals(Service.START_STICKY,service.onStartCommand(null,0,5));awaitWorker();assertEquals(2,created.get());assertTrue(next.closed);errorContains("next worker accepted");
 }
 @Test public void systemRevokeCancelsBlockedPreparation()throws Exception{cancelBlockedPreparation(true);}
 @Test public void destructionCancelsBlockedPreparation()throws Exception{cancelBlockedPreparation(false);}
 private void cancelBlockedPreparation(boolean revoke)throws Exception{
  configure();Mode.managed=true;
  java.util.concurrent.CountDownLatch entered=new java.util.concurrent.CountDownLatch(1),release=new java.util.concurrent.CountDownLatch(1);
  java.util.concurrent.atomic.AtomicInteger starts=new java.util.concurrent.atomic.AtomicInteger(),creates=new java.util.concurrent.atomic.AtomicInteger();
  mobile.Node blocked=new mobile.Node(){@Override public String prepare(String a,String b,String c,String d){entered.countDown();try{if(!release.await(5,java.util.concurrent.TimeUnit.SECONDS))throw new AssertionError("release timed out");}catch(InterruptedException e){throw new AssertionError(e);}return "100.64.0.2";}@Override public void start(long fd,String a,String b,String c,String d){starts.incrementAndGet();}};
  mobile.Mobile.factory=()->{creates.incrementAndGet();return blocked;};
  MnetVpnService replacement=Robolectric.buildService(MnetVpnService.class).create().get();
  try{
   service.onStartCommand(null,0,1);assertTrue(entered.await(5,java.util.concurrent.TimeUnit.SECONDS));assertSame(blocked,MnetVpnService.currentNode());
   if(revoke)service.onRevoke();else service.onDestroy();
   assertTrue(blocked.closed);assertNull(MnetVpnService.currentNode());assertFalse(running());assertTrue(Shadows.shadowOf(service).isStoppedBySelf());
   // The gate belongs to the process, not only the stopped service instance.
   assertEquals(Service.START_NOT_STICKY,replacement.onStartCommand(null,0,1));assertEquals(1,creates.get());
   release.countDown();awaitWorker();assertEquals(0,starts.get());
   mobile.Node next=new mobile.Node(){@Override public String prepare(String a,String b,String c,String d){throw new IllegalStateException("replacement accepted after exit");}};
   mobile.Mobile.factory=()->{creates.incrementAndGet();return next;};
   assertEquals(Service.START_STICKY,replacement.onStartCommand(null,0,2));
   MnetVpnService original=service;service=replacement;try{awaitWorker();}finally{service=original;}
   assertEquals(2,creates.get());assertTrue(next.closed);assertNull(MnetVpnService.currentNode());errorContains("replacement accepted after exit");
  }finally{release.countDown();awaitWorker();replacement.onDestroy();}
 }
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
 @Test public void upRequiresExplicitDNSListenerReadiness()throws Exception{
  java.lang.reflect.Method m=MnetVpnService.class.getDeclaredMethod("checkedConnectionStatus",String.class);m.setAccessible(true);
  for(String json:new String[]{"{\"status\":\"up\"}","{\"status\":\"up\",\"dns_ready\":false}"}){
   try{m.invoke(null,json);fail("unavailable resolver must reject up");}catch(java.lang.reflect.InvocationTargetException e){assertTrue(e.getCause() instanceof IllegalStateException);assertTrue(e.getCause().getMessage().contains("Private DNS"));}
  }
  String ready="{\"status\":\"up\",\"dns_ready\":true}";assertEquals(ready,m.invoke(null,ready));
  String joining="{\"status\":\"joining\",\"dns_ready\":false}";assertEquals(joining,m.invoke(null,joining));
 }
 @Test public void unavailableDNSStopsActualWorkerBeforeUpBroadcast()throws Exception{
  configure();java.util.concurrent.atomic.AtomicInteger starts=new java.util.concurrent.atomic.AtomicInteger();
  mobile.Node missingDNS=new mobile.Node(){
   @Override public void start(long fd,String a,String b,String c,String d){assertEquals(4242,fd);starts.incrementAndGet();}
   @Override public String status(){return "{\"status\":\"up\",\"dns_ready\":false}";}
  };
  mobile.Mobile.factory=()->missingDNS;
  service.onStartCommand(null,0,1);awaitWorker();
  assertEquals(broadcasts().stream().map(i->String.valueOf(i.getStringExtra(MnetVpnService.EXTRA_STATUS))).collect(java.util.stream.Collectors.joining(";")),1,starts.get());assertTrue(missingDNS.closed);assertFalse(running());assertNull(MnetVpnService.currentNode());assertTrue(Shadows.shadowOf(service).isStoppedBySelf());
  errorContains("Private DNS is unavailable");
  assertFalse(broadcasts().stream().anyMatch(i->{String j=i.getStringExtra(MnetVpnService.EXTRA_STATUS);return j!=null&&j.contains("\"status\":\"up\"");}));
 }
 @Test public void managedStaleActionCannotAdvertiseUnavailableDNS()throws Exception{
  Mode.managed=true;mobile.Node failed=new mobile.Node(){@Override public String status(){return "{\"status\":\"up\",\"dns_ready\":false}";}};node(failed);
  service.onStartCommand(new Intent().setAction("disconnect"),0,1);
  assertTrue(failed.closed);assertNull(MnetVpnService.currentNode());
  if(android.os.Build.VERSION.SDK_INT>=29)errorContains("Private DNS is unavailable");
 }
}
