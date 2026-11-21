package daemon

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"fintex.au/mnet/internal/stun"
)

func probeID(n uint64) (id stun.TxID) {
	binary.BigEndian.PutUint64(id[:], n)
	return
}

func bindingReply(id stun.TxID) []byte {
	packet := make([]byte, 32)
	binary.BigEndian.PutUint16(packet[0:], 0x0101)
	binary.BigEndian.PutUint16(packet[2:], 12)
	binary.BigEndian.PutUint32(packet[4:], 0x2112a442)
	copy(packet[8:20], id[:])
	binary.BigEndian.PutUint16(packet[20:], 0x0001)
	binary.BigEndian.PutUint16(packet[22:], 8)
	packet[25] = 1
	binary.BigEndian.PutUint16(packet[26:], 54321)
	copy(packet[28:], []byte{192, 0, 2, 10})
	return packet
}

func TestSilentStunProbesExpireWithoutResponses(t *testing.T) {
	d := &Daemon{stunTx: map[stun.TxID]time.Time{}}
	attachActualTestOwner(t, d)
	now := time.Unix(10000, 0)
	for n := uint64(0); n < 10000; n++ {
		d.startStunProbe(probeID(n), now.Add(time.Duration(n)*45*time.Second))
		if len(d.stunTx) != 1 {
			t.Fatalf("silent probe %d retained %d transactions", n, len(d.stunTx))
		}
	}
}

func TestStunExpiryPreservesInFlightBoundary(t *testing.T) {
	now := time.Now()
	expired, boundary, recent, next := probeID(1), probeID(2), probeID(3), probeID(4)
	d := &Daemon{stunTx: map[stun.TxID]time.Time{
		expired:  now.Add(-10*time.Second - time.Nanosecond),
		boundary: now.Add(-10 * time.Second),
		recent:   now.Add(-time.Second),
	}, stunResult: "stale"}
	attachActualTestOwner(t, d)
	d.startStunProbe(next, now)
	if _, ok := d.stunTx[expired]; ok {
		t.Fatal("expired probe retained")
	}
	for _, id := range []stun.TxID{boundary, recent, next} {
		if _, ok := d.stunTx[id]; !ok {
			t.Fatal("valid in-flight probe lost")
		}
	}
	if d.stunResult != "" {
		t.Fatal("new discovery retained prior endpoint result")
	}
}

func TestStunOnlyAcceptsMatchingUnexpiredResponse(t *testing.T) {
	now := time.Now()
	expired, valid := probeID(1), probeID(2)
	d := &Daemon{stunTx: map[stun.TxID]time.Time{expired: now.Add(-11 * time.Second), valid: now}}
	attachActualTestOwner(t, d)
	d.onStun(bindingReply(expired))
	if d.stunResult != "" {
		t.Fatal("expired response accepted")
	}
	d.onStun(bindingReply(probeID(3)))
	if d.stunResult != "" {
		t.Fatal("unmatched response accepted")
	}
	d.onStun([]byte{0, 1})
	if _, ok := d.stunTx[valid]; !ok {
		t.Fatal("malformed response consumed valid request")
	}
	d.onStun(bindingReply(valid))
	if d.stunResult != "192.0.2.10:54321" || len(d.stunTx) != 0 {
		t.Fatal("valid response not consumed correctly")
	}
}

func TestStunProbeAndResponseRace(t *testing.T) {
	d := &Daemon{stunTx: map[stun.TxID]time.Time{}}
	attachActualTestOwner(t, d)
	var workers sync.WaitGroup
	for worker := uint64(0); worker < 4; worker++ {
		workers.Add(1)
		go func(worker uint64) {
			defer workers.Done()
			for n := uint64(0); n < 100; n++ {
				id := probeID(worker*100 + n)
				d.startStunProbe(id, time.Now())
				d.onStun(bindingReply(id))
			}
		}(worker)
	}
	workers.Wait()
	if len(d.stunTx) != 0 {
		t.Fatal("completed probes retained")
	}
}
