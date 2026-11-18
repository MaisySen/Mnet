package mobile;
public class SessionViewer {
 public final java.util.List<String> input=new java.util.concurrent.CopyOnWriteArrayList<>();
 public volatile boolean closed;
 public void sendInputJSON(String json){input.add(json);}
 public void sendChat(String text){}
 public volatile String controlState="unavailable";
 public int requests,cancels;
 public String controlStatus(){return "{\"state\":\""+controlState+"\"}";}
 public void requestControl(){requests++;controlState="pending";}
 public void cancelControlRequest(){cancels++;controlState="declined";}
 public void close(){closed=true;}
}
