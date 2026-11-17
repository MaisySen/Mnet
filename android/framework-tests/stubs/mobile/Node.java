package mobile;
public class Node {
 public boolean closed; public int statusReads;
 public String prepare(String server,String network,String name,String key){return "100.64.0.2";}
 public void start(long fd,String server,String network,String name,String key){}
 public String status(){statusReads++;return "{\"status\":\"up\",\"dns_ready\":true,\"ipv4\":\"100.64.0.2\"}";}
 public void close(){closed=true;}
 public void startSessionHost(HostCallbacks c){}
 public void stopSessionHost(){}
 public SessionViewer connectSession(String peer,ViewerCallbacks callback){return new SessionViewer();}
}
