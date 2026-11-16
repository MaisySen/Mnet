package au.fintex.mnet;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.IntentFilter;
import android.os.Build;

// Keep API26/33 entry points behind version checks. The legacy receiver uses
// the same signature-only permission, rather than becoming an exported input.
final class VpnPlatform {
    private VpnPlatform() {}

    static Notification.Builder notification(Context context, String channel, String label) {
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationManager manager = context.getSystemService(NotificationManager.class);
            if (manager.getNotificationChannel(channel) == null) {
                manager.createNotificationChannel(new NotificationChannel(
                        channel, label, NotificationManager.IMPORTANCE_LOW));
            }
            return new Notification.Builder(context, channel);
        }
        return new Notification.Builder(context).setPriority(Notification.PRIORITY_LOW);
    }

    static void registerPrivateReceiver(Context context, BroadcastReceiver receiver, IntentFilter filter) {
        String permission = context.getPackageName() + ".permission.PRIVATE_STATUS";
        if (Build.VERSION.SDK_INT >= 33) {
            context.registerReceiver(receiver, filter, permission, null, Context.RECEIVER_NOT_EXPORTED);
        } else {
            context.registerReceiver(receiver, filter, permission, null);
        }
    }
}
