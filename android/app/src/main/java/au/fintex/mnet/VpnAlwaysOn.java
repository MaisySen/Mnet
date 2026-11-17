package au.fintex.mnet;

import android.net.VpnService;
import android.os.Build;

// API29 exposes the current system mode directly. Older APIs remain unknown;
// do not guess from a null restart intent or silently alter their lifecycle.
final class VpnAlwaysOn {
    private VpnAlwaysOn() {}
    static boolean managed(VpnService service) {
        return Build.VERSION.SDK_INT >= 29 && service.isAlwaysOn();
    }
}
