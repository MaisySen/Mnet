package mobile;
public class SessionConnect {
 private final Node node;
 public volatile boolean canceled;
 public SessionConnect(Node owner){node=owner;}
 public SessionViewer connectWithControl(String peer,ViewerCallbacks cb,ControlCallbacks control){
  if(canceled)throw new IllegalStateException("canceled");
  SessionViewer v=node.connectSessionWithControl(peer,cb,control);
  if(canceled){v.close();throw new IllegalStateException("canceled");}
  return v;
 }
 public void cancel(){canceled=true;}
}
