# Ping one current unambiguous peer

`mnet ping <exact-peer>` resolves the supplied name, FQDN or IPv4 address against the current private daemon status. It now refuses ambiguous matches instead of silently choosing the last map entry.

The local daemon must report `up`. A cached peer table while down, preparing, awaiting approval or key-expired does not authorize this diagnostic to pick a destination. The selector must match exactly one row, and that row's IPv4 address must occur once in the current table. Duplicate names, alias/name collisions and duplicate address ownership fail closed. An explicit unique address can disambiguate duplicate names, but not duplicate address ownership.

Only a canonical unicast IPv4 literal from that row is passed as the OS ping target. Hostname text, IPv6/mapped forms, unspecified/multicast/limited-broadcast addresses and option-like malformed map values are refused before dispatch. The command requires exactly one selector; extra arguments are not silently ignored. Fixed error messages do not echo untrusted selector text into the terminal.

A peer's offline/unknown handshake status does not prevent an explicit ping. Diagnosing that state is a reason to run the command. A successful ICMP reply checks address reachability; it does not authenticate a remote person or device key, prove application health, grant Remote Desktop/Shared Desk input, or establish production readiness. This command does not change enrollment, routing, permissions, firewall or DNS. It does not actively reconnect or repair a network.

## Acceptance

A failing-first test reproduced two same-name peers silently selecting the last address. Unit cases now cover unique name/FQDN/address, cross-alias collisions, duplicate rows/addresses, invalid address text, exact argument count and local non-Up states. Focused repeated race tests pass.

The existing private-Unix status fixture also runs the actual built CLI with a disposable local `ping` executable stub. It verifies the unique selectors dispatch exactly the existing platform arguments and canonical address, while duplicate/down/malformed/extra targets never run the executable. The fixture verifies the status request capability too. The stub performs no ICMP or network operation. This is CLI selection/dispatch acceptance, not real OS ping, physical-network or live-account evidence. Hosted outcomes and full source gates are accounted separately per commit.
