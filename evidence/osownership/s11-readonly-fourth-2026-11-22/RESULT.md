# Fourth single released read-only bootstrap: PASS

Exactly one invocation source7fdec1c binary5c39bc3b. Outer uid1000.
Fresh user+net namespace, mapped inner uid0 but all5 capability sets0,
NNP1, Seccomp2, filters1 versus baseline0, FDs exactly[0,1,2]. Built-in
owner-signal0 returned EPERM1. Same client/owner namespace7/4026532293,
distinct outer7/4026531833 unchanged. Client joined and reaped, resident
reaped/custody closed, final cleanup_confirmed=true. Within-run raw inventory
hash equal initial/final ef9324cefb05ad6972b3fad706e982edcfad15044cd62ae08d5a65074acfaae5.
No cross-run hash equivalence claim; volatile fields can still false-fail.

Driver906/start324 exit0/reaped,16.417799ms. Resident915/start324,
client938/start325. Wrapper additionally sampled937/start325 under915, same
executable; its exact role is NOT established by the sampled records and is
not silently called the client or ignored. All4 recorded lifetimes were absent
in immediate post-run checks. Sampling every1ms and task-leader children view
can miss short-lived/unrecorded descendants; not broad absence/containment proof.
The environment restarted from saved files after completion, so this packet
preserves the immediate wrapper checks rather than claiming a new later scan.
Client custody_close=false is no pidfd custody close attempted by that helper,
not failed reaping; parent overwrites child-claimed lifecycle from Wait evidence.

stdout preserved unchanged, outer stderr empty; no raw private child retention.
No timeout/external signals, retry, extra probe, TUN, networking/sysctl writes,
traffic, activation, application I/O or mutation adapter exercise. All3 earlier
failures preserved unchanged. Offline JSON assertion of all named pass oracles
included. This narrow successful bootstrap does not prove arbitrary-app sandbox,
OS interface/route ownership, tunnel path or complete installed product. No
subsequent execution released.
