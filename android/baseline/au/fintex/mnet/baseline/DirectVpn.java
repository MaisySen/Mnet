package au.fintex.mnet.baseline;
import android.net.VpnService;
import android.os.*;
import android.system.*;
import android.app.*;
import android.content.*;
import java.io.*;

// Separate throwaway app, not linked into or shipped with Mnet.
public final class DirectVpn extends VpnService {
 public static volatile DirectVpn current;
 public static volatile boolean workerAlive;
 public static volatile int fixtureQueries,fixtureReplies;
 private volatile boolean stopped;
 private ParcelFileDescriptor tunnel;
 @Override public int onStartCommand(Intent intent,int flags,int startId){
  if(tunnel!=null)return START_NOT_STICKY;
  getSystemService(NotificationManager.class).createNotificationChannel(new NotificationChannel("baseline","Disposable VPN baseline",NotificationManager.IMPORTANCE_LOW));
  startForeground(1,new Notification.Builder(this,"baseline").setContentTitle("Disposable VPN baseline").setSmallIcon(android.R.drawable.ic_lock_lock).build());
  try{
   tunnel=new Builder().setSession("Disposable baseline").setMtu(1280).addAddress("100.64.0.1",32).addRoute("100.64.0.0",10).addRoute("198.18.0.53",32).addDnsServer("198.18.0.53").establish();
   if(tunnel==null)throw new IOException("VPN refused");
   fixtureQueries=fixtureReplies=0;current=this;workerAlive=true;
   final FileDescriptor fd=tunnel.getFileDescriptor();
   new Thread(()->{
    try{
     byte[]buffer=new byte[32768];StructPollfd poll=new StructPollfd();poll.fd=fd;poll.events=(short)OsConstants.POLLIN;
     while(!stopped){
      if(Os.poll(new StructPollfd[]{poll},1000)<=0)continue;
      int count;
      try{count=Os.read(fd,buffer,0,buffer.length);}catch(ErrnoException e){if(e.errno==OsConstants.EAGAIN)continue;throw e;}
      if(count<=0)break;int h=(buffer[0]&15)*4;boolean fixture=count>=h+10&&buffer[9]==17&&PacketDNS.word(buffer,h+8)==0x4d4e;
      if(fixture)fixtureQueries++;
      byte[]reply=PacketDNS.reply(buffer,count);if(reply!=null){int n=Os.write(fd,reply,0,reply.length);if(fixture&&n==reply.length)fixtureReplies++;}
     }
    }catch(Exception ignored){}finally{workerAlive=false;}
   },"baseline-direct-dns").start();
  }catch(Exception failure){stopSelf();}
  return START_NOT_STICKY;
 }
 @Override public void onDestroy(){stopped=true;current=null;try{if(tunnel!=null)tunnel.close();}catch(IOException ignored){}stopForeground(true);super.onDestroy();}
}
