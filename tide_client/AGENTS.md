# Repository Guidelines

## Project Structure & Module Organization
`tide_client` is the edge collector running on Raspberry Pi/Linux.
- `main_linux.go`: process entrypoint and lifecycle.
- `controller/`: startup wiring, device registration, station sync, maintenance jobs.
- `device/`: sensor and camera drivers (RS485, SDI-12, I2C, GPIO, mock).
- `connWrap/`: UART/TCP transport helpers.
- `db/`: SQLite persistence and cleanup.
- `global/`: config loading and shared runtime settings.
- Config samples: `config.template.json`, `devices_*.json`, `configs/*/`.

## Build, Test, and Development Commands
Run inside `tide_client/` for local module work.
- `go build`: build current platform binary.
- `CC='arm-linux-gnueabihf-gcc' GOARCH='arm' GOARM=7 go build`: Raspberry Pi 32-bit build.
- `CC='aarch64-linux-gnu-gcc' GOARCH='arm64' go build`: Raspberry Pi 64-bit build.
- `go test ./...`: run all client tests.
- `go test ./controller -run TestName`: run focused controller tests.
- `sqlite3 /home/pi/tide/data.db` then `.read tide_client/schema.sql`: initialize local DB.

## Coding Style & Naming Conventions
- Follow standard Go style and run `gofmt` before commit.
- File names use snake_case when matching hardware/domain terms (for example `ott_SE200.go`).
- Keep driver-specific logic in `device/`; shared parsing/retry logic belongs in helpers.
- Prefer structured logging with `slog` and include station/device context in log fields.

## Testing Guidelines
- Keep tests next to source as `*_test.go`.
- Favor table-driven tests for protocol parsing, serial framing, and DB edge cases.
- For hardware-facing code, isolate serial/TCP behavior behind wrappers and test with mocks.

## Commit & Pull Request Guidelines
- Use concise imperative subjects: `Fix ...`, `Refactor ...`, `Add ...`.
- Keep one concern per commit (driver, db, sync, or config).
- PRs should include changed device/config impact, test command results, and migration notes for `config.json`/schema changes.
