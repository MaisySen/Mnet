//go:build linux

package ifacectl

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func fakeSubnetRoutes(t *testing.T) (*SubnetRoutes, *[]string) {
	t.Helper()
	var changes []string
	r := NewSubnetRoutes("mnet0")
	r.metric = "1234567"
	r.command = func(args ...string) ([]byte, error) {
		if slices.Contains(args, "-j") {
			return []byte(`[{"dst":"198.18.10.0/24","dev":"eth0"},{"dst":"100.64.0.0/10","dev":"mnet0"},{"dst":"default","dev":"eth0"}]`), nil
		}
		changes = append(changes, strings.Join(args, " "))
		return nil, nil
	}
	return r, &changes
}
func TestSubnetOwnerAddsWithdrawsAndCloses(t *testing.T) {
	r, changes := fakeSubnetRoutes(t)
	if err := r.Sync([]string{"10.9.0.2/24"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Sync([]string{"10.9.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	if len(*changes) != 1 {
		t.Fatal("no-op rewrote route")
	}
	if err := r.Sync([]string{"10.10.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"-4 route add 10.9.0.0/24 dev mnet0 proto 186 metric 1234567", "-4 route del 10.9.0.0/24 dev mnet0 proto 186 metric 1234567", "-4 route add 10.10.0.0/24 dev mnet0 proto 186 metric 1234567", "-4 route del 10.10.0.0/24 dev mnet0 proto 186 metric 1234567"}
	if !slices.Equal(*changes, want) {
		t.Fatalf("%v", *changes)
	}
}
func TestSubnetOwnerRefusesConflictsAndUnsafe(t *testing.T) {
	for _, prefix := range []string{"198.18.10.0/24", "198.18.10.8/32", "198.18.0.0/16", "0.0.0.0/0", "100.64.0.0/24", "127.0.0.0/8", "169.254.0.0/16", "224.0.0.0/4", "::/0", "bad"} {
		t.Run(prefix, func(t *testing.T) {
			r, changes := fakeSubnetRoutes(t)
			if r.Sync([]string{prefix}) == nil {
				t.Fatal("accepted")
			}
			if len(*changes) != 0 {
				t.Fatal("changed unrelated routing")
			}
		})
	}
}
func TestSubnetOwnerRollsBackFailedAdd(t *testing.T) {
	r, changes := fakeSubnetRoutes(t)
	base := r.command
	r.command = func(args ...string) ([]byte, error) {
		if slices.Contains(args, "add") && slices.Contains(args, "10.9.0.0/24") {
			return []byte("fixture failure"), fmt.Errorf("failed")
		}
		return base(args...)
	}
	if r.Sync([]string{"10.8.0.0/24", "10.9.0.0/24"}) == nil {
		t.Fatal("lost failure")
	}
	if len(r.owned) != 0 || len(*changes) != 2 || !strings.Contains((*changes)[1], "del 10.8.0.0/24") {
		t.Fatal("did not roll back new route")
	}
}
func TestSubnetOwnerDoesNotDeleteReplacement(t *testing.T) {
	r, _ := fakeSubnetRoutes(t)
	r.owned["10.9.0.0/24"] = true
	r.command = func(args ...string) ([]byte, error) {
		if slices.Contains(args, "-j") {
			return json.Marshal([]routeEntry{{Dst: "10.9.0.0/24", Dev: "eth0", Metric: 55, Protocol: "static"}})
		}
		if !slices.Contains(args, r.metric) || !slices.Contains(args, "186") {
			t.Fatal("unscoped delete")
		}
		return nil, fmt.Errorf("owned route gone")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if len(r.owned) != 0 {
		t.Fatal("did not release absent ownership")
	}
}
