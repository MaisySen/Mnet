#!/usr/bin/env python3
"""One minimal-VPN baseline, same disposable images, no Mnet engine or tracing."""
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import time

ROOT=Path(__file__).resolve().parents[1]
SDK=Path(os.environ['ANDROID_HOME'])
ANDROID=SDK/'platforms/android-36/android.jar'
BT=SDK/'build-tools/36.0.0'
APP='au.fintex.mnet.baseline'
FIXTURE=APP+'.fixture'

def run(args,timeout=90):
    p=subprocess.run([str(x) for x in args],capture_output=True,timeout=timeout)
    if p.returncode:raise RuntimeError('baseline child failed')
    return p.stdout

def build(root):
    source=ROOT/'android/baseline/au/fintex/mnet/baseline'
    classes=root/'classes';classes.mkdir()
    run(['javac','-source','11','-target','11','-cp',ANDROID,'-d',classes,*sorted(source.glob('*.java'))])
    # Current direct responder's independent wire regression, no Android APIs.
    run(['java','-cp',str(classes)+':'+str(ANDROID),'au.fintex.mnet.baseline.PacketDNS'])
    key=root/'debug.keystore'
    run(['keytool','-genkeypair','-keystore',key,'-storepass','android','-keypass','android','-alias','baseline','-keyalg','RSA','-keysize','2048','-validity','2','-dname','CN=Disposable baseline'])
    for kind,package,names in (
        ('app',APP,('DirectVpn','ConsentActivity','PacketDNS')),
        ('fixture',FIXTURE,('OwnerInstrumentation','IndependentInstrumentation','Probe','PacketDNS'))):
        folder=root/kind;folder.mkdir();dex=folder/'dex';dex.mkdir()
        permissions=''.join('<uses-permission android:name="android.permission.'+p+'"/>' for p in ('INTERNET','ACCESS_NETWORK_STATE')+ (('FOREGROUND_SERVICE','FOREGROUND_SERVICE_SPECIAL_USE','POST_NOTIFICATIONS') if kind=='app' else ()))
        if kind=='app':
            contents='<activity android:name="'+APP+'.ConsentActivity" android:exported="true"/><service android:name="'+APP+'.DirectVpn" android:exported="false" android:permission="android.permission.BIND_VPN_SERVICE" android:foregroundServiceType="specialUse"><property android:name="android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE" android:value="Disposable baseline VPN"/><intent-filter><action android:name="android.net.VpnService"/></intent-filter></service>'
            instruments=''
        else:
            contents='';instruments=''.join('<instrumentation android:name="'+APP+'.'+name+'" android:targetPackage="'+target+'"/>' for name,target in (('OwnerInstrumentation',APP),('IndependentInstrumentation',FIXTURE)))
        manifest=folder/'AndroidManifest.xml'
        manifest.write_text('<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="'+package+'"><uses-sdk android:minSdkVersion="24" android:targetSdkVersion="36"/>'+permissions+'<application android:label="Disposable baseline" android:debuggable="true" android:allowBackup="false" android:theme="@android:style/Theme.Material.Light.NoActionBar">'+contents+'</application>'+instruments+'</manifest>')
        run([BT/'aapt2','link','-o',folder/'base.apk','-I',ANDROID,'--manifest',manifest])
        selected=[]
        for name in names:selected+=sorted(classes.glob('au/fintex/mnet/baseline/'+name+'*.class'))
        run([BT/'d8','--min-api','24','--lib',ANDROID,'--classpath',classes,'--output',dex,*selected])
        shutil.copy(folder/'base.apk',folder/'raw.apk')
        subprocess.run(['zip','-q',str(folder/'raw.apk'),'classes.dex'],cwd=dex,check=True,capture_output=True)
        run([BT/'zipalign','-f','4',folder/'raw.apk',folder/'unsigned.apk'])
        run([BT/'apksigner','sign','--ks',key,'--ks-key-alias','baseline','--ks-pass','pass:android','--key-pass','pass:android','--out',folder/'signed.apk',folder/'unsigned.apk'])
    return root/'app/signed.apk',root/'fixture/signed.apk'

def main():
    if os.environ.get('GITHUB_ACTIONS')!='true':raise RuntimeError('disposable runner only')
    out=Path(os.environ['MNET_BASELINE_OUTPUT']);out.mkdir(parents=True,exist_ok=True)
    root=Path(tempfile.mkdtemp(prefix='mnet-baseline-'));os.chmod(root,0o700)
    report={'scope':'minimal-direct-VPN-baseline','mnet_readiness_unchanged':True,'results':[]};stage='build';emulator=None;owner=None
    adb=[SDK/'platform-tools/adb','-s','emulator-5554']
    def shell(*args,timeout=20):return run(adb+['shell',shlex.join(str(x) for x in args)],timeout)
    def wait(check,seconds):
        end=time.monotonic()+seconds
        while time.monotonic()<end:
            try:
                if check():return
            except (OSError,RuntimeError,subprocess.TimeoutExpired):pass
            time.sleep(.15)
        raise RuntimeError('bounded baseline await')
    def safe_row(row):
        safe={k:row[k] for k in ('role','phase') if row.get(k) in ('owner','independent','initial','reconnect')}
        for k in ('os_dns',):
            if type(row.get(k)) is bool:safe[k]=row[k]
        for k in ('fixture_queries_seen','fixture_replies_written'):
            if type(row.get(k)) is int:safe[k]=row[k]
        for k in ('original_udp','fresh_connected_contrast'):
            if type(row.get(k)) is dict:
                value=row[k];safe[k]={name:value[name] for name in ('passed','timeout','denied') if type(value.get(name)) is bool}
                if value.get('stage') in ('create','network-bind','source-bind','connect','send','receive','parse'):safe[k]['stage']=value['stage']
        return safe
    try:
        app,fixture=build(root);stage='boot'
        log=open(root/'emulator.log','wb')
        emulator=subprocess.Popen([str(SDK/'emulator/emulator'),'-avd','mnet-baseline','-port','5554','-no-window','-no-audio','-no-boot-anim','-no-snapshot','-gpu','swiftshader','-memory','2048','-cores','2'],stdout=log,stderr=subprocess.STDOUT)
        wait(lambda:shell('getprop','sys.boot_completed',timeout=8).strip()==b'1',300)
        before=shell('getenforce').strip();report['selinux_before_enforcing']=before==b'Enforcing'
        # No adb root, appops, permissions grants, security toggles or trace hooks.
        shell('input','keyevent','82');run(adb+['install','-r',app]);run(adb+['install','-r',fixture]);stage='owner-start'
        with open(root/'owner.log','wb') as f:
            owner=subprocess.Popen([str(x) for x in adb+['shell','am','instrument','-w','-r',FIXTURE+'/'+APP+'.OwnerInstrumentation']],stdout=f,stderr=subprocess.STDOUT)
            for phase in (0,1):
                stage='await-phase-'+str(phase)
                wait(lambda:owner.poll() is not None or shell('run-as',APP,'test','-f','files/ready-'+str(phase))==b'',90)
                if owner.poll() is not None:break
                stage='independent-phase-'+str(phase)
                independent=shell('am','instrument','-w','-r','-e','phase',str(phase),FIXTURE+'/'+APP+'.IndependentInstrumentation',timeout=60)
                report['independent_'+str(phase)+'_completed']=b'COMPLETE independent baseline' in independent
                row=json.loads(run(adb+['exec-out','run-as',FIXTURE,'cat','files/baseline-independent-'+str(phase)+'.json']))
                report['results'].append(safe_row(row));shell('run-as',APP,'touch','files/done-'+str(phase))
            stage='owner-finish';owner.wait(timeout=75)
        owner_output=(root/'owner.log').read_text(errors='replace')
        report['owner_completed']='COMPLETE minimal baseline' in owner_output
        rows=json.loads(run(adb+['exec-out','run-as',APP,'cat','files/baseline-result.json']))
        report['results'] += [safe_row(row) for row in rows[:2]]
        report['selinux_after_enforcing']=shell('getenforce').strip()==b'Enforcing';report['security_policy_unchanged']=before==shell('getenforce').strip();report['status']='complete' if report['owner_completed'] and len(report['results'])==4 else 'inconclusive'
    except (OSError,RuntimeError,ValueError,subprocess.TimeoutExpired) as error:report.update(status='inconclusive',stage=stage,error_type=type(error).__name__)
    finally:
        if owner is not None and owner.poll() is None:owner.terminate();owner.wait(timeout=5)
        if emulator is not None:
            emulator.terminate()
            try:emulator.wait(timeout=15)
            except subprocess.TimeoutExpired:emulator.kill();emulator.wait(timeout=5)
        (out/'report.json').write_text(json.dumps(report,indent=2));shutil.rmtree(root)
if __name__=='__main__':main()
