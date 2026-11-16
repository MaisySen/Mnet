package mobile;
public class SessionViewer {
 public final java.util.List<String> input=new java.util.concurrent.CopyOnWriteArrayList<>();
 public volatile boolean closed;
 public void sendInputJSON(String json){input.add(json);}
 public void sendChat(String text){}
 public void close(){closed=true;}
}
