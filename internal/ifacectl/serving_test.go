package ifacectl

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func fakeServing() (*Serving, *[][]string) {
	calls := [][]string{}
	s := &Serving{iface: "mnet0", tag: "mnet-fixture", readForwarding: func() ([]byte, error) { return []byte("1\n"), nil }, command: func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string{}, args...))
		return nil, nil
	}}
	return s, &calls
}
func TestServingScopedOwnershipAndCleanup(t *testing.T) {
	s, calls := fakeServing()
	if err := s.Start([]string{"10.9.0.7/24", "10.9.0.0/24", "192.168.2.0/24"}); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatal("duplicate route added")
	}
	additions := append([][]string{}, (*calls)...)
	for _, rule := range additions {
		joined := strings.Join(rule, " ")
		for _, must := range []string{"-s 100.64.0.0/10 -d ", "! -o mnet0", "--comment mnet-fixture", "-j MASQUERADE"} {
			if !strings.Contains(joined, must) {
				t.Fatal(joined)
			}
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for i, added := range additions {
		want := append([]string{}, added...)
		want[4] = "-D"
		if !reflect.DeepEqual(want, (*calls)[3-i]) {
			t.Fatalf("cleanup not exact owned rule: %v", *calls)
		}
	}
	if err := s.Close(); err != nil || len(*calls) != 4 {
		t.Fatal("close not idempotent")
	}
}
func TestServingValidationMakesNoChanges(t *testing.T) {
	for _, routes := range [][]string{nil, {"0.0.0.0/0"}, {"8.8.8.0/24"}, {"10.0.0.0/7"}, {"fd00::/64"}, {"bad"}} {
		s, calls := fakeServing()
		if err := s.Start(routes); err == nil || len(*calls) != 0 {
			t.Fatalf("invalid input changed NAT: %v", routes)
		}
	}
	s, calls := fakeServing()
	s.readForwarding = func() ([]byte, error) { return []byte("0"), nil }
	if err := s.Start([]string{"10.9.0.0/24"}); err == nil || len(*calls) != 0 {
		t.Fatal("disabled forwarding not rejected without mutation")
	}
}
func TestServingRollbackRetainsFailedCleanup(t *testing.T) {
	s, calls := fakeServing()
	base := s.command
	failAdd, failDelete := true, true
	s.command = func(args ...string) ([]byte, error) {
		_, _ = base(args...)
		if args[4] == "-A" && strings.Contains(strings.Join(args, " "), "192.168.") && failAdd {
			return nil, errors.New("add failure")
		}
		if args[4] == "-D" && failDelete {
			return nil, errors.New("cleanup failure")
		}
		return nil, nil
	}
	if err := s.Start([]string{"10.9.0.0/24", "192.168.1.0/24"}); err == nil {
		t.Fatal("add failure lost")
	}
	if len(s.owned) != 1 || len(*calls) != 3 {
		t.Fatal("failed rollback ownership lost")
	}
	failAdd, failDelete = false, false
	if err := s.Close(); err != nil || len(s.owned) != 0 {
		t.Fatal("cleanup retry failed")
	}
}
