# metrics 模块

Prometheus 指标服务：HTTP 请求指标 + 业务 gauge（Hub/DB/EventBus/磁盘/CPU）。

## API

- `New(snapshot func() Snapshot) *Server`：`snapshot` 为空时业务 gauge 保持 0。
- `(*Server).Middleware()`：Gin 中间件，统计请求数与耗时。
- `(*Server).Handler()`：`/metrics` 处理器（独立 registry，不污染默认）。
- `RequireToken(next, token)`：`METRICS_TOKEN` 鉴权包装。

## Snapshot 字段

`CPUPercent`、Hub 三项（房间/参与者/在线用户）、`WSClientDropped`、DB 连接与等待统计、EventBus 连接与丢弃计数、磁盘使用。由 `server_stats.go` 采集。

## 约束

- 指标名与标签是对外契约，改名视为破坏性变更。
- 不要把用户标识等高基数维度做成标签。
- `/metrics` 默认受 `METRICS_TOKEN` 保护；未配置 token 时按 `RequireToken` 的实现决定是否放行，改动前先确认线上暴露面。
