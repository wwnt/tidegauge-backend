# TideGauge

TideGauge is a monorepo for tide station data collection and central synchronization.

## Modules

- `tide_server/`: central service (API, auth, sync, PostgreSQL).
- `tide_client/`: edge collector (sensors/camera, SQLite).
- `uart-tcp-forward/`: serial-to-TCP forwarder for field devices.
- `arduino/`: firmware and protocol notes.

## Sync

- Station sync: `tide_client -> tide_server` (data, status, camera snapshot).
- Relay sync: `tide_server -> tide_server` (cascade deployments).
- After connection gaps, missing data is replayed, then realtime sync resumes.

## Docs

- Documentation index: `docs/README.md`
- Server guide: `tide_server/README.md`
- Client guide: `tide_client/README.md`
- UART forwarder guide: `uart-tcp-forward/README.md`
- Sync V2 protocol: `docs/protocols/sync-v2-protocol.md`
- Proto definition: `proto/sync/v2/sync_v2.proto`

