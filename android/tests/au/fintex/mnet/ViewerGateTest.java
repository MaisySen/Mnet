package au.fintex.mnet;

public final class ViewerGateTest {
    public static void main(String[] args) throws Exception {
        ViewerGate<Object> gate = new ViewerGate<>();
        ViewerGate.Ticket first = gate.begin();
        if (first == null || gate.begin() != null) throw new AssertionError("overlapping connects");
        gate.end(first);
        ViewerGate.Ticket next = gate.begin();
        Object current = new Object();
        if (gate.publish(first, new Object()) || gate.active(first)) throw new AssertionError("stale publication/callback");
        if (!gate.publish(next, current) || gate.end(first) != null || gate.current() != current) throw new AssertionError("replacement isolation");
        if (gate.close() != current || gate.close() != null || gate.begin() != null || gate.publish(next, new Object())) throw new AssertionError("permanent close");
        for (int i = 0; i < 1000; i++) {
            ViewerGate<Object> raced = new ViewerGate<>();
            ViewerGate.Ticket ticket = raced.begin();
            Object offered = new Object();
            java.util.concurrent.atomic.AtomicBoolean published = new java.util.concurrent.atomic.AtomicBoolean();
            java.util.concurrent.atomic.AtomicReference<Object> removed = new java.util.concurrent.atomic.AtomicReference<>();
            Thread a = new Thread(() -> published.set(raced.publish(ticket, offered)));
            Thread b = new Thread(() -> removed.set(raced.close()));
            a.start(); b.start(); a.join(); b.join();
            if (raced.current() != null || raced.active(ticket) || published.get() != (removed.get() == offered)) throw new AssertionError("publish/destroy ownership");
        }
        System.out.println("PASS viewer attempt exclusivity, stale callbacks/publication, close and1000 publish/destroy races; JVM only");
    }
}
