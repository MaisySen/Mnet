package mobile;
public class Mobile {
 public static volatile java.util.function.Supplier<Node> factory=Node::new;
 public static Node newNode(String path){return factory.get();}
}
