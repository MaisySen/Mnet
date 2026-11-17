package au.fintex.mnet;

import java.util.UUID;

// One hosting attempt's input permission. View acceptance creates a fresh
// handle, never input authority. Late dialogs cannot affect the next viewer.
final class SessionControl {
    private String session;
    private boolean allowed;
    synchronized String begin() { session=UUID.randomUUID().toString();allowed=false;return session; }
    synchronized String current() { return session; }
    synchronized boolean allowed() { return session!=null&&allowed; }
    synchronized boolean decide(String id,boolean value) {
        if(id==null||!id.equals(session))return false;
        allowed=value;return true;
    }
    synchronized void end(){session=null;allowed=false;}
}
