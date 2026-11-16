package au.fintex.mnet;
public final class ViewerTapTest {
 public static void main(String[] args) {
  ViewerTap tap=new ViewerTap();
  for(int i=0;i<1000;i++){
   tap.down(10,10);tap.move(11,11,4);
   if(!tap.up(11,11,4) || tap.up(11,11,4))throw new AssertionError("tap not exactly once");
  }
  tap.down(10,10);tap.cancel();if(tap.up(10,10,4))throw new AssertionError("canceled tap injected");
  tap.down(10,10);tap.move(20,20,4);tap.move(10,10,4);if(tap.up(10,10,4))throw new AssertionError("drag return injected");
  tap.down(10,10);if(tap.up(30,10,4))throw new AssertionError("moved release injected");
  System.out.println("PASS viewer single completed tap1000 repeats, duplicate-up/cancel/drag suppression; JVM only");
 }
}
