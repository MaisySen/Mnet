package daemon

import (
	"fmt"
	"testing"

	"fintex.au/mnet/internal/netmap"
)

// This benchmarks the endpoint-filter portion of one map application, not
// WireGuard configuration, real devices, network throughput or full applyMap.
func BenchmarkMapEndpointFiltering(b *testing.B) {
	for _, implementation := range []string{"original", "indexed"} {
		b.Run(implementation, func(b *testing.B) {
			for _, count := range []int{100, 1000, 10000} {
				b.Run(fmt.Sprint(count), func(b *testing.B) {
					peers := make([]netmap.Node, count)
					for i := range peers {
						peers[i].Routes = []string{fmt.Sprintf("10.%d.%d.0/24", i/256, i%256)}
						peers[i].Endpoints = []string{"192.0.2.7:12345", "10.0.0.7:12345"}
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						kept := 0
						if implementation == "original" {
							for _, peer := range peers {
								kept += len(originalEndpointFilter(peer.Endpoints, peers))
							}
						} else {
							filter := newEndpointRouteFilter(peers)
							for _, peer := range peers {
								kept += len(filter.safe(peer.Endpoints))
							}
						}
						if kept != count {
							b.Fatal("endpoint safety result changed", kept)
						}
					}
				})
			}
		})
	}
}
