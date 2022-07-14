- [What do you need](#what-do-you-need)
- [Flash Arduino](#flash-arduino)
- [Build client](#build-client)
- [Sensor Wiring](#sensor-wiring)
    - [RS485](#rs485)
        - [Ascii](#ascii)
        - [Modbus](#modbus)
    - [SDI-12](#sdi-12)
    - [Analog](#analog)
    - [GPIO](#gpio)

# What do you need

- Raspberry Pi 4 Model B
- SanDisk 256GB MAX Endurance microSD Card
- Arduino Uno (Optional, for SDI-12 and anolog read use)
- Usb to rs485 (Optional)
- Rs485 Hub (Optional)

# Prerequisite

- Ftp server
- Syncthing

## Ftp Server

1. `sudo apt install vsftpd`
2. Config monitor's ftp config

## Syncthing

1. Following https://docs.syncthing.net/users/autostart.html
2. Add ftp directory to syncthing
3. Add Service to remote device

# Flash Arduino

1. Install Arduino Ide from Windows Store
2. Open arduino/arduino.ino
3. Connect arduino via usb cable
4. Click upload

# Build client

```shell
apt install gcc-arm-linux-gnueabihf
CC='arm-linux-gnueabihf-gcc' GOARCH='arm' GOARM=7 go build
// or arm64
CC='arm-linux-gnueabihf-gcc' GOARCH='arm64' go build
```

# Sensor Wiring

## RS485

### Ascii

| RS485  | A            | B     | VCC   | GND       |
|--------|--------------|-------|-------|-----------|
| HMP155 | pink         | brown | blue  | red       |
| PWD50  | brown        | white | red   | black     |
| WMT700 | brown-yellow | black | white | gray-pink |

### Modbus

## SDI-12

| SDI-12 | VCC   | GND   | DATA   | SDI-12 GND |
|--------|-------|-------|--------|------------|
| SE200  | brown | white | yellow | green      |
| PLS-C  | red   | blue  | gray   |            |

## Analog

DRD11A: yellow

## GPIO

DRD11A: blue

![img.png](../resources/DRD11A.png)
