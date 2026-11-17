package au.fintex.mnet.fixture;

import java.io.*;
import java.net.*;
import java.util.Arrays;

// Disposable native fixture only. No app/native status shortcut: these queries
// cross the Android OS VPN and the production userspace DNS endpoint. Exported
// summaries contain no addresses, names, status JSON or enrollment material.
final class DNSProof {
    static volatile String phase="none";
    static volatile int step;
    static volatile boolean sourceMesh,questionMismatch,badName,badBounds,sourceResolver,sourceUnderlay,sourceAny;
    static volatile boolean shortResponse,wrongID,notResponse,rcodeError,noAnswers,addressMismatch;
    static volatile boolean invalidArgument,networkUnreachable,permissionDenied,addressUnavailable,networkDown;
    static String failureDiagnostics(Throwable error){
        for(Throwable cause=error;cause!=null;cause=cause.getCause())if(cause instanceof android.system.ErrnoException){int number=((android.system.ErrnoException)cause).errno;invalidArgument|=number==android.system.OsConstants.EINVAL;networkUnreachable|=number==android.system.OsConstants.ENETUNREACH;permissionDenied|=number==android.system.OsConstants.EPERM||number==android.system.OsConstants.EACCES;addressUnavailable|=number==android.system.OsConstants.EADDRNOTAVAIL;networkDown|=number==android.system.OsConstants.ENETDOWN;}
        return diagnostics()+" dnsEINVAL="+invalidArgument+" dnsUnreachable="+networkUnreachable+" dnsDenied="+permissionDenied+" dnsNoAddress="+addressUnavailable+" dnsNetworkDown="+networkDown;
    }
    static String diagnostics(){return " dnsCreate="+(step==1)+" dnsConnect="+(step==2)+" dnsSend="+(step==3)+" dnsReceive="+(step==4)+" dnsParse="+(step==5)+" dnsUDP="+"udp".equals(phase)+" dnsTCP="+"tcp".equals(phase)+" dnsOS="+"os".equals(phase)+" dnsSourceMesh="+sourceMesh+" dnsSourceResolver="+sourceResolver+" dnsSourceUnderlay="+sourceUnderlay+" dnsSourceAny="+sourceAny+" dnsQuestionMismatch="+questionMismatch+" dnsBadName="+badName+" dnsBadBounds="+badBounds+" dnsShort="+shortResponse+" dnsWrongID="+wrongID+" dnsNotResponse="+notResponse+" dnsRcode="+rcodeError+" dnsNoAnswers="+noAnswers+" dnsAddressMismatch="+addressMismatch;}
    static void verify(String expected) throws Exception {
        String name = "userspace-peer.android-emulator.mnet";
        phase="udp";
        byte[] query = query(name);
        byte[] udp;
        step=1;
        try (DatagramSocket socket = new DatagramSocket()) {
            step=2;
            socket.connect(InetAddress.getByAddress(new byte[]{(byte)100,64,0,0}),53);
            String source=socket.getLocalAddress().getHostAddress();sourceMesh=source.startsWith("100.");sourceResolver="198.18.0.54".equals(source);sourceUnderlay=source.startsWith("10.");sourceAny=socket.getLocalAddress().isAnyLocalAddress();
            socket.setSoTimeout(5000);
            step=3;
            socket.send(new DatagramPacket(query,query.length));
            byte[] data = new byte[4096]; DatagramPacket packet = new DatagramPacket(data,data.length);
            step=4;
            socket.receive(packet);udp=Arrays.copyOf(data,packet.getLength());
        }
        step=5;
        check(udp,expected);
        phase="tcp";
        try (Socket socket = new Socket()) {
            socket.connect(new InetSocketAddress(InetAddress.getByAddress(new byte[]{(byte)100,64,0,0}),53),5000);
            socket.setSoTimeout(5000);
            DataOutputStream output = new DataOutputStream(socket.getOutputStream());
            output.writeShort(query.length);output.write(query);output.flush();
            DataInputStream input = new DataInputStream(socket.getInputStream());
            int length=input.readUnsignedShort();if(length<12||length>4096)throw new IOException("DNS length");
            byte[] data=new byte[length];input.readFully(data);check(data,expected);
        }
        phase="os";
        boolean resolved = false;
        for (InetAddress address : InetAddress.getAllByName(name)) resolved |= expected.equals(address.getHostAddress());
        if (!resolved) throw new IOException("OS DNS mismatch");
        phase="complete";
    }
    private static byte[] query(String name) throws Exception {
        ByteArrayOutputStream bytes=new ByteArrayOutputStream();DataOutputStream out=new DataOutputStream(bytes);
        out.writeShort(0x4d4e);out.writeShort(0x0100);out.writeShort(1);out.writeShort(0);out.writeShort(0);out.writeShort(0);
        for(String label:name.split("\\.")){byte[] value=label.getBytes("US-ASCII");out.writeByte(value.length);out.write(value);}
        out.writeByte(0);out.writeShort(1);out.writeShort(1);return bytes.toByteArray();
    }
    private static int word(byte[] data,int offset)throws IOException{
        if(offset<0||offset+2>data.length){badBounds=true;throw new IOException("DNS bounds");}return ((data[offset]&255)<<8)|(data[offset+1]&255);
    }
    private static int skipName(byte[] data,int offset)throws IOException{
        while(offset<data.length){int size=data[offset++]&255;if(size==0)return offset;if((size&0xc0)==0xc0){word(data,offset-1);return offset+1;}if(size>63||offset+size>data.length){badName=true;throw new IOException("DNS label");}offset+=size;}
        badName=true;throw new IOException("DNS name");
    }
    private static void check(byte[] data,String expected)throws Exception{
        shortResponse=data.length<12;
        if(shortResponse)throw new IOException("DNS header");
        wrongID=word(data,0)!=0x4d4e;notResponse=(word(data,2)&0x8000)==0;rcodeError=(word(data,2)&15)!=0;noAnswers=word(data,6)==0;
        questionMismatch=word(data,4)!=1;
        if(wrongID||notResponse||rcodeError||questionMismatch)throw new IOException("DNS header");
        int position=skipName(data,12)+4;int answers=word(data,6);boolean found=false;
        for(int i=0;i<answers;i++){
            position=skipName(data,position);int type=word(data,position),klass=word(data,position+2),length=word(data,position+8);position+=10;
            if(position+length>data.length){badBounds=true;throw new IOException("DNS answer bounds");}
            if(type==1&&klass==1&&length==4)found|=expected.equals(InetAddress.getByAddress(Arrays.copyOfRange(data,position,position+length)).getHostAddress());
            position+=length;
        }
        addressMismatch=!found;
        if(!found)throw new IOException("DNS answer mismatch");
    }
}
