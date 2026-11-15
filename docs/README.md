# Mnet documentation

These documents describe a development project, not a production-readiness guarantee. Start with the status page before installing or relying on a feature.

| Document | Owns |
| --- | --- |
| [Project status](STATUS.md) | Source, released artifacts, last deployment verification, evidence and open work. |
| [Product scope](product-scope.md) | Desired account/device/startup experience and current platform gaps. |
| [Operations](operations.md) | Setup, local state, service operation, authentication, routing limits and recovery. |
| [Architecture](architecture.md) | Components and trust boundaries. |
| [Session layer](session-layer.md) | Protocol, host authorization, queue/stream limits and file publication. |
| [Session reliability](session-reliability.md) | Reproduction commands, baseline failures, fixes and loss-test limits. |
| [Native platform validation](native-platform-validation.md) | Exact platform checks and physical-device gaps. |
| [Scale testing](scale-testing.md) | Dated synthetic measurements, methods, raw benchmark references and limits. |
| [Releases and development history](RELEASES.md) | Shipped test versions and historical work, not a current feature guarantee. |
| [Android store draft](PLAY-LISTING.md) | Unapproved listing and data-disclosure work, not ready-to-publish claims. |
| [iOS source guide](../ios/README.md) | Source integration, toolchain and remaining signing/device validation. |

## Keeping documents current alongside code

A feature change must update its owning guide, regression commands and open limitations. A release must also update STATUS and RELEASES with the exact source, artifact version and verification layer. Do not infer deployment from a source push or infer physical-platform support from cross-compilation.

Keep measurements and failures as dated history. Add a correction or a newer result instead of rewriting an old benchmark as though it used the new implementation. Link to evidence when a status changes. Never commit tokens, private keys, enrollment keys, local-state files or raw credential-bearing logs.
