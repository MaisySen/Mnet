// Synthetic sockets and own temporary TUN only. No app/account/peer state.
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <linux/if_tun.h>
#include <net/if.h>
#include <netinet/in.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <unistd.h>
static void put16(unsigned char*p,unsigned n){p[0]=n>>8;p[1]=n;}
static unsigned csum(unsigned char*p,unsigned len){unsigned s=0;for(unsigned i=0;i+1<len;i+=2)s+=((unsigned)p[i]<<8)|p[i+1];if(len&1)s+=(unsigned)p[len-1]<<8;while(s>>16)s=(s&65535)+(s>>16);return (~s)&65535;}
static int bound(const char*ip,unsigned*port){int fd=socket(AF_INET,SOCK_DGRAM,0);if(fd<0)return -1;struct sockaddr_in a={.sin_family=AF_INET};inet_pton(AF_INET,ip,&a.sin_addr);if(bind(fd,(void*)&a,sizeof(a))){close(fd);return -1;}socklen_t n=sizeof(a);getsockname(fd,(void*)&a,&n);*port=ntohs(a.sin_port);struct timeval t={1,0};setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&t,sizeof(t));return fd;}
static void packet(unsigned char*p,unsigned port,int corrupt){memset(p,0,35);p[0]=0x45;put16(p+2,35);p[8]=64;p[9]=17;inet_pton(AF_INET,"198.18.0.53",p+12);inet_pton(AF_INET,"198.18.0.54",p+16);put16(p+10,csum(p,20));put16(p+20,45997);put16(p+22,port);put16(p+24,15);memcpy(p+28,"tunjoin",7);unsigned char pseudo[28]={0};memcpy(pseudo,p+12,8);pseudo[9]=17;put16(pseudo+10,15);memcpy(pseudo+12,p+20,15);put16(p+26,csum(pseudo,27));if(corrupt)p[34]^=1;}
int main(void){
 unsigned source=0,dest=0;int sender=bound("127.0.0.1",&source),receiver=bound("127.0.0.1",&dest);if(sender<0||receiver<0)return 2;
 unsigned mark=0;socklen_t marklen=sizeof(mark);int mark_read=getsockopt(sender,SOL_SOCKET,SO_MARK,&mark,&marklen)==0;
 struct sockaddr_in a={.sin_family=AF_INET,.sin_port=htons(dest),.sin_addr={.s_addr=htonl(INADDR_LOOPBACK)}};
 int sent=sendto(sender,"joinudp",7,0,(void*)&a,sizeof(a));char buffer[32];int received=recv(receiver,buffer,sizeof(buffer),0);
 printf("{\"pid\":%d,\"udp_source\":%u,\"udp_dest\":%u,\"mark_read\":%s,\"mark\":%u,\"udp_sent\":%d,\"udp_received\":%d",getpid(),source,dest,mark_read?"true":"false",mark,sent,received);
 close(sender);close(receiver);
 int tun=open("/dev/tun",O_RDWR|O_NONBLOCK);if(tun<0)tun=open("/dev/net/tun",O_RDWR|O_NONBLOCK);
 struct ifreq req={0};strcpy(req.ifr_name,"mnetcap0");req.ifr_flags=IFF_TUN|IFF_NO_PI;
 int created=tun>=0&&ioctl(tun,TUNSETIFF,&req)==0;
 printf(",\"tun_created\":%s",created?"true":"false");
 if(created){
  // Only this helper's nonpersistent interface, removed by closing its fd.
  int configured=system("ip addr add 198.18.0.54/32 dev mnetcap0 && ip link set mnetcap0 up")==0;
  printf(",\"tun_configured\":%s",configured?"true":"false");
  unsigned port=0;int sock=configured?bound("198.18.0.54",&port):-1;
  if(sock>=0){unsigned char p[35];packet(p,port,0);int wrote=write(tun,p,sizeof(p));int got=recv(sock,buffer,sizeof(buffer),0);packet(p,port,1);int badwrite=write(tun,p,sizeof(p));int badgot=recv(sock,buffer,sizeof(buffer),0);int baderr=errno;
   printf(",\"tun_dest\":%u,\"tun_valid_written\":%d,\"tun_valid_received\":%d,\"tun_corrupt_written\":%d,\"tun_corrupt_received\":%d,\"tun_corrupt_timeout\":%s",port,wrote,got,badwrite,badgot,badgot<0&&(baderr==EAGAIN||baderr==EWOULDBLOCK)?"true":"false");close(sock);}
 }
 if(tun>=0)close(tun);puts("}");return 0;
}
