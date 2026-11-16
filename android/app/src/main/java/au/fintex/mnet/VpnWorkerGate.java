package au.fintex.mnet;

// Cancellation does not release ownership. Only the exiting worker can do so.
// This prevents a replacement worker from sharing the same saved identity.
final class VpnWorkerGate {
    static final class Ticket {
        private boolean cancelled;
    }
    private Ticket owner;
    synchronized Ticket begin() {
        if (owner != null) return null;
        owner = new Ticket();
        return owner;
    }
    synchronized boolean active(Ticket ticket) {
        return ticket != null && owner == ticket && !ticket.cancelled;
    }
    synchronized void cancel(Ticket ticket) {
        if (owner == ticket && ticket != null) ticket.cancelled = true;
    }
    synchronized void finish(Ticket ticket) {
        if (owner == ticket) owner = null;
    }
}
