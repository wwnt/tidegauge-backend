- [1. Prerequisite](#1-prerequisite)
- [2. Install Syncthing](#2-install-syncthing)
- [3. Init postgresql database](#3-init-postgresql-database)
- [4. Build](#4-build)
    - [4.1. Windows or Linux](#41-windows-or-linux)
    - [4.2. Docker](#42-docker)
- [5. Run](#5-run)
    - [5.1. Linux Service](#51-linux-service)
    - [5.2. Windows Service](#52-windows-service)
    - [5.3. Docker](#53-docker)

# 1. Prerequisite

- Postgresql
- Syncthing

# 2. Install Syncthing

following https://docs.syncthing.net/users/autostart.html

# 3. Init postgresql database

`psql -d tidegauge -U postgres -f tide_server/schema.sql`

# 4. Build

## 4.1. Windows or Linux

`go build`

```shell
$ ./tide_server -h
Usage of ./tide_server:
  -config string
        Config file (default "config.json")
  -debug
        debug mode (default true)
  -dir string
        working dir (default ".")
```

## 4.2. Docker

```shell
docker build -f tide_server/Dockerfile -t wwnt/tide-server .
```

# 5. Run

## 5.1. Linux Service

```shell
cp tidegauge.service /etc/systemd/system
sudoedit /etc/systemd/system/tidegauge.service
# change USER,GROUP,WorkingDirectory,ExecStart to your own
```

## 5.2. Windows Service

```shell
New-Service -Name "tide" -BinaryPathName "E:\tide\tide_server.exe -dir E:\tide" -StartupType "AutomaticDelayedStart"
```

## 5.3. Docker

```yaml
version: "3"
services:
  tide-server:
    image: wwnt/tide-server
    container_name: tide-server
    volumes:
      - ./docker:/var/tide_server
    ports:
      - "7100:7100"
      - "7102:7102"
    restart: unless-stopped
```

