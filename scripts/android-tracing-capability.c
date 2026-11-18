// Empty disposable image capability probe, never loaded into Mnet.
#include <errno.h>
#include <linux/bpf.h>
#include <netinet/in.h>
#include <stdio.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ptrace.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
int main(int argc,char**argv){
 if(argc!=2)return 2;
 if(!strcmp(argv[1],"bpf")){
  struct bpf_insn insns[]={
   {.code=BPF_ALU64|BPF_MOV|BPF_K,.dst_reg=BPF_REG_0,.imm=0},
   {.code=BPF_JMP|BPF_EXIT}
  };
  union bpf_attr attr={0};attr.prog_type=BPF_PROG_TYPE_KPROBE;attr.insn_cnt=2;
  attr.insns=(unsigned long)insns;attr.license=(unsigned long)"GPL";
  int fd=syscall(__NR_bpf,BPF_PROG_LOAD,&attr,sizeof(attr));int error=errno;
  printf("{\"load_allowed\":%s,\"permission_denied\":%s,\"unsupported\":%s,\"errno\":%d}\n",fd>=0?"true":"false",error==EPERM||error==EACCES?"true":"false",error==ENOSYS||error==EOPNOTSUPP?"true":"false",fd>=0?0:error);
  if(fd>=0)close(fd);return 0;
 }
 if(!strcmp(argv[1],"ptrace")){
  pid_t child=fork();if(child<0)return 2;
  if(!child){if(ptrace(PTRACE_TRACEME,0,0,0)<0)_exit(2);raise(SIGSTOP);_exit(0);}
  int status;waitpid(child,&status,0);int ok=WIFSTOPPED(status);
  if(ok){ptrace(PTRACE_DETACH,child,0,0);waitpid(child,&status,0);}
  printf("{\"child_trace_allowed\":%s}\n",ok?"true":"false");return 0;
 }
 if(!strcmp(argv[1],"traffic")){
  printf("%d\n",getpid());fflush(stdout);sleep(2);
  int fd=socket(AF_INET,SOCK_DGRAM,0);if(fd<0)return 2;
  struct sockaddr_in to={.sin_family=AF_INET,.sin_port=htons(9),.sin_addr={.s_addr=htonl(INADDR_LOOPBACK)}};
  int result=sendto(fd,"c",1,0,(struct sockaddr*)&to,sizeof(to));close(fd);
  return result==1?0:3;
 }
 return 2;
}
