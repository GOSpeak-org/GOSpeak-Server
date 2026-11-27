# 后端能力插件化：市场安装 + 人工管理运行时 + gRPC/TCP + 插件页面挂载

> 【状态】2026-11-27 核验：该设计已落地（插件平台已从 feature-plugin-platform 迁移），仅供历史参考。

日期：2026-11-24
分支：待创建（建议 feature/plugin-subprocess）

## 背景与目标

团队希望用任意语言编写扩展（内容审核、外部通知、自定义页面等），不修改主仓库即可挂载。方案参照 AstrBot + VS Code：插件从**插件市场/GitHub 在线拉取安装**（拉取即信任）；以**子进程**运行，主程序通过 gRPC over 本地 TCP 回环端口通信。插件注册后直接拥有宿主能力（无 token、无能力点、无 Principal 改造）；Node.js 运行时由 **runtime 插件**提供，但**启动时零自动下载**——由管理员在**管理页面**选择要下载/切换哪个版本，软链到 PATH 全局共享。

## 已确认的决策

| 议题 | 决策 |
| --- | --- |
| 信任模型 | **拉取即信任**：从市场/GitHub 拉取的插件不经审核、不验签，安装后直接拥有宿主全部能力；无 SHA-256 白名单、无沙箱、无运行期权限边界 |
| 插件来源 | 插件市场 / GitHub URL / 本地文件上传（管理 API 或页面触发）；支持安装、升级（重新拉取）、卸载、热重载 |
| 实现语言 | 异构；主程序只按约定协议启动进程，当前以 Node.js 插件为主 |
| 运行形态 | 子进程（fork/exec + 本地 TCP 回环 + 心跳 + 退避重启 + 优雅关闭） |
| 配置 | 无插件 env；根卷硬编码 /app/plugins（plugin.RootDir） |
| 运行时管理（多语言预留） | runtime 插件（Go 静态二进制，最先启动）**不自动下载**任何版本；管理员通过管理页面按语言选版本 → runtime 下载（校验官方 SHASUMS256）→ 软链到 runtime/bin/；Dockerfile 预置 PATH；首版实现 **Node.js**，Python 仅预留接口与页面入口（不实现下载）；若启动时无已装 Node → 主程序 panic |
| 插件依赖 | 插件自己 npm ci（装自己的 node_modules，不共享）；metadata.yaml 可配 npm_registry |
| 插件 API | gRPC Server over 127.0.0.1:<动态端口>；读 GetUser/ListRooms，写 MuteUser/CreateRoom（委托现有 service，系统身份）；无 GetNode RPC |
| SFU 范围 | 不新增 SFU 后端；插件仅间接调用现有 LiveKit 能力 |
| 审计 | 插件写操作 ActorKind=plugin，沿用现有 action/target；审计旁路 |
| 插件页面 | 注册时声明路由 + 模式（spa 反代 / iframe），管理员确认路由前缀 |

## 非目标

- 不做插件安全沙箱、能力点、token、Principal 改造——拉取即信任模型不做运行期权限限制。
- 不做插件代码签名/市场验签——用户需自行甄别来源（文档显著警告，参照 AstrBot）。
- 不新增 SFU provider、不引入外部 NATS、不做远程插件进程（子进程限本机）。
- 不做多实例（socket/端口按实例独立；当前单实例）。
- 不改现有用户权限体系与前端页面源码（插件页面通过独立路由挂载）。

## 架构总览

```
GOSpeak 主进程（PluginManager + 管理 API/WebUI）
  │  0. 拉起 gospeak.runtime（Go 静态二进制，无自动下载）
  │     → 检查 runtime/bin/node 是否已由管理员安装过
  │     → 未安装 → 主程序 panic（提示去管理页面选择运行时）
  │  1. 管理员在页面选版本 → runtime 下载+校验+软链 → 重启业务插件
  │  2. 管理员从市场/GitHub 安装插件 → 拉到 bin/<id>/ → npm ci → 启动 → Register
  │  3. 运行中：插件 API（读写委托 service）/ Hook 推送 / 页面代理 / 审计
```

## 设计

### A. 通信协议（.proto，单一事实源）

- RegistrationService.Register({plugin_id,name,version,hook_types,pages}) → {api_endpoint,confirmed_pages}（api_endpoint = 127.0.0.1:<port>）；Heartbeat 保活。无 GetNode RPC。
- PluginAPIService：读 GetUser(user_uuid)/ListRooms(domain_uuid)；写 MuteUser/CreateRoom（reason + request_id 幂等；operator 用系统保留用户 UUID，seeds 定义不可登录）。KickMember/UpdateMessageStatus 首版无 service 契约不提供。字段一律 *_uuid。
- PluginHookService.HandleEvent：主程序订阅 bus 内部事件 → 白名单裁剪 → gRPC 推送；敏感事件不推。
- **RuntimeService**（runtime 插件实现，主程序调用）：Install({kind: "node"|"python", version, mirror?} → {installed, unsupported?})、List({kind?} → [{kind, version, active}])、Use({kind, version} → {ok})。首版仅 kind=node 实现完整下载/校验/软链；kind=python 返回 `unsupported=true`（预留接入，后续 Task 实现 python-build-standalone 下载）。runtime 启动时不自动调用任何方法。
- 跨语言生成：Go 用 protoc-gen-go/grpc；Node.js 插件用 grpc-js + @grpc/proto-loader（由 JS SDK 封装）。

### B. 目录布局与 metadata.yaml

```
/app/plugins/                          # 固定根卷 gospeak-plugins（持久化）
├── bin/
│   ├── gospeak.runtime/entrypoint     # runtime 插件（Go 静态二进制）
│   └── com.example.audit/             # 从市场/GitHub 拉取的插件代码（可被升级覆盖）
│       ├── metadata.yaml / package.json / package-lock.json / index.js
│       └── node_modules/              # 插件自己 npm ci
└── runtime/
    ├── bin/node                       # 当前激活的 Node.js 软链（PATH 指向这里）
    ├── download/node/v20.19.0/        # Node 版本缓存（python/ 目录预留）
    ├── credentials/                   # 一次性凭据文件（含端口）
    └── state/<plugin_id>/             # 插件数据（升级/卸载不删）
```

metadata.yaml（插件作者提供，来自市场/GitHub 仓库）：
```yaml
runtime: node                          # 预留：python（首版未开放，页面/接口已留）
plugin_id: com.example.audit
name: 内容审核插件
version: 1.2.0                         # 插件版本
entrypoint: index.js                   # 可选，默认 index.js
npm_registry: https://registry.npmmirror.com  # 可选
npm_args: ["--omit=dev"]               # 可选
```
- 主程序动态分配 127.0.0.1 端口，端口经一次性凭据文件（0600）传递；GOSPEAK_PLUGIN_CREDENTIAL 指向该文件（唯一注入变量）；JS SDK 读取后连接。
- Dockerfile 预置 ENV PATH=/app/plugins/runtime/bin:$PATH。
- bin/ 下目录可含 npm 正常软链（.bin/）；不做递归符号链接拒绝（避免误伤 npm）。

### C. runtime 插件（人工管理运行时）

- 启动顺序：主程序校验 runtime 插件二进制 → exec → Register 握手 → 检查 runtime/bin/node 是否存在且可执行。
- **零自动下载**：runtime 插件启动时不下载任何运行时版本。若 runtime/bin/node 不存在（Node 未安装）→ 主程序 panic（日志提示管理员去管理页面选择运行时）；Python 缺失不影响启动（未安装 Python 插件时无需运行时）。
- 管理员通过管理页面/API 调用 RuntimeService.Install({kind:"node", version, mirror?})：下载 tarball → 校验官方 SHASUMS256 → 解压到 download/node/v<version>/；List 查看已装版本；Use 切换软链并触发所有 Node 插件重启。kind=python 调用返回 unsupported（页面显示入口但提示未开放）。
- 镜像/镜像地址由管理员在页面指定（如国内镜像），runtime 插件不硬编码版本。
- runtime 插件运行中崩溃 → 退避重启；重启期间不新启业务插件。

### D. 插件生命周期（AstrBot 风格：市场/GitHub 安装 + 重载）

- 安装：POST /api/v1/plugin/install {source: github|market|file, url/ref/file} → 拉取到 bin/<id>/ → 校验 metadata → npm ci → 启动 → Register。拉取即信任，无审核/验签。
- 升级：POST /api/v1/plugin/upgrade {plugin_id} → 重新拉取最新版本覆盖 bin/<id>/ → npm ci → 重启插件（state 目录保留）。失败保留旧版本日志告警。
- 热重载：POST /api/v1/plugin/reload {plugin_id} → 停进程 → 重新 exec 当前目录 → Register（不拉新代码，用于配置/调试）。
- 卸载：POST /api/v1/plugin/uninstall {plugin_id} → 停进程 → 删 bin/<id>/ → 保留 runtime/state/<id>/ 与审计记录。
- 开关：POST /api/v1/plugin/enable|disable（持久化到 plugin_registrations.enabled，重启后保持）。
- 业务插件失败（拉取/npm ci/Register 失败）→ 跳过 + 告警，不 panic（只有 runtime 无 node 才 panic）。
- 优雅关闭：HookDispatcher 停 → Manager.Stop（grpc.GracefulStop，独立 10s ctx）→ bus 关 → audit.Stop。
- 日志/错误：插件 stdout/stderr 接主 logger（component=plugin:<id>）；新增 ReportError RPC 结构化错误上报。
- 熔断：连续 10 次 Hook 超时/错误 → 自动 disable 该插件 + 告警。

### E. 注册表、配置与数据

- plugin_registrations：plugin_id / source / version / enabled / allowed_route_prefix / 时间戳（无 sha256 字段）。
- 插件配置：plugin_configs（plugin_id, config_json），管理 API PUT 配置；插件通过 GetConfig RPC 按需拉取（缓存 30s）。
- 插件数据：通用 plugin_kv（plugin_id, key, value_json）+ KVSave/KVLoad RPC；大块临时数据放 runtime/state/<id>/（经 GOSPEAK_PLUGIN_STATE_DIR 注入）。插件不直接访问主库。
- 幂等：plugin_request_log(plugin_id, request_id, created_at)，保留 24h，jobs 周期清理。
- 审计：Entry.ActorKind（默认 user）+ AuditLog.actor_kind 列；插件写操作 ActorKind=plugin，沿用现有 action/target；旁路不阻断。
- 管理端权限：以上管理 API 走 protected 组 + 现有管理员角色（不新增权限码）。
- Python 预留：proto kind 字段、runtime/download/python/ 目录、管理页面 Python tab（提示未开放）；python-build-standalone 下载实现留后续 Task，不在首版。

### F. JS SDK（packages/plugin-sdk）

- workspace 包 @gospeak/plugin-sdk，内置 proto 与生成代码，全封装：createPlugin({onEvent}) → {api.getUser/muteUser/createRoom, on(event, handler), config, kv, reportError}。
- 插件作者零接触 grpc-js / proto / 端口 / 凭据文件；SDK 内部读 GOSPEAK_PLUGIN_CREDENTIAL 拿端口。
- 示例插件放 test/plugin/testdata/，作为模板；文档说明复制模板 → 改 metadata → 实现 Hook → 调 API。

### G. 可观测性与部署

- gRPC 标准错误码；指标 gospeak_plugin_ 前缀（安装数、活跃数、重启数、API 调用/错误数）；plugin_id 低基数标签。
- compose 增加 gospeak-plugins 卷挂载 /app/plugins；镜像 Dockerfile 加 ENV PATH；Node.js 不进镜像（管理员页面选装）。
- grpc/protobuf 转 direct；proto 生成物提交，CI proto-check。

### H. 插件页面挂载

- 声明与路由：Register 上报 pages（route_path/mode/entry_url），强制 /plugins/<plugin_id>/ 前缀，管理员确认后挂载。
- spa 反代：Gin 通配路由优先于 SPA fallback，复用 app/shared/proxyrules；浏览器代理端点 POST /api/v1/plugin/browser/call + /refresh（用户 JWT 校验 + 路由前缀约束）。
- iframe：`src/pages/(app)/plugins/$pluginId/$.tsx`（tsr generate 提交 routeTree.gen.ts）；sandbox + origin/source/type 校验 + CSP frame-ancestors。
- 主题：postMessage 同步 theme.css CSS 变量；用户访问按现有 protected + 角色控制。

## 风险与警告

| 风险 | 缓解 |
| --- | --- |
| **恶意插件获得宿主全部能力** | **无技术缓解**——拉取即信任，用户必须自行甄别插件来源（显著文档警告，参照 AstrBot） |
| 插件崩溃/失控 | 子进程隔离 + 心跳 + 退避 + 连续失败 disable；容器 cgroup 兜底资源 |
| runtime 无 Node | 启动 panic，提示去管理页面选装 |
| npm 依赖问题 | 各插件 node_modules 隔离 + package-lock 锁定 |
| 页面越界 | sandbox + origin 校验 + CSP |

**参考实践**：AstrBot（市场/GitHub 安装 + 热重载 + 数据分离目录 + 来源警告）；VS Code Extension Host（子进程 + 单份共享 Node + package.json 入口）；HashiCorp go-plugin / Grafana（子进程 + gRPC 异构范式）。

## 实施顺序

1. proto（含通用 RuntimeService.Install/List/Use，python 预留）与生成物、CI proto-check。
2. JS SDK 包（packages/plugin-sdk）+ 示例插件模板。
3. 数据表（注册表/配置/KV/请求日志 + actor_kind）+ 审计扩展 + 系统用户 UUID + compose 卷。
4. runtime 插件（零自动下载 + 按管理指令下载/切换 Node）+ 无 node 时 panic；镜像 ENV PATH。
5. PluginManager（市场/GitHub 拉取 + npm ci + exec + TCP + 心跳 + 优雅关闭 + 安装/升级/重载/卸载/开关）。
6. 注册握手 + 插件 API（读写 + 配置/KV/错误上报 + 审计）。
7. Hook 推送 + 页面挂载（spa 反代 + iframe + 浏览器代理）。
8. 指标 + 管理页面（运行时选择、插件安装/升级）+ 端到端测试。