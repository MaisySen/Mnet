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
 public void onCreate(Bundle args){super.onCreate(args);peer=args.getString("peer");start();}
 public void onStart(){Bundle result=new Bundle();try{stage="os-udp-tcp-dns";DNSProof.verify(peer);hashRoundTrip();hashRoundTrip();result.putString("summary","PASS independent app unbound TCP bidirectional4MiB twice and OS/UDP/TCP DNS");finish(Activity.RESULT_OK,result);}catch(Throwable e){result.putString("summary","FAIL independent app stage="+stage+" type="+e.getClass().getSimpleName()+DNSProof.diagnostics());finish(Activity.RESULT_CANCELED,result);}}
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
