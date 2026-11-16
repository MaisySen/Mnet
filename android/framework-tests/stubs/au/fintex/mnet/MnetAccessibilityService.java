package au.fintex.mnet;
public class MnetAccessibilityService {
 public static boolean hosting;
 public static MnetAccessibilityService current;
 public int injections;
 public static void setHosting(boolean value){hosting=value;}
 public static MnetAccessibilityService instance(){return current;}
 public void inject(String json,float scale){injections++;}
}
