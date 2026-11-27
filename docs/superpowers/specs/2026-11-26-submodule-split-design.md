# 多平台仓库结构调整设计

> 【状态】2026-11-27 核验：该方案已被「聚合壳 + 三子仓」取代废弃，仅供历史参考；实际采用方案见 `2026-11-26-two-repo-split-design.md`。

## 意图与原因

`app/web`（React 前端）、`app/desktop`（Go + MyGo 桌面宿主）、`app/mobile`（Capacitor 移动壳）、`packages/`（前端共享包）当前与主仓库同仓开发。核心痛点是**各端发布产物挤在同一个 GitHub Release 条目里**（服务端二进制、桌面安装包混在一起），难以管理与分发。因此把它们拆成独立 Git 仓库、各自独立 Release，同时保持同一套代码与工具链不变。`web` 还作为独立发布组件产出静态前端压缩包，使前端可通过外部静态目录热更新，不再嵌入二进制。

## 目标

- 四个目录以 submodule 形式存在于相同路径：`app/web`、`app/desktop`、`app/mobile`、`packages`。
- `GOSpeak-Web` 内嵌套挂载 `packages` submodule，使 `workspace:*` 与源码直连保持可用。
- 每个 submodule 保留该目录在主仓库中的完整 Git 历史。
- 主仓库 HEAD 仅记录四个 submodule 的 commit 指针（gitlink），不再跟踪目录内文件。
- **各端拥有独立 GitHub Release**：server 在主仓库、web 在 GOSpeak-Web、desktop 在 GOSpeak-Desktop、mobile 在 GOSpeak-Mobile，产物不再混在同一 Release 条目。
- 所有现有构建、lint、typecheck、CI 脚本在 submodule checkout 后继续工作，路径不变。
- `web` 作为第四个独立发布组件，产出 `gospeak-web-v<semver>.tar.gz` 静态压缩包，支持运行时替换外部静态目录而不需要重新构建 Go 二进制；前端不再嵌入任何二进制。
- 给出清晰、可回滚的执行步骤。

## 版本与发布模型（metadata.yaml 分层管理 + 独立发布）

- **方案 C：主仓库总控 + 子仓库各自一份 `metadata.yaml`**，分层聚合。
- 主仓库总控 `metadata.yaml` 的 `components.<name>.version` 是最终事实源；子仓库 `metadata.yaml` 的 `version` 必须一致，由 CI 校验，`metadata_ref` 指向子仓库文件。
- 子仓库 `metadata.yaml` 还承载组件元数据（name/description/platforms/artifact_type/changelog），供 release-notes 与 build-changelog 消费。
- 删除 `.github/versions/*.yaml`，版本与元数据全部迁移到 `metadata.yaml`。
- **版本号不统一发布**：server / web / desktop / mobile 各自独立 SemVer，互不跟随；`packages` 有独立 version 但不发 Release。
- 每个组件独立 tag：`gospeak-server-v<semver>` / `gospeak-web-v<semver>` / `gospeak-desktop-v<semver>` / `gospeak-mobile-v<semver>`；**各自在自己仓库创建独立 Release**，独立 changelog、独立产物。
- 版本 bump 只针对发生变更的组件：先改该子仓库 `metadata.yaml` 与版本文件并提交，再更新主仓库总控 `metadata.yaml` 的 version/tag 与 gitlink，最后打该组件 tag。
- 子仓库 commit message 约定：`chore(<component>): bump version to <semver>`，gitlink bump 与版本发布解耦。
- **metadata ↔ tag 协同**：tag 一律由 metadata 经 `release-naming.mjs` 派生（不手写）；metadata 中显式写的 `tag` 字段必须与派生结果一致；tag 存在性驱动发布且幂等；子仓库 metadata 版本必须与总控一致才允许打 tag；annotated tag 打在「更新 metadata + gitlink 的提交」上。删除现有 `version-bump.yml` 中强制 server/desktop 同版本的 lockstep 约束。
- mobile 产物仍由独立 `build-mobile.yml` 发布，不进入主 release workflow 的构建阶段。

## web 外部更新整合

内嵌前端（`go:embed`）只是兜底。服务端 `serveSPA` 的优先级链为 `STATIC_DIR > /app/static > ./static > go:embed`：`app/server/internal/router/router.go` 在外部目录存在且含 `index.html` 时优先用 `http.FileServer(http.Dir(staticDir))`，否则回退 `webui.FS()`。因此前端更新无需重编 Go 二进制，只需替换外部目录内容。

配套决策：
- `web` 作为独立组件进入 metadata 分层模型，独立 tag `gospeak-web-v<semver>`，发布产物为 `dist/` 打包的静态压缩包（非二进制）。
- `release.yml` 在 web tag 事件时 checkout 主仓库并 `git submodule update --init app/web`，构建前端并产出压缩包作为 Release 附件。
- 运维把压缩包解压到 `STATIC_DIR` 指向的目录即完成热更新；embed 中仍是「最后一次 server 发布时 checkout 的 submodule 产物」，作为不带外部目录时的兜底。
- `serveSPA` 对 `index.html` 设置 `Cache-Control: no-store`，避免更新后浏览器缓存的旧首页引用已删除的 hash 资源；带 hash 的静态资源可长缓存。
- 回滚策略：保留上一版静态目录（如 `ui.old`），切回即恢复旧前端。

## 前端去除 embed 与产物分发

内嵌前端不再是兜底：`go:embed` 从服务端与桌面宿主中移除，前端产物作为独立压缩包与二进制一起发布。服务端只从外部静态目录（`STATIC_DIR` / `/app/static` / `./static`）加载前端；Docker 镜像 final stage 直接包含 `/app/static`；桌面安装包内含外部前端目录。这样前端可以独立发版与热更新，二进制体积不再随前端增长。

## CI 归属

CI/发布 workflow 按代码归属划分，不再集中在主仓库：

- 主仓库：只跑 server/Go/scripts/protocol/集成测试，负责 server 发布与 Docker 镜像；为 `app/shared/proxyrules` 打 Go module tag。
- GOSpeak-Web：lint/typecheck/test/build 与前端静态压缩包发布（tag `gospeak-web-v*`，发布到本仓 Release）。
- GOSpeak-Packages：四个 workspace 包的 test/typecheck/build，无 Release。
- GOSpeak-Desktop：Go/mygo 构建、安装包发布（tag `gospeak-desktop-v*`，发布到本仓 Release）。
- GOSpeak-Mobile：Capacitor sync、gradle/xcode 构建与 APK/IPA 发布（tag `gospeak-mobile-v*`，发布到本仓 Release）。

主仓库的路径过滤器只跟踪 gitlink（`app/web`、`app/desktop`、`app/mobile`、`packages`），不再匹配子目录内文件；子仓库 push/tag 触发自身 CI/Release，主仓库靠 gitlink 更新触发自身 CI。**各仓 Releases 页面天然分开，产物不再挤在一起。**

## 更新检测链路

server / web / desktop / mobile 各自去对应仓库检查最新 Release，不再从主仓库统一读取：

| 组件 | 仓库 | tag 前缀 |
|---|---|---|
| server | GOSpeak | `v` |
| web | GOSpeak-Web | `gospeak-web-v` |
| desktop | GOSpeak-Desktop | `gospeak-desktop-v` |
| mobile | GOSpeak-Mobile | `gospeak-mobile-v` |

各组件独立缓存与负缓存，避免互相污染；前端「关于」页新增 web 前端更新检测入口，桌面 updater 改查 GOSpeak-Desktop 仓库。

## 发布链路

**一组件一 Release，各在自己仓库创建**（这是本次拆分的核心目标）：

| 组件 | 发布仓库 | tag | 产物 |
|---|---|---|---|
| server | GOSpeak | `gospeak-server-v*` | 二进制 + `static/` |
| web | GOSpeak-Web | `gospeak-web-v*` | 前端静态压缩包 |
| desktop | GOSpeak-Desktop | `gospeak-desktop-v*` | dmg / deb / exe / install.sh |
| mobile | GOSpeak-Mobile | `gospeak-mobile-v*` | APK / IPA |

各仓用自己的 tag 触发自己的 `release.yml`、创建自己的 Release。server 包中的 `static/` 必须来自该 tag commit 上的 `app/web` gitlink 对应版本（禁止浮动引用分支），发布说明标注 `bundled-web`；web 可单独发布与热更新，无需动 server。四组件均支持 `-rc.N` 预发布段，**rc 直接进入 latest 推荐**：更新检测不过滤 rc，GitHub latest 可指向 rc，移动端去除 `--prerelease` 标记；`compareVersion` 仍保持同版本正式版 > rc，因此 rc 仅在高于所有正式版时被推荐，正式版发布后自动让位。文档站是否部署 rc 由 docs.yml 独立决定。

## 非目标

- 不把 `packages/components`、`packages/sfu-client`、`packages/desktop`、`packages/mobile-capabilities` 拆成 submodule；它们继续作为 workspace 包留在主仓库。
- 不改动三端业务代码、构建配置、UI token、协议生成逻辑。
- 不改动服务端分层与路由结构；唯一例外是 `serveSPA` 为 `index.html` 补 `Cache-Control: no-store` 响应头（见「web 外部更新整合」）。
- 不切换分支策略，仍按 `release` 为唯一事实源。

## 反目标（做什么会导致失败）

- 丢失四个目录的历史。
- submodule 路径改变（例如改成 `modules/web`），导致 Vite / Turbo / CI / AGENTS.md 中大量路径失效。
- 主仓库同时以普通目录和 submodule 形式跟踪同一路径，产生 git index 冲突。
- 未处理当前工作区未提交改动就直接拆分，导致 `app/web` 内改动丢失。
- 拆分后未跑 `pnpm typecheck`、`pnpm lint`、三目标构建与 Go 构建，导致 CI 回归。

## 约束

- 当前工作目录：`/Users/noelorin/Projects/GOSpeak`，当前分支 `dev-1124`，主仓库远端 `https://github.com/NoelOrin/GOSpeak.git`。
- 当前工作区有大量未提交改动，其中 `app/web/src/...` 有多个修改；拆分前必须显式决定这些改动的归属与落点。
- 仓库规范：Conventional Commits；分支从最新 `origin/release` 签出；文档与代码同 PR 更新；不留过期文档。
- 子仓库默认命名：
  - `https://github.com/NoelOrin/GOSpeak-Web.git`
  - `https://github.com/NoelOrin/GOSpeak-Desktop.git`
  - `https://github.com/NoelOrin/GOSpeak-Mobile.git`
- 执行工具：Git 内置 `git filter-branch`（`--subdirectory-filter` 抽取目录、`--tree-filter` 移动目录）。**不使用 `git filter-repo`**（本机未安装）**也不用 `git subtree`**（本机无该命令）。
- `git filter-branch` 会重写历史，**只能在临时 clone 中运行**，绝不直接在主仓库工作区运行。

## 方案

### 采用的方案：filter-branch 抽取历史 + 原地 submodule

对每个目录重复：

1. 从主仓库做一个临时裸 clone 或普通 clone。
2. 在该 clone 中用 `git filter-branch -f --prune-empty --subdirectory-filter <dir> -- --all` 把历史裁剪为该目录，并把目录内容提升到子仓库根（也可保留目录层级，见「决策点」）。
3. 推送到对应远端仓库的 `release` 分支。
4. 在主仓库工作区：先处理未提交改动，再 `git rm -r --cached <dir>`，删除本地目录内容，`git submodule add -b release <remote> <dir>`。
5. 提交主仓库的 `.gitmodules` 与 gitlink 变更。

### 为什么不用其他方案

- **空仓库快照**：最快，但丢失四个目录历史，不符合需求。
- **本地相对路径 submodule**：适合临时验证，但不适合团队协作与 CI，不能作为目标方案。

## 决策点（执行前由维护者确认）

1. 子仓库内目录结构：
   - **推荐 A1**：子仓库根就是原目录内容（例如 `GOSpeak-Web/src/...` 对应主仓库 `app/web/src/...`）。执行时用 `--to-subdirectory-filter` 的反向策略或 `--path app/web` 后移动内容到根。
   - **A2**：子仓库保留原目录层级（`GOSpeak-Mono/app/web/...`），主仓库 submodule 路径仍为 `app/web`，但子仓库根目录多一层 `app/web`。不推荐，会让子仓库独立开发体验变差。
2. 当前未提交改动处理：
   - **推荐 B1**：把 `app/web` 的未提交改动先在主仓库当前分支提交（按 Conventional Commits），再执行拆分；拆分时该提交会进入子仓库历史。
   - **B2**：把未提交改动 stash，拆分后在子仓库中 reapply。风险是 stash 与主仓库历史分叉，操作更复杂。
3. 子仓库默认分支：推荐 `release`，与主仓库一致。

## 关键风险与缓解

- 历史抽取耗时/体积大：先在临时 clone 中执行。`filter-branch` 是 Git 内置命令，无需安装；它比 filter-repo 慢，但本项目历史规模（1435 文件）可接受。
- 主仓库 index 冲突：先 `git rm -r --cached` 再 `submodule add`，不要混用。
- CI / 工具链路径失效：submodule 路径与原目录一致，理论上无需改路径；但仍需跑全量门禁验证。
- 当前工作区改动丢失：执行前先 `git status --short` 与 `git diff` 存档，按决策点提交或 stash。
- `filter-branch` 的 gotchas 警告：用 `FILTER_BRANCH_SQUELCH_WARNING=1` 抑制；运行后需 `git update-ref -d refs/original/refs/heads/<branch>` 清理备份引用，避免推送时带出旧 ref。

## 留给执行者

- 确认决策点 1/2/3。
- 在 GitHub 上创建三个空仓库（或提供已存在的空仓库地址）。
- 使用 `superpowers:writing-plans` 生成的计划逐步执行。
