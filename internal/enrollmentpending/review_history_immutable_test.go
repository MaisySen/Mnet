package enrollmentpending

import (
 "context"
 "encoding/json"
 "encoding/pem"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
 contract "fintex.au/mnet/internal/enrollmentcontract"
)

func TestReviewHistoryCannotReplacePreviouslyPinnedHistory(t *testing.T) {
 if os.Getenv("REVIEW_HISTORY_CHILD") != "1" {
  cmd:=exec.Command(os.Args[0],"-test.run=^TestReviewHistoryCannotReplacePreviouslyPinnedHistory$","-test.v")
  cmd.Env=append(os.Environ(),"REVIEW_HISTORY_CHILD=1")
  out,err:=cmd.CombinedOutput(); if err!=nil {t.Fatalf("child: %v\n%s",err,out)}; return
 }
 var c contract.Challenge
 calls:=0
 s:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++
  node:="first-node"; if calls>1 {node="different-node"}
  receipt,err:=contract.ReceiptV2ForChallenge(c,node,code(),c.Binding.CheckedAtMS+1); if err!=nil {t.Error(err)}
  w.Header().Set("Content-Type","application/json");w.Header().Set("Cache-Control","no-store")
  json.NewEncoder(w).Encode(contract.HistoryReceiptWire{Version:1,Receipt:receipt,ReceiptDigest:contract.ReceiptV2Digest(receipt)})
 }))
 defer s.Close()
 cert:=filepath.Join(t.TempDir(),"root.pem");os.WriteFile(cert,pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:s.Certificate().Raw}),0600)
 t.Setenv("SSL_CERT_FILE",cert);t.Setenv("SSL_CERT_DIR",t.TempDir())
 j,challenge,cfg:=historyReady(t,s.URL);c=challenge
 dir:=j.owner.Parent()
 first,err:=j.RecoverHistoryDATA(context.Background(),c.Binding.DevicePublicKey,func()SourceConfig{return cfg},time.Second,2)
 if err!=nil {t.Fatal(err)}
 j.Close(); j,err=Open(dir);if err!=nil {t.Fatal(err)};defer j.Close()
 second,err:=j.RecoverHistoryDATA(context.Background(),c.Binding.DevicePublicKey,func()SourceConfig{return cfg},time.Second,2)
 if err==nil && first.Receipt!=second.Receipt {t.Fatalf("IMMUTABILITY FAILURE: second lookup replaced durable first history receipt with different receipt; calls=%d",calls)}
}
