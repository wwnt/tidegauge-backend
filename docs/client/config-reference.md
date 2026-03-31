# Client Config Reference

Purpose: describe `tide_client` runtime configuration and sample device config structure.

Audience: operators and developers working on station deployment or debugging local device setup.

Related Docs: [tide_client guide](../../tide_client/README.md), [Reverse SSH tunnel guide](reverse-ssh-tunnel.md), [SDI-12 noise mitigation](sdi-12-noise-mitigation.md)

<!-- TOC -->
* [Client Config Reference](#client-config-reference)
  * [config.json](#configjson)
  * [listen](#listen)
  * [server](#server)
  * [identifier](#identifier)
  * [db](#db)
    * [db.dsn](#dbdsn)
    * [db.hold_days](#dbhold_days)
  * [cameras](#cameras)
    * [cameras.ftp](#camerasftp)
      * [cameras.ftp.path](#camerasftppath)
      * [cameras.ftp.hold_days](#camerasftphold_days)
    * [cameras.list](#cameraslist)
      * [cameras.list.camera1](#cameraslistcamera1)
        * [cameras.list.camera1.snapshot | username | password](#cameraslistcamera1snapshot--username--password)
  * [devices](#devices)
    * [devices.uart | tcp | gpio](#devicesuart--tcp--gpio)
* [devices_uart_arduino.json](#devices_uart_arduinojson)
  * [port | read_timeout | mode | model](#port--read_timeout--mode--model)
  * [config](#config)
  * [config.sdi12 | analog](#configsdi12--analog)
    * [config.sdi12[].model](#configsdi12model)
    * [config.sdi12[].config](#configsdi12config)
    * [config.analog[]](#configanalog)
* [devices_uart_rs485_modbus.json](#devices_uart_rs485_modbusjson)
  * [config[]](#config-1)
<!-- TOC -->

## config.json

You can view the sample file `config.template.json`

## listen

Http service listening address, used to provide pprof service, only useful to developers.

## server

Backend server address, data will be uploaded to this server.

## identifier

There may be multiple tide gauge stations connected to the backend server, so an identifier is needed to distinguish them.

## db

Sqlite Database configuration.

### db.dsn

This is also known as a DSN (Data Source Name) string. [Check the sqlite driver documentation](https://pkg.go.dev/github.com/mattn/go-sqlite3).

### db.hold_days

The number of days to keep time-series data in the local SQLite database.
The sample configuration defaults to `90`, which is about 3 months.

## cameras

Cameras configuration.

### cameras.ftp

Ftp server running on the Raspberry Pi will be provided to the camera.

#### cameras.ftp.path

The path of the ftp server, the camera will upload the image to this path.

#### cameras.ftp.hold_days

The number of days to keep the image on the ftp server.

### cameras.list

Configuration of each camera.

#### cameras.list.camera1

**camera1** is the camera name, and the camera will create a directory with this name in the root directory of ftp,
and then all photos will be written to this directory.

##### cameras.list.camera1.snapshot | username | password

**snapshot** is the camera's snapshot url(you can get it through the onvif protocol),
**username** and **password** are the username and password required to access.

## devices

List of configuration files for different connection methods

### devices.uart | tcp | gpio

Each connection method can have multiple config files.

For direct USB/serial `Modbus RTU` deployments, prefer `devices_uart_rs485_modbus.json`.
Use `devices_tcp_rs485_modbus.json` only when the RS485 bus is behind a serial-to-TCP gateway.
Do not load both files for the same physical bus at the same time.

# devices_uart_arduino.json

This sample config file exists under [`devices.uart`](#devicesuart--tcp--gpio), so it is connected via uart.

## port | read_timeout | mode | model

These are the uart configurations. In this example it will be read in [tide_client/controller/add_uart_devices.go](../../tide_client/controller/add_uart_devices.go)

## config

According to the value of [model](#port--read_timeout--mode--model), it corresponds to the configuration of different devices.
In this example it will be read in [device/transport_arduino.go](../../tide_client/device/transport_arduino.go)

## config.sdi12 | analog

Arduino can connect multiple sdi-12 or analog devices, so it is an array.

### config.sdi12[].model

Device model connected via sdi-12

### config.sdi12[].config

Configuration that will be read by the corresponding sdi-12 device. For example: [device/sdi12_ott_pls_c.go](../../tide_client/device/sdi12_ott_pls_c.go)

A few notes:

1. device_name: The name of the device, and it should be unique under a tide gauge station
2. item_type: The type of data, that will be used for front-end display and it is repeatable.
3. item_name: The name of the data.
   Because there may be multiple same item_types (such as air temperature) under a tide gauge station,
   we need another name to distinguish them.
4. cron: The cron expression used to read the data.
   [Check the cron documentation](https://pkg.go.dev/github.com/robfig/cron/v3),
   and Seconds field is optional.[Check the code where is setting `cron.SecondOptional`](../../tide_client/global/config.go)

### config.analog[]

Config that will be read by analog device.

# devices_uart_rs485_modbus.json

This sample config file exists under [`devices.uart`](#devicesuart--tcp--gpio), so it is connected via a local UART
RS485 adapter.

It reuses the `uart-rs485` transport and can host multiple Modbus RTU devices on the same bus, for example
`VEGAPULS61` and `ANALOG-VOLTAGE-MODBUS`.

## config[]

Each item in `config[]` is dispatched by [`tide_client/device/transport_uart_rs485.go`](../../tide_client/device/transport_uart_rs485.go).

`ANALOG-VOLTAGE-MODBUS` reads one input register with Modbus function `0x04` from register `0`, decodes the `uint16`
payload, and publishes the result as `rain_intensity`.
