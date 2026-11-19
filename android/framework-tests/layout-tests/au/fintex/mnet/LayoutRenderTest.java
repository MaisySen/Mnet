package au.fintex.mnet;

import android.app.Activity;
import android.content.BroadcastReceiver;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.widget.ScrollView;
import android.widget.TextView;
import java.io.File;
import java.io.FileOutputStream;
import java.lang.reflect.Field;
import org.junit.Test;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.annotation.*;
import static org.junit.Assert.*;

/** Actual production Views and compiled resources. Transport and OS are simulated. */
@RunWith(RobolectricTestRunner.class)
@Config(sdk=33, qualifiers="w390dp-h844dp-mdpi")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
public class LayoutRenderTest {
    private static Object field(Object o,String name)throws Exception {
        Field f=o.getClass().getDeclaredField(name);f.setAccessible(true);return f.get(o);
    }
    private static View root(Activity a) {
        return ((ViewGroup)a.findViewById(android.R.id.content)).getChildAt(0);
    }
    private static void layout(Activity a, int width, int height) {
        View v=root(a);
        v.measure(View.MeasureSpec.makeMeasureSpec(width,View.MeasureSpec.EXACTLY),View.MeasureSpec.makeMeasureSpec(height,View.MeasureSpec.EXACTLY));
        v.layout(0,0,width,height);
        Shadows.shadowOf(android.os.Looper.getMainLooper()).idle();
    }
    private static void capture(Activity a,String name,int width,int height)throws Exception {
        layout(a,width,height);
        Bitmap bitmap=Bitmap.createBitmap(width,height,Bitmap.Config.ARGB_8888);
        Canvas canvas = new Canvas(bitmap);
        canvas.drawColor(AppUi.CANVAS);
        ViewGroup shell=(ViewGroup)root(a);
        ScrollView scroll=(ScrollView)shell.getChildAt(0);
        canvas.save();
        canvas.clipRect(0,0,scroll.getWidth(),scroll.getHeight());
        canvas.translate(0,-scroll.getScrollY());
        scroll.getChildAt(0).draw(canvas);
        canvas.restore();
        if(shell.getChildCount()>1){View footer=shell.getChildAt(1);canvas.save();canvas.translate(footer.getLeft(),footer.getTop());footer.draw(canvas);canvas.restore();}
        File file=new File(System.getProperty("mnet.layout.output"),name+".png");
        try(FileOutputStream out=new FileOutputStream(file)){assertTrue(bitmap.compress(Bitmap.CompressFormat.PNG,100,out));}
        bitmap.recycle();
    }
    private static void status(MainActivity a,String json,boolean always)throws Exception {
        ((BroadcastReceiver)field(a,"statusReceiver")).onReceive(a,new Intent(MnetVpnService.ACTION_STATUS)
                .putExtra(MnetVpnService.EXTRA_STATUS,json).putExtra(MnetVpnService.EXTRA_ALWAYS_ON,always));
    }
    private void home(String prefix,int width,int height,float fontScale)throws Exception {
        android.content.res.Configuration config=RuntimeEnvironment.getApplication().getResources().getConfiguration();
        config.fontScale=fontScale;
        RuntimeEnvironment.getApplication().getResources().updateConfiguration(config,null);
        try(var c=Robolectric.buildActivity(MainActivity.class).setup()){
            MainActivity a=c.get(); capture(a,prefix+"-home",width,height);
            assertTrue(((TextView)field(a,"connectionTitle")).getText().toString().contains("Not connected"));
            assertEquals(View.GONE,((View)field(a,"setupFields")).getVisibility());
            ((Button)field(a,"setupToggle")).performClick();
            assertEquals(View.VISIBLE,((View)field(a,"setupFields")).getVisibility());
            layout(a,width,height);
            ScrollView scroll=(ScrollView)((ViewGroup)root(a)).getChildAt(0);
            View settings=(View)field(a,"setupToggle");
            android.graphics.Rect rect=new android.graphics.Rect();settings.getDrawingRect(rect);
            scroll.offsetDescendantRectToMyCoords(settings,rect);scroll.scrollTo(0,Math.max(0,rect.top-20));
            capture(a,prefix+"-settings",width,height);
            scroll.scrollTo(0,scroll.getChildAt(0).getHeight());capture(a,prefix+"-settings-bottom",width,height);
            ((Button)field(a,"setupToggle")).performClick();scroll.scrollTo(0,0);
            status(a,"{\"status\":\"waiting-approval\",\"approval_url\":\"https://example.invalid/device/fixture\"}",false);
            capture(a,prefix+"-approval",width,height);
            status(a,"{\"status\":\"up\",\"ipv4\":\"100.64.0.2\",\"peers\":[{\"name\":\"Example laptop\",\"ipv4\":\"100.64.0.3\",\"online\":true}]}",true);
            assertFalse(a.findViewById(R.id.connect_button).isEnabled());
            capture(a,prefix+"-managed",width,height);
            status(a,"{\"status\":\"down\",\"last_error\":\"The control server could not be reached. Check the server URL and your internet connection.\"}",false);
            scroll.scrollTo(0,scroll.getChildAt(0).getHeight());capture(a,prefix+"-error-details",width,height);
            // Every lower control is reachable by scrolling, also at enlarged text.
            scroll.scrollTo(0,scroll.getChildAt(0).getHeight());layout(a,width,height);
            View support=a.findViewById(R.id.privacy_support_button);
            android.graphics.Rect supportBounds=new android.graphics.Rect();support.getDrawingRect(supportBounds);
            scroll.offsetDescendantRectToMyCoords(support,supportBounds);
            assertTrue("support not reachable",supportBounds.bottom-scroll.getScrollY()<=scroll.getHeight());
            a.findViewById(R.id.view_peer_button).performClick();
            assertNotNull(org.robolectric.shadows.ShadowAlertDialog.getLatestAlertDialog());
            assertNull(Shadows.shadowOf(a).getNextStartedService());
        }
    }
    @Test public void standardHome()throws Exception{home("390",390,844,1f);}
    @Test @Config(qualifiers="w320dp-h720dp-mdpi") public void compactLargeTextHome()throws Exception{home("320-large",320,720,1.6f);}
    @Test @Config(qualifiers="w740dp-h360dp-land-mdpi") public void landscapeHome()throws Exception{home("740-land",740,360,1f);}
    @Test public void hosting()throws Exception{
        try(var c=Robolectric.buildActivity(HostingActivity.class).setup()){
            HostingActivity a=c.get();capture(a,"390-host",390,844);
            ScrollView hostScroll=(ScrollView)((ViewGroup)root(a)).getChildAt(0);
            hostScroll.scrollTo(0,hostScroll.getChildAt(0).getHeight());capture(a,"390-host-bottom",390,844);
            assertTrue(a.findViewById(R.id.start_hosting_button).isEnabled());
            assertFalse(((Button)field(a,"stop")).isEnabled());
            var service=Robolectric.buildService(HostSessionService.class).create();
            try{
                Field running=HostSessionService.class.getDeclaredField("running");running.setAccessible(true);running.set(service.get(),true);
                var refresh=HostingActivity.class.getDeclaredMethod("refreshHostingState");refresh.setAccessible(true);refresh.invoke(a);
                ((ScrollView)((ViewGroup)root(a)).getChildAt(0)).scrollTo(0,0);
                capture(a,"390-host-active",390,844);
                assertTrue(((Button)field(a,"stop")).isEnabled());
                assertFalse(a.findViewById(R.id.start_hosting_button).isEnabled());
            }finally{service.destroy();}
            assertNull(Shadows.shadowOf(a).getNextStartedService());
            assertNull(Shadows.shadowOf(a).getNextStartedActivity());
        }
    }
    @Test @Config(qualifiers="w320dp-h720dp-mdpi") public void largeHosting()throws Exception{
        var resources=RuntimeEnvironment.getApplication().getResources();var config=resources.getConfiguration();config.fontScale=1.6f;resources.updateConfiguration(config,null);
        try(var c=Robolectric.buildActivity(HostingActivity.class).setup()){
            HostingActivity a=c.get();capture(a,"320-large-host",320,720);
            ScrollView hostScroll=(ScrollView)((ViewGroup)root(a)).getChildAt(0);
            hostScroll.scrollTo(0,hostScroll.getChildAt(0).getHeight());capture(a,"320-large-host-bottom",320,720);
            View stop=(View)field(a,"stop");android.graphics.Rect bounds=new android.graphics.Rect();stop.getDrawingRect(bounds);
            ((ViewGroup)root(a)).offsetDescendantRectToMyCoords(stop,bounds);
            assertTrue("stop outside viewport",bounds.top>=0&&bounds.bottom<=720);
            ScrollView scroll=(ScrollView)((ViewGroup)root(a)).getChildAt(0);
            assertTrue("not actually scrollable",scroll.getChildAt(0).getHeight()>scroll.getHeight());
            assertTrue("bottom unreachable",scroll.getScrollY()>0);
            assertTrue("bottom clipped",scroll.getChildAt(0).getHeight()-scroll.getScrollY()<=scroll.getHeight());
        }
    }
}
