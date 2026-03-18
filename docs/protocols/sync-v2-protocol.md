# Sync V2 协议说明

Purpose: document Sync V2 runtime flow, code organization, and endpoint behavior for both station sync and relay sync.

Audience: developers working on Sync V2 client, server, or protocol changes.

Related Docs: [Documentation index](../README.md), [proto README](../../proto/sync/v2/README.md), [tide_server guide](../../tide_server/README.md), [tide_client guide](../../tide_client/README.md)

本文档描述基于 HTTP Upgrade + `yamux` + protobuf-delimited 的 v2 数据同步协议，包含两条链路：

- **站点同步**：tide_client ↔ tide_server（边缘采集器 → 中心服务）
- **级联同步**：tide_server ↔ tide_server（下游服务 → 上游服务）

Proto 定义：`proto/sync/v2/sync_v2.proto`

Go 生成包：`pkg/pb/syncproto`

## 代码组织与接线

当前 Sync V2 代码按职责拆成 4 层：

- `internal/syncv2`：共享路径常量、HTTP Upgrade、protobuf-delimited stream wrapper、`StationInfo` 转换。
- `tide_client/syncv2`：站点侧 v2 client，负责握手、replay、实时转发和命令流快照。
- `tide_server/syncv2/station`：站点接入 handler/server/store/registry。
- `tide_server/syncv2/relay`：级联同步的上游 handler/server 和下游 client/apply/store。

controller 层只负责接线，不再承载协议主体：

- 客户端接线：`tide_client/controller/sync_v2.go`
- 服务端路由：`tide_server/controller/router.go`
- 服务端依赖装配：`tide_server/controller/syncv2_adapters.go`

运行时固定入口如下：

- `POST /sync_v2/station`：站点同步，HTTP Upgrade + `yamux` + `StationMessage`，不使用 token。
- `POST /sync_v2/relay`：级联同步，HTTP Upgrade + `RelayMessage`，由 `/login` 获取 Bearer token 后接入。

---

## 一、站点同步（tide_client → tide_server）

### 相关文件

| 角色    | 文件                                          |
|-------|---------------------------------------------|
| 共享实现  | `internal/syncv2/*`                         |
| 客户端实现 | `tide_client/syncv2/*`                      |
| 客户端接线 | `tide_client/controller/sync_v2.go`         |
| 服务端实现 | `tide_server/syncv2/station/*`              |
| 服务端路由 | `tide_server/controller/router.go`          |
| 服务端装配 | `tide_server/controller/syncv2_adapters.go` |

### 消息时序

```
Client                              Server
  │                                    │
  │──── ClientHello ──────────────────>│  1. 握手：站点标识 + 协议版本
  │<──── ServerHello ────────────-─────│  2. 服务端确认
  │──── StationInfo ──────────────────>│  3. 设备列表 + 摄像头列表
  │<──── ItemsLatest ────────────-─────│  4. 服务端已有的每个 item 最新时间戳
  │<──── StatusLatest ──────────-──────│  5. 服务端已有的最新状态日志行号
  │                                    │
  │──── DataBatch(replay=true) ───-───>│  6. 补发缺失的历史数据（可能多批）
  │──── ItemStatusBatch(replay=true) ─>│  7. 补发缺失的状态日志
  │                                    │
  │──── DataBatch(replay=false) ──-───>│  8. 实时数据（持续）
  │──── ItemStatusBatch(replay=false) >│  9. 实时状态变更（持续）
  │──── RpiStatus ────────────────-───>│ 10. 树莓派状态（持续）
  │                                    │
  │<──── CameraSnapshotRequest ───-────│ 11. 服务端请求摄像头快照（按需）
  │──── CameraSnapshotResponse ──-────>│ 12. 回传完整快照数据或错误
```

### 流程详解

#### 1. 建立连接

客户端通过 HTTP Upgrade 连接服务端 `/sync_v2/station`，随后在该连接上建立 `yamux` 会话：

- 主子流（stream-1）：持续收发 `StationMessage`（握手/replay/实时）
- 命令子流（按需新开）：处理按次命令（当前为摄像头抓拍）

#### 2. 握手

- 客户端发送 `ClientHello{station_identifier, protocol_version}`
- 服务端根据 `station_identifier` 查找数据库中的站点 UUID
- 同一站点同一时间只允许一个 v2 连接（通过 `sync.Map` 去重）
- 服务端返回 `ServerHello{server_version}`

#### 3. 站点信息同步

- 客户端发送 `StationInfo`（设备 map、摄像头列表）
- 服务端将设备、item、摄像头信息同步到数据库，更新站点状态为"正常"

#### 4. 历史数据补发（Replay）

服务端告知客户端已有数据的最新位置：

- `ItemsLatest`：每个 item 的最新数据时间戳（毫秒）
- `StatusLatest`：最新状态日志的 RowID

客户端据此查询本地 SQLite，补发缺失数据：

- 数据：按 128 条一批发送 `DataBatch{replay=true}`
- 状态日志：一次性发送 `ItemStatusBatch{replay=true}`

**关键设计**：replay 阶段持有 `ingestMu` 锁，replay 完成后立即订阅本地 pubsub 增量通道，然后释放锁。这保证 replay 和实时数据之间无间隙。

#### 5. 实时数据转发

客户端从本地 pubsub 收到新数据后，转换为 protobuf 帧写入 `outgoing` channel（容量 2048），由独立 goroutine 发送到流上：

- `MsgData` / `MsgGpioData` → `DataBatch{replay=false}`
- `MsgItemStatus` → `ItemStatusBatch{replay=false}`
- `MsgRpiStatus` → `RpiStatus`

#### 6. 摄像头快照

服务端通过现有摄像头 HTTP 接口触发时，会优先走 v2 命令子流，发送 `CameraSnapshotRequest{camera_name}`。客户端在该子流中回传单帧 `CameraSnapshotResponse{data,error}` 并关闭子流。主同步流不受影响，后续可扩展更多命令类型。

如果目标站点当前没有 v2 连接，服务端再退回旧抓拍链路；外部 HTTP API 不变。

#### 7. 服务端数据处理

- 数据写入 PostgreSQL（`ON CONFLICT DO NOTHING` 防重复）
- replay 数据发布到 `missDataPubSub`，实时数据发布到 `dataPubSubDelay`
- GPIO 数据同时更新 item 状态
- 状态日志保存并发布到 `configPubSub`

### 断线重连

客户端断开后每 3 秒重试。

- 只有当 `sync_v2.enabled=false` 或 `sync_v2.addr` 为空时，客户端才会直接走 v1 同步。
- 一旦启用 v2，单次 v2 会话退出后会继续重试 v2，不会因为连接失败自动切回 v1。

---

## 二、级联同步（tide_server → tide_server）

### 相关文件

| 角色    | 文件                                               |
|-------|--------------------------------------------------|
| 共享实现  | `internal/syncv2/*`                              |
| 下游客户端 | `tide_server/syncv2/relay/*`                     |
| 上游服务端 | `tide_server/syncv2/relay/*`                     |
| 路由入口  | `tide_server/controller/router.go`               |
| 依赖装配  | `tide_server/controller/syncv2_adapters.go`      |
| 启动入口  | `tide_server/controller/sync_client.go`          |

### 消息时序

```
Downstream                           Upstream
  │                                    │
  │── HTTP Upgrade (Bearer token) ───>│  1. HTTP 层认证
  │── RelayDownstreamHello ───────────────>│  2. 协议握手开始
  │<── RelayUpstreamHello ─────────────────│  3. 上游确认
  │<── RelayAvailableItems ────────────────│  4. 可用 item 列表
  │<── RelayConfigBatch(full_sync) ────│  5. 全量配置同步
  │                                    │
  │── RelayStatusLatest ──────────>│  6. 下游已有的状态日志位置
  │<── RelayConfigBatch(events) ───────│  7. 补发缺失的状态日志
  │── RelayItemsLatest ───────────>│  8. 下游已有的数据位置
  │<── RelayDataBatch [多批] ──────────│  9. 补发缺失的数据
  │<── RelayDataBatch(空) ─────────────│ 10. 补发结束标记
  │                                    │
  │<── RelayConfigBatch(events) ───────│ 11. 增量配置变更（持续）
  │<── RelayDataBatch ─────────────────│ 12. 实时数据（持续）
  │<── RelayStatusEvent ───────────────│ 13. 实时状态变更（持续）
  │<── RelayAvailableItems ────────────────│ 14. 可用 item 变更（按需）
```

### 流程详解

#### 1. 认证

下游先使用配置的上游账号（`username/password`）调用 `/login` 获取 access token，再以 `Authorization: Bearer <token>` 发起 `/sync_v2/relay` 的 HTTP Upgrade。上游使用统一 token 认证逻辑并按认证账号权限过滤同步数据。

管理员用户无数据过滤；普通用户根据权限过滤可见的站点和数据。

#### 2. 全量配置同步

上游发送：

- `RelayAvailableItems`：按站点分组的可用 item 列表
- `RelayConfigBatch{full_sync=true}`：所有站点完整信息（含设备、item、设备记录）

下游对比本地数据库，同步新增/更新/删除。

**关键设计**：上游在全量同步之前先订阅 pubsub，确保同步期间的增量变更不会丢失。

#### 3. 缺失数据补发

- 下游发送 `RelayStatusLatest`（每个站点的最新状态日志行号）
- 上游补发缺失的状态日志，封装为 `RelayConfigBatch{events}`
- 下游发送 `RelayItemsLatest`（每个站点每个 item 的最新时间戳）
- 上游补发缺失的数据，发送多个 `RelayDataBatch`，以空批次标记结束

#### 4. 增量同步

上游从 pubsub 订阅通道持续读取变更，转换为 protobuf 帧发送：

- 配置变更 → `RelayConfigBatch{events=[RelayConfigEvent{type, payload}]}`
- 数据 → `RelayDataBatch{station_id, data_type, points}`
- 状态变更 → `RelayStatusEvent{station_id, identifier, ...}`
- 可用 item 变更 → `RelayAvailableItems`

### 断线重连

下游断开后每 10 秒重试。断开时清理本地可用 item 记录（`db.RemoveAvailableByUpstreamId`）。

---

## 三、配置

`sync_v2` 只改变同步实现，不改变现有业务 HTTP API 路径。

### 客户端配置（tide_client/config.json）

```json
{
    "sync_v2": {
        "enabled": true,
        "addr": "http://192.168.1.3:7100"
    }
}
```

### 服务端配置（tide_server/config.json）

```json
{
    "sync_v2": {
        "enabled": true
    }
}
```

级联同步使用 upstream 账号登录后获得的 Bearer token；站点同步链路不使用 token。
