package au.fintex.mnet;

import java.util.UUID;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

// A dialog can answer only its exact request. Each worker waits on its own
// immutable request, never on a process-global latch that another run replaced.
final class SessionConsent {
    static final class Request {
        final String id = UUID.randomUUID().toString();
        final Object owner;
        final CountDownLatch answer = new CountDownLatch(1);
        boolean accepted;
        boolean resolved;
        Request(Object owner) { this.owner = owner; }
    }
    private Request current;

    synchronized Request begin(Object owner) {
        if (current != null) return null;
        current = new Request(owner);
        return current;
    }
    synchronized boolean pending(String id) {
        return id != null && current != null && current.id.equals(id) && !current.resolved;
    }
    synchronized void answer(String id, boolean accept) {
        if (!pending(id)) return;
        current.accepted = accept;
        current.resolved = true;
        current.answer.countDown();
    }
    boolean await(Request request, long timeout, TimeUnit unit) throws InterruptedException {
        try {
            if (!request.answer.await(timeout, unit)) return false;
            synchronized (this) {
                return current == request && request.resolved && request.accepted;
            }
        } finally {
            synchronized (this) {
                if (current == request) current = null;
            }
        }
    }
    synchronized void cancel(Object owner) {
        if (current == null || current.owner != owner) return;
        Request canceled = current;
        current = null;
        canceled.accepted = false;
        canceled.resolved = true;
        canceled.answer.countDown();
    }
}
