package au.fintex.mnet.fixture;

// Fixture predicates only. No app authority, UI action or transport behavior.
final class ControlAcceptanceState {
 static boolean fontViewingReady(Object before,Object current,float scale,int frameWidth,
                                 String state,int inputs,boolean visibleBounds){
  return current!=null&&current!=before&&Math.abs(scale-1.3f)<0.01f&&frameWidth>0
      &&"unavailable".equals(state)&&inputs==0&&visibleBounds;
 }
 static boolean admissionFailed(String state){
  return "no-authorized-peer".equals(state)||"tcp-dial-failed".equals(state)||"hello-failed".equals(state);
 }
}
