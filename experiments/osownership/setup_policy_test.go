//go:build mnet_enrollment_development

package osownership

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestFirstStepFlagsShareActualPerOperationSource(t *testing.T) {
	token, _, _ := setupRig(t)
	flags, e := mutationRequestFlags(16)
	if e != nil || flags != 5 || binary.NativeEndian.Uint16(token.request[6:]) != flags {
		t.Fatal("wrong existing-link update flags", flags, e)
	}
	if flags&(0x200|0x400) != 0 {
		t.Fatal("exclusive creation on update")
	}
}
func TestTypedFirstLinkSenderExactPermitBeforeFakeEffect(t *testing.T) {
	token, l, _ := setupRig(t)
	permit := &firstLinkSendPermit{token: token}
	before := l.snapshot()
	called := false
	if e := permit.send(token.request, 77, func() error {
		called = true
		if l.snapshot().generation <= before.generation || !l.snapshot().uncertain {
			t.Fatal("not invalidated")
		}
		return nil
	}); e != nil || !called {
		t.Fatal(e)
	}
	if e := permit.send(token.request, 77, func() error { t.Fatal("repeat send"); return nil }); e == nil {
		t.Fatal("reused permit")
	}
	for _, kind := range []uint16{17, 19, 20, 24} {
		token, _, _ := setupRig(t)
		bad := append([]byte(nil), token.request...)
		binary.NativeEndian.PutUint16(bad[4:], kind)
		if e := (&firstLinkSendPermit{token: token}).send(bad, 77, func() error { t.Fatal("wrong kind sent"); return nil }); e == nil {
			t.Fatal("bypass", kind)
		}
	}
}
func TestVolatilePolicyCountersOnlyPreserveSafetyAndOriginalBytes(t *testing.T) {
	token, _, _ := setupRig(t)
	expected := cloneRawExact(token.expectedRaw)
	expected.Links[0].Raw[7] = make([]byte, 96)
	expected.Links[0].Raw[23] = make([]byte, 200)
	actual := cloneRawExact(expected)
	actual.Links[0].Raw[7][0] = 8
	actual.Links[0].Raw[23][8] = 9
	families, _ := decodeAttributes(actual.Links[0].Raw[26])
	a6, _ := decodeAttributes(families[10])
	a6[3][8] = 99
	a6[6][8] = 33
	a6[5][4] = 88
	body, _ := replaceAttrValues(families[10], a6)
	families[10] = body
	actual.Links[0].Raw[26], _ = replaceAttrValues(actual.Links[0].Raw[26], families)
	before := cloneRawExact(actual)
	if !exactSetupPostMatches(actual, expected) || !reflect.DeepEqual(before, actual) {
		t.Fatal("known counters refused/mutated")
	}
	for _, mode := range []string{"stats-size", "missing-stats", "carrier", "flags", "conf", "cache-reachable", "count", "unknown"} {
		bad := cloneRawExact(actual)
		switch mode {
		case "stats-size":
			bad.Links[0].Raw[7] = make([]byte, 100)
		case "missing-stats":
			delete(bad.Links[0].Raw, 7)
		case "carrier":
			bad.Links[0].Raw[33] = []byte{1}
		case "flags":
			bad.Links[0].Flags ^= 1
		case "unknown":
			bad.Links[0].Raw[999] = []byte{1}
		default:
			f, _ := decodeAttributes(bad.Links[0].Raw[26])
			a, _ := decodeAttributes(f[10])
			if mode == "conf" {
				a[2][4] ^= 1
			}
			if mode == "cache-reachable" {
				a[5][8] ^= 1
			}
			if mode == "count" {
				a[3][0]++
			}
			f[10], _ = replaceAttrValues(f[10], a)
			bad.Links[0].Raw[26], _ = replaceAttrValues(bad.Links[0].Raw[26], f)
		}
		if exactSetupPostMatches(bad, expected) {
			t.Fatal("safety drift masked", mode)
		}
	}
}
