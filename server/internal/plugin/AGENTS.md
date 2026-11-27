# plugin 模块

GOSpeak 能力插件运行层：子进程管理、gRPC 注册/API/Hook、运行时管理、页面挂载与浏览器代理。

## 职责与边界

- `manager.go` / `lifecycle.go`：插件安装、升级、重载、卸载、启停与退避重启；`runtime` 相关文件只做人工选版的 Node.js 运行时安装/切换（Python 仅预留接口）。
- `api.go` / `hook.go` / `pageproxy.go` / `browser_proxy.go`：插件 API、Hook 推送与页面挂载；写操作委托现有 service 并以系统身份记录 `ActorKind=plugin` 审计（旁路）。
- 管理 HTTP 入口在 `internal/handler` 与 `internal/router/routes/plugin`；前端管理页在 Client 仓的 `web/src/pages/(app)/manage/plugins`。

## 单一事实源

- **`proxyrules` 是副本**：源实现在 GOSpeak-Client 仓的 `proxyrules/`（桌面与移动端 gomobile 共用），服务端 `internal/plugin/proxyrules/` 与之逐字节一致，由 `scripts/check-proxyrules-sync.mjs` 在 CI 强制（漂移即红）。改规则只能改客户端仓，再 `--fix` 同步。
- **协议只认** `server/plugin/proto/plugin.proto`，Go 生成物在 `internal/plugin/gen/`（勿手改），JS SDK 副本由 Client 仓的 `packages/plugin-sdk/scripts/copy-proto.mjs` 同步（勿手改副本）。
- 错误码只认 `internal/pkg/errors.go`；gRPC 调用以标准 `codes` 返回，不在这里另造业务码。
- 管理 API 精确路由以 `internal/router/routes/plugin/routes.go` 为准，不在注释里复述清单。

## 运行时与安全约束

- 根卷硬编码 `/app/plugins`；镜像 `Dockerfile` 预置 `ENV PATH=/app/plugins/runtime/bin:$PATH`。**没有插件 env**，无需在 `deploy/.env.example` 增加任何插件变量。
- **零自动下载**：runtime 插件启动时不会下载任何版本；未安装 Node 时主程序 panic，由管理员在管理页面选择版本、下载校验并切换软链。
- 插件进程经本地 TCP 回环 gRPC 通信，凭据由一次性文件注入（`GOSPEAK_PLUGIN_CREDENTIAL`）；插件注册后即可调用宿主能力，无需额外凭据或权限配置。
- **拉取即信任**：市场/GitHub/本地文件上传安装后，插件直接拥有宿主全部能力；无审核、无验签、无沙箱、无运行期权限边界。文档与管理页必须显著提示这一风险，来源由管理员自行甄别。
