//go:build multies_native_fixture

package control

// Opt-in actual Multies coordinator participant. Private files contain only
// disposable local fixture inputs; this is not an installed production policy.
import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	contract "fintex.au/mnet/internal/enrollmentcontract"
)

func TestActualMultiesStoreSourceFixture(t *testing.T) {
	dir := os.Getenv("MNET_NATIVE_FIXTURE_DIR")
	if dir == "" {
		t.Skip("explicit disposable Multies coordinator required")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "registrar-fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Config   registrarSourceConfig         `json:"config"`
		Request  contract.SourceReserveRequest `json:"request"`
		Exchange contract.SourceExchange       `json:"exchange"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal("invalid disposable registrar fixture")
	}
	path := filepath.Join(dir, "control", "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if fixture.Config.Sources == nil || len(fixture.Config.Sources) != 1 {
		t.Fatal("missing independent service config")
	}
	configRaw, _ := json.Marshal(fixture.Config)
	if err := os.WriteFile(s.ownedPath+".registrar-sources-v1.json", configRaw, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.ownedPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	data, err := s.reserveRegistrarSource(context.Background(), "fixture", fixture.Request)
	if err != nil {
		t.Fatal("actual source reserve refused", err)
	}
	service := fixture.Config.Sources[0]
	// Independent native observation comes from completed own-PKCE, not fabricated
	// from this service response. The two sides must name the SAME source row.
	raw, err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := contract.ServiceReserveDescriptorDigest(fixture.Request, service.ServiceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contract.DecodeSourceReserve(raw, fixture.Exchange, service.ServiceID, fixture.Request.ReserveID, expected, fixture.Request.Mapping, time.Now()); err != nil {
		t.Fatal("service/native source row didn't match", err)
	}
	if _, err := s.reserveRegistrarSource(context.Background(), "fixture", fixture.Request); err == nil {
		t.Fatal("spent service request repeated")
	}
	after, _ := os.ReadFile(s.ownedPath)
	if !bytes.Equal(before, after) || len(s.data.Nodes) != 0 || s.data.Registrar != nil {
		t.Fatal("source reservation DATA mutated Store")
	}
	// Result has no ticket or secret. Coordinator uses it only to compare source
	// row/descriptor and assert no authority. Real reserve contract stays DATA.
	if err := os.WriteFile(filepath.Join(dir, "registrar-outcome.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
