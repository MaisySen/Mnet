package au.fintex.mnet;

import android.app.Activity;
import android.content.Intent;
import android.content.pm.ProviderInfo;
import android.net.Uri;
import android.os.Bundle;
import android.os.Looper;
import java.io.File;
import java.io.IOException;
import java.nio.file.Files;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import org.junit.*;
import org.junit.runner.RunWith;
import org.robolectric.*;
import org.robolectric.android.controller.ActivityController;
import org.robolectric.annotation.Config;
import org.robolectric.shadows.ShadowActivity;
import org.robolectric.shadows.ShadowContentResolver;
import static org.junit.Assert.*;
import static org.robolectric.Shadows.shadowOf;

@RunWith(RobolectricTestRunner.class)
@Config(manifest=Config.NONE,sdk={24,25,26,33})
public class DocumentSaveActivityTest {
    File file;
    DocumentExportTest.FakeProvider provider;
    List<DocumentSaveActivity.Pending> requests=new ArrayList<>();
    static final String DIGEST="ab".repeat(32);
    @Before public void setup()throws Exception {
        file=File.createTempFile("mnet-selection-", ".bin");
        provider=new DocumentExportTest.FakeProvider(file);
        ProviderInfo info=new ProviderInfo();info.authority="mnet.fixture";
        provider.attachInfo(RuntimeEnvironment.getApplication(),info);
        ShadowContentResolver.registerProviderInternal("mnet.fixture",provider);
    }
    @After public void cleanup()throws Exception {
        for(DocumentSaveActivity.Pending pending:requests)pending.cancel();
        if(provider.last!=null)provider.last.close();
        Files.deleteIfExists(file.toPath());
    }
    static class Answer implements DocumentSaveActivity.Answer {
        int calls;DocumentSaveActivity.Pending request;DocumentExport output;
        @Override public void selected(DocumentSaveActivity.Pending request,DocumentExport output){calls++;this.request=request;this.output=output;}
    }
    DocumentSaveActivity.Pending prepare(Answer answer) {
        DocumentSaveActivity.Pending request=DocumentSaveActivity.prepare(
            new DocumentSaveActivity.Offer("session","request",17,"example.bin",1,DIGEST),20_000,answer);
        assertNotNull(request);requests.add(request);return request;
    }
    ActivityController<DocumentSaveActivity> launch(DocumentSaveActivity.Pending request) {
        Intent intent=request.launchIntent(RuntimeEnvironment.getApplication());assertNotNull(intent);
        return Robolectric.buildActivity(DocumentSaveActivity.class,intent).create().start().resume();
    }
    Intent result(){return new Intent().setData(Uri.parse("content://mnet.fixture/new-document"));}
    @Test public void realCreateDocumentIntentAndUnopenedOutput()throws Exception {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        ActivityController<DocumentSaveActivity> activity=launch(request);
        ShadowActivity.IntentForResult launched=shadowOf(activity.get()).getNextStartedActivityForResult();
        assertEquals(Intent.ACTION_CREATE_DOCUMENT,launched.intent.getAction());
        assertTrue(launched.intent.hasCategory(Intent.CATEGORY_OPENABLE));
        assertEquals("application/octet-stream",launched.intent.getType());
        assertEquals("example.bin",launched.intent.getStringExtra(Intent.EXTRA_TITLE));
        assertEquals(0,launched.intent.getFlags());assertEquals(0,provider.calls);
        activity.get().onActivityResult(launched.requestCode,Activity.RESULT_OK,result());
        assertEquals(1,answer.calls);assertSame(request,answer.request);assertNotNull(answer.output);
        assertEquals(0,provider.calls);assertFalse(activity.get().isFinishing());
        answer.output.open();answer.output.write(new byte[]{7},0,1);answer.output.finish();
        assertArrayEquals(new byte[]{7},Files.readAllBytes(file.toPath()));
        request.cancel();assertTrue(activity.get().isFinishing());
        activity.pause().stop().destroy();
    }
    @Test public void wrongCodeCannotConsumeAndDuplicatesCannotReplay() {
        Answer answer=new Answer();ActivityController<DocumentSaveActivity> activity=launch(prepare(answer));
        activity.get().onActivityResult(42,Activity.RESULT_OK,result());assertEquals(0,answer.calls);
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(1,answer.calls);
        activity.pause().stop().destroy();
    }
    @Test public void canceledRequestCannotApproveIdenticalNewOffer()throws Exception {
        Answer old=new Answer(),fresh=new Answer();DocumentSaveActivity.Pending first=prepare(old);
        ActivityController<DocumentSaveActivity> stale=launch(first);first.cancel();
        ActivityController<DocumentSaveActivity> current=launch(prepare(fresh));
        stale.get().onActivityResult(1,Activity.RESULT_OK,result());
        assertEquals(1,old.calls);assertNull(old.output);assertEquals(0,fresh.calls);assertEquals(0,provider.calls);
        current.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(1,fresh.calls);assertNotNull(fresh.output);
        stale.pause().stop().destroy();current.pause().stop().destroy();
    }
    @Test public void expiredSelectionCannotExport() {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        ActivityController<DocumentSaveActivity> activity=launch(request);
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(21));
        assertEquals(1,answer.calls);assertNull(answer.output);
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(1,answer.calls);assertEquals(0,provider.calls);
        activity.pause().stop().destroy();
    }
    @Test public void resultRejectsDenialEmptyAndNonContentUri() {
        for(int variant=0;variant<4;variant++) {
            Answer answer=new Answer();ActivityController<DocumentSaveActivity> activity=launch(prepare(answer));
            Intent data=variant==1?null:variant==2?new Intent():new Intent().setData(Uri.parse("file:///private"));
            activity.get().onActivityResult(1,variant==0?Activity.RESULT_CANCELED:Activity.RESULT_OK,data);
            assertEquals(1,answer.calls);assertNull(answer.output);assertEquals(0,provider.calls);
            activity.pause().stop().destroy();
        }
    }
    @Test public void destroyBeforeAnswerRetiresSelection() {
        Answer answer=new Answer();ActivityController<DocumentSaveActivity> activity=launch(prepare(answer));
        activity.pause().stop().destroy();assertEquals(1,answer.calls);assertNull(answer.output);
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(1,answer.calls);assertEquals(0,provider.calls);
    }
    @Test public void savedInstanceNeverRestoresConsent() {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        Intent intent=request.launchIntent(RuntimeEnvironment.getApplication());
        ActivityController<DocumentSaveActivity> activity=Robolectric.buildActivity(DocumentSaveActivity.class,intent).create(new Bundle());
        assertTrue(activity.get().isFinishing());assertNull(shadowOf(activity.get()).getNextStartedActivityForResult());
        assertEquals(1,answer.calls);assertNull(answer.output);activity.destroy();
    }
    @Test public void recreatedActivityCannotClaimRetiredLaunch() {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        Intent intent=request.launchIntent(RuntimeEnvironment.getApplication());
        ActivityController<DocumentSaveActivity> original=Robolectric.buildActivity(DocumentSaveActivity.class,intent).create().start().resume();
        assertNotNull(shadowOf(original.get()).getNextStartedActivityForResult());
        Bundle saved=new Bundle();original.saveInstanceState(saved).pause().stop().destroy();
        ActivityController<DocumentSaveActivity> recreated=Robolectric.buildActivity(DocumentSaveActivity.class,intent).create(saved);
        assertTrue(recreated.get().isFinishing());assertNull(shadowOf(recreated.get()).getNextStartedActivityForResult());
        recreated.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(1,answer.calls);assertNull(answer.output);
        assertEquals(0,provider.calls);recreated.destroy();
    }
    @Test public void duplicateInstanceCannotStealOneLaunch() {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        Intent intent=request.launchIntent(RuntimeEnvironment.getApplication());
        ActivityController<DocumentSaveActivity> first=Robolectric.buildActivity(DocumentSaveActivity.class,intent).create();
        assertNotNull(shadowOf(first.get()).getNextStartedActivityForResult());
        ActivityController<DocumentSaveActivity> duplicate=Robolectric.buildActivity(DocumentSaveActivity.class,intent).create();
        assertTrue(duplicate.get().isFinishing());assertNull(shadowOf(duplicate.get()).getNextStartedActivityForResult());
        duplicate.destroy();assertEquals(0,answer.calls);
        first.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(1,answer.calls);assertNotNull(answer.output);
        first.destroy();
    }
    @Test public void unavailableOrForeignHandleCannotLaunchPicker() {
        ActivityController<DocumentSaveActivity> activity=Robolectric.buildActivity(DocumentSaveActivity.class,new Intent()).create();
        assertTrue(activity.get().isFinishing());assertNull(shadowOf(activity.get()).getNextStartedActivityForResult());
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());assertEquals(0,provider.calls);activity.destroy();
    }
    @Test public void localSelectionHasOneLaunchAndBoundedExpiry() {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        assertNotNull(request.launchIntent(RuntimeEnvironment.getApplication()));
        assertNull(request.launchIntent(RuntimeEnvironment.getApplication()));
        for(int i=1;i<8;i++)prepare(new Answer());
        assertNull(DocumentSaveActivity.prepare(request.offer,20_000,new Answer()));
        assertNull(DocumentSaveActivity.prepare(request.offer,20_001,new Answer()));
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(21));
        assertNotNull(prepare(new Answer()));assertEquals(1,answer.calls);assertNull(answer.output);
    }
    @Test public void ownerCancellationAfterHandoffClosesOutput()throws Exception {
        Answer answer=new Answer();DocumentSaveActivity.Pending request=prepare(answer);
        ActivityController<DocumentSaveActivity> activity=launch(request);
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());answer.output.open();
        request.cancel();assertFalse(provider.last.getFileDescriptor().valid());
        try{answer.output.write(new byte[]{1},0,1);fail("canceled output writable");}catch(IOException expected){}
        assertEquals(1,answer.calls);activity.pause().stop().destroy();
    }
    @Test public void destructionAfterSelectionCancelsUnopenedOutput()throws Exception {
        Answer answer=new Answer();ActivityController<DocumentSaveActivity> activity=launch(prepare(answer));
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());assertNotNull(answer.output);
        activity.pause().stop().destroy();
        try{answer.output.open();fail("destroyed grant owner usable");}catch(IOException expected){}
        assertEquals(0,provider.calls);assertEquals(1,answer.calls);
    }
    @Test public void selectedOutputHoldsBoundedSlotUntilOwnerRelease() {
        List<ActivityController<DocumentSaveActivity>> activities=new ArrayList<>();
        DocumentSaveActivity.Pending first=null;
        for(int i=0;i<8;i++) {
            DocumentSaveActivity.Pending request=prepare(new Answer());if(first==null)first=request;
            ActivityController<DocumentSaveActivity> activity=launch(request);activities.add(activity);
            activity.get().onActivityResult(1,Activity.RESULT_OK,result());
        }
        assertNull(DocumentSaveActivity.prepare(first.offer,20_000,new Answer()));
        first.cancel();assertNotNull(prepare(new Answer()));
        for(ActivityController<DocumentSaveActivity> activity:activities)activity.pause().stop().destroy();
    }
    @Test public void failedHandoffCallbackClosesUnopenedOutput() {
        final DocumentExport[] received={null};
        DocumentSaveActivity.Pending request=DocumentSaveActivity.prepare(
            new DocumentSaveActivity.Offer("session","request",17,"example.bin",1,DIGEST),20_000,
            (owned,output)->{received[0]=output;throw new IllegalStateException("simulated unavailable owner");});
        requests.add(request);ActivityController<DocumentSaveActivity> activity=launch(request);
        activity.get().onActivityResult(1,Activity.RESULT_OK,result());assertNotNull(received[0]);
        try{received[0].open();fail("abandoned output usable");}catch(IOException expected){}
        assertEquals(0,provider.calls);activity.pause().stop().destroy();
    }
}
