package au.fintex.mnet.fixture;
import android.app.*;
import android.os.*;
import java.net.*;
import java.io.*;
import java.security.*;
import java.util.*;
// Runs in the separately installed fixture app UID, not the VPN app process.
public class IndependentAppInstrumentation extends Instrumentation {
 private String peer;
 private String stage="created";
 private String initialNetwork="";
 public void onCreate(Bundle args){super.onCreate(args);peer=args.getString("peer");start();}
 public void onStart(){
  Bundle result=new Bundle();boolean dnsPassed=false,tcpPassed=false,osDnsPassed=false;Throwable dnsFailure=null;
  try{
   DNSProof.configureTrace(new File(getTargetContext().getFilesDir(),"fixture-dns-sockets.json"));
   initialNetwork=NetworkSnapshot.capture(getTargetContext());stage="os-udp-tcp-dns";
   try{DNSProof.verify(peer);dnsPassed=true;osDnsPassed=true;}catch(Exception failure){
    dnsFailure=failure;
    // Test ordinary OS hostname lookup even when raw UDP/53 is refused.
    // Keep the original raw transport failure and overall failed verdict.
    try{DNSProof.verifySystem(peer,null);osDnsPassed=true;}catch(Exception ignored){}
   }
   // A DNS failure is still an overall failure. Independently test numeric-IP
   // TCP so diagnostics distinguish resolver-only from all-traffic failures.
   hashRoundTrip();hashRoundTrip();tcpPassed=true;
   if(dnsFailure!=null){stage="os-udp-tcp-dns";throw dnsFailure;}
   result.putString("summary","PASS independent app unbound TCP bidirectional4MiB twice and OS/UDP/TCP DNS");finish(Activity.RESULT_OK,result);
  }catch(Throwable e){result.putString("summary","FAIL independent app stage="+stage+" type="+e.getClass().getSimpleName()+" independentDNSPassed="+dnsPassed+" independentOSDNSPassed="+osDnsPassed+" independentTCPPassed="+tcpPassed+DNSProof.failureDiagnostics(dnsFailure!=null?dnsFailure:e)+" before"+initialNetwork+" after"+NetworkSnapshot.capture(getTargetContext()));finish(Activity.RESULT_CANCELED,result);}
 }

    private void hashRoundTrip() throws Exception {
        stage = "mesh-hash-round-trip";
        // This address has no host TCP listener. Reaching it requires the real
        // Android VPN fd, WireGuard and the Go peer's userspace network stack.
        byte[] sent = new byte[4 << 20];
        byte[] unit = "kernel-peer-proof".getBytes("UTF-8");
        for (int i = 0; i < sent.length; i++) sent[i] = unit[i % unit.length];
        try (Socket socket = new Socket()) {
            stage = "mesh-tcp-local-bind";

            stage = "mesh-tcp-connect";
            socket.connect(new InetSocketAddress(peer, 45993), 45000);
            socket.setSoTimeout(45000);
            stage = "mesh-outbound-send";
            socket.getOutputStream().write(sent);
            DataInputStream input = new DataInputStream(socket.getInputStream());
            stage = "mesh-outbound-hash-read";
            byte[] hash = new byte[32]; input.readFully(hash);
            if (!Arrays.equals(hash, MessageDigest.getInstance("SHA-256").digest(sent))) throw new IOException("outbound digest mismatch");
            stage = "mesh-reverse-read";
            byte[] reverse = new byte[4 << 20]; input.readFully(reverse);
            byte[] expectedUnit = "reverse-native-peer-proof".getBytes("UTF-8");
            for (int i = 0; i < reverse.length; i++) if (reverse[i] != expectedUnit[i % expectedUnit.length]) throw new IOException("reverse bytes mismatch");
            stage = "mesh-reverse-hash-send";
            socket.getOutputStream().write(MessageDigest.getInstance("SHA-256").digest(reverse));
            if (input.readByte() != 'O' || input.readByte() != 'K') throw new IOException("reverse acknowledgment mismatch");
        }
    }
}
