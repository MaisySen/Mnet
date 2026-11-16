# Proof-only userspace network adapter

Derived from WireGuard Go tun/netstack/tun.go at pinned module revision b3bc704ef0ee (2026-10-24), copyright WireGuard LLC, MIT license copied in LICENSE. Only the TUN bridge and TCP dial/listen entry points needed by Mnet's diagnostic proof are retained. DNS and UDP convenience APIs are omitted.

The underlying gVisor module stays pinned in go.mod. No dependency fork or local module replacement is used. The only recovery change uses its exported TCPRecovery option to select classic SACK recovery. Set MNET_PROOF_RACK=1 to compare the unmodified default RACK setting.

This package is imported only by diagnostic proofs, including the disposable native-kernel CI fixture's same-host userspace peer. That peer has no OS TCP listener; the native host connects through its real kernel VPN interface, but this does not prove a second kernel host or public internet path. Actual desktop/mobile sessions use OS TCP on their VPN interfaces. Do not treat proof-stack configuration as a production OS TCP fix.
