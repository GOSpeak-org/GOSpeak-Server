# webui 模块

内嵌前端构建产物，供单二进制部署直接托管 SPA。

## 文件

| 文件 | 构建标签 | 行为 |
|------|----------|------|
| `embed.go` | 默认（`!noembedweb`） | `//go:embed all:dist`，`HasAssets()` 为真 |
| `stub.go` | `noembedweb` | `HasAssets()` 恒 false，开发模式不嵌前端 |

## API

`HasAssets() bool`、`FS() http.FileSystem`。

## 约定

- 仓库内保留 `EMBED_PLACEHOLDER` 保证 `go:embed` 合法；真实产物由构建流程写入 `dist/`。
- 静态目录优先级在 router 侧决定（`STATIC_DIR` > `/app/static` > `./static` > 内嵌资源）。
- 开发时用 `noembedweb` 或不打包，前端由 Vite 独立托管。
- 不要把大文件（视频/模型）塞进 embed，会显著增大二进制。
