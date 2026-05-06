# tide_server (central API)

## Layout
- `main.go`: server entrypoint and startup flow.
- `controller/`: HTTP/WebSocket handlers, Sync V2 routing, station operations.
- `syncv2/station/`, `syncv2/relay/`: Sync V2 server-side implementation (station ingress handler/server/store/registry; relay upstream handler and downstream client/apply/store).
- `db/`: PostgreSQL data access and query helpers.
- `auth/`: Keycloak integration, JWT, and permission checks.
- `global/`: config loading and shared runtime state.
- `schema.sql`: baseline schema; `docs/server/api-guide.md`: endpoint usage guide.
