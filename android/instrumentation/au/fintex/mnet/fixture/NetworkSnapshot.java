package au.fintex.mnet.fixture;

import android.content.Context;
import android.content.pm.PackageManager;
import android.net.*;
import android.os.PowerManager;
import java.net.InetAddress;

// Disposable fixture diagnostics only. Never binds a socket, selects a network,
// changes policy, grants permission or exports identifiers/configuration.
final class NetworkSnapshot {
    static String capture(Context context) {
        boolean readable=false,active=false,vpn=false,internet=false,dns=false,route=false;
        boolean granted=false,dataRestricted=false,powerSave=false,backgroundRestricted=false;
        try {
            granted=context.checkSelfPermission("android.permission.INTERNET")==PackageManager.PERMISSION_GRANTED;
            ConnectivityManager manager=(ConnectivityManager)context.getSystemService(Context.CONNECTIVITY_SERVICE);
            Network network=manager.getActiveNetwork();active=network!=null;
            NetworkCapabilities capabilities=manager.getNetworkCapabilities(network);
            if(capabilities!=null){vpn=capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN);internet=capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET);}
            LinkProperties properties=manager.getLinkProperties(network);
            InetAddress resolver=InetAddress.getByAddress(new byte[]{(byte)198,18,0,53});
            if(properties!=null){
                dns=properties.getDnsServers().contains(resolver);
                for(RouteInfo candidate:properties.getRoutes())route|=candidate.getType()==RouteInfo.RTN_UNICAST&&candidate.matches(resolver);
            }
            dataRestricted=manager.getRestrictBackgroundStatus()==ConnectivityManager.RESTRICT_BACKGROUND_STATUS_ENABLED;
            PowerManager power=(PowerManager)context.getSystemService(Context.POWER_SERVICE);
            powerSave=power.isPowerSaveMode();
            android.app.ActivityManager activity=(android.app.ActivityManager)context.getSystemService(Context.ACTIVITY_SERVICE);
            backgroundRestricted=activity.isBackgroundRestricted();
            readable=true;
        }catch(Exception ignored){ /* Do not replace the original failure or emit raw errors. */ }
        return " netSnapshot="+readable+" netActive="+active+" netVPN="+vpn+" netInternet="+internet+" netResolver="+dns+" netResolverRoute="+route+" netInternetGranted="+granted+" netDataRestricted="+dataRestricted+" netPowerSave="+powerSave+" netBackgroundRestricted="+backgroundRestricted;
    }
}
