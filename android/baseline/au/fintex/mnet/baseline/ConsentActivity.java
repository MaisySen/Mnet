package au.fintex.mnet.baseline;
import android.app.*;
import android.content.*;
import android.net.*;
import android.os.*;
public final class ConsentActivity extends Activity {
 @Override public void onCreate(Bundle state){super.onCreate(state);Intent consent=VpnService.prepare(this);if(consent==null)startVpn();else startActivityForResult(consent,1);}
 @Override protected void onActivityResult(int request,int result,Intent data){super.onActivityResult(request,result,data);if(request==1&&result==RESULT_OK)startVpn();}
 private void startVpn(){startForegroundService(new Intent(this,DirectVpn.class));}
}
