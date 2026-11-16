package au.fintex.mnet;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.VpnService;
import android.os.ParcelFileDescriptor;
import android.util.Log;

import org.json.JSONObject;

import mobile.Mobile;
import mobile.Node;

// MnetVpnService owns the Android tunnel: it joins the mesh, builds the
// VPN interface with the assigned address, hands the fd to the Go node
// engine, and keeps a foreground notification up while connected.
public class MnetVpnService extends VpnService {

    public static final String ACTION_STATUS = "au.fintex.mnet.STATUS";
    public static final String EXTRA_STATUS = "status_json";

    private static final String TAG = "MnetVPN";
    private static final String CHANNEL = "mesh";
    private static final int NOTIF_ID = 1;

    private static volatile Node sharedNode;

    // currentNode exposes the live node to the session services.
    public static Node currentNode() {
        return sharedNode;
    }

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
        try {
            VpnStartupConfig config = new VpnStartupConfig(
                    prefs.getString("server", ""), prefs.getString("network", ""),
                    prefs.getString("authkey", ""), prefs.getString("name", android.os.Build.MODEL),
                    prefs.getString("exitnode", ""));
            String server = config.server;
            String network = config.network;
            String authKey = config.authKey;
            String name = config.name;
            node = Mobile.newNode(getFilesDir() + "/mnet");
            sharedNode = node;
            Log.i(TAG, "prepare: joining " + network + " on " + server);
            String ipv4 = node.prepare(server, network, name, authKey);
            Log.i(TAG, "prepare ok: mesh ip " + ipv4);

            Builder b = new Builder()
                    .setSession("Mnet")
                    .setMtu(1280)
                    .addAddress(ipv4, 32)
                    .addRoute("100.64.0.0", 10)
                    .addDnsServer(ipv4);
            ParcelFileDescriptor pfd = b.establish();
            if (pfd == null) {
                throw new IllegalStateException("System refused the VPN interface. Check VPN consent in Android settings.");
            }
            int fd = pfd.detachFd(); // the Go side owns it now
            Log.i(TAG, "interface established, starting node");
            node.start(fd, server, network, name, authKey);
            Log.i(TAG, "node up: " + node.status());
            broadcastLoop();
        } catch (Exception e) {
            String msg = e.getMessage() == null ? e.toString() : e.getMessage();
            Log.e(TAG, "mesh failed: " + msg, e);
            stopMesh(msg);
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
        stopMesh(null);
    }

    // stopMesh tears the tunnel down. A failure reason is kept in the
    // final broadcast so the activity shows it instead of a bare down.
    private void stopMesh(String lastError) {
        running = false;
        sharedNode = null;
        if (node != null) {
            node.stop();
            node = null;
        }
        stopForeground(true);
        stopSelf();
        if (lastError != null) {
            broadcast("{\"status\":\"down\",\"last_error\":"
                    + JSONObject.quote(lastError) + "}");
        } else {
            broadcast("{\"status\":\"down\"}");
        }
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
