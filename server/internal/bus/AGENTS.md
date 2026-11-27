# bus 模块

进程内嵌事件总线与共享状态（JetStream KV）。**固定使用内嵌 NATS，不支持外部 `NATS_URL`。**

## 能力

| 能力 | 后端 | 用途 |
|------|------|------|
| EventBus | 内嵌 NATS | WS 跨实例 fanout（room/namespace）与内部事件 |
| MembershipStore | NATS KV | 房间成员 / stream 映射 |
| AuthStore | NATS KV | JWT 黑名单 + 签名密钥轮换 |
| JobQueue | JetStream | 异步任务（聊天持久化、禁言到期） |
| WAL | 本地文件 | 可选：断线事件磁盘持久化 |

## 入口

`Init(cfg InitConfig) (EventBus, cleanup func(), error)`，见 `factory.go`。

`InitConfig` 字段：`Prefix` / `Name` / `ConnectTimeout` / `EmbeddedPort`（<=0 随机）/ `StoreDir`（空则临时目录）/ `Deliverer` / `RemoteHook` / `WALPath`。

- `StoreDir` 非空时 JetStream 跨重启保留 KV（签名密钥、黑名单）；生产装配由 `config.NATStoreDirPath()` 提供（显式 `NATS_STORE_DIR` 优先，否则回退 DB 同级 `nats/` 目录），compose 以 `gospeak-nats` 卷挂载保证跨容器重建留存。仅测试/未配置场景才会落临时目录（`MkdirTemp` + Shutdown 清理）。
- `cleanup` 必须在进程退出时调用：先 Close bus，再 Shutdown 内嵌服务；持久化 StoreDir 不会被 Shutdown 删除。
- 内嵌总线**不做选主**，也不做写面 fence（单实例架构）。

## 约束

- 客户端广播走 `publishRoom` / `publishNamespace`，不要裸 `BroadcastToRoom`。
- membership 写入按 `InstanceID` 合并，禁止整房覆盖远端成员。
- stream leave/kick/disconnect 必须 `DeleteStream`。
- 不要重新引入 `STATE_STORE` / 外部 NATS / 内存降级分支——目前不存在这些配置项。
