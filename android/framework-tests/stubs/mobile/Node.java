package mobile;
public class Node {
 public SessionConnect lastConnect;
 public SessionConnect newSessionConnect(){lastConnect=new SessionConnect(this);return lastConnect;}
 public boolean closed; public int statusReads;
 public String prepare(String server,String network,String name,String key){return "100.64.0.2";}
 public void start(long fd,String server,String network,String name,String key){}
 public String status(){statusReads++;return "{\"status\":\"up\",\"dns_ready\":true,\"ipv4\":\"100.64.0.2\"}";}
 public void close(){closed=true;}
 public void startSessionHost(HostCallbacks c){}
 public void startSessionHostWithControl(HostCallbacks c,ControlCallbacks control){}
 public String wireSession="fixture-session",wireRequest="fixture-request",wireState="unavailable";public long wireRevision=1;
 public String sessionControlStatus(){return "{\"session\":\""+wireSession+"\",\"request\":\""+wireRequest+"\",\"revision\":"+wireRevision+",\"state\":\""+wireState+"\"}";}
 public void decideSessionControl(String session,String request,boolean allow){if(!wireSession.equals(session)||!wireRequest.equals(request)||!(wireState.equals("pending")||wireState.equals("paused")))throw new IllegalStateException();wireState=allow?"allowed":"declined";wireRevision++;}
 public void pauseSessionControl(String session,String request){if(!wireState.equals("allowed"))throw new IllegalStateException();wireState="paused";wireRevision++;}
 public void revokeSessionControl(String session,String request){wireState="revoked";wireRevision++;}
 public void stopSessionHost(){}
 public SessionViewer connectSession(String peer,ViewerCallbacks callback){return new SessionViewer();}
 public SessionViewer connectSessionWithControl(String peer,ViewerCallbacks callback,ControlCallbacks control){return connectSession(peer,callback);}
}
