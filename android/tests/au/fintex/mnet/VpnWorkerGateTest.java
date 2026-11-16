package au.fintex.mnet;

import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;

public final class VpnWorkerGateTest {
    public static void main(String[] args) throws Exception {
        VpnWorkerGate gate = new VpnWorkerGate();
        VpnWorkerGate.Ticket first = gate.begin();
        if (!gate.active(first) || gate.begin() != null) throw new AssertionError("duplicate worker");
        gate.cancel(first);
        if (gate.active(first) || gate.begin() != null) throw new AssertionError("cancel released ownership before exit");
        gate.finish(first);
        VpnWorkerGate.Ticket next = gate.begin();
        gate.cancel(first);
        gate.finish(first);
        if (!gate.active(next) || gate.active(first) || gate.begin() != null) throw new AssertionError("stale worker altered replacement");
        gate.finish(next);
        for (int round = 0; round < 100; round++) {
            CountDownLatch start = new CountDownLatch(1);
            CountDownLatch acquired = new CountDownLatch(16);
            AtomicInteger winners = new AtomicInteger();
            Thread[] threads = new Thread[16];
            for (int i = 0; i < threads.length; i++) {
                threads[i] = new Thread(() -> {
                    try {
                        start.await();
                        VpnWorkerGate.Ticket ticket = gate.begin();
                        if (ticket != null) winners.incrementAndGet();
                        acquired.countDown();
                        acquired.await();
                        if (ticket != null) gate.finish(ticket);
                    } catch (InterruptedException e) { throw new AssertionError(e); }
                });
                threads[i].start();
            }
            start.countDown();
            for (Thread thread : threads) thread.join();
            if (winners.get() != 1) throw new AssertionError("concurrent worker ownership failed");
        }
        System.out.println("PASS Android worker ownership cancellation/stale ticket/100 x16 contenders; JVM only");
    }
}
