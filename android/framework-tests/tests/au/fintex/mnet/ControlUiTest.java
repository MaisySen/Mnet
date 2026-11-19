package au.fintex.mnet;

import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.TextView;
import java.io.File;
import java.io.FileOutputStream;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.concurrent.atomic.AtomicReference;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.android.controller.ActivityController;
import org.robolectric.annotation.Config;
import org.robolectric.annotation.GraphicsMode;
import static org.junit.Assert.*;

// Actual Activity layout/rendering, with fake peer transport. These PNGs are
// source UI evidence, never Android device/JNI/network acceptance.
@RunWith(RobolectricTestRunner.class)
@Config(manifest=Config.NONE,sdk=33,qualifiers="mdpi")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
public class ControlUiTest {
 private static Object field(Object object,String name)throws Exception{Field f=object.getClass().getDeclaredField(name);f.setAccessible(true);return f.get(object);}
 private static void idle(){Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();}
 private static void state(ViewerActivity activity,String state)throws Exception{Method m=ViewerActivity.class.getDeclaredMethod("applyControl",String.class);m.setAccessible(true);m.invoke(activity,state);}
 private static void render(View root,String name,int width)throws Exception{
  boolean dialog=name.startsWith("host-");
  root.measure(View.MeasureSpec.makeMeasureSpec(width,View.MeasureSpec.EXACTLY),View.MeasureSpec.makeMeasureSpec(800,dialog?View.MeasureSpec.AT_MOST:View.MeasureSpec.EXACTLY));int height=root.getMeasuredHeight();root.layout(0,0,width,height);
  Bitmap bitmap=Bitmap.createBitmap(width,height,Bitmap.Config.ARGB_8888);Canvas canvas=new Canvas(bitmap);canvas.drawColor(Color.WHITE);root.draw(canvas);
  String out=System.getenv("MNET_CONTROL_UI_DIR");if(out!=null){File dir=new File(out);assertTrue(dir.isDirectory()||dir.mkdirs());try(FileOutputStream stream=new FileOutputStream(new File(dir,name+"-"+width+".png"))){assertTrue(bitmap.compress(Bitmap.CompressFormat.PNG,100,stream));}}
  bitmap.recycle();
 }
 @Test public void viewerStatesRenderAndGateRequests()throws Exception{
  mobile.SessionViewer viewer=new mobile.SessionViewer();
  AtomicReference<mobile.ControlCallbacks> callbacks=new AtomicReference<>();
  MnetVpnService.node=new mobile.Node(){@Override public mobile.SessionViewer connectSessionWithControl(String peer,mobile.ViewerCallbacks cb,mobile.ControlCallbacks control){callbacks.set(control);return viewer;}};
  ActivityController<ViewerActivity> controller=Robolectric.buildActivity(ViewerActivity.class,new Intent().putExtra("peer_ip","100.64.0.2")).create().start().resume();
  try{
   ViewerActivity activity=controller.get();
   long until=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(5);
   @SuppressWarnings("unchecked") ViewerGate<mobile.SessionViewer> gate=(ViewerGate<mobile.SessionViewer>)field(activity,"viewers");
   while(gate.current()==null&&System.nanoTime()<until){idle();Thread.sleep(1);}assertSame(viewer,gate.current());
   Button button=(Button)field(activity,"controlRequest");TextView status=(TextView)field(activity,"controlStatus");
   Button end=(Button)field(activity,"endViewing");assertEquals("End viewing",end.getText().toString());assertTrue(end.isEnabled());
   assertFalse(button.isEnabled());assertFalse((boolean)field(activity,"controlAllowed"));
   View root=((ViewGroup)activity.findViewById(android.R.id.content)).getChildAt(0);
   // Placeholder comes only from this isolated fixture, not a real peer.
   ((View)field(activity,"screen")).setBackgroundColor(Color.rgb(32,46,64));
   for(String mode:new String[]{"unavailable","available","pending","allowed","paused","declined","revoked"}){
    state(activity,mode);
    assertEquals(mode.equals("allowed"),(boolean)field(activity,"controlAllowed"));
    assertEquals(mode.equals("pending"),button.getText().toString().equals("Cancel request"));
    assertEquals(mode.equals("available")||mode.equals("pending")||mode.equals("declined")||mode.equals("revoked"),button.isEnabled());
    assertFalse(status.getText().toString().isEmpty());
    for(int width:new int[]{320,390}){render(root,"viewer-"+mode,width);assertTrue(button.getHeight()>=48);assertTrue(status.getBottom()<=button.getTop());assertTrue(button.getRight()<=width);assertTrue(end.getHeight()>=48);assertTrue(end.getTop()>=0);android.graphics.Rect endRect=new android.graphics.Rect();end.getDrawingRect(endRect);((ViewGroup)root).offsetDescendantRectToMyCoords(end,endRect);assertTrue(endRect.bottom<=root.getHeight());assertTrue(endRect.top>=((ViewGroup)root).getChildAt(0).getBottom());}
   }
   state(activity,"available");button.performClick();
   until=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);while(viewer.requests==0&&System.nanoTime()<until){idle();Thread.sleep(1);}assertEquals(1,viewer.requests);
   callbacks.get().onControl("{\"state\":\"pending\"}");idle();assertTrue(button.isEnabled());button.performClick();
   until=System.nanoTime()+java.util.concurrent.TimeUnit.SECONDS.toNanos(2);while(viewer.cancels==0&&System.nanoTime()<until){idle();Thread.sleep(1);}assertEquals(1,viewer.cancels);
   callbacks.get().onControl("{\"state\":\"revoked\"}");idle();assertFalse((boolean)field(activity,"controlAllowed"));
   end.performClick();idle();assertTrue(activity.isFinishing());assertTrue(viewer.closed);assertNull(gate.current());assertFalse(end.isEnabled());
   callbacks.get().onControl("{\"state\":\"allowed\"}");idle();assertFalse((boolean)field(activity,"controlAllowed"));
   render(root,"viewer-ended",320);
  }finally{controller.pause().stop().destroy();MnetVpnService.node=null;}
 }
 @Test public void hostBoundDecisionDialogRenders()throws Exception{
  org.robolectric.android.controller.ServiceController<HostSessionService> service=Robolectric.buildService(HostSessionService.class).create();
  ActivityController<HostingActivity> controller=Robolectric.buildActivity(HostingActivity.class).create().start().resume();
  MnetAccessibilityService.current=new MnetAccessibilityService();
  try{
   for(String name:new String[]{"running","autoAccept"}){Field f=HostSessionService.class.getDeclaredField(name);f.setAccessible(true);f.set(service.get(),true);}
   assertTrue(service.get().acceptSession("viewer"));ServiceTest.requestControl(service.get());
   HostingActivity activity=controller.get();activity.showControlConsent();idle();
   android.app.AlertDialog dialog=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();
   assertTrue(dialog.isShowing());assertFalse(HostSessionService.controlAllowed());
   for(int width:new int[]{320,390})render(dialog.getWindow().getDecorView(),"host-allow-control",width);
   dialog.getButton(android.app.AlertDialog.BUTTON_POSITIVE).performClick();idle();assertTrue(HostSessionService.controlAllowed());
   activity.showControlConsent();idle();dialog=org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog();assertFalse(HostSessionService.controlAllowed());
   for(int width:new int[]{320,390})render(dialog.getWindow().getDecorView(),"host-revoke-control",width);
  }finally{controller.pause().stop().destroy();service.destroy();MnetAccessibilityService.current=null;}
 }
}
