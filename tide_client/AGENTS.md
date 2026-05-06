# tide_client (edge collector)

## Layout
- `main_linux.go`: process entrypoint and lifecycle.
- `controller/`: wiring, device registration, station sync, maintenance jobs.
- `device/`: sensor/camera drivers (RS485, SDI-12, I2C, GPIO, ONVIF camera).
- `protocol/`: bus-level protocol implementations (`arduino`, `modbusrtu`, `sdi12`, `textline`).
- `connWrap/`: UART/TCP transport wrappers.
- `syncv2/`: Sync V2 client (handshake, replay, real-time push, command sub-streams).
- `db/`: SQLite persistence and cleanup.
- `global/`: config loading and shared runtime settings.
- Config samples: `config.template.json` at module root; per-station bundles (`config.json` + `devices_*.json`) under `configs_backup/<station>/` (gitignored).