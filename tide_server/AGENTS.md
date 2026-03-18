# tide_server (central API)

## Layout
- `main.go`: server entrypoint and startup flow.
- `controller/`: HTTP/WebSocket handlers, sync endpoints, station operations.
- `db/`: PostgreSQL data access and query helpers.
- `auth/`: Keycloak integration, JWT, and permission checks.
- `global/`: config loading and shared runtime state.
- `schema.sql`: baseline schema; `docs/server/api-guide.md`: endpoint usage guide.
