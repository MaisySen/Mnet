package au.fintex.mnet;

import android.content.*;
import android.database.Cursor;
import android.net.Uri;
import android.os.*;
import java.io.*;
import java.nio.file.Files;
import java.util.concurrent.*;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.annotation.Config;
import org.robolectric.shadows.ShadowContentResolver;
import static org.junit.Assert.*;

@RunWith(RobolectricTestRunner.class)
@Config(manifest=Config.NONE,sdk={24,25,26,33})
public class DocumentExportTest {
    static final Uri URI=Uri.parse("content://mnet.fixture/new-document");
    FakeProvider provider;ContentResolver resolver;File target;
    @Before public void setup()throws Exception {
        target=File.createTempFile("mnet-provider-", ".bin");target.deleteOnExit();
        provider=new FakeProvider(target);
        android.content.pm.ProviderInfo info=new android.content.pm.ProviderInfo();
        info.authority="mnet.fixture";
        provider.attachInfo(RuntimeEnvironment.getApplication(),info);
        ShadowContentResolver.registerProviderInternal("mnet.fixture",provider);
        resolver=RuntimeEnvironment.getApplication().getContentResolver();
    }
    @After public void cleanup()throws Exception {
        if(provider.last!=null)provider.last.close();
        if(provider.reader!=null)provider.reader.close();
        Files.deleteIfExists(target.toPath());
    }
    interface Work {void run()throws Exception;}
    void refused(Work work)throws Exception{try{work.run();fail("accepted invalid export");}catch(IOException e){assertEquals("Document export unavailable",e.getMessage());}}
    @Test public void exactBytesAndOneUseCompletion()throws Exception {
        DocumentExport export=new DocumentExport(resolver,URI,3);
        export.open();refused(export::open);
        export.write(new byte[]{1,2,3},0,3);export.finish();
        assertArrayEquals(new byte[]{1,2,3},Files.readAllBytes(target.toPath()));
        assertFalse(provider.last.getFileDescriptor().valid());
        refused(export::finish);export.cancel();export.cancel();
        assertArrayEquals(new byte[]{1,2,3},Files.readAllBytes(target.toPath()));
        assertEquals("w",provider.mode);
    }
    @Test public void shortAndOverflowCannotFinish()throws Exception {
        for(boolean overflow:new boolean[]{false,true}){
            DocumentExport export=new DocumentExport(resolver,URI,2);export.open();
            if(overflow)refused(()->export.write(new byte[]{1,2,3},0,3));else export.write(new byte[]{1},0,1);
            refused(export::finish);assertFalse(provider.last.getFileDescriptor().valid());
        }
    }
    @Test public void nullAndFailedOpenStayClosed()throws Exception {
        for(String mode:new String[]{"null","failure"}){
            provider.behavior=mode;DocumentExport export=new DocumentExport(resolver,URI,0);
            refused(export::open);refused(export::finish);export.cancel();
        }
    }
    @Test public void cancelBeforeOpenNeverTouchesProvider()throws Exception {
        DocumentExport export=new DocumentExport(resolver,URI,0);export.cancel();
        refused(export::open);assertEquals(0,provider.calls);
    }
    @Test public void cancelClosesAndBlocksFurtherBytes()throws Exception {
        DocumentExport export=new DocumentExport(resolver,URI,3);export.open();export.cancel();
        assertFalse(provider.last.getFileDescriptor().valid());
        refused(()->export.write(new byte[]{1},0,1));refused(export::finish);
    }
    @Test public void lateIgnoringProviderOpenIsReleased()throws Exception {
        provider.entered=new CountDownLatch(1);provider.release=new CountDownLatch(1);
        DocumentExport export=new DocumentExport(resolver,URI,0);
        ExecutorService worker=Executors.newSingleThreadExecutor();
        try{
            Future<Boolean> future=worker.submit(()->{try{export.open();return false;}catch(IOException e){return true;}});
            assertTrue(provider.entered.await(2,TimeUnit.SECONDS));export.cancel();provider.release.countDown();
            assertTrue(future.get(2,TimeUnit.SECONDS));
            assertNotNull(provider.last);assertFalse(provider.last.getFileDescriptor().valid());
        }finally{provider.release.countDown();worker.shutdownNow();}
    }
    @Test public void simulatedPipeOutputDoesNotRequestSeekOrSync()throws Exception {
        provider.behavior="pipe";DocumentExport export=new DocumentExport(resolver,URI,3);export.open();
        export.write(new byte[]{4,5,6},0,3);export.finish();
        try(InputStream input=new ParcelFileDescriptor.AutoCloseInputStream(provider.reader)){
            assertEquals(4,input.read());assertEquals(5,input.read());assertEquals(6,input.read());assertEquals(-1,input.read());
        }
    }
    @Test public void invalidLocalSelectionsNeverOpenProvider()throws Exception {
        for(Uri uri:new Uri[]{null,Uri.parse("file:///private"),Uri.parse("https://example.invalid/file")}){
            try{new DocumentExport(resolver,uri,0);fail("non-content selection accepted");}catch(IllegalArgumentException expected){}
        }
        try{new DocumentExport(resolver,URI,-1);fail("negative size");}catch(IllegalArgumentException expected){}
        assertEquals(0,provider.calls);
    }
    @Test public void streamConstructionFailuresCloseAcquiredDescriptor()throws Exception {
        for(boolean returnsNull:new boolean[]{false,true}) {
            DocumentExport export=new DocumentExport(resolver,URI,0,descriptor->{
                if(returnsNull)return null;
                throw new IOException("simulated private construction failure");
            });
            refused(export::open);assertFalse(provider.last.getFileDescriptor().valid());
            refused(export::open);
        }
    }
    @Test public void writeAndCloseFailuresReleaseDescriptor()throws Exception {
        for(boolean failWrite:new boolean[]{false,true}) {
            DocumentExport export=new DocumentExport(resolver,URI,1,descriptor->new FilterOutputStream(
                new ParcelFileDescriptor.AutoCloseOutputStream(descriptor)) {
                @Override public void write(byte[] bytes,int offset,int count)throws IOException {
                    if(failWrite)throw new IOException("simulated private write failure");
                    out.write(bytes,offset,count);
                }
                @Override public void close()throws IOException {throw new IOException("simulated private close failure");}
            });
            export.open();
            if(failWrite)refused(()->export.write(new byte[]{1},0,1));
            else export.write(new byte[]{1},0,1);
            refused(export::finish);assertFalse(provider.last.getFileDescriptor().valid());
        }
    }
    @Test public void cancellationDuringNoncooperativeWriteCannotFinish()throws Exception {
        CountDownLatch entered=new CountDownLatch(1),release=new CountDownLatch(1);
        DocumentExport export=new DocumentExport(resolver,URI,1,descriptor->new FilterOutputStream(
            new ParcelFileDescriptor.AutoCloseOutputStream(descriptor)) {
            @Override public void write(byte[] bytes,int offset,int count)throws IOException {
                entered.countDown();try{release.await();}catch(InterruptedException e){throw new IOException(e);}
                // Explicit fake that ignores descriptor close, not an actual provider guarantee.
            }
        });
        export.open();ExecutorService worker=Executors.newSingleThreadExecutor();
        try {
            Future<Boolean> result=worker.submit(()->{try{export.write(new byte[]{1},0,1);return false;}catch(IOException e){return true;}});
            assertTrue(entered.await(2,TimeUnit.SECONDS));export.cancel();release.countDown();
            assertTrue(result.get(2,TimeUnit.SECONDS));refused(export::finish);
            assertFalse(provider.last.getFileDescriptor().valid());
        }finally{release.countDown();worker.shutdownNow();}
    }
    @Test public void cancelDuringLateCloseCannotReportSuccess()throws Exception {
        CountDownLatch entered=new CountDownLatch(1),release=new CountDownLatch(1);
        DocumentExport export=new DocumentExport(resolver,URI,0,descriptor->new FilterOutputStream(
            new ParcelFileDescriptor.AutoCloseOutputStream(descriptor)) {
            @Override public void close()throws IOException {
                entered.countDown();try{release.await();}catch(InterruptedException e){throw new IOException(e);}
                out.close();
            }
        });
        export.open();ExecutorService worker=Executors.newSingleThreadExecutor();
        try {
            Future<Boolean> result=worker.submit(()->{try{export.finish();return false;}catch(IOException e){return true;}});
            assertTrue(entered.await(2,TimeUnit.SECONDS));export.cancel();release.countDown();
            assertTrue(result.get(2,TimeUnit.SECONDS));assertFalse(provider.last.getFileDescriptor().valid());
        }finally{release.countDown();worker.shutdownNow();}
    }
    @Test public void readOnlyDescriptorWriteFailureCannotFinish()throws Exception {
        provider.behavior="read-only";
        DocumentExport export=new DocumentExport(resolver,URI,1);export.open();
        refused(()->export.write(new byte[]{1},0,1));refused(export::finish);
        assertFalse(provider.last.getFileDescriptor().valid());
    }
    static class FakeProvider extends ContentProvider {
        File target;ParcelFileDescriptor last,reader;int calls;String mode,behavior="file";CountDownLatch entered,release;
        FakeProvider(File file){target=file;}
        @Override public ParcelFileDescriptor openFile(Uri uri,String mode)throws FileNotFoundException{
            calls++;this.mode=mode;
            if(entered!=null){entered.countDown();try{release.await();}catch(InterruptedException e){throw new FileNotFoundException();}}
            if("null".equals(behavior))return null;
            if("failure".equals(behavior))throw new FileNotFoundException("private provider details");
            if("pipe".equals(behavior)){try{ParcelFileDescriptor[] pipe=ParcelFileDescriptor.createPipe();reader=pipe[0];return last=pipe[1];}catch(IOException e){throw new FileNotFoundException();}}
            if("read-only".equals(behavior))return last=ParcelFileDescriptor.open(target,ParcelFileDescriptor.MODE_READ_ONLY);
            return last=ParcelFileDescriptor.open(target,ParcelFileDescriptor.MODE_WRITE_ONLY|ParcelFileDescriptor.MODE_TRUNCATE);
        }
        @Override public boolean onCreate(){return true;}
        @Override public Cursor query(Uri u,String[] p,String s,String[] a,String o){return null;}
        @Override public String getType(Uri u){return "application/octet-stream";}
        @Override public Uri insert(Uri u,ContentValues v){return null;}
        @Override public int update(Uri u,ContentValues v,String s,String[] a){return 0;}
        @Override public int delete(Uri u,String s,String[] a){return 0;}
    }
}
