# buildinfo 模块

版本信息来源。

## API

- `ResolveVersion()`：启动时确定版本号。
- `readInfoFile(name)`：读仓库根 `VERSION` 与 `COMMIT`（另有 `ResolveCommit()` 查找同链）。
- `semverPattern`：校验 `X.Y.Z[-pre]` 形态。

## 优先级

链接期 `ldflags` 注入 > `VERSION` 文件 > 兜底默认值。CI 发布时用 `-ldflags` 覆盖（见 `release.yml`），本地开发走文件。

## 约束

- 版本号必须与 `.github/versions/*.yaml` 的声明一致，由 `scripts/check-version.mjs` 在 CI 校验。
- 不要在多处硬编码版本字符串；`pnpm version:check` 会抓不一致。
- `go run main.go version` 是用户可见输出，改格式要考虑脚本解析。
