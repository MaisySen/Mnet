package au.fintex.mnet;

import android.media.projection.MediaProjection;
import android.os.Handler;

// Register before virtual-display creation. Framework stop and app cleanup can
// race; notify the service once and do not recursively stop on an app close.
final class ProjectionGuard implements AutoCloseable {
    private final MediaProjection projection;
    private final Runnable stopped;
    private boolean closed;
    private final MediaProjection.Callback callback = new MediaProjection.Callback() {
        @Override
        public void onStop() {
            synchronized (ProjectionGuard.this) {
                if (closed) return;
                closed = true;
            }
            projection.unregisterCallback(this);
            stopped.run();
        }
    };

    ProjectionGuard(MediaProjection projection, Handler handler, Runnable stopped) {
        if (projection == null) throw new IllegalStateException("Screen capture consent is unavailable. Start hosting again.");
        this.projection = projection;
        this.stopped = stopped;
        projection.registerCallback(callback, handler);
    }

    @Override
    public void close() {
        synchronized (this) {
            if (closed) return;
            closed = true;
        }
        projection.unregisterCallback(callback);
        projection.stop();
    }
}
