package au.fintex.mnet;
public final class SessionControlTest{
 public static void main(String[]args)throws Exception{
  SessionControl state=new SessionControl();if(state.allowed()||state.decide("old",true))throw new AssertionError("zero granted");
  String first=state.begin();if(state.allowed())throw new AssertionError("view grants control");
  if(!state.decide(first,true)||!state.allowed())throw new AssertionError("explicit control lost");
  if(!state.decide(first,false)||state.allowed())throw new AssertionError("revoke lost");
  String second=state.begin();if(first.equals(second)||state.decide(first,true)||state.allowed())throw new AssertionError("old dialog replay");
  state.end();if(state.decide(second,true)||state.allowed()||state.current()!=null)throw new AssertionError("end revived");
  for(int i=0;i<1000;i++){
   String id=state.begin();Thread allow=new Thread(()->state.decide(id,true));Thread end=new Thread(state::end);
   allow.start();end.start();allow.join();end.join();if(state.allowed())throw new AssertionError("end/allow race");
  }
  System.out.println("PASS session input separate approval/revoke, exact-dialog binding and1000end/allow races; JVM only");
 }
}
