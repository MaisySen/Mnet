//go:build mnet_enrollment_development

package osownership

// Generated from pinned Linux v6.17 netdev_features.h enum and common.c name
// initializers. Runtime STRSET must match every observed index/name, not count.
var pinnedFeatureNamesV617 = [...]string{
	"tx-scatter-gather",              // 0
	"tx-checksum-ipv4",               // 1
	"",                               // 2
	"tx-checksum-ip-generic",         // 3
	"tx-checksum-ipv6",               // 4
	"highdma",                        // 5
	"tx-scatter-gather-fraglist",     // 6
	"tx-vlan-hw-insert",              // 7
	"rx-vlan-hw-parse",               // 8
	"rx-vlan-filter",                 // 9
	"vlan-challenged",                // 10
	"tx-generic-segmentation",        // 11
	"",                               // 12
	"",                               // 13
	"rx-gro",                         // 14
	"rx-lro",                         // 15
	"tx-tcp-segmentation",            // 16
	"tx-gso-robust",                  // 17
	"tx-tcp-ecn-segmentation",        // 18
	"tx-tcp-mangleid-segmentation",   // 19
	"tx-tcp6-segmentation",           // 20
	"tx-fcoe-segmentation",           // 21
	"tx-gre-segmentation",            // 22
	"tx-gre-csum-segmentation",       // 23
	"tx-ipxip4-segmentation",         // 24
	"tx-ipxip6-segmentation",         // 25
	"tx-udp_tnl-segmentation",        // 26
	"tx-udp_tnl-csum-segmentation",   // 27
	"tx-gso-partial",                 // 28
	"tx-tunnel-remcsum-segmentation", // 29
	"tx-sctp-segmentation",           // 30
	"tx-esp-segmentation",            // 31
	"",                               // 32
	"tx-udp-segmentation",            // 33
	"tx-gso-list",                    // 34
	"tx-tcp-accecn-segmentation",     // 35
	"tx-checksum-fcoe-crc",           // 36
	"tx-checksum-sctp",               // 37
	"rx-ntuple-filter",               // 38
	"rx-hashing",                     // 39
	"rx-checksum",                    // 40
	"tx-nocache-copy",                // 41
	"loopback",                       // 42
	"rx-fcs",                         // 43
	"rx-all",                         // 44
	"tx-vlan-stag-hw-insert",         // 45
	"rx-vlan-stag-hw-parse",          // 46
	"rx-vlan-stag-filter",            // 47
	"l2-fwd-offload",                 // 48
	"hw-tc-offload",                  // 49
	"esp-hw-offload",                 // 50
	"esp-tx-csum-hw-offload",         // 51
	"rx-udp_tunnel-port-offload",     // 52
	"tls-hw-tx-offload",              // 53
	"tls-hw-rx-offload",              // 54
	"rx-gro-hw",                      // 55
	"tls-hw-record",                  // 56
	"rx-gro-list",                    // 57
	"macsec-hw-offload",              // 58
	"rx-udp-gro-forwarding",          // 59
	"hsr-tag-ins-offload",            // 60
	"hsr-tag-rm-offload",             // 61
	"hsr-fwd-offload",                // 62
	"hsr-dup-offload",                // 63
}
