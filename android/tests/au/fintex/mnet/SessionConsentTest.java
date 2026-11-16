package au.fintex.mnet;

import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

public final class SessionConsentTest {
    public static void main(String[] args) throws Exception {
        SessionConsent broker = new SessionConsent();
        Object oldOwner = new Object(), newOwner = new Object();
        SessionConsent.Request old = broker.begin(oldOwner);
        if (broker.begin(newOwner) != null) throw new AssertionError("overlapping prompt");
        broker.cancel(oldOwner);
        SessionConsent.Request next = broker.begin(newOwner);
        broker.answer(old.id, true);
        broker.cancel(oldOwner);
        if (!broker.pending(next.id)) throw new AssertionError("stale dialog/stop changed current request");
        if (broker.await(old, 1, TimeUnit.MILLISECONDS)) throw new AssertionError("canceled old worker accepted");
        if (!broker.pending(next.id)) throw new AssertionError("old await cleared replacement");
        broker.answer(next.id, true);
        broker.answer(next.id, false);
        if (!broker.await(next, 1, TimeUnit.SECONDS)) throw new AssertionError("exact first answer lost");
        SessionConsent.Request expired = broker.begin(newOwner);
        if (broker.await(expired, 1, TimeUnit.MILLISECONDS)) throw new AssertionError("timeout accepted");
        broker.answer(expired.id, true);
        if (broker.pending(expired.id)) throw new AssertionError("expired dialog revived");
        for (int i=0; i<1000; i++) {
            SessionConsent b = new SessionConsent();
            SessionConsent.Request r = b.begin(oldOwner);
            Thread answer = new Thread(() -> b.answer(r.id, true));
            Thread cancel = new Thread(() -> b.cancel(oldOwner));
            answer.start(); cancel.start(); answer.join(); cancel.join();
            if (b.await(r, 1, TimeUnit.MILLISECONDS)) throw new AssertionError("cancel did not invalidate accepted result");
        }
        System.out.println("PASS exact consent request/owner, stale dialog/worker isolation, timeout/first-answer and1000 answer/cancel races; JVM only");
    }
}
