package au.fintex.mnet;

import android.app.Notification;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.graphics.Bitmap;
import android.graphics.PixelFormat;
import android.hardware.display.DisplayManager;
import android.hardware.display.VirtualDisplay;
import android.media.Image;
import android.media.ImageReader;
import android.media.projection.MediaProjection;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.IBinder;
import android.util.DisplayMetrics;
import android.view.WindowManager;

import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import mobile.HostCallbacks;
import mobile.Node;

// HostSessionService hosts one remote session on this phone: it
// captures the screen with MediaProjection and feeds frames to the Go
// engine, which serves them to a connected viewer. Consent for each
// session comes from HostingActivity (auto-accept switch or a dialog).
public class HostSessionService extends Service implements HostCallbacks {

    public static final String ACTION_LOG = "au.fintex.mnet.SESSION_LOG";
    public static final String EXTRA_LOG = "log";
    public static final String ACTION_ASK = "au.fintex.mnet.SESSION_ASK";
    public static final String EXTRA_PEER = "peer";

    private static final String CHANNEL = "session";
    private static final int NOTIF_ID = 2;

    // Set by HostingActivity.
    public static final AtomicBoolean autoAccept = new AtomicBoolean(false);
    private static volatile Boolean pendingAnswer;
    private static volatile CountDownLatch pendingLatch;

    // answerSession is called by HostingActivity's consent dialog.
    public static void answerSession(boolean accept) {
        CountDownLatch l = pendingLatch;
        if (l != null) {
            pendingAnswer = accept;
            l.countDown();
        }
    }

    private Node node;
    private MediaProjection projection;
    private VirtualDisplay display;
    private ImageReader reader;
    private HandlerThread captureThread;
    private Handler captureHandler;

    private int frameW, frameH;
    private volatile byte[] latestJpeg;
    private volatile long latestSeq;
    private volatile long servedSeq;
    private volatile boolean running;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent == null || !intent.hasExtra("resultCode")) {
            stopSelf();
            return START_NOT_STICKY;
        }
        node = MnetVpnService.currentNode();
        if (node == null) {
            log("connect the mesh first");
            stopSelf();
            return START_NOT_STICKY;
        }
        startFg();
        int resultCode = intent.getIntExtra("resultCode", 0);
        Intent data = intent.getParcelableExtra("data");
        startCapture(resultCode, data);
        running = true;
        MnetAccessibilityService.setHosting(true);
        new Thread(() -> {
            try {
                node.startSessionHost(this);
                log("hosting on the mesh; waiting for a viewer");
            } catch (Exception e) {
                log("host failed: " + e.getMessage());
                stopSelf();
            }
        }, "session-host").start();
        return START_STICKY;
    }

    private void startFg() {
        Notification n = VpnPlatform.notification(this, CHANNEL, "Mnet sessions")
                .setContentTitle("Mnet session hosting")
                .setContentText("This phone can be viewed and controlled over the mesh")
                .setSmallIcon(android.R.drawable.presence_video_online)
                .setOngoing(true)
                .build();
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIF_ID, n,
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION);
        } else {
            startForeground(NOTIF_ID, n);
        }
    }

    private void startCapture(int resultCode, Intent data) {
        WindowManager wm = (WindowManager) getSystemService(Context.WINDOW_SERVICE);
        DisplayMetrics dm = new DisplayMetrics();
        wm.getDefaultDisplay().getRealMetrics(dm);
        int devW = dm.widthPixels, devH = dm.heightPixels;
        int scale = 1;
        while (Math.max(devW, devH) / scale > 1280) {
            scale++;
        }
        frameW = devW / scale;
        frameH = devH / scale;

        captureThread = new HandlerThread("capture");
        captureThread.start();
        captureHandler = new Handler(captureThread.getLooper());

        MediaProjectionManager mpm = getSystemService(MediaProjectionManager.class);
        projection = mpm.getMediaProjection(resultCode, data);
        reader = ImageReader.newInstance(frameW, frameH, PixelFormat.RGBA_8888, 2);
        display = projection.createVirtualDisplay("mnet-session", frameW, frameH,
                dm.densityDpi, DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR,
                reader.getSurface(), null, captureHandler);
        reader.setOnImageAvailableListener(r -> {
            Image img = r.acquireLatestImage();
            if (img == null) {
                return;
            }
            try {
                Image.Plane plane = img.getPlanes()[0];
                ByteBuffer buf = plane.getBuffer();
                int pixelStride = plane.getPixelStride();
                int rowStride = plane.getRowStride();
                int rowPadding = rowStride - pixelStride * frameW;
                Bitmap bmp = Bitmap.createBitmap(frameW + rowPadding / pixelStride,
                        frameH, Bitmap.Config.ARGB_8888);
                bmp.copyPixelsFromBuffer(buf);
                Bitmap cropped = Bitmap.createBitmap(bmp, 0, 0, frameW, frameH);
                ByteArrayOutputStream out = new ByteArrayOutputStream();
                cropped.compress(Bitmap.CompressFormat.JPEG, 70, out);
                latestJpeg = out.toByteArray();
                latestSeq++;
            } catch (Exception ignored) {
            } finally {
                img.close();
            }
        }, captureHandler);
    }

    // HostCallbacks

    @Override
    public byte[] capture() {
        if (latestSeq == servedSeq) {
            return null;
        }
        servedSeq = latestSeq;
        return latestJpeg;
    }

    @Override
    public long screenWidth() {
        return frameW;
    }

    @Override
    public long screenHeight() {
        return frameH;
    }

    @Override
    public void inject(String evJson) {
        MnetAccessibilityService svc = MnetAccessibilityService.instance();
        if (svc != null) {
            WindowManager wm = (WindowManager) getSystemService(Context.WINDOW_SERVICE);
            DisplayMetrics dm = new DisplayMetrics();
            wm.getDefaultDisplay().getRealMetrics(dm);
            float scale = (float) dm.widthPixels / frameW;
            svc.inject(evJson, scale);
        }
    }

    @Override
    public boolean acceptSession(String peerNodeName) {
        if (autoAccept.get()) {
            log("accepted session from " + peerNodeName + " (auto-accept)");
            return true;
        }
        pendingLatch = new CountDownLatch(1);
        pendingAnswer = null;
        Intent ask = new Intent(ACTION_ASK).putExtra(EXTRA_PEER, peerNodeName);
        ask.setPackage(getPackageName());
        sendBroadcast(ask);
        try {
            if (pendingLatch.await(60, TimeUnit.SECONDS)) {
                return Boolean.TRUE.equals(pendingAnswer);
            }
        } catch (InterruptedException ignored) {
        }
        return false;
    }

    @Override
    public void onChat(String msgJson) {
        log("chat: " + msgJson);
    }

    @Override
    public void onFile(String name, long size) {
        log("received file: " + name + " (" + size + " bytes)");
    }

    @Override
    public void onSessionEnd() {
        log("session ended");
    }

    private void log(String s) {
        Intent i = new Intent(ACTION_LOG).putExtra(EXTRA_LOG, s);
        i.setPackage(getPackageName());
        sendBroadcast(i);
    }

    @Override
    public void onDestroy() {
        running = false;
        MnetAccessibilityService.setHosting(false);
        if (node != null) {
            try {
                node.stopSessionHost();
            } catch (Exception ignored) {
            }
        }
        if (display != null) {
            display.release();
        }
        if (reader != null) {
            reader.close();
        }
        if (projection != null) {
            projection.stop();
        }
        if (captureThread != null) {
            captureThread.quitSafely();
        }
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }
}
