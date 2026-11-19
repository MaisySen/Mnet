package au.fintex.mnet;

import android.content.ContentResolver;
import android.net.Uri;
import android.os.CancellationSignal;
import android.os.ParcelFileDescriptor;
import java.io.IOException;
import java.io.OutputStream;

// Disabled preparation: package-private, no Activity/service/Go caller. The URI
// must eventually come from an exact current local picker decision, never a peer.
// Java owns the PFD throughout; no getFd/detachFd or URI-to-path conversion.
final class DocumentExport {
    // Narrow output construction seam for fault tests. The normal constructor
    // always uses Android's descriptor-owning stream.
    interface OutputFactory { OutputStream create(ParcelFileDescriptor descriptor) throws IOException; }
    private final OutputFactory outputs;
    private final ContentResolver resolver;
    private final Uri uri;
    private final long expected;
    private final CancellationSignal signal = new CancellationSignal();
    private boolean opened, canceled, finished;
    private ParcelFileDescriptor descriptor;
    private OutputStream output;
    private long written;

    DocumentExport(ContentResolver resolver, Uri uri, long expected) {
        this(resolver, uri, expected, ParcelFileDescriptor.AutoCloseOutputStream::new);
    }

    DocumentExport(ContentResolver resolver, Uri uri, long expected, OutputFactory outputs) {
        if (resolver == null || outputs == null || uri == null || !"content".equals(uri.getScheme()) || expected < 0)
            throw new IllegalArgumentException("Invalid local document selection");
        this.resolver = resolver; this.uri = uri; this.expected = expected; this.outputs = outputs;
    }

    void open() throws IOException {
        synchronized (this) {
            if (opened || canceled) throw unavailable();
            opened = true;
        }
        ParcelFileDescriptor selected = null;
        try {
            // A provider can return a pipe or null. No seek/sync/atomic rename is
            // assumed. The selected document must be new, not an existing target.
            selected = resolver.openFileDescriptor(uri, "w", signal);
            synchronized (this) {
                if (selected == null || canceled) throw unavailable();
                descriptor = selected;
                output = outputs.create(selected);
                if (output == null) throw unavailable();
                selected = null;
            }
        } catch (Exception failure) {
            cancel();
            throw unavailable(); // do not expose provider/URI/error details
        } finally {
            if (selected != null) closeFailed(selected);
        }
    }

    // Single owner worker invokes write/finish; cancellation may be concurrent.
    void write(byte[] bytes, int offset, int count) throws IOException {
        if (bytes == null || offset < 0 || count < 0 || offset > bytes.length - count)
            throw new IndexOutOfBoundsException();
        OutputStream owned;
        synchronized (this) {
            owned = canceled || finished || output == null || count > expected - written ? null : output;
        }
        if (owned == null) { cancel(); throw unavailable(); }
        try {
            owned.write(bytes, offset, count);
            synchronized (this) {
                if (canceled) throw unavailable();
                written += count;
            }
        } catch (IOException | RuntimeException failure) { cancel(); throw unavailable(); }
    }

    void finish() throws IOException {
        OutputStream owned;
        synchronized (this) {
            owned = canceled || finished || output == null || written != expected ? null : output;
            if (owned != null) finished = true;
        }
        if (owned == null) { cancel(); throw unavailable(); }
        try {
            // Close reports observed local completion only, not cloud durability.
            owned.close();
            synchronized (this) {
                if (canceled) throw unavailable();
                descriptor = null; output = null;
            }
        } catch (IOException | RuntimeException failure) { cancel(); throw unavailable(); }
    }

    void cancel() {
        ParcelFileDescriptor owned;
        synchronized (this) {
            if (canceled || (finished && descriptor == null)) return;
            canceled = true;
            owned = descriptor;
            descriptor = null; output = null;
        }
        try { signal.cancel(); }
        catch (RuntimeException ignored) { /* a provider cancellation callback is best-effort */ }
        finally { if (owned != null) closeFailed(owned); }
    }

    private static void closeFailed(ParcelFileDescriptor descriptor) {
        try { descriptor.closeWithError("Mnet file export canceled"); }
        catch (IOException | RuntimeException ignored) { /* reliable error delivery is best-effort */ }
        finally { try { descriptor.close(); } catch (IOException | RuntimeException ignored) { } }
    }
    private static IOException unavailable() { return new IOException("Document export unavailable"); }
}
