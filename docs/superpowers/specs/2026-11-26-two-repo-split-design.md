# 聚合壳 + 三子仓设计

> 状态：**已定稿**。用户选定「顶级聚合仓（壳）+ 平级子仓」，客户端内部不拆。
> 取代：[Submodule 拆分计划](../plans/2026-11-26-submodule-split.md)。

## 1. 目标与非目标

**目标**
- 一个顶级聚合仓（壳）作为唯一入口，`git clone --recursive` 一次拿全部。
- 壳下面是**平级**子仓，**只一层父子、无嵌套**：`server` / `client` / `docs`。
- 客户端内部保持一仓：`web packages desktop mobile proxyrules` 同仓平级，**零内部 submodule**。
- `proxyrules` 从 `shared/` 提到客户端仓根，避免无谓嵌套。

**非目标**
- 不按功能域重组代码。
- 不改运行时行为、协议、API。
- 不引入新构建工具。

## 2. 结构

```
GOSpeak（顶级聚合壳，现有仓）
  .gitmodules            ← 3 条，全部平级
  README.md              ← 说明三子仓与一次 clone 命令
  server/  → GOSpeak-Server
  client/  → GOSpeak-Client
  docs/    → GOSpeak-Docs
```

```
GOSpeak-Server（服务端仓）
  server/                ← 原 app/server（Go 后端，module GOSpeak）
  test/                  ← 原 test（集成测试，拉起临时 server）
  deploy/                ← 原 deploy（compose / Caddyfile / LiveKit / .env.example）
  docs/                  ← 原 docs（设计、运维、评审档案）
  scripts/               ← 服务端与 release 脚本
  Dockerfile  VERSION  metadata.yaml  CHANGELOG.md
```

```
GOSpeak-Client（客户端仓）
  web/                   ← 原 app/web
  packages/              ← 原 packages（components / sfu-client / desktop / mobile-capabilities）
  desktop/               ← 原 app/desktop（Go + MyGo）
  mobile/                ← 原 app/mobile（Capacitor）
  proxyrules/            ← ★ 提到仓根（原 app/shared/proxyrules）
  scripts/               ← 客户端门禁脚本
  pnpm-workspace.yaml  biome.json  lefthook.yml  turbo.json  .gitignore
```

```
GOSpeak-Docs（文档站仓）
  .vitepress/  guide/  api/  package.json   ← 仓根即原 app/docs 内容
```

### 2.1 三条 submodule（全部平级，无嵌套）

```
[submodule "server"]
	path = server
	url = https://github.com/GOSpeak-org/GOSpeak-Server.git
	branch = release
[submodule "client"]
	path = client
	url = https://github.com/GOSpeak-org/GOSpeak-Client.git
	branch = release
[submodule "docs"]
	path = docs
	url = https://github.com/GOSpeak-org/GOSpeak-Docs.git
	branch = release
```

**关键**：子仓之间**互不引用**。客户端仓没有 submodule；服务端仓没有 submodule。所以不存在「改 A 仓 → 回父仓 bump → 再去 B 仓」的链式跳转——只有壳需要 bump，且是可选择的（不 bump 也能各自独立开发）。

### 2.2 proxyrules 提到客户端仓根

- 原位置 `app/shared/proxyrules/` → 新位置 `client/proxyrules/`
- `desktop/go.mod` 的 `replace ... => ../proxyrules`（从 `desktop/` 上溯一级即仓根）
- `mobile` 的 `gomobile bind` 用 `../proxyrules`（同仓，无需 AAR 直链）
- **无变更就不发新版本**：`proxyrules-v*` tag 只在其内容变化时打；打 tag 时 CI 补一道校验，与上一版比较 `proxyrules/**` 无差异则失败并提示「内容未变，无需发版」

## 3. 相对路径兼容性（核心优势）

| 引用点 | 现状 | 新结构 | 是否变化 |
|---|---|---|---|
| `desktop/mygo.config.ts` → `cp -R ../web/dist` | `app/desktop` → `app/web` | `desktop/` → `web/` | **不变** |
| `mobile/capacitor.config.ts` → `"../web/dist-mobile"` | `app/mobile` → `app/web` | `mobile/` → `web/` | **不变** |
| `desktop/go.mod` → `replace => ../shared/proxyrules` | `app/desktop` → `app/shared` | `desktop/` → `proxyrules/` | 改一级（`../proxyrules`） |
| `web/tailwind.config.ts` → `../../packages/components/src` | `app/web` 上溯两级 | `web/` 上溯一级 | 改一级（`../packages/...`） |
| `web/scripts/capture-storybook-baseline.mjs` 的 `repoRoot` | `../../..` | `../..` | 改一级 |

## 4. 跨仓断点（只有两处）

### 4.1 协议生成

协议定义在 `server/internal/protocol`，生成物消费方在 `client/web/src/protocol/generated`。

**方案（golden 为事实源）**：
1. 服务端仓提交 golden（`server/internal/genprotocol/testdata/golden/` 已有三份）
2. `gen_contract_test` 改为只比 golden，去掉对 `app/web` 的硬依赖
3. `gen-protocol` 的 `-out` 必填
4. 客户端新增 `scripts/check-protocol-sync.mjs`：用 `gh api` 拉服务端 golden 与本地 generated 逐字节比对

### 4.2 Dockerfile

服务端镜像不再构建前端，改为直链下载客户端产物：

```dockerfile
ARG CLIENT_WEB_URL
ADD $CLIENT_WEB_URL /tmp/web-dist.tar.gz
RUN mkdir -p /app/static && tar -xzf /tmp/web-dist.tar.gz -C /app/static \
 && test -f /app/static/index.html
```

## 5. 脚本归属

| 脚本 | 读什么 | 归属 |
|---|---|---|
| `check-desktop-commands.mjs` | desktop/ + packages/desktop | 客户端 |
| `check-hook-deps.mjs` | web/ | 客户端 |
| `check-mobile-bridge.mjs` | mobile/ + packages/mobile-capabilities | 客户端 |
| `check-radius-tokens.mjs` / `check-ui-tokens.mjs` | web/ + packages/components | 客户端 |
| `mobile-bind-proxyrules.mjs` | mobile/ + proxyrules/ | 客户端 |
| `mobile-version.mjs` | mobile/ + web/ | 客户端 |
| `check-version.mjs` | desktop/ + mobile/ + VERSION | 拆分：客户端校验三版本文件；服务端校验 `VERSION` |
| `release-naming.mjs` / `release-notes.mjs` / `build-changelog.mjs` | 无目录依赖 | 两仓各一份 |
| `gen:protocol` | server/ | 服务端 |

## 6. 四个仓的处置

| 仓 | 来源 | 历史 |
|---|---|---|
| `GOSpeak`（壳） | 现有仓**保留**，清空代码目录、加 `.gitmodules` | 保留 181 条 + 22 个 tag（作为历史记录） |
| `GOSpeak-Server` | 新建，从现有仓抽 `app/server` + `test` + `deploy` + `docs` + `Dockerfile` | `filter-branch` 抽取 |
| `GOSpeak-Client` | 新建，从现有仓抽 `app/{web,desktop,mobile}` + `app/shared/proxyrules`→`proxyrules` + `packages` | `filter-branch --tree-filter` 重排 |
| `GOSpeak-Docs` | 新建，从现有仓抽 `app/docs` | `filter-branch --subdirectory-filter` |

**为什么保留壳的历史与 tag**：`gospeak-server-v1.3.0-rc.1` 等 tag 是已发布版本的记录，删掉会丢失可追溯性。它们留在壳上作为历史，新版本在各自子仓打新 tag。

**永不复用 `NoelOrin/GOSpeak` 这个路径**：已装机客户端的更新检测与所有老安装包链接都指向它，靠 GitHub 的仓库级 301 活着。

## 7. 风险

| 风险 | 缓解 |
|---|---|
| 壳与子仓指针漂移 | 壳的 gitlink 只在「需要统一快照」时 bump；日常开发各子仓独立。提供 `scripts/check-shell-drift.mjs` 只报告不自动改 |
| 协议生成物跨仓不同步 | 客户端 `check-protocol-sync.mjs` 比对 golden |
| 客户端首个版本低于现网导致更新失联 | 客户端版本定为 `1.4.0-rc.1`（> 现网桌面 1.3.0-rc.1 / 移动 1.1.0-rc.2） |
| 拆分期间工作区改动丢失 | 先 `git bundle` + `pre-shell-split` tag |
| 壳读代码时只能看到指针 | README 写明「开发请 clone 对应子仓」，壳只用于统一归档 |

## 8. 与旧方案关系

取代 [Submodule 拆分计划](../plans/2026-11-26-submodule-split.md)：
- 挂载点从 6 个降到 **3 个**，且**全部平级无嵌套**
- 客户端仓内部 **0 submodule**
- 不使用 `git submodule update --remote`
- 保留其已交付的前置成果：一组件一 Release（`2026-11-26-per-component-release.md`）
