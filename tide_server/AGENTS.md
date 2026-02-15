# Repository Guidelines

## Project Structure & Module Organization
`tide_server` provides API, auth, sync, and storage for tide-gauge deployments.
- `main.go`: server entrypoint and startup flow.
- `controller/`: HTTP/WebSocket handlers, sync endpoints, station operations.
- `db/`: PostgreSQL data access layer and query helpers.
- `auth/`: Keycloak integration, JWT, and permission checks.
- `global/`: config loading and shared runtime state.
- `schema.sql`: baseline PostgreSQL schema; `API.md` documents exposed endpoints.

## Build, Test, and Development Commands
Run inside `tide_server/` for server-focused work.
- `go build`: build server binary.
- `go test ./...`: run all server tests.
- `go test ./controller -run TestName`: run focused controller tests.
- `./tide_server -config config.json`: run with local config.
- `./tide_server -initKeycloak`: interactive Keycloak bootstrap.
- `docker build -f tide_server/Dockerfile -t wwnt/tide-server .`: build image.
- `psql -d tidegauge -U postgres -f tide_server/schema.sql`: initialize DB schema.

## Coding Style & Naming Conventions
- Use `gofmt` formatting and idiomatic Go naming.
- Keep HTTP contract logic in `controller/`, persistence concerns in `db/`, identity logic in `auth/`.
- Prefer explicit error wrapping and structured logs (`slog`) for request and sync flows.
- Keep SQL/schema changes backward-compatible when possible and document migration impact.

## Testing Guidelines
- Co-locate tests (`*_test.go`) with implementation.
- Add tests for controller auth boundaries, sync correctness, and DB query behavior.
- Use targeted runs during iteration, then run `go test ./...` before submitting PR.

## Commit & Pull Request Guidelines
- Follow concise imperative commit subjects.
- Separate refactors from behavior changes when practical.
- PRs should include API/schema impact, test evidence, and deployment notes (Keycloak, Docker, or service updates).
