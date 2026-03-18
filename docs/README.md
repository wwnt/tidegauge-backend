# Documentation Index

This index is the entry point for human-facing project documentation.

Machine-only Markdown such as `AGENTS.md`, `CLAUDE.md`, and `.claude/plans/*.md` is intentionally kept out of this tree because those files serve tooling and agent workflows rather than operator or developer onboarding.

## Start Here

- [Monorepo overview](../README.md): top-level project summary, module map, and key documentation entry points.

## Module Guides

- [tide_client guide](../tide_client/README.md): install, deploy, sync setup, and field device wiring for edge collectors.
- [tide_server guide](../tide_server/README.md): database, runtime, deployment, and Sync V2 server integration for the central service.
- [uart-tcp-forward guide](../uart-tcp-forward/README.md): serial-to-TCP bridge build and service setup.
- [arduino guide](../arduino/README.md): firmware flashing and supported serial command usage.

## Protocols And Specs

- [Sync V2 protocol](protocols/sync-v2-protocol.md): station sync and relay sync runtime flow, endpoints, and implementation layout.
- [Sync V2 proto README](../proto/sync/v2/README.md): protobuf contract source, generated package, and regeneration notes.

## Operations And Troubleshooting

- [Client config reference](client/config-reference.md): `tide_client` configuration fields and sample device config structure.
- [Reverse SSH tunnel](client/reverse-ssh-tunnel.md): systemd-based reverse tunnel setup for remote access.
- [SDI-12 noise mitigation](client/sdi-12-noise-mitigation.md): field guidance for noisy SDI-12 deployments.
- [Server API guide](server/api-guide.md): login flow and basic data retrieval workflow against the server HTTP API.

## Machine-only Markdown

- Root and module `AGENTS.md`: agent-facing repository layout instructions.
- Root and module `CLAUDE.md`: reserved tool-specific files, currently empty.
- `.claude/plans/*.md`: hidden planning artifacts, not part of the human-facing docs set.
