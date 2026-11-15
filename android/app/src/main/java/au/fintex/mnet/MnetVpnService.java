package au.fintex.mnet;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.VpnService;
import android.os.ParcelFileDescriptor;

import org.json.JSONObject;

import mobile.Mobile;
import mobile.Node;

// MnetVpnService owns the Android tunnel: it joins the mesh, builds the
// VPN interface with the assigned address, hands the fd to the Go node
// engine, and keeps a foreground notification up while connected.
public class MnetVpnService extends VpnService {

    public static final String ACTION_STATUS = "au.fintex.mnet.STATUS";
    public static final String EXTRA_STATUS = "status_json";

    private static final String CHANNEL = "mesh";
    private static final int NOTIF_ID = 1;

    private Node node;
    private Thread worker;
    private volatile boolean running;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && "disconnect".equals(intent.getAction())) {
            stopMesh();
            return START_NOT_STICKY;
        }
        if (running) {
            return START_STICKY;
        }
        running = true;
        startForeground(NOTIF_ID, buildNotification(getString(R.string.notification_title)));
        worker = new Thread(this::runMesh, "mnet-vpn");
        worker.start();
        return START_STICKY;
    }

    private void runMesh() {
        SharedPreferences prefs = getSharedPreferences("mnet", MODE_PRIVATE);
        String server = prefs.getString("server", "");
        String network = prefs.getString("network", "");
        String authKey = prefs.getString("authkey", "");
        String name = prefs.getString("name", android.os.Build.MODEL);
        try {
            node = Mobile.newNode(getFilesDir() + "/mnet");
            String ipv4 = node.prepare(server, network, name, authKey);

            Builder b = new Builder()
                    .setSession("Mnet")
                    .setMtu(1280)
                    .addAddress(ipv4, 32)
                    .addRoute("100.64.0.0", 10)
                    .addDnsServer(ipv4);
            ParcelFileDescriptor pfd = b.establish();
            if (pfd == null) {
                broadcast("error: system refused the VPN interface");
                stopSelf();
                return;
            }
            int fd = pfd.detachFd(); // the Go side owns it now
            node.start(fd, server, network, name, authKey);
            broadcastLoop();
        } catch (Exception e) {
            broadcast("error: " + e.getMessage());
            stopMesh();
        }
    }

    // broadcastLoop pushes status JSON to the activity once a second.
    private void broadcastLoop() {
        while (running && node != null) {
            broadcast(node.status());
            try {
                Thread.sleep(1000);
            } catch (InterruptedException ignored) {
                return;
            }
        }
    }

    private void broadcast(String json) {
        Intent i = new Intent(ACTION_STATUS);
        i.setPackage(getPackageName());
        i.putExtra(EXTRA_STATUS, json);
        sendBroadcast(i);
        try {
            JSONObject st = new JSONObject(json);
            if ("up".equals(st.optString("status"))) {
                String ip = st.optString("ipv4");
                NotificationManager nm = getSystemService(NotificationManager.class);
                nm.notify(NOTIF_ID, buildNotification("Mnet: " + ip));
            }
        } catch (Exception ignored) {
        }
    }

    private Notification buildNotification(String text) {
        NotificationManager nm = getSystemService(NotificationManager.class);
        if (nm.getNotificationChannel(CHANNEL) == null) {
            nm.createNotificationChannel(new NotificationChannel(
                    CHANNEL, getString(R.string.notification_channel),
                    NotificationManager.IMPORTANCE_LOW));
        }
        Intent stop = new Intent(this, MnetVpnService.class).setAction("disconnect");
        return new Notification.Builder(this, CHANNEL)
                .setContentTitle(getString(R.string.app_name))
                .setContentText(text)
                .setSmallIcon(R.drawable.ic_mnet)
                .setOngoing(true)
                .addAction(new Notification.Action.Builder(
                        null, getString(R.string.disconnect),
                        android.app.PendingIntent.getService(this, 0, stop,
                                android.app.PendingIntent.FLAG_IMMUTABLE)).build())
                .build();
    }

    private void stopMesh() {
        running = false;
        if (node != null) {
            node.stop();
            node = null;
        }
        stopForeground(true);
        stopSelf();
        broadcast("{\"status\":\"down\"}");
    }

    @Override
    public void onDestroy() {
        stopMesh();
        super.onDestroy();
    }

    @Override
    public void onRevoke() {
        stopMesh();
        super.onRevoke();
    }
}
