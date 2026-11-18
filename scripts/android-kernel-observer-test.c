#define OBSERVER_TEST
#include "android-kernel-observer.c"
#include <assert.h>
static void put16(unsigned char*p,unsigned n){p[0]=n>>8;p[1]=n;}
int main(void){
 unsigned char p[64]={0x45};put16(p+2,40);p[8]=64;p[9]=17;
 p[12]=100;p[15]=1;p[16]=198;p[17]=18;p[19]=53;
 put16(p+20,40001);put16(p+22,53);put16(p+24,20);put16(p+28,0x4d4e);
 put16(p+10,checksum(p,20,0));
 unsigned pseudo=u16(p+12)+u16(p+14)+u16(p+16)+u16(p+18)+17+20;
 put16(p+26,checksum(p+20,20,pseudo));
 assert(checksum(p,20,0)==0&&checksum(p+20,20,pseudo)==0);
 observe(p,40,1,9);assert(dns_events==1);
 p[30]^=1;assert(checksum(p+20,20,pseudo)!=0);observe(p,40,0,9);assert(dns_events==2);
 observe(p,39,0,9);assert(dns_events==2);put16(p+24,200);observe(p,40,0,9);assert(dns_events==2);
 put16(p+24,20);put16(p+6,0x2000);observe(p,40,0,9);assert(dns_events==2);
 put16(p+6,0);put16(p+28,0x1111);observe(p,40,0,9);assert(dns_events==2);
 p[9]=6;put16(p+20,40002);put16(p+22,45993);p[32]=0x50;p[33]=2;
 observe(p,40,1,9);assert(nflows==1&&flows[0].syn==1&&flows[0].bytes==0);
 observe(p,40,1,10);assert(nflows==2); // new TUN is never merged with old
 p[33]=4;observe(p,40,1,9);assert(flows[0].rst==1);
 // Every packet truncation is safe, including impossible IPv4/TCP header sizes.
 for(unsigned n=0;n<40;n++)observe(p,n,0,9);
 p[32]=0xf0;observe(p,40,0,9);assert(flows[0].packets==2);
 puts("PASS observer checksum, corruption, truncation, fragments, tuple filter, TCP lifecycle/interface separation");
 return 0;
}
