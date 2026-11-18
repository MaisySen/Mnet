// Disposable emulator observer. No payloads, addresses, keys or raw logs leave
// this process. Only the fixture's DNS id and TCP port are observed.
#include <arpa/inet.h>
#include <errno.h>
#include <linux/if_ether.h>
#include <linux/if_packet.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>
static volatile sig_atomic_t stopped;
static void stop(int sig){(void)sig;stopped=1;}
static unsigned u16(const unsigned char*p){return ((unsigned)p[0]<<8)|p[1];}
static uint32_t u32(const unsigned char*p){return ((uint32_t)u16(p)<<16)|u16(p+2);}
static unsigned checksum(const unsigned char*p,size_t n,uint32_t sum){while(n>=2){sum+=u16(p);p+=2;n-=2;}if(n)sum+=(unsigned)*p<<8;while(sum>>16)sum=(sum&65535)+(sum>>16);return (~sum)&65535;}
static long ms(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return t.tv_sec*1000L+t.tv_nsec/1000000;}
struct flow{unsigned port,dir,ifindex;unsigned packets,repeated,bytes,syn,fin,rst,zero_window;uint32_t last_seq,last_ack;long first_ms,last_ms;};
static struct flow flows[32];static unsigned nflows,dns_events;static long start;
// input counters are system-wide within a disposable emulator, not attributed
// to one packet. Correlate with the narrow packet trace, never call them proof.
static void counters(const char*phase){
 FILE*f=fopen("/proc/net/snmp","r");if(!f){printf("{\"event\":\"kernel_udp\",\"readable\":false}\n");return;}
 char names[4096],values[4096];
 while(fgets(names,sizeof(names),f)){if(!strncmp(names,"Udp:",4)&&fgets(values,sizeof(values),f)){
  char*ns=NULL,*vs=NULL;char*n=strtok_r(names," \n",&ns),*v=strtok_r(values," \n",&vs);
  printf("{\"event\":\"kernel_udp\",\"phase\":\"%s\",\"readable\":true",phase);
  while((n=strtok_r(NULL," \n",&ns))&&(v=strtok_r(NULL," \n",&vs))){
   if(!strcmp(n,"InDatagrams")||!strcmp(n,"NoPorts")||!strcmp(n,"InErrors")||!strcmp(n,"RcvbufErrors")||!strcmp(n,"InCsumErrors")){unsigned long long x=0;if(sscanf(v,"%llu",&x)==1)printf(",\"%s\":%llu",n,x);}
  }puts("}");break;
 }}
 fclose(f);fflush(stdout);
}
static void observe(const unsigned char*p,size_t n,int outgoing,int ifindex){
 if(n<20||p[0]>>4!=4)return;unsigned h=(p[0]&15)*4,total=u16(p+2);
 if(h<20||total>n||total<h||(u16(p+6)&0x3fff))return;
 const unsigned char*q=p+h;unsigned len=total-h;
 if(p[9]==17&&len>=20&&(u16(q)==53||u16(q+2)==53)&&u16(q+8)==0x4d4e&&dns_events<128){
  unsigned udp_len=u16(q+4);if(udp_len<20||udp_len>len)return;
  uint32_t pseudo=u16(p+12)+u16(p+14)+u16(p+16)+u16(p+18)+17+udp_len;
  printf("{\"event\":\"dns_wire\",\"ms\":%ld,\"outgoing\":%s,\"ifindex\":%d,\"reply\":%s,\"port\":%u,\"ip_checksum_ok\":%s,\"udp_checksum_ok\":%s,\"udp_checksum_present\":%s,\"bytes\":%u}\n",ms(),outgoing?"true":"false",ifindex,(u16(q+10)&0x8000)?"true":"false",u16(q)==53?u16(q+2):u16(q),checksum(p,h,0)==0?"true":"false",checksum(q,udp_len,pseudo)==0?"true":"false",u16(q+6)?"true":"false",udp_len);dns_events++;fflush(stdout);
 }
 if(p[9]==6&&len>=20&&(u16(q)==45993||u16(q+2)==45993)){
  unsigned th=(q[12]>>4)*4;if(th<20||th>len)return;
  unsigned dir=u16(q)==45993,port=dir?u16(q+2):u16(q);struct flow*f=NULL;
  for(unsigned i=0;i<nflows;i++)if(flows[i].dir==dir&&flows[i].port==port&&flows[i].ifindex==(unsigned)ifindex){f=&flows[i];break;}
  if(!f&&nflows<32){f=&flows[nflows++];f->port=port;f->dir=dir;f->ifindex=(unsigned)ifindex;f->first_ms=ms();}
  if(!f)return;uint32_t seq=u32(q+4);if(f->packets&&seq==f->last_seq&&len>th)f->repeated++;
  f->last_ms=ms();f->packets++;f->bytes+=len-th;f->last_seq=seq;f->last_ack=u32(q+8);f->syn+=(q[13]&2)!=0;f->fin+=(q[13]&1)!=0;f->rst+=(q[13]&4)!=0;f->zero_window+=u16(q+14)==0;
 }
}
#ifndef OBSERVER_TEST
int main(void){
 signal(SIGTERM,stop);signal(SIGINT,stop);start=ms();
 int fd=socket(AF_PACKET,SOCK_DGRAM,htons(ETH_P_IP));
 if(fd<0){printf("{\"event\":\"capture_capability\",\"available\":false,\"denied\":%s}\n",errno==EPERM||errno==EACCES?"true":"false");return 2;}
 int receive_buffer=1<<20;setsockopt(fd,SOL_SOCKET,SO_RCVBUF,&receive_buffer,sizeof(receive_buffer));
 struct timeval timeout={1,0};setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&timeout,sizeof(timeout));
 puts("{\"event\":\"capture_capability\",\"available\":true}");counters("before");
 unsigned char buf[65536];while(!stopped&&ms()-start<900000){struct sockaddr_ll addr;socklen_t size=sizeof(addr);ssize_t n=recvfrom(fd,buf,sizeof(buf),0,(struct sockaddr*)&addr,&size);if(n>0)observe(buf,(size_t)n,addr.sll_pkttype==PACKET_OUTGOING,addr.sll_ifindex);else if(n<0&&errno!=EAGAIN&&errno!=EWOULDBLOCK&&errno!=EINTR)break;}
 counters("after");for(unsigned i=0;i<nflows;i++){struct flow*f=&flows[i];printf("{\"event\":\"tcp_flow\",\"port\":%u,\"from_peer\":%s,\"packets\":%u,\"payload_bytes\":%u,\"same_last_sequence\":%u,\"syn\":%u,\"fin\":%u,\"rst\":%u,\"zero_window\":%u,\"last_seq\":%u,\"last_ack\":%u,\"ifindex\":%u,\"first_ms\":%ld,\"last_ms\":%ld}\n",f->port,f->dir?"true":"false",f->packets,f->bytes,f->repeated,f->syn,f->fin,f->rst,f->zero_window,f->last_seq,f->last_ack,f->ifindex,f->first_ms,f->last_ms);}
 struct tpacket_stats stats={0};socklen_t stats_size=sizeof(stats);int stats_ok=getsockopt(fd,SOL_PACKET,PACKET_STATISTICS,&stats,&stats_size)==0;
 printf("{\"event\":\"capture_closed\",\"complete\":true,\"stats_readable\":%s,\"capture_packets\":%u,\"capture_drops\":%u}\n",stats_ok?"true":"false",stats.tp_packets,stats.tp_drops);close(fd);return 0;
}
#endif
