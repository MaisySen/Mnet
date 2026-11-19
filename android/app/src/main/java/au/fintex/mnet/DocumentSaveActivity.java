package au.fintex.mnet;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

// Disabled native routing preparation. NOT in the manifest and no app/Go caller.
// One Activity instance owns one picker launch. Never reuse projection routing,
// restore approval, or derive a destination from remotely supplied URI/path text.
public final class DocumentSaveActivity extends Activity {
    private static final String EXTRA = "au.fintex.mnet.LOCAL_DOCUMENT_REQUEST";
    private static final int PICK_DOCUMENT = 1;
    private static final long MAX_WAIT_MS = 20_000;
    private static final int MAX_PENDING = 8;
    private static final Map<String, Pending> pending = new HashMap<>();
    private static final Handler main = new Handler(Looper.getMainLooper());

    // Snapshot must eventually come from the exact current Go approval request.
    // This disabled class does not authenticate it or bridge that decision.
    static final class Offer {
        final String session, request, name, sha256;
        final long stream, size;
        Offer(String session, String request, long stream, String name, long size, String sha256) {
            if (session == null || session.isEmpty() || request == null || request.isEmpty()
                    || stream <= 0 || stream > 0xffffffffL || name == null || name.isEmpty()
                    || name.length() > 255 || size < 0 || sha256 == null
                    || !sha256.matches("[a-fA-F0-9]{64}"))
                throw new IllegalArgumentException("Invalid local file offer");
            this.session = session; this.request = request; this.stream = stream;
            this.name = name; this.size = size; this.sha256 = sha256;
        }
        @Override public String toString() { return "local document offer"; }
    }
    interface Answer { void selected(Pending request, DocumentExport output); }

    static final class Pending {
        final Offer offer;
        private final String id = UUID.randomUUID().toString();
        private final long deadline;
        private Answer answer;
        private final Runnable expiry = this::cancel;
        private boolean intentCreated, attached, resolved;
        private DocumentExport output;
        private DocumentSaveActivity activity;
        private Pending(Offer offer, long wait, Answer answer) {
            this.offer = offer; this.answer = answer;
            deadline = SystemClock.elapsedRealtime() + wait;
        }
        Intent launchIntent(Context context) {
            synchronized (pending) {
                if (context == null || resolved || intentCreated || !current(this)) return null;
                intentCreated = true;
                return new Intent(context, DocumentSaveActivity.class).putExtra(EXTRA, id);
            }
        }
        // The future session owner retains this handle even after selection and
        // cancels it if Go rejects the exact decision or that session ends.
        // After successful export it also cancels/releases the handle. This
        // keeps the receiving Activity/grant lifetime alive until work is done.
        void cancel() { resolve(this, null); }
        @Override public String toString() { return "local document selection"; }
    }

    static Pending prepare(Offer offer, long waitMillis, Answer answer) {
        if (offer == null || answer == null || waitMillis <= 0 || waitMillis > MAX_WAIT_MS) return null;
        synchronized (pending) {
            if (pending.size() >= MAX_PENDING) return null;
            Pending request = new Pending(offer, waitMillis, answer);
            pending.put(request.id, request);
            main.postDelayed(request.expiry, waitMillis);
            return request;
        }
    }
    private static boolean current(Pending request) {
        return pending.get(request.id) == request && SystemClock.elapsedRealtime() < request.deadline;
    }
    private static void resolve(Pending request, DocumentExport selected) {
        Answer callback = null;
        DocumentExport cancel = null;
        DocumentSaveActivity finish = null;
        synchronized (pending) {
            if (request.resolved) {
                if (selected != null) cancel = selected;
                else {
                    cancel = request.output; request.output = null;
                    finish = request.activity; request.activity = null;
                    pending.remove(request.id);
                }
            } else {
                if (!current(request)) { cancel = selected; selected = null; }
                request.resolved = true;
                main.removeCallbacks(request.expiry);
                request.output = selected;
                callback = request.answer;
                request.answer = null; // do not retain a retired UI/service callback
                if (selected == null) {
                    pending.remove(request.id);
                    finish = request.activity; request.activity = null;
                }
                // Successful selection holds its bounded slot and Activity until
                // the session owner releases it. Do not finish away the grant.
            }
        }
        if (cancel != null) cancel.cancel();
        if (finish != null) {
            DocumentSaveActivity owned = finish;
            if (Looper.myLooper() == Looper.getMainLooper()) owned.finish();
            else main.post(owned::finish);
        }
        if (callback != null) {
            try { callback.selected(request, selected); }
            catch (RuntimeException failure) { request.cancel(); }
        }
    }

    private Pending request;
    private boolean pickerConsumed;
    @Override protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        String id = getIntent() == null ? null : getIntent().getStringExtra(EXTRA);
        synchronized (pending) {
            Pending candidate = pending.get(id);
            if (candidate != null && !candidate.attached && candidate.intentCreated && current(candidate)) {
                candidate.attached = true;
                candidate.activity = this;
                request = candidate;
            }
        }
        // No saved request/URI/permission or consent survives recreation. A new
        // process has an empty registry. A new offer needs a new local launch.
        if (saved != null || request == null) {
            if (request != null) request.cancel();
            finish(); return;
        }
        Intent picker = new Intent(Intent.ACTION_CREATE_DOCUMENT)
                .addCategory(Intent.CATEGORY_OPENABLE)
                .setType("application/octet-stream")
                .putExtra(Intent.EXTRA_TITLE, request.offer.name);
        // Do not request tree access or persistable grants. No output opens here.
        try { startActivityForResult(picker, PICK_DOCUMENT); }
        catch (RuntimeException failure) { request.cancel(); finish(); }
    }
    @Override protected void onActivityResult(int code, int result, Intent data) {
        super.onActivityResult(code, result, data);
        if (code != PICK_DOCUMENT || request == null || pickerConsumed) return;
        Pending owned = request;
        pickerConsumed = true; // consume before callback, including failure/empty result
        Uri uri = result == RESULT_OK && data != null ? data.getData() : null;
        if (uri == null || !"content".equals(uri.getScheme())) owned.cancel();
        else resolve(owned, new DocumentExport(getContentResolver(), uri, owned.offer.size));
        // Successful output retains this Activity; owner cancellation/release or
        // destruction closes it. Installed grant behavior remains unverified.
    }
    @Override protected void onDestroy() {
        if (request != null) { request.cancel(); request = null; }
        super.onDestroy();
    }
}
