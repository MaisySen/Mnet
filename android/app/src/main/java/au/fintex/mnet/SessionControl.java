package au.fintex.mnet;

import java.util.UUID;

// Viewing never grants input. Each local decision has a one-use handle;
// opening newer host controls invalidates earlier dialogs, even for one viewer.
final class SessionControl {
    static final class Decision {
        final String id;
        final boolean wasAllowed;
        Decision(String id, boolean wasAllowed) { this.id=id;this.wasAllowed=wasAllowed; }
    }
    private String session;
    private boolean allowed;
    synchronized String begin() { session=UUID.randomUUID().toString();allowed=false;return session; }
    synchronized String current() { return session; }
    synchronized boolean allowed() { return session!=null&&allowed; }
    synchronized Decision openDecision() {
        if(session==null)return null;
        boolean previous=allowed;
        allowed=false;
        session=UUID.randomUUID().toString();
        return new Decision(session,previous);
    }
    synchronized boolean decide(String id,boolean value) {
        if(id==null||!id.equals(session))return false;
        allowed=value;
        session=UUID.randomUUID().toString();
        return true;
    }
    synchronized void end(){session=null;allowed=false;}
}
