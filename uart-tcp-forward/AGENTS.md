# Repository Guidelines

## Project Structure & Module Organization
`uart-tcp-forward` is a small utility that bridges a serial device to a TCP listener.
- `main.go`: CLI parsing, serial open, TCP accept, and byte forwarding loop.
- `uart-tcp-forward.service`: example systemd unit for deployment.
- `README.md`: usage examples and Raspberry Pi service setup.

## Build, Test, and Development Commands
Run inside `uart-tcp-forward/`.
- `go build`: build local binary.
- `GOARCH='arm' GOARM=7 go build`: Raspberry Pi 32-bit build.
- `GOARCH='arm64' go build`: Raspberry Pi 64-bit build.
- `./uart-tcp-forward -h`: show CLI flags.
- `./uart-tcp-forward -l :7000 -s /dev/ttyUSB0`: start a forwarder instance.

## Coding Style & Naming Conventions
- Keep implementation minimal and explicit; this binary should remain single-purpose.
- Use `gofmt` and idiomatic Go error handling.
- Preserve stable CLI behavior when adding options; avoid breaking existing service units.

## Testing Guidelines
- Add `*_test.go` next to parsing/utility logic when extracted from `main.go`.
- Validate serial framing and connection lifecycle changes with focused local runs.
- For behavior changes, include manual verification notes (listen port, serial path, reconnect behavior).

## Commit & Pull Request Guidelines
- Use short imperative commits (`Fix reconnect`, `Add parity validation`, etc.).
- Keep PRs narrowly scoped and include tested CLI examples.
- Mention systemd impact if flags/defaults changed (`uart-tcp-forward.service`).
