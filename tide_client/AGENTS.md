# tide_client (edge collector)

## Layout
- `main_linux.go`: process entrypoint and lifecycle.
- `controller/`: wiring, device registration, station sync, maintenance jobs.
- `device/`: sensor/camera drivers (RS485, SDI-12, I2C, GPIO, mock).
- `connWrap/`: UART/TCP transport wrappers.
- `db/`: SQLite persistence and cleanup.
- `global/`: config loading and shared runtime settings.
- Config samples: `config.template.json`, `devices_*.json`, `configs/*/`.