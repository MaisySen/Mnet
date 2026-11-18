package au.fintex.mnet.fixture;

public final class DNSProofStateTest {
    public static void main(String[] args) {
        DNSProof.phase="udp";DNSProof.step=4;
        DNSProof.sourceMesh=DNSProof.permissionDenied=DNSProof.wrongID=true;
        DNSProof.reset();
        String fresh=DNSProof.diagnostics();
        if(fresh.contains("=true")||!DNSProof.phase.equals("none")||DNSProof.step!=0||DNSProof.permissionDenied)throw new AssertionError("stale diagnostics survived reset");
        DNSProof.sourceMesh=DNSProof.permissionDenied=true;
        try { DNSProof.verify("unused",null,"invalid fixture address!");throw new AssertionError("expected invalid address"); }
        catch(java.io.IOException expected) { if(DNSProof.sourceMesh||DNSProof.permissionDenied)throw new AssertionError("verify did not reset"); }
        catch(Exception unexpected) { throw new AssertionError(unexpected); }
        for(int step=1;step<=7;step++){
            DNSProof.step=step;
            String diagnostic=DNSProof.diagnostics();
            int count=diagnostic.split("=true",-1).length-1;
            if(count!=1)throw new AssertionError("ambiguous operation stage");
        }
        DNSProof.phase="tcp";DNSProof.step=6;
        if(!DNSProof.diagnostics().contains("dnsNetworkBind=true")||!DNSProof.diagnostics().contains("dnsTCP=true"))throw new AssertionError("TCP bind misclassified");
        DNSProof.reset();
        System.out.println("PASS DNS diagnostic reset and distinct socket/network-bind/source-bind stages; JVM only");
    }
}
