# Repository Guidelines

## Architecture Overview
This project implements a tide-gauge platform with edge collection and centralized services.
- Sync model: downstream nodes poll upstream events and perform full DB resync after connection gaps.
- Key diagrams: `resources/Architecture.png` and `resources/sync.png`.
- Go toolchain target: Go `1.25` (see `go.mod`).

## Project Structure & Module Organization
This repository is a Go monorepo for a tide-gauge system.
- `tide_server/`: backend API, auth, sync logic, PostgreSQL schema (`schema.sql`), and Docker/service files.
- `tide_client/`: edge collector for sensors/camera, SQLite schema (`schema.sql`), device drivers, and service files.
- `uart-tcp-forward/`: serial-to-TCP forwarding utility for field devices.
- `common/` and `pkg/`: shared types and reusable libraries.
- `arduino/`: firmware sketch (`arduino.ino`) and protocol notes.
- `resources/`: architecture and setup images used in docs.

## Module-Specific Notes
For detailed workflows in each area, check:
- `tide_server/CLAUDE.md`
- `tide_client/CLAUDE.md`
- `uart-tcp-forward/CLAUDE.md`
- `arduino/CLAUDE.md`

## Build, Test, and Development Commands
Run commands from repository root unless noted.
- `go test ./...`: run all unit/integration tests across modules.
- `go build ./tide_server`: build server binary.
- `go build ./tide_client`: build client binary (Linux entrypoint is `main_linux.go`).
- `go build ./uart-tcp-forward`: build serial forwarder.
- `docker build -f tide_server/Dockerfile -t wwnt/tide-server .`: build server container.
- `psql -d tidegauge -U postgres -f tide_server/schema.sql`: initialize server DB schema.

## Coding Style & Naming Conventions
- Use standard Go formatting: run `gofmt` on edited files before committing.
- Follow idiomatic Go naming: exported identifiers in `CamelCase`, internal helpers in `camelCase`, package names lowercase.
- Keep package boundaries clear: server code stays in `tide_server/...`, client hardware logic in `tide_client/device/...`.
- Prefer structured logging with `slog` (consistent with recent refactors).

## Testing Guidelines
- Place tests beside code using `*_test.go` (examples: `tide_server/db/*_test.go`, `tide_client/controller/*_test.go`).
- Name tests with `TestXxx` and favor table-driven tests for protocol/DB edge cases.
- Run focused tests during development, e.g. `go test ./tide_server/controller -run TestName`.

## Commit & Pull Request Guidelines
- Match existing history style: short, imperative subjects (e.g., `Refactor ...`, `Fix ...`, `Add ...`).
- Keep commits scoped to one concern (API, DB, device driver, or infra change).
- PRs should include:
  - what changed and why,
  - impacted module(s) (`tide_server`, `tide_client`, etc.),
  - test evidence (`go test` output summary),
  - config/schema notes when relevant (`config.json`, SQL, systemd/Docker).
