package au.fintex.mnet.baseline;
import java.io.*;
import java.net.*;
import java.util.*;
import org.json.*;

public final class Probe {
 public static byte[] query()throws Exception{
  ByteArrayOutputStream b=new ByteArrayOutputStream();DataOutputStream o=new DataOutputStream(b);
  o.writeShort(0x4d4e);o.writeShort(0x0100);o.writeShort(1);o.writeShort(0);o.writeShort(0);o.writeShort(0);
  for(String label:"userspace-peer.android-emulator.mnet".split("\\.")){byte[]v=label.getBytes("US-ASCII");o.writeByte(v.length);o.write(v);}o.writeByte(0);o.writeShort(1);o.writeShort(1);return b.toByteArray();
 }
 public static JSONObject udp(android.content.Context context,boolean owner){
  JSONObject row=new JSONObject();String stage="create";
  try(DatagramSocket socket=new DatagramSocket(null)){
   if(owner){stage="network-bind";context.getSystemService(android.net.ConnectivityManager.class).getActiveNetwork().bindSocket(socket);stage="source-bind";socket.bind(new InetSocketAddress("100.64.0.1",0));}
   stage="connect";InetAddress resolver=InetAddress.getByAddress(new byte[]{(byte)198,18,0,53});socket.connect(new InetSocketAddress(resolver,53));socket.setSoTimeout(5000);
   byte[]q=query();stage="send";socket.send(new DatagramPacket(q,q.length,resolver,53));stage="receive";byte[]b=new byte[4096];DatagramPacket p=new DatagramPacket(b,b.length);socket.receive(p);stage="parse";
   if(p.getLength()<28||PacketDNS.word(b,0)!=0x4d4e||(PacketDNS.word(b,2)&0x800f)!=0x8000||PacketDNS.word(b,6)!=1)throw new IOException("DNS response");
   int end=p.getLength();if(b[end-4]!=100||b[end-3]!=64||b[end-2]!=0||b[end-1]!=2)throw new IOException("DNS answer");
   row.put("passed",true);
  }catch(Exception error){try{row.put("passed",false);row.put("stage",stage);row.put("timeout",error instanceof SocketTimeoutException);boolean denied=false;for(Throwable t=error;t!=null;t=t.getCause())if(t instanceof android.system.ErrnoException){int e=((android.system.ErrnoException)t).errno;denied|=e==android.system.OsConstants.EPERM||e==android.system.OsConstants.EACCES;}row.put("denied",denied);}catch(JSONException ignored){}}
  return row;
 }
 public static boolean system(android.content.Context context,boolean owner){try{android.net.Network n=owner?context.getSystemService(android.net.ConnectivityManager.class).getActiveNetwork():null;InetAddress[]addresses=n==null?InetAddress.getAllByName("userspace-peer.android-emulator.mnet"):n.getAllByName("userspace-peer.android-emulator.mnet");for(InetAddress a:addresses)if("100.64.0.2".equals(a.getHostAddress()))return true;}catch(Exception ignored){}return false;}
}
