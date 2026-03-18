# uart-tcp-forward (serial ↔ TCP bridge)

## Layout
- `main.go`: flag parsing, serial open, TCP accept, byte forwarding loop.
- `uart-tcp-forward.service`: example systemd unit for deployment.
- `README.md`: usage and Raspberry Pi service setup.