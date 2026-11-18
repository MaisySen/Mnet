package au.fintex.mnet;
import android.content.*;import android.app.*;import org.junit.*;import org.junit.runner.RunWith;import org.robolectric.*;import org.robolectric.android.controller.ServiceController;import org.robolectric.annotation.Config;import static org.junit.Assert.*;import static org.robolectric.Shadows.shadowOf;
@RunWith(RobolectricTestRunner.class) @Config(manifest=Config.NONE,sdk={24,25,26,33,34})
public class ServiceTest{
 @Test public void captureInputNeedsInitializedFrame() throws Exception {
  ServiceController<HostSessionService> controller=Robolectric.buildService(HostSessionService.class).create();
  MnetAccessibilityService sink=new MnetAccessibilityService();MnetAccessibilityService.current=sink;
  try {
   setField(controller.get(),"running",true);
   controller.get().inject("{\"kind\":\"pointer\",\"action\":\"click\",\"x\":0,\"y\":0}");
   assertEquals("no injection before capture dimensions",0,sink.injections);
  } finally {controller.destroy();MnetAccessibilityService.current=null;}
 }
 @Test public void changedDisplayGeometryCannotInjectOldFrameCoordinates() throws Exception {
  ServiceController<HostSessionService> controller=Robolectric.buildService(HostSessionService.class).create();
  MnetAccessibilityService sink=new MnetAccessibilityService();MnetAccessibilityService.current=sink;
  try {
   HostSessionService service=controller.get();
   android.util.DisplayMetrics metrics=new android.util.DisplayMetrics();
   ((android.view.WindowManager)service.getSystemService(Context.WINDOW_SERVICE)).getDefaultDisplay().getRealMetrics(metrics);
   setField(service,"running",true);setField(service,"frameW",metrics.widthPixels);setField(service,"frameH",metrics.heightPixels);
   setField(service,"captureDisplayW",metrics.widthPixels);setField(service,"captureDisplayH",metrics.heightPixels);
   setField(service,"captureRotation",((android.view.WindowManager)service.getSystemService(Context.WINDOW_SERVICE)).getDefaultDisplay().getRotation());
   approveControl(service);
   service.inject("{\"kind\":\"pointer\",\"action\":\"click\"}");assertEquals(1,sink.injections);
   setField(service,"captureDisplayW",metrics.widthPixels+1);
   service.inject("{\"kind\":\"pointer\",\"action\":\"click\"}");assertEquals("old frame after display-width change",1,sink.injections);
   setField(service,"captureDisplayW",metrics.widthPixels);
   service.inject("{\"kind\":\"pointer\",\"action\":\"click\"}");assertEquals("geometry refusal stays latched until restart",1,sink.injections);
   setField(service,"inputGeometryInvalid",false);setField(service,"captureDisplayH",metrics.heightPixels+1);
   service.inject("{\"kind\":\"pointer\",\"action\":\"click\"}");assertEquals("old frame after display-height change",1,sink.injections);
   setField(service,"inputGeometryInvalid",false);setField(service,"captureDisplayH",metrics.heightPixels);
   setField(service,"captureRotation",-1);service.inject("{\"kind\":\"pointer\",\"action\":\"click\"}");assertEquals("old frame after rotation",1,sink.injections);
  } finally {controller.destroy();MnetAccessibilityService.current=null;}
 }
 @Test public void hostRefusesMalformedUnsupportedAndOutsidePointerInput() throws Exception {
  ServiceController<HostSessionService> controller=Robolectric.buildService(HostSessionService.class).create();
  MnetAccessibilityService sink=new MnetAccessibilityService();MnetAccessibilityService.current=sink;
  try {
   HostSessionService service=controller.get();android.util.DisplayMetrics metrics=new android.util.DisplayMetrics();
   android.view.Display display=((android.view.WindowManager)service.getSystemService(Context.WINDOW_SERVICE)).getDefaultDisplay();display.getRealMetrics(metrics);
   setField(service,"running",true);setField(service,"frameW",metrics.widthPixels);setField(service,"frameH",metrics.heightPixels);
   setField(service,"captureDisplayW",metrics.widthPixels);setField(service,"captureDisplayH",metrics.heightPixels);setField(service,"captureRotation",display.getRotation());
   approveControl(service);
   String valid="{\"kind\":\"pointer\",\"action\":\"click\"}";
   service.inject(valid);assertEquals("Go omits zero coordinates",1,sink.injections);
   for(String invalid:new String[]{null,"bad","{}",valid.replace("click","down"),valid.replace("click","up"),valid.replace("click","move"),valid.replace("pointer","key"),
    valid.replace("}",",\"x\":-1}"),valid.replace("}",",\"x\":"+metrics.widthPixels+"}"),valid.replace("}",",\"y\":"+metrics.heightPixels+"}"),
    valid.replace("}",",\"x\":0.5}"),valid.replace("}",",\"x\":\"1\"}"),valid.replace("}",",\"x\":null}")}){
    service.inject(invalid);assertEquals("invalid input forwarded: "+invalid,1,sink.injections);
   }
   service.inject(valid.replace("click","scroll"));assertEquals(2,sink.injections);
  } finally {controller.destroy();MnetAccessibilityService.current=null;}
 }
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
 private static void approveControl(HostSessionService service)throws Exception{
  setField(service,"autoAccept",true);assertTrue(service.acceptSession("fixture-peer"));
  assertFalse(HostSessionService.controlAllowed());
  assertTrue(HostSessionService.answerControl(HostSessionService.controlSession(),true));
 }
 @Test public void viewOnlyControlRevokeAndSessionEndEnforceInput()throws Exception{
  ServiceController<HostSessionService> controller=Robolectric.buildService(HostSessionService.class).create();
  MnetAccessibilityService sink=new MnetAccessibilityService();MnetAccessibilityService.current=sink;
  try{
   HostSessionService service=controller.get();android.util.DisplayMetrics metrics=new android.util.DisplayMetrics();
   android.view.Display display=((android.view.WindowManager)service.getSystemService(Context.WINDOW_SERVICE)).getDefaultDisplay();display.getRealMetrics(metrics);
   setField(service,"running",true);setField(service,"autoAccept",true);setField(service,"frameW",metrics.widthPixels);setField(service,"frameH",metrics.heightPixels);
   setField(service,"captureDisplayW",metrics.widthPixels);setField(service,"captureDisplayH",metrics.heightPixels);setField(service,"captureRotation",display.getRotation());
   assertTrue(service.acceptSession("viewer"));String first=HostSessionService.controlSession();assertNotNull(first);
   String input="{\"kind\":\"pointer\",\"action\":\"click\"}";
   service.inject(input);assertEquals(0,sink.injections);assertFalse(MnetAccessibilityService.hosting);
   assertTrue(HostSessionService.answerControl(first,true));service.inject(input);assertEquals(1,sink.injections);
   assertFalse(HostSessionService.answerControl(first,false));
   assertTrue(HostSessionService.answerControl(HostSessionService.controlSession(),false));service.inject(input);assertEquals(1,sink.injections);
   assertTrue(service.acceptSession("next"));assertNotEquals(first,HostSessionService.controlSession());
   assertFalse(HostSessionService.answerControl(first,true));service.inject(input);assertEquals(1,sink.injections);
   String next=HostSessionService.controlSession();assertTrue(HostSessionService.answerControl(next,true));
   service.onSessionEnd();assertFalse(HostSessionService.answerControl(next,true));service.inject(input);assertEquals(1,sink.injections);
   assertNull(HostSessionService.controlSession());assertFalse(MnetAccessibilityService.hosting);
  }finally{controller.destroy();MnetAccessibilityService.current=null;}
 }
 private static void setField(HostSessionService service,String field,Object value) throws Exception {
  java.lang.reflect.Field f=HostSessionService.class.getDeclaredField(field);f.setAccessible(true);f.set(service,value);
 }
 private static String awaitRequest(int earlier) throws Exception {
  long deadline=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);
  while(System.nanoTime()<deadline){
   java.util.List<Intent> events=shadowOf(RuntimeEnvironment.getApplication()).getBroadcastIntents();
   for(int i=earlier;i<events.size();i++)if(HostSessionService.ACTION_ASK.equals(events.get(i).getAction()))return events.get(i).getStringExtra(HostSessionService.EXTRA_REQUEST);
   Thread.sleep(1);
  }
  throw new AssertionError("consent request not broadcast");
 }
 @Test public void exactServiceConsentCannotReuseOldDialog() throws Exception {
  ServiceController<HostSessionService> c=Robolectric.buildService(HostSessionService.class).create();HostSessionService service=c.get();
  setField(service,"running",true);
  java.util.concurrent.ExecutorService worker=java.util.concurrent.Executors.newSingleThreadExecutor();
  try {
   int before=shadowOf(RuntimeEnvironment.getApplication()).getBroadcastIntents().size();
   java.util.concurrent.Future<Boolean> first=worker.submit(()->service.acceptSession("first"));
   String firstId=awaitRequest(before);assertTrue(HostSessionService.consentPending(firstId));
   HostSessionService.answerSession(firstId,false);assertFalse(first.get(1,java.util.concurrent.TimeUnit.SECONDS));
   before=shadowOf(RuntimeEnvironment.getApplication()).getBroadcastIntents().size();
   java.util.concurrent.Future<Boolean> second=worker.submit(()->service.acceptSession("second"));
   String secondId=awaitRequest(before);assertNotEquals(firstId,secondId);
   HostSessionService.answerSession(firstId,true);assertTrue(HostSessionService.consentPending(secondId));assertFalse(second.isDone());
   HostSessionService.answerSession(secondId,true);assertTrue(second.get(1,java.util.concurrent.TimeUnit.SECONDS));
  }finally{worker.shutdownNow();c.destroy();}
 }
 @Test public void autoAcceptIsInstanceLocalAndStopClearsIt() throws Exception {
  ServiceController<HostSessionService> old=Robolectric.buildService(HostSessionService.class).create();
  ServiceController<HostSessionService> fresh=Robolectric.buildService(HostSessionService.class).create();
  try {
   setField(old.get(),"running",true);setField(old.get(),"autoAccept",true);
   assertTrue(old.get().acceptSession("auto"));
   java.lang.reflect.Field option=HostSessionService.class.getDeclaredField("autoAccept");option.setAccessible(true);
   assertFalse(option.getBoolean(fresh.get()));
   old.get().onStartCommand(new Intent().setAction("stop"),0,1);
   assertFalse(option.getBoolean(old.get()));assertFalse(old.get().acceptSession("stopped"));
  }finally{old.destroy();fresh.destroy();}
 }
 @Test public void captureEncoderAcceptsPaddedAndExactRows() {
  for(int padding:new int[]{0,8}){
   int width=16,height=8,stride=width*4+padding;
   java.nio.ByteBuffer pixels=java.nio.ByteBuffer.allocate(stride*height);
   while(pixels.hasRemaining())pixels.put((byte)0x7f);pixels.rewind();
   byte[] encoded=CaptureEncoder.jpeg(pixels,width,height,4,stride);
   assertTrue(encoded.length>0);
   android.graphics.Bitmap decoded=android.graphics.BitmapFactory.decodeByteArray(encoded,0,encoded.length);
   assertNotNull(decoded);assertEquals(width,decoded.getWidth());assertEquals(height,decoded.getHeight());decoded.recycle();
  }
 }
 @Test public void captureLastRowNeedsNoTrailingPadding() {
  int width=16,height=8,stride=72;
  java.nio.ByteBuffer pixels=java.nio.ByteBuffer.allocateDirect(stride*(height-1)+width*4);
  while(pixels.hasRemaining())pixels.put((byte)0x7f);pixels.rewind();
  int position=pixels.position(),limit=pixels.limit();
  byte[] encoded=CaptureEncoder.jpeg(pixels,width,height,4,stride);
  assertTrue(encoded.length>0);assertEquals(position,pixels.position());assertEquals(limit,pixels.limit());
  java.nio.ByteBuffer offset=java.nio.ByteBuffer.allocateDirect(7+stride*(height-1)+width*4);
  offset.position(7);
  assertTrue(CaptureEncoder.jpeg(offset,width,height,4,stride).length>0);assertEquals(7,offset.position());
 }
 @Test public void captureEncoderRefusesInvalidLayouts() {
  for(int[] layout:new int[][]{{0,8,4,64},{16,8,2,64},{16,8,4,60},{16,8,4,65}}){
   try{CaptureEncoder.jpeg(java.nio.ByteBuffer.allocate(1024),layout[0],layout[1],layout[2],layout[3]);fail("invalid layout accepted");}catch(IllegalArgumentException expected){}
  }
  try{CaptureEncoder.jpeg(java.nio.ByteBuffer.allocate(4),16,8,4,64);fail("short frame accepted");}catch(RuntimeException expected){}
 }
 @Test public void stopAndProcessRestartStayNonSticky(){
  ServiceController<HostSessionService> c=Robolectric.buildService(HostSessionService.class).create();HostSessionService s=c.get();
  assertEquals(Service.START_NOT_STICKY,s.onStartCommand(null,0,1));
  assertEquals(Service.START_NOT_STICKY,s.onStartCommand(new Intent().setAction("stop"),0,2));
  assertTrue(shadowOf(s).isStoppedBySelf());assertFalse(s.acceptSession("unapproved"));c.destroy();
 }
}
