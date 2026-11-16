package au.fintex.mnet;

import android.app.Notification;
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

    private static final VpnWorkerGate WORKERS = new VpnWorkerGate();
    private VpnWorkerGate.Ticket ticket;
    private volatile Node node;
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
        VpnWorkerGate.Ticket acquired = WORKERS.begin();
        if (acquired == null) {
            broadcast("{\"status\":\"down\",\"last_error\":\"Previous VPN worker is still stopping. Retry after it exits.\"}");
            return START_NOT_STICKY;
        }
        ticket = acquired;
        running = true;
        try {
            startForeground(NOTIF_ID, buildNotification(getString(R.string.notification_title)));
            worker = new Thread(() -> runMesh(acquired), "mnet-vpn");
            worker.start();
        } catch (RuntimeException failure) {
            WORKERS.cancel(acquired);
            WORKERS.finish(acquired);
            running = false;
            stopForeground(true);
            stopSelf();
            Log.e(TAG, "VPN foreground startup failed; details withheld from general logs");
            return START_NOT_STICKY;
        }
        return START_STICKY;
    }

    private void runMesh(VpnWorkerGate.Ticket run) {
        Node localNode = null;
        try {
            SharedPreferences prefs = getSharedPreferences("mnet", MODE_PRIVATE);
            VpnStartupConfig config = new VpnStartupConfig(
                    prefs.getString("server", ""), prefs.getString("network", ""),
                    prefs.getString("authkey", ""), prefs.getString("name", android.os.Build.MODEL),
                    prefs.getString("exitnode", ""));
            String server = config.server;
            String network = config.network;
            String authKey = config.authKey;
            String name = config.name;
            if (!WORKERS.active(run)) return;
            localNode = Mobile.newNode(getFilesDir() + "/mnet");
            synchronized (this) {
                if (!WORKERS.active(run)) return;
                node = localNode;
                sharedNode = localNode;
            }
            Log.i(TAG, "preparing private mesh connection");
            String ipv4 = localNode.prepare(server, network, name, authKey);
            if (!WORKERS.active(run)) return;
            Log.i(TAG, "mesh preparation complete");

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
            if (!WORKERS.active(run)) {
                pfd.close();
                return;
            }
            int fd = pfd.detachFd(); // the Go side owns it now
            Log.i(TAG, "interface established, starting node");
            localNode.start(fd, server, network, name, authKey);
            Log.i(TAG, "mesh node started; details stay in private app status");
            if (WORKERS.active(run)) broadcastLoop(run, localNode);
        } catch (Exception e) {
            String msg = e.getMessage() == null ? e.toString() : e.getMessage();
            Log.e(TAG, "mesh failed; details stay in private app status");
            if (WORKERS.active(run)) stopMesh(msg);
        } finally {
            try {
                if (localNode != null) localNode.close();
            } finally {
                synchronized (this) {
                    if (node == localNode) node = null;
                    if (sharedNode == localNode) sharedNode = null;
                    if (ticket == run) running = false;
                }
                WORKERS.finish(run);
            }
        }
    }

    // broadcastLoop pushes status JSON to the activity once a second.
    private void broadcastLoop(VpnWorkerGate.Ticket run, Node localNode) {
        while (WORKERS.active(run)) {
            broadcast(localNode.status());
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
        Intent stop = new Intent(this, MnetVpnService.class).setAction("disconnect");
        return VpnPlatform.notification(this, CHANNEL, getString(R.string.notification_channel))
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
        Node stopping;
        synchronized (this) {
            WORKERS.cancel(ticket);
            running = false;
            stopping = node;
            if (sharedNode == stopping) sharedNode = null;
            node = null;
        }
        if (stopping != null) stopping.close();
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
