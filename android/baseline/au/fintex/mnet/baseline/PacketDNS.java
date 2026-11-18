package au.fintex.mnet.baseline;

import java.util.Arrays;

// Test-only direct DNS responder. No network, engine, keys or gVisor.
public final class PacketDNS {
 public static int word(byte[]p,int n){return ((p[n]&255)<<8)|(p[n+1]&255);}
 static void put(byte[]p,int n,int v){p[n]=(byte)(v>>8);p[n+1]=(byte)v;}
 static int checksum(byte[]p,int off,int len,int sum){for(;len>=2;off+=2,len-=2)sum+=word(p,off);if(len>0)sum+=(p[off]&255)<<8;while((sum>>>16)!=0)sum=(sum&65535)+(sum>>>16);return (~sum)&65535;}
 public static byte[] reply(byte[]p,int len){
  if(len<40||(p[0]>>4&15)!=4||p[9]!=17)return null;
  int h=(p[0]&15)*4;if(h<20||h+20>len||(word(p,6)&0x3fff)!=0||word(p,h+2)!=53)return null;
  if(p[16]!=(byte)198||p[17]!=18||p[18]!=0||p[19]!=53)return null;
  int size=word(p,h+4),dns=h+8,end=dns+12;
  if(size<20||h+size>len||word(p,dns+4)!=1||(word(p,dns+2)&0x8000)!=0)return null;
  while(end<h+size){int n=p[end++]&255;if(n==0)break;if(n>63||end+n>h+size)return null;end+=n;}
  if(end+4>h+size)return null;
  int type=word(p,end),klass=word(p,end+2);end+=4;
  boolean answer=type==1&&klass==1;
  int dl=end-dns+(answer?16:0);byte[]out=new byte[28+dl];
  out[0]=0x45;put(out,2,out.length);out[8]=64;out[9]=17;
  System.arraycopy(p,16,out,12,4);System.arraycopy(p,12,out,16,4);
  put(out,20,53);put(out,22,word(p,h));put(out,24,out.length-20);
  System.arraycopy(p,dns,out,28,end-dns);put(out,30,0x8180);put(out,34,answer?1:0);put(out,36,0);put(out,38,0);
  if(answer){int at=28+end-dns;put(out,at,0xc00c);put(out,at+2,1);put(out,at+4,1);put(out,at+8,30);put(out,at+10,4);out[at+12]=100;out[at+13]=64;out[at+15]=2;}
  put(out,10,checksum(out,0,20,0));int pseudo=17+out.length-20;for(int i=12;i<20;i+=2)pseudo+=word(out,i);
  int check=checksum(out,20,out.length-20,pseudo);put(out,26,check==0?65535:check);return out;
 }
 public static void main(String[]args)throws Exception{
  byte[]query=au.fintex.mnet.baseline.Probe.query();byte[]p=new byte[28+query.length];p[0]=0x45;p[9]=17;put(p,2,p.length);p[12]=100;p[13]=64;p[15]=1;p[16]=(byte)198;p[17]=18;p[19]=53;put(p,20,40001);put(p,22,53);put(p,24,p.length-20);System.arraycopy(query,0,p,28,query.length);
  byte[]r=reply(p,p.length);if(r==null||checksum(r,0,20,0)!=0||word(r,34)!=1||word(r,22)!=40001)throw new AssertionError("wire response");int sum=17+r.length-20;for(int i=12;i<20;i+=2)sum+=word(r,i);if(checksum(r,20,r.length-20,sum)!=0)throw new AssertionError("UDP checksum");
  for(int n=0;n<p.length;n++)if(reply(p,n)!=null)throw new AssertionError("truncation accepted");
  System.out.println("PASS direct baseline packet tuple, lengths, DNS answer, IP/UDP checksums and truncation");
 }
}
