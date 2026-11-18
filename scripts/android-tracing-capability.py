#!/usr/bin/env python3
"""One empty-image tracing feasibility check. No Mnet/peer, no real user state."""
import gzip
import json
import os
from pathlib import Path
import subprocess
import time

SDK=Path(os.environ['ANDROID_HOME'])
ADB=[str(SDK/'platform-tools/adb'),'-s','emulator-5554']
OUT=Path(os.environ['MNET_CAPABILITY_OUTPUT'])

def command(args,timeout=20):
    return subprocess.run([str(x) for x in args],capture_output=True,timeout=timeout)
def adb(*args,timeout=20):
    return command(ADB+list(args),timeout)
def shell(*args,timeout=20):
    return adb('shell',*args,timeout=timeout)
def text(result):
    return result.stdout.decode(errors='replace') if result.returncode==0 else ''
def exists(path):
    return shell('test','-r',path).returncode==0

def main():
    if os.environ.get('GITHUB_ACTIONS')!='true': raise RuntimeError('disposable CI only')
    report={'root':False,'security_policy_unchanged':True,'tracefs':{},'hooks':{},'events':{}}
    private_log=open('/tmp/mnet-capability-emulator.log','wb')
    emulator=subprocess.Popen([str(SDK/'emulator/emulator'),'-avd','mnet-capability','-port','5554','-no-window','-no-audio','-no-boot-anim','-no-snapshot','-gpu','swiftshader','-memory','2048','-cores','2'],stdout=private_log,stderr=subprocess.STDOUT)
    # Emulator stdout is deliberately not exported; process exit is recorded.
    hooks=[]
    instance=None
    try:
        deadline=time.monotonic()+300
        while time.monotonic()<deadline:
            if emulator.poll() is not None: raise RuntimeError('emulator exited')
            if text(shell('getprop','sys.boot_completed',timeout=8)).strip()=='1': break
            time.sleep(1)
        else: raise RuntimeError('boot deadline')
        policy_before=text(shell('getenforce')).strip()
        adb('root');adb('wait-for-device',timeout=30)
        report['root']=text(shell('id','-u')).strip()=='0'
        report['selinux_enforcing_before']=policy_before=='Enforcing'
        compiler=Path(os.environ['ANDROID_NDK_HOME'])/'toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android24-clang'
        built=command([compiler,'-Wall','-Wextra','-Werror','-O2','scripts/android-tracing-capability.c','-o','/tmp/mnet-trace-capability'])
        report['helper_compiled']=built.returncode==0
        if built.returncode: return
        adb('push','/tmp/mnet-trace-capability','/data/local/tmp/mnet-trace-capability')
        shell('chmod','700','/data/local/tmp/mnet-trace-capability')
        for mode in ('bpf','ptrace'):
            value=shell('/data/local/tmp/mnet-trace-capability',mode)
            try:
                row=json.loads(text(value))
                report[mode]={k:v for k,v in row.items() if k in ('load_allowed','permission_denied','unsupported','errno','child_trace_allowed') and type(v) in (bool,int)}
            except ValueError: report[mode]={'result_unavailable':True}
        report['kernel_btf_readable']=exists('/sys/kernel/btf/vmlinux')
        config=adb('exec-out','cat','/proc/config.gz')
        if config.returncode==0:
            try:
                contents=gzip.decompress(config.stdout).decode()
                report['config']={name:('CONFIG_'+name+'=y') in contents for name in ('BPF','BPF_SYSCALL','BPF_EVENTS','KPROBES','KPROBE_EVENTS','FTRACE','FTRACE_SYSCALLS','DEBUG_INFO_BTF','TRACEPOINTS')}
            except (OSError,UnicodeError): report['config_unavailable']=True
        else: report['config_unavailable']=True
        trace=None
        for candidate in ('/sys/kernel/tracing','/sys/kernel/debug/tracing'):
            if exists(candidate+'/available_events'):
                trace=candidate;break
        report['tracefs']['readable']=trace is not None
        if trace is None:return
        available=text(shell('cat',trace+'/available_events')).splitlines()
        formats={}
        for name in ('syscalls:sys_enter_sendto','syscalls:sys_exit_sendto','syscalls:sys_enter_setsockopt','syscalls:sys_exit_setsockopt','skb:kfree_skb','net:net_dev_queue','net:netif_receive_skb','sock:inet_sock_set_state'):
            present=name in available
            fmt=text(shell('cat',trace+'/events/'+name.replace(':','/')+'/format')) if present else ''
            report['events'][name]={'available':present,'format_readable':bool(fmt),'has_drop_reason':' reason;' in fmt or ' reason[' in fmt,'has_socket_cookie':'cookie' in fmt,'has_mark':'mark' in fmt,'has_uid':'uid' in fmt}
            formats[name]=fmt
        instance=trace+'/instances/mnet_capability'
        created=shell('mkdir',instance)
        report['tracefs']['instance_created']=created.returncode==0
        if created.returncode:return
        functions=text(shell('cat',trace+'/available_filter_functions'))
        names={line.split()[0] for line in functions.splitlines() if line.split()}
        # Only own unique probes on known kernel function names; never a security
        # policy change. Remove each one and the isolated instance in finally.
        for function in ('udp_sendmsg','security_socket_sendmsg','ip_route_output_flow','tun_get_user'):
            row={'function_listed':function in names}
            name='mnet_cap_'+function
            declaration='r:kprobes/'+name+' '+function+' result=$retval'
            result=shell('sh','-c','printf "%s\\n" "$1" >> "$2"','probe',declaration,trace+'/kprobe_events')
            row['registered']=result.returncode==0
            if row['registered']:
                hooks.append((name,trace))
                enabled=shell('sh','-c','printf 1 > "$1"','probe',instance+'/events/kprobes/'+name+'/enable')
                row['enabled_in_instance']=enabled.returncode==0
            report['hooks'][function]=row
        for name in ('syscalls:sys_enter_sendto','syscalls:sys_exit_sendto','skb:kfree_skb'):
            if name in available:
                report['events'][name]['enabled_in_instance']=shell('sh','-c','printf 1 > "$1"','probe',instance+'/events/'+name.replace(':','/')+'/enable').returncode==0
        shell('sh','-c','printf 1 > "$1"','probe',instance+'/tracing_on')
        synthetic=shell('/data/local/tmp/mnet-trace-capability','traffic')
        shell('sh','-c','printf 0 > "$1"','probe',instance+'/tracing_on')
        # Do not upload raw trace: only event presence/return classifications.
        content=text(shell('cat',instance+'/trace'))
        report['synthetic_loopback_send_succeeded']=synthetic.returncode==0
        for function,row in report['hooks'].items():
            lines=[x for x in content.splitlines() if 'mnet_cap_'+function+':' in x]
            row['recorded_return']=bool(lines)
            row['zero_return_seen']=any('result=0x0' in x or 'result=0 ' in x or x.endswith('result=0') for x in lines)
        for name,row in report['events'].items():
            row['recorded']=name.split(':')[1]+':' in content
        report['selinux_enforcing_after']=text(shell('getenforce')).strip()=='Enforcing'
        report['security_policy_unchanged']=policy_before==text(shell('getenforce')).strip()
    except (OSError,RuntimeError,subprocess.TimeoutExpired) as error:
        report['error_type']=type(error).__name__
    finally:
        if instance is not None:
            shell('sh','-c','printf 0 > "$1"','probe',instance+'/tracing_on')
            shell('sh','-c','printf 0 > "$1"','probe',instance+'/events/enable')
            shell('rmdir',instance)
        for name,trace in hooks:
            shell('sh','-c','printf "%s\\n" "$1" >> "$2"','probe','-:kprobes/'+name,trace+'/kprobe_events')
        OUT.parent.mkdir(parents=True,exist_ok=True)
        OUT.write_text(json.dumps(report,indent=2))
        emulator.terminate()
        try:emulator.wait(timeout=15)
        except subprocess.TimeoutExpired:emulator.kill();emulator.wait(timeout=5)
        private_log.close()
if __name__=='__main__':main()
