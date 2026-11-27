# config 模块

环境变量解析、归一化与启动期校验。**全部配置来自环境，没有独立配置文件。**

## 文件

| 文件 | 职责 |
|------|------|
| `config.go` | `Config` 结构（goenv tag + 默认值）、`Load`/`MustLoad`、派生方法 |
| `config_normalize.go` | `normalize()`：DB 别名归一（如 Turso）、LiveKit URL 规整 |
| `config_validate.go` | `Validate()`：启动期尽早失败 |

## 加载优先级

`GOSPEAK_ENV_FILE`（显式）> 工作目录 `.env.<env>` / `.env` > 仓库根 `deploy/.env`。已存在的进程环境变量不被覆盖。入口 `LoadEnvFiles(appEnv)`。

## 关键单点

- `DefaultSFUProvider = "livekit"` 是 provider 名的**唯一字面量**，sfu 包与校验都引用它，不要另写字面量。
- `NATStoreDirPath()` 决定 JetStream 持久化目录（空则临时目录，重启丢密钥/黑名单）。
- `EffectiveDSN()`、`IsTurso()` 处理 SQLite/Turso DSN 差异。
- `IsProduction()` / `IsDevelopment()` 驱动日志与安全默认值。

## 校验会拦住的典型错误

`DB_TYPE` 不在白名单、非 SQLite 缺 `DB_HOST`、`SFU_PROVIDER` 非 livekit、`STORAGE_TYPE` 非 local/s3、`ACCESS_COOKIE_NAME` 与 `REFRESH_COOKIE_NAME` 相同、`COOKIE_PATH` 不以 `/` 开头、`COOKIE_SECURE`/`COOKIE_SAMESITE` 取值非法；生产环境 `STORAGE_ENCRYPT_KEY` 必须是 64 位 hex。

## 约定

- 新增配置必须同时给 `env` tag、`envDefault`、并在 `Validate` 加白名单/必填校验（需要时）。
- **不要把配置读取散落到包级 init**；模块通过 `Configure(cfg)` 注入，或由 `server/gin.go` 传参。
- 完整字段清单以本包代码为准，不要在文档里抄一份（易腐化）。
