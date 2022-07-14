# Prerequisite

- Postgresql 
- Syncthing

# Build

### Docker

```shell
docker build -f tide_server/Dockerfile -t wwnt/tide-server .
```

# Run

**Windows Service**

```shell
New-Service -Name "tide" -BinaryPathName "E:\tide\tide_server.exe" -StartupType "AutomaticDelayedStart"
```

**Docker compose**

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

# Install Syncthing

following https://docs.syncthing.net/users/autostart.html