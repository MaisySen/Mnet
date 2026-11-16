package au.fintex.mnet;

// Pure lifecycle state. A canceled attempt cannot publish or deliver callbacks
// into a replacement. The owner closes rejected/removed handles outside locks.
final class ViewerGate<T> {
    static final class Ticket {}
    private Ticket current;
    private T value;
    private boolean closed;
    synchronized Ticket begin() {
        if (closed || current != null) return null;
        current = new Ticket();
        return current;
    }
    synchronized boolean active(Ticket ticket) {
        return !closed && ticket != null && current == ticket;
    }
    synchronized boolean publish(Ticket ticket, T handle) {
        if (!active(ticket) || value != null) return false;
        value = handle;
        return true;
    }
    synchronized T current() { return value; }
    synchronized T end(Ticket ticket) {
        if (!active(ticket)) return null;
        T previous = value;
        value = null;
        current = null;
        return previous;
    }
    synchronized T close() {
        closed = true;
        T previous = value;
        value = null;
        current = null;
        return previous;
    }
}
