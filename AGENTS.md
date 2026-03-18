# Tide Gauge Monorepo

## Overview
- Sync model: downstream polls upstream events; gaps trigger a full DB resync.

## Modules
- `tide_server/`: central API/auth/sync + Postgres schema and deployment.
- `tide_client/`: edge collector + device drivers + SQLite storage.
- `uart-tcp-forward/`: serial device ↔ TCP bridge utility.
- `common/`, `pkg/`: shared libraries.
- `arduino/`: firmware and protocol notes.

## Specs
- Sync V2: `docs/protocols/sync-v2-protocol.md`
- Proto: `proto/sync/v2/sync_v2.proto`

## Tooling
- Do not set `GOCACHE` to a path under this repo and do not create a `.gocache/` directory in the workspace.
- When running Go commands that would populate the default Go build cache (e.g. `go test ./...`), run them outside the sandbox so Go can use its normal cache location.
