package mobile;
public class Node {
 public void startSessionHost(HostCallbacks c){}
 public void stopSessionHost(){}
 public SessionViewer connectSession(String peer,ViewerCallbacks callback){return new SessionViewer();}
}
