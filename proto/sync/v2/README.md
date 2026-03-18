# Sync v2 (protobuf-delimited stream)

Sync v2 uses protobuf messages over HTTP-upgraded full-duplex streams.

This directory is the wire-contract source for both Sync V2 links:

- station sync: `tide_client -> tide_server`
- relay sync: `tide_server -> tide_server`

## Key files
- `proto/sync/v2/sync_v2.proto`
- `pkg/pb/syncproto/sync_v2.pb.go`
- `internal/syncv2/*`
- `tide_client/syncv2/*`
- `tide_client/controller/sync_v2.go`
- `tide_server/syncv2/station/*`
- `tide_server/syncv2/relay/*`
- `tide_server/controller/router.go`
- `tide_server/controller/sync_client.go`
- `tide_server/controller/syncv2_adapters.go`

## Messages and endpoints

- `StationMessage`: station sync frames on `POST /sync_v2/station`
- `RelayMessage`: relay sync frames on `POST /sync_v2/relay`
- relay auth flow: downstream calls `POST /login`, then upgrades `/sync_v2/relay` with `Authorization: Bearer <access_token>`

`pkg/pb/syncproto` is the generated Go package. `internal/syncv2` is the shared runtime helper package. Keep those names distinct in docs and code review.

## Generation command
```powershell
protoc --go_out=. --go_opt=module=tide proto/sync/v2/sync_v2.proto
```

## Compatibility notes

- Any proto change must update both client and server consumers.
- Regenerate `pkg/pb/syncproto/sync_v2.pb.go` after changing `sync_v2.proto`.
- Runtime flow, replay order and module responsibilities are documented in `docs/protocols/sync-v2-protocol.md`.
