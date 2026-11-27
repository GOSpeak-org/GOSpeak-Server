# scripts — 工程门禁与发布脚本

Node 原生脚本（仅用 `node:*` 内置模块，零第三方依赖）。多数是 **CI / lefthook 门禁**，改了会影响全仓。

## 文件

| 脚本 | 作用 | 门禁位置 |
|------|------|----------|
| `check-version.mjs` | 版本一致性：以 `.github/versions/*.yaml` 为单一事实源 | CI `test-scripts` / 发布 |
| `release-naming.mjs` | 产物命名规范 `gospeak-<component>-v<semver>[-os-arch][.ext]` | release 工作流 |
| `release-notes.mjs` | 按组件生成 Release 正文（`--component server\|desktop\|mobile`；server 含二进制表+Docker+部署指引，desktop 只含安装包表，mobile 只含 APK 说明；一组件一 Release） | release 工作流 |
| `build-changelog.mjs` | 按版本生成 CHANGELOG 段并幂等插入顶部 | 发版 |
| `check-proxyrules-sync.mjs` | 服务端 `server/internal/plugin/proxyrules/` 副本 ↔ GOSpeak-Client 仓 `proxyrules/` 源实现逐字节一致（sha256）；`--fix` 用远端内容覆盖副本 | CI `proxyrules-sync` job（gate 必过） |
| `dev-livekit.sh` | 本地 LiveKit 开发启动（宿主机二进制 `.local/bin/livekit-server`） | 手动 |

> 前端与客户端门禁（UI token / 圆角 / hook-deps / 桌面桥 / 移动桥 / gomobile 绑定）随三仓拆分归 GOSpeak-Client 仓的 `scripts/`，本仓不再维护；本仓 CI `lint` 只跑 `go vet` 与 `deploy/init.sh --check`。

## 约定

- **发布类脚本必须配同名 `.test.mjs`**（`build-changelog` / `check-version` / `release-naming` / `release-notes` / `check-proxyrules-sync`），由 `node --test` 在 CI `test-scripts` job 跑；改脚本必须同步改测试。
- `proxyrules` 在服务端是**副本**，单一事实源在 GOSpeak-Client 仓。改规则只能改客户端仓，再用 `check-proxyrules-sync.mjs --fix` 同步；不要直接改副本（CI 会红）。
- 版本单一事实源在 `.github/versions/*.yaml`，不要在脚本里硬编码版本号。
- `check-version.mjs` 覆盖 server / desktop / mobile：mobile 以 `.github/versions/mobile.yaml` 为声明源，校验 Client 仓 `mobile/package.json`、Android `build.gradle`、iOS `project.pbxproj`；`release-naming.mjs` 已定义 mobile tag 前缀（`gospeak-mobile-v`）。
- `dev-livekit.sh` 依赖本地编译的 LiveKit 二进制，找不到时会打印编译指引。
