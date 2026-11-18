package au.fintex.mnet.baseline;
import android.app.*;
import android.os.*;
import java.io.*;
import org.json.*;
public final class IndependentInstrumentation extends Instrumentation {
 private String phase;
 @Override public void onCreate(Bundle args){super.onCreate(args);phase=args.getString("phase");start();}
 @Override public void onStart(){Bundle result=new Bundle();try{
  JSONObject row=new JSONObject();row.put("phase","1".equals(phase)?"reconnect":"initial");row.put("role","independent");row.put("original_udp",Probe.udp(getTargetContext(),false));row.put("os_dns",Probe.system(getTargetContext(),false));
  try(FileOutputStream f=new FileOutputStream(new File(getTargetContext().getFilesDir(),"baseline-independent-"+phase+".json"))){f.write(row.toString().getBytes("UTF-8"));}
  result.putString("summary","COMPLETE independent baseline; not a Mnet verdict");finish(Activity.RESULT_OK,result);
 }catch(Throwable e){result.putString("summary","INCONCLUSIVE independent baseline "+e.getClass().getSimpleName());finish(Activity.RESULT_CANCELED,result);}}
}
