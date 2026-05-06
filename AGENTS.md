# Tide Gauge Monorepo

## Overview
- Sync model: long-lived HTTP Upgrade + `yamux` streams carrying protobuf-delimited frames. On (re)connect the side that has gaps sends `ItemsLatest`/`StatusLatest` watermarks, the peer replays the missing rows, then both sides switch to live push.

## Modules
- `tide_server/`: central API/auth/sync + Postgres schema and deployment.
- `tide_client/`: edge collector + device drivers + SQLite storage.
- `uart-tcp-forward/`: serial device ↔ TCP bridge utility.
- `internal/syncv2/`, `internal/upstreamauth/`: shared Sync V2 transport/codec and upstream auth helpers.
- `common/`, `pkg/`: shared libraries.
- `arduino/`: firmware and protocol notes.
- `resources/`: architecture diagrams and sensor configuration screenshots.

## Specs
- Sync V2: `docs/protocols/sync-v2-protocol.md`
- Proto: `proto/sync/v2/sync_v2.proto`

## Tooling
- Do not set `GOCACHE` to a path under this repo and do not create a `.gocache/` directory in the workspace.
- When running Go commands that would populate the default Go build cache (e.g. `go test ./...`), run them outside the sandbox so Go can use its normal cache location.
