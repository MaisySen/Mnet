package au.fintex.mnet;
import android.content.Intent;
import android.view.*;
import android.widget.ImageView;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.android.controller.ActivityController;
import org.robolectric.annotation.Config;
import static org.junit.Assert.*;

@RunWith(RobolectricTestRunner.class) @Config(manifest=Config.NONE,sdk={24,25,26,33})
public class ViewerActivityTest {
 private static int activityFrameWidth(ViewerActivity activity)throws Exception{
  java.lang.reflect.Field field=ViewerActivity.class.getDeclaredField("frameW");field.setAccessible(true);return field.getInt(activity);
 }
 private static void frame(mobile.ViewerCallbacks callbacks,int width,int height){
  android.graphics.Bitmap bitmap=android.graphics.Bitmap.createBitmap(width,height,android.graphics.Bitmap.Config.ARGB_8888);
  java.io.ByteArrayOutputStream encoded=new java.io.ByteArrayOutputStream();
  bitmap.compress(android.graphics.Bitmap.CompressFormat.JPEG,80,encoded);bitmap.recycle();
  callbacks.onFrame(encoded.toByteArray());Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
 }
 private static void touch(ImageView screen,int action,float x,float y){
  android.view.MotionEvent e=android.view.MotionEvent.obtain(1,2,action,x,y,0);
  screen.dispatchTouchEvent(e);e.recycle();
 }
 @Test public void completedMotionEventInjectsOnceAndCancelDragDoNot() throws Exception {
  mobile.SessionViewer viewer=new mobile.SessionViewer();
  java.util.concurrent.atomic.AtomicReference<mobile.ViewerCallbacks> callbacks=new java.util.concurrent.atomic.AtomicReference<>();
  java.util.concurrent.atomic.AtomicReference<mobile.ControlCallbacks> controls=new java.util.concurrent.atomic.AtomicReference<>();
  MnetVpnService.node=new mobile.Node(){@Override public mobile.SessionViewer connectSessionWithControl(String peer,mobile.ViewerCallbacks cb,mobile.ControlCallbacks control){callbacks.set(cb);controls.set(control);return viewer;}};
  ActivityController<ViewerActivity> controller=Robolectric.buildActivity(ViewerActivity.class,new Intent().putExtra("peer_ip","100.64.0.2")).create();
  try {
  ViewerActivity activity=controller.get();
  java.lang.reflect.Field screenField=ViewerActivity.class.getDeclaredField("screen");screenField.setAccessible(true);ImageView screen=(ImageView)screenField.get(activity);screen.layout(0,0,400,400);
  java.lang.reflect.Field gateField=ViewerActivity.class.getDeclaredField("viewers");gateField.setAccessible(true);
  @SuppressWarnings("unchecked") ViewerGate<mobile.SessionViewer> gate=(ViewerGate<mobile.SessionViewer>)gateField.get(activity);
  long publishedBy=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(5);
  while(gate.current()==null&&System.nanoTime()<publishedBy){Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Thread.sleep(1);}
  assertSame(viewer,gate.current());
   touch(screen,MotionEvent.ACTION_DOWN,200,200);touch(screen,MotionEvent.ACTION_UP,200,200);
   Thread.sleep(50);assertTrue("no input before any decoded frame",viewer.input.isEmpty());
   frame(callbacks.get(),400,400);
   touch(screen,MotionEvent.ACTION_DOWN,200,200);touch(screen,MotionEvent.ACTION_UP,200,200);Thread.sleep(30);assertTrue("decoded viewing alone never grants input",viewer.input.isEmpty());
   controls.get().onControl("{\"state\":\"allowed\"}");Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   touch(screen,MotionEvent.ACTION_DOWN,200,200);assertTrue(viewer.input.isEmpty());
   touch(screen,MotionEvent.ACTION_UP,200,200);
   long deadline=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(5);
   while(viewer.input.size()<1&&System.nanoTime()<deadline)Thread.sleep(1);
   assertEquals(1,viewer.input.size());assertTrue(viewer.input.get(0).contains("click"));
   touch(screen,MotionEvent.ACTION_UP,200,200);
   touch(screen,MotionEvent.ACTION_DOWN,200,200);touch(screen,MotionEvent.ACTION_CANCEL,200,200);touch(screen,MotionEvent.ACTION_UP,200,200);
   touch(screen,MotionEvent.ACTION_DOWN,200,200);touch(screen,MotionEvent.ACTION_MOVE,300,300);touch(screen,MotionEvent.ACTION_UP,200,200);
   // A resize invalidates a pending tap's coordinate system.
   touch(screen,MotionEvent.ACTION_DOWN,200,200);frame(callbacks.get(),800,400);touch(screen,MotionEvent.ACTION_UP,200,200);
   Thread.sleep(40);assertEquals(1,viewer.input.size());
   touch(screen,MotionEvent.ACTION_DOWN,200,200);
   callbacks.get().onClosed();Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   assertEquals(0,activityFrameWidth(activity));assertNull(screen.getDrawable());
   touch(screen,MotionEvent.ACTION_UP,200,200);Thread.sleep(40);assertEquals(1,viewer.input.size());
  }finally{controller.destroy();MnetVpnService.node=null;}
  assertTrue(viewer.closed);
 }

 @Test public void realNestedRootTapAndScrollDoNotConfuseRemoteInput()throws Exception{
  mobile.SessionViewer viewer=new mobile.SessionViewer();
  java.util.concurrent.atomic.AtomicReference<mobile.ViewerCallbacks> callbacks=new java.util.concurrent.atomic.AtomicReference<>();
  java.util.concurrent.atomic.AtomicReference<mobile.ControlCallbacks> controls=new java.util.concurrent.atomic.AtomicReference<>();
  MnetVpnService.node=new mobile.Node(){@Override public mobile.SessionViewer connectSessionWithControl(String peer,mobile.ViewerCallbacks cb,mobile.ControlCallbacks control){callbacks.set(cb);controls.set(control);return viewer;}};
  ActivityController<ViewerActivity> controller=Robolectric.buildActivity(ViewerActivity.class,new Intent().putExtra("peer_ip","100.64.0.2")).setup();
  try{
   ViewerActivity activity=controller.get();
   @SuppressWarnings("unchecked") ViewerGate<mobile.SessionViewer> gate=(ViewerGate<mobile.SessionViewer>)field(activity,"viewers");
   long deadline=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(5);
   while(gate.current()==null&&System.nanoTime()<deadline){Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Thread.sleep(1);}assertSame(viewer,gate.current());
   ViewGroup root=(ViewGroup)((ViewGroup)activity.findViewById(android.R.id.content)).getChildAt(0);
   root.measure(View.MeasureSpec.makeMeasureSpec(390,View.MeasureSpec.EXACTLY),View.MeasureSpec.makeMeasureSpec(640,View.MeasureSpec.EXACTLY));root.layout(0,0,390,640);
   frame(callbacks.get(),400,400);controls.get().onControl("{\"state\":\"allowed\"}");Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   ImageView screen=(ImageView)field(activity,"screen");android.graphics.Rect bounds=new android.graphics.Rect();screen.getDrawingRect(bounds);root.offsetDescendantRectToMyCoords(screen,bounds);
   float x=bounds.exactCenterX(),y=bounds.exactCenterY();
   long now=android.os.SystemClock.uptimeMillis();
   for(int action:new int[]{MotionEvent.ACTION_DOWN,MotionEvent.ACTION_UP}){MotionEvent event=MotionEvent.obtain(now,now+10,action,x,y,0);root.dispatchTouchEvent(event);event.recycle();}
   deadline=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);while(viewer.input.size()<1&&System.nanoTime()<deadline)Thread.sleep(1);assertEquals(1,viewer.input.size());
   for(int i=0;i<4;i++){MotionEvent event=MotionEvent.obtain(now+100,now+100+i*20,new int[]{MotionEvent.ACTION_DOWN,MotionEvent.ACTION_MOVE,MotionEvent.ACTION_MOVE,MotionEvent.ACTION_UP}[i],x,y-new int[]{0,50,150,150}[i],0);root.dispatchTouchEvent(event);event.recycle();}
   Thread.sleep(50);assertEquals("scroll must not become a remote tap",1,viewer.input.size());
   assertTrue("whole viewer must actually scroll",((android.widget.ScrollView)root.getChildAt(0)).getScrollY()>0);
  }finally{controller.pause().stop().destroy();MnetVpnService.node=null;}
 }
 private static Object field(Object object,String name)throws Exception{java.lang.reflect.Field f=object.getClass().getDeclaredField(name);f.setAccessible(true);return f.get(object);}
 @Test public void visibleStopClosesLateUnpublishedViewerWithoutReconnect()throws Exception{
  java.util.concurrent.CountDownLatch entered=new java.util.concurrent.CountDownLatch(1),release=new java.util.concurrent.CountDownLatch(1);
  java.util.concurrent.atomic.AtomicInteger connects=new java.util.concurrent.atomic.AtomicInteger();
  mobile.SessionViewer late=new mobile.SessionViewer();
  MnetVpnService.node=new mobile.Node(){@Override public mobile.SessionViewer connectSessionWithControl(String peer,mobile.ViewerCallbacks cb,mobile.ControlCallbacks control){connects.incrementAndGet();entered.countDown();try{release.await();}catch(InterruptedException e){Thread.currentThread().interrupt();}return late;}};
  ActivityController<ViewerActivity> controller=Robolectric.buildActivity(ViewerActivity.class,new Intent().putExtra("peer_ip","100.64.0.2")).create().start().resume();
  try{
   ViewerActivity activity=controller.get();assertTrue(entered.await(2,java.util.concurrent.TimeUnit.SECONDS));
   android.widget.Button end=(android.widget.Button)field(activity,"endViewing");assertEquals(View.VISIBLE,end.getVisibility());end.performClick();
   assertTrue(activity.isFinishing());assertEquals(0,activityFrameWidth(activity));assertFalse(end.isEnabled());
   assertTrue("End cancels its pending native attempt immediately",MnetVpnService.node.lastConnect.canceled);
   release.countDown();long until=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);
   while(!late.closed&&System.nanoTime()<until){Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Thread.sleep(1);}
   assertTrue("late publication still executes owned cleanup",late.closed);
   Shadows.shadowOf(android.os.Looper.getMainLooper()).idleFor(java.time.Duration.ofSeconds(30));assertEquals(1,connects.get());
  }finally{release.countDown();controller.pause().stop().destroy();MnetVpnService.node=null;}
 }
 @Test public void visibleStopClearsFrameAndLateCallbacksCannotRevive()throws Exception{
  java.util.concurrent.atomic.AtomicReference<mobile.ViewerCallbacks> callbacks=new java.util.concurrent.atomic.AtomicReference<>();
  java.util.concurrent.atomic.AtomicReference<mobile.ControlCallbacks> controls=new java.util.concurrent.atomic.AtomicReference<>();mobile.SessionViewer viewer=new mobile.SessionViewer();
  MnetVpnService.node=new mobile.Node(){@Override public mobile.SessionViewer connectSessionWithControl(String peer,mobile.ViewerCallbacks cb,mobile.ControlCallbacks control){callbacks.set(cb);controls.set(control);return viewer;}};
  ActivityController<ViewerActivity> controller=Robolectric.buildActivity(ViewerActivity.class,new Intent().putExtra("peer_ip","100.64.0.2")).create().start().resume();
  try{
   ViewerActivity activity=controller.get();@SuppressWarnings("unchecked") ViewerGate<mobile.SessionViewer> gate=(ViewerGate<mobile.SessionViewer>)field(activity,"viewers");
   long until=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);while(gate.current()==null&&System.nanoTime()<until){Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();Thread.sleep(1);}assertSame(viewer,gate.current());
   frame(callbacks.get(),320,480);controls.get().onControl("{\"state\":\"allowed\"}");Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
   ImageView screen=(ImageView)field(activity,"screen");assertNotNull(screen.getDrawable());android.widget.Button end=(android.widget.Button)field(activity,"endViewing");end.performClick();
   assertEquals(0,activityFrameWidth(activity));assertNull(screen.getDrawable());assertTrue(viewer.closed);assertFalse((boolean)field(activity,"controlAllowed"));
   frame(callbacks.get(),320,480);controls.get().onControl("{\"state\":\"allowed\"}");callbacks.get().onClosed();Shadows.shadowOf(android.os.Looper.getMainLooper()).idleFor(java.time.Duration.ofSeconds(30));
   assertNull(gate.current());assertNull(screen.getDrawable());assertFalse((boolean)field(activity,"controlAllowed"));assertFalse((boolean)field(activity,"retryPending"));
  }finally{controller.pause().stop().destroy();MnetVpnService.node=null;}
 }
}
