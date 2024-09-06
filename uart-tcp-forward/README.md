# Help

```
pi@raspberrypi:~ $ ./uart-tcp-forward -h
Usage of ./uart-tcp-forward:
  -d int
        data bits (default 8)
  -debug
        print data
  -l string
        listen tcp addr (default ":7000"). Check https://pkg.go.dev/net#Listen
  -parity string
        parity (None, Odd, Even) (default "None")
  -r int
        baud rate (default 9600)
  -s string
        serial port name (default "/dev/ttyUSB0")
```

# Build on wsl2 or Linux

```shell
cd uart-tcp-forward

# Raspberry Pi OS
GOARCH='arm' GOARM=7 go build

# Raspberry Pi OS (64-bit)
GOARCH=arm64 go build

scp ./uart-tcp-forward pi@192.168.1.25:/tmp
```

# Running each forwarder as a service

```shell
mkdir /home/pi/tide
cp /tmp/uart-tcp-forward /home/pi/tide/uart-tcp-forward
# each forwarder service file
sudoedit /etc/systemd/system/arduino-tcp-forward.service
sudoedit /etc/systemd/system/rs485-tcp-forward.service
```

 Each forwarder has different `ExecStart` command:

```
[Unit]
Description=Uart Tcp Forward Service
After=network.target

[Service]
RestartSec=5s
User=pi
Group=pi
WorkingDirectory=/home/pi/tide

ExecStart=/home/pi/tide/uart-tcp-forward -l :7000 -s /dev/serial/by-id/usb-FTDI_FT232R_USB_UART_AK06YNFW-if00-port0
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Checkout Running log

```shell
pi@raspberrypi:~ $ journalctl -u rs485-tcp-forward.service
-- Journal begins at Sat 2021-10-30 20:43:13 CST, ends at Thu 2022-09-01 12:18:24 CST. --
Sep 01 12:18:13 raspberrypi systemd[1]: Started Uart Tcp Forward Service.
Sep 01 12:18:13 raspberrypi uart-tcp-forward[2619]: 2022/09/01 12:18:13 main.go:47: listen on: :7001
Sep 01 12:18:22 raspberrypi uart-tcp-forward[2619]: 2022/09/01 12:18:22 main.go:57: connected from: 192.168.1.38:47486
Sep 01 12:18:24 raspberrypi uart-tcp-forward[2619]: 2022/09/01 12:18:24 main.go:65: connected to: /dev/serial/by-id/usb-FTDI_FT232R_USB_UART_AK06YNFW-if00-port0
```