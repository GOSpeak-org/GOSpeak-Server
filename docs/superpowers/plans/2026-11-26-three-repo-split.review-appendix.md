# 聚合壳 + 三子仓拆分 Implementation Plan

> 【状态】2026-11-27 核验：该评审附录对应的三仓拆分已交付，仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把现有单仓变成「顶级聚合壳 `GOSpeak` + 三个平级子仓」，壳用 3 条 submodule 挂 `GOSpeak-Server` / `GOSpeak-Client` / `GOSpeak-Docs`，客户端内部零 submodule、`proxyrules` 提到仓根。

**Architecture:** 现有 `GOSpeak-org/GOSpeak` 原地变成聚合壳（保留 181 条历史与 22 个 tag），清空代码目录后加 `.gitmodules`。三个子仓在临时 clone 里用 `filter-branch` 抽取各自历史并首次推送。客户端仓把 `app/{web,desktop,mobile}` 与 `packages` 提到仓根、`app/shared/proxyrules` 提到仓根 `proxyrules/`。三条相对路径（`../web/dist`、`../web/dist-mobile`、`../proxyrules`）在新结构中成立。

**Tech Stack:** Git 2.50 · pnpm 10.11 · Go 1.27 · GitHub Actions · VitePress

**Spec:** `docs/superpowers/specs/2026-11-26-two-repo-split-design.md`

## Global Constraints

- 工作目录 `/Users/noelorin/Projects/GOSpeak`；四个仓全部 **public**，统一放在组织 **`GOSpeak-org`** 下：`GOSpeak-org/GOSpeak`（壳）、`GOSpeak-org/GOSpeak-Server`、`-Client`、`-Docs`。
- 组织为 free 套餐；public 仓 Actions 分钟数免费；组织级 secret 可用（未来签名材料可下沉）。
- **只有一层父子**：壳挂 3 条 submodule，子仓之间互不引用；客户端仓内部 0 submodule。
- 客户端组件目录在仓内**平级**：`web/`、`packages/`、`desktop/`、`mobile/`、`proxyrules/`。
- 服务端目录扁平：`server/`（原 `app/server`）、`test/`、`deploy/`、`docs/`、`scripts/`。
- 禁止在任何 CI/脚本里使用 `git submodule update --remote`。
- 壳的 gitlink 只在「需要统一快照」时人工 bump；日常开发各子仓独立。
- 客户端首个版本 `1.4.0-rc.1`（必须 > 现网 `1.3.0-rc.1` / `1.1.0-rc.2`）。
- 代码不加注释（复杂逻辑除外）、不用 emoji；Conventional Commits。

## Review Focus

- **相对路径因多/少一层而断**：`client/desktop/mygo.config.ts` 的 `../web/dist`、`client/mobile/capacitor.config.ts` 的 `../web/dist-mobile`、`client/desktop/go.mod` 的 `../proxyrules` 必须成立；Task 8 钉住。
- **proxyrules 提根后 go.mod 失配**：`desktop/go.mod` 的 replace 必须改成 `../proxyrules`，否则 `go build` 报找不到模块；Task 8 钉住。
- **协议生成物跨仓不同步**：服务端改协议、客户端 generated 未跟；Task 6 用 golden 比对钉住。
- **Dockerfile 仍依赖前端源码**：服务端仓已无 `web/`，镜像必须走直链；Task 7 钉住。
- **壳被误当成开发仓**：开发者 `cd server` 进壳里的 submodule 做提交，容易在 detached HEAD 上丢提交；Task 10 在 README 与 AGENTS 里写明。
- **客户端首个版本降级**：低于现网会让在装客户端更新检测静默失联；Task 11 钉住。

---

## 文件结构

**新建（`GOSpeak-Server`）**

| 路径 | 来自 |
|---|---|
| `server/` | `app/server` |
| `test/` | `test` |
| `deploy/` | `deploy` |
| `docs/` | `docs` |
| `Dockerfile`、`VERSION`、`metadata.yaml`、`CHANGELOG.md` | 同名文件 |
| `scripts/` | 服务端与 release 类脚本 |

**新建（`GOSpeak-Client`）**

| 路径 | 来自 | 说明 |
|---|---|---|
| `web/` | `app/web` | |
| `packages/` | `packages` | |
| `desktop/` | `app/desktop` | |
| `mobile/` | `app/mobile` | |
| `proxyrules/` | `app/shared/proxyrules` | **提到仓根** |
| `scripts/` | 客户端门禁脚本 | 见 §5 归属表 |
| `pnpm-workspace.yaml`、`.gitignore`、`package.json` | 新写 | |

**新建（`GOSpeak-Docs`）**

| 路径 | 来自 |
|---|---|
| 仓根 | `app/docs` 全部内容 |

**改造（`GOSpeak` 壳）**

| 路径 | 改动 |
|---|---|
| 全部代码目录 | 删除（`app/`、`packages/`、`test/`、`deploy/`、`docs/`、`scripts/`、`Dockerfile`、`VERSION` 等） |
| `.gitmodules` | 新增，3 条平级 |
| `README.md` | 重写为「这是聚合壳 + 如何 clone 与开发」 |

---

## CI 与组织兼容 review 补丁（ci-ops + org-compat，2026-11-26）

> 两个 reviewer 独立命中同一批问题，已合并去重。**B1–B5 为阻塞级**。

### 阻塞

**B1. 已装机客户端的更新通道会冻死——仓库地址编译在二进制里，改不了。**

`app/desktop/updater_release.go:33`：`githubAPIReleases = "https://api.github.com/repos/NoelOrin/GOSpeak/releases?per_page=30"`，`pickDesktopRelease`(`:63-82`) 只从**这一个仓**挑 `gospeak-desktop-v*`。`app/server/internal/service/system_update_service.go:21-22` 同样硬编码。

这个常量**已编译进 1.3.0-rc.1 的装机二进制**。仓转移后 `NoelOrin/GOSpeak` 301 → `GOSpeak-org/GOSpeak`，而壳仓的 `.github` 在 Task 5 被删、新桌面版发在 `GOSpeak-Client` → **老客户端永远只看得到冻结的 1.3.0-rc.1 列表**，`compareReleaseVersion(latest, current) > 0` 恒为 false。

注意：设计 §7 只写了「客户端首版必须 > 现网」，那只解决了版本号序，没解决**通道指向**。

**改法**：**壳仓必须保留一条桥接 Release** —— 在 `GOSpeak-org/GOSpeak` 打 `gospeak-desktop-v1.4.0-rc.1`，资产名保持 `gospeak-desktop-v1.4.0-rc.1-<os>-<arch>.<ext>`（`pickDesktopAsset:217` 按此前缀匹配），内容为 v1.4.0 安装包或指向 Client 的下载页说明。Task 11 验收改为「用仍读壳仓的旧客户端实测能看到 1.4.0」。**1.4.0 稳定版发布前，壳仓的 desktop 发布链不能先删。**

**B2. 发布编排链路整体无归属——执行完 Task 5，四个仓里没有一条发布链。**（ci-ops C1 + org-compat #3）

`.github/` 实含 **8 个** workflow（`release.yml` 835 行、`version-bump.yml` 249、`docker.yml` 147、`build-mobile.yml` 218、`docs.yml` 222、`security.yml` 111、`pr-title.yml` 31、`ci.yml` 486）+ `.github/actions/{pnpm-install,go-build-cache}` + `.github/versions/{server,desktop,mobile}.yaml` + `.github/dependabot.yml`。

Task 2（L210）／Task 3（L266）／Task 5（L345）的 `rm -rf ... .github` 把它们全删了，而 Task 9 只要求「两仓各建 `ci.yml`」。

且 `version-bump.yml:215` 用 `uses: ./.github/workflows/release.yml`（同仓相对路径，**跨仓无法复用**），`:235/:246` 同理调 `build-mobile.yml`/`docker.yml`。

**直接后果**：Task 11 的 `node scripts/mobile-version.mjs --check` 会 ENOENT —— `mobile-version.mjs:26` 读 `.github/versions/mobile.yaml`，该文件已删。同理 `check-version.mjs:104` 的唯一事实源是 `.github/versions/<component>.yaml`，「首版 > 现网」这条约束**没有任何门禁**能校验。

**改法**：Task 9 改为逐项清单：

| 资产 | 去向 |
|---|---|
| `release.yml`（去 desktop）+ `version-bump.yml`（收窄）+ `docker.yml` + `versions/server.yaml` + 两个 action + `security.yml` + `pr-title.yml` | `GOSpeak-Server` |
| `release.yml`（desktop 分支）+ `build-mobile.yml` + `versions/{desktop,mobile,web}.yaml` + `actions/pnpm-install` + `security.yml` + `pr-title.yml` | `GOSpeak-Client` |
| 文档站 workflow | `GOSpeak-Docs` |
| 轻量 drift 校验（见 B4） | `GOSpeak` 壳 |

Task 11 的 Files 还要补：`desktop/mygo.config.ts`（`:21` 仍是 `1.3.0-rc.1`，打包出的 bundle 版本会与 package.json 双源漂移，`updater.go:55-70` 注释明确警告这会「forever offer its own version as an update」）、`.github/versions/desktop.yaml`、`.github/versions/mobile.yaml`、`mobile/android/app/build.gradle`、`mobile/ios/.../project.pbxproj`。

**B3. GHCR 镜像名迁移无人负责，存量部署会永久停在旧镜像。**（ci-ops C5 + org-compat #2）

`docker.yml:123` 是 `images: ghcr.io/${{ github.repository }}` → 拆仓后必然变成新名字。而所有外部入口写死旧名：`deploy/docker-compose.yml:31`、`docker-compose.standard.yml:35`（`${GOSPEAK_IMAGE:-ghcr.io/noelorin/gospeak:latest}`，且 `pull_policy: always`）、`deploy/DEPLOY.md:9`、`deploy/.env.example:40`、`app/docs/deploy/{compose,docker,index}.md` 共 10+ 处。

Task 10 Step 2 的 grep 模式 `'GOSpeak-org/GOSpeak[^-]'` **匹配不到 `ghcr.io/...`**。

**改法**：明确 `docker.yml → GOSpeak-Server`，镜像统一 `ghcr.io/gospeak-org/gospeak-server`；Task 0 增加「实测旧 package 是否随转移、能否继续 pull」；同步改 compose 默认值 + DEPLOY.md + docs 三篇；旧包 `latest` 打终版并注明迁移目标。

**B4. 服务端 `ci.yml` 的 `build-server` / `protocol-check` 拆仓后必然失败，且与 Task 6 自相矛盾。**

- `ci.yml:204-214` `build-server` 拉同 run 的 `web-dist` artifact 再 `cp -R app/web/dist/. app/server/internal/webui/dist/`；
- `ci.yml:239-243` `protocol-check` 跑**无参** `go run ./cmd/gen-protocol` 后 `git diff --exit-code -- ../web/src/protocol/generated`；
- 而 Task 6 Step 3 要求「缺 `-out` 时报错退出」、Step 2 要求删掉 `app/web` 读取。

**改法**：Task 9 显式写死新命令 —— `protocol-check` 改 `go run ./cmd/gen-protocol -update-golden && git diff --exit-code -- internal/genprotocol/testdata/golden`；`build-server` 二选一写死（「不再 embed」或「按 `CLIENT_WEB_TAG` 从 Client Release 下载」）。

**B5. 非 Docker 产物（10 平台二进制）的前端来源断裂，且失败静默。**

`release.yml:359-381` 的 `build-release-binaries` 依赖同 run 的 `release-web-dist-server` 工件（`:294-299`，`retention-days: 3`，只是 workflow artifact，不是 Release 资产）来做 `cp -R app/web/dist/. app/server/internal/webui/dist/`。

而 `router.go:167-205` 装载优先级 `STATIC_DIR > /app/static > ./static > go:embed`，皆空时**静默跳过**；`internal/webui/dist` 只有入仓的 `EMBED_PLACEHOLDER`，`HasAssets()` 为 false —— 二进制/镜像能构建、能过现有断言，**只是没有 UI**。

**改法**：Task 7 补三点 —— (a) `release.yml` 的 server 二进制改为从 Client Release 下载 `web-dist.tar.gz`；(b) `Dockerfile` 加断言 `test -f /app/static/index.html`；(c) `docker.yml` 的 verify(`:46-53`) 与 publish(`:145-147`) 都补 `CLIENT_WEB_URL` build-arg，并给 `ADD` 加 `--checksum`。

### 高

**H1. Task 10 的 grep 模式写反了——查的是拆完才存在的新名字，当前树零命中。**

计划用 `grep -rn 'GOSpeak-org/GOSpeak[^-]'`，这是**拆分后的名字**。真实存在的旧引用（已实测）：
- `app/desktop/updater.go:39`：`releasePageURL = "https://github.com/NoelOrin/GOSpeak/releases?q=gospeak-desktop-v"` —— Task 10 Files **漏了 `updater.go`**
- `app/docs/.vitepress/theme/components/ClientDownload.vue:5`：`const REPO = 'NoelOrin/GOSpeak'` —— **未列入 Files**
- `app/docs/.vitepress/config.ts:112` socialLinks
- `app/docs/guide/{desktop,quickstart,faq}.md`
- `deploy/install-binary.sh:4,41`
- `README.md:58`、`docs/deployment-guide.md:198`
- `app/server/main.go:15-18`、`app/server/docs/{swagger.yaml,docs.go,swagger.json}`（`NoelOrin/GoRTC` 历史名）

**改法**：grep 改为 `grep -rnE 'NoelOrin/(GOSpeak|GoRTC)|noelorin\.github\.io'`（排除 `objectstore` 模块路径，那是真依赖）；Files 清单补齐上述文件。

**H2. 文档站 `base` 硬编码 `/GOSpeak/`，Pages 链整体断裂，旧站 URL 不重定向。**

`app/docs/.vitepress/config.ts:4`：`const base = isProd ? '/GOSpeak/' : '/'`。拆到 `GOSpeak-org/GOSpeak-Docs` 后 Pages 地址变为 `https://GOSpeak-org.github.io/GOSpeak-Docs/`，base 必须改 `/GOSpeak-Docs/`，否则产物里所有 `/GOSpeak/guide/*.html` 链接全 404。

`docs.yml` 是**唯一** Pages 部署者，且 gate 强绑定同仓 server tag（`case "$TAG" in gospeak-server-v*`），拆仓后永远收不到 → 链路失效。

旧站 `https://noelorin.github.io/GOSpeak/` 转移后**不重定向**（Pages 无此机制），README/文档里到处是该地址。

**改法**：Task 4 增加「改 base + 新建 docs 仓 docs.yml + 重新启用 Pages/Actions source」；跨仓触发用 `gh workflow run`（organ secret）；设计 §7 补「旧 Pages URL 迁移 + 保留一版跳转页」。

**H3. 服务端仓删掉 `package.json` / `pnpm-workspace.yaml` 后，`check-version` 与 `pnpm test:server` 都会失败。**

`check-version.mjs:104-123` 的 server 分支读 `.github/versions/server.yaml` + `VERSION` + **`package.json`** 三方互比；而 Task 2 L213 恰恰 `rm -rf ... package.json pnpm-workspace.yaml`。

Task 7 Step 2 的 `cd server && pnpm test:server`，该脚本只存在于根 `package.json:19`，依赖 `pnpm-workspace.yaml:4` 的 `test` 条目与 `test/package.json` 的 vitest —— 三者都被删。

**改法**：服务端仓显式新建 `package.json`（private，`version` 与 `VERSION` 同步，带 `test:server`）与最小 `pnpm-workspace.yaml`（只含 `test`）。

**H4. 壳仓完全没有 CI；设计承诺的 `check-shell-drift.mjs` 与 `proxyrules-v*` 变更门禁都没落到 Task。**

设计 §7 写「提供 `scripts/check-shell-drift.mjs` 只报告不自动改」、§2.2 写「`proxyrules-v*` tag 打时 CI 校验与上一版无差异则失败」，但 14 个 Task 里 `grep 'shell-drift'` / `'proxyrules-v'` **零命中**。

另：`.gitmodules` 用绝对 https URL 而非相对路径，fork 后 `--recursive` 仍指向 org 原仓，配合「壳仓无 CI」，指针漂移无人发现。

**改法**：壳仓加约 20 行 workflow（解析 `git submodule status` + `git ls-remote` 校验 gitlink SHA 可达 + drift 报告，`push/pull_request` + `schedule`）；新增 proxyrules 变更门禁 Task；`.gitmodules` 考虑相对 URL 或在 README 说明 fork 需自改。

**H5. 「一组件一 Release」与拆仓后的正文/产物互引冲突，`web` 组件在命名规范里不存在。**

`release-notes.mjs:240-247` 的 `containerPackageUrl` 硬编码 `pkgs/container/gospeak`；`:42` 的桌面下载表格 URL 全指向同一 repo。拆仓后 server Release 里的桌面表格会 404。

`release-naming.mjs:22` 的 `COMPONENTS = ["server","desktop","mobile"]` **没有 web**；而计划里**没有任何任务产出 `web-dist.tar.gz` 这个 Release 资产**（现网 `build-web` 只传 3 天保留期的 workflow artifact）。

**改法**：Client 仓把 `web` 加入 COMPONENTS，产出 `gospeak-web-v<ver>-dist.tar.gz` + SHA256 挂 Client Release；正文按组件隔离；`containerPackageUrl` 按仓名派生。

### 中

**M1. `check-protocol-sync.mjs` 建了但没接进门禁。** Task 6 Step 4 只写手动 Run，Task 9 客户端 job 列表没有它。
**改法**：Client `ci.yml` 加 `protocol-sync` job 并纳入 gate 硬性必过（不接受 skipped）；写明拉的是服务端 `release` 分支还是某个 tag。

**M2. `ci.yml` 的 job 集合被削掉 4 个，且 `paths-filter` 未重写会全 false。**
现 job：`changes/lint/typecheck/build-js/build-server/protocol-check/test-server/test-server-race/test-web/test-packages/test-scripts/desktop-go/workflow-lint/gate`。计划漏掉 `test-server-race`（`:273-297`，并发敏感包必过 `-race`）、`test-scripts`（`:341-371`）、`workflow-lint`（`:430-450`）、lint 里的 `./deploy/init.sh --check`（`:125-128`）。
更隐蔽：`ci.yml:45-74` 过滤器全按旧路径写死，照抄则**所有 filter 为 false → 各 job skipped → gate 只在 protocol-check 上要求非 skipped（`:476-480`）→ 整条 CI 静默变绿**。
**改法**：Task 9 给出两仓完整 job 清单（含 race / test-scripts / workflow-lint / deploy guard）与重写的 `filters:`；迁移后跑一次空 PR 观察 job 是否真的执行。

**M3. 一键安装命令拆仓后 404。** `app/docs/deploy/{index,binary}.md`、`deploy/DEPLOY.md:97`、`docs/deployment-guide.md:198`、`README.md:58` 都用 `raw.githubusercontent.com/NoelOrin/GOSpeak/release/deploy/install-binary.sh`；`deploy/` 被 Task 5 从壳仓删除 → raw 路径 404。
**改法**：改为 `GOSpeak-org/GOSpeak-Server/release/deploy/install-binary.sh` 并同步脚本内 `GOSPEAK_REPO`；或在壳仓 `release` 分支留 3 行转发脚本。

**M4. dependabot 三份配置随 `.github` 消失。** 现 `.github/dependabot.yml` 有 `npm /`、`gomod /app/server`、`github-actions /` 3 条。
**改法**：Server 仓 `gomod /server` + `github-actions /`；Client 仓 `npm /` + `gomod /desktop` + `github-actions /`；Docs 仓按需 npm。

**M5. Release 资产 URL 依赖「旧路径永不复用」。** 老安装包链接靠仓库级 301 存活（`allowedDownloadHosts` 白名单含 github.com，走重定向没问题），但一旦日后在个人号重建同名仓，全部老链接与已装机 App 里烧死的 API 地址一起 404。
**改法**：设计 §6 写死「永不重建 `NoelOrin/GOSpeak`」；Task 12 加断言 `curl -sI <资产 URL> | head -1` 应为 301 且 Location 指向 org。

### 低

**L1. Docs 仓缺 LICENSE。** Server 仓显式 `mv LICENSE`，Client 仓隐性保留，壳仓保留，但 Docs 仓用 `--subdirectory-filter app/docs` 不会带 LICENSE，而站点 footer 声明 Apache 2.0。
**改法**：Task 4 推送前 `cp <原仓>/LICENSE .`。

### 两个 reviewer 均确认无问题的角度
- GOSpeak-org free 套餐 / public 仓 Actions 免费：表述正确，public 仓标准 runner 不计分钟，free org 私有仓无硬上限，3 条 submodule 指向同 org public 仓可匿名拉取。**前提是四仓保持 public**。真正会卡的是 Pages 需在新仓重新启用环境与权限（见 H2）。
- CODEOWNERS：仓里本来没有，不涉及迁移。

---

## 服务端与协议 review 补丁（server-proto，2026-11-26）

> 与上文 B4/B5、M1 有交叠的部分已合并，本节只记**新增**结论。

### 阻塞级补充

**P1. `check-protocol-sync` 唯一的自动钩子随 `lefthook.yml` 一起被删。**
（补充 M1）Task 2 L213 / Task 3 L269 的 tree-filter 删掉 `lefthook.yml`，两仓都没接手；现有唯一的协议钩子是 `lefthook.yml:34-36`（`pnpm gen:protocol && git diff --exit-code -- app/web/src/protocol/generated`）。拆仓后「协议不同步」这条风险在自动门禁里**零防线**，只剩一个没人跑的手工脚本。
**改法**：客户端 `ci.yml` 加**无条件运行**的 `protocol-sync` job（理由照抄现 `ci.yml:221-232`：「无条件运行，否则按路径跳过会变 skipped 卡死 PR」），并入 gate 必需项；客户端 `lefthook.yml` pre-push 加同名钩子。

### 高

**P2. 客户端没有生成能力，脚本只报错不修复 → 链路只闭一半。**
客户端仓无 Go 模块，`cmd/gen-protocol` 与 `internal/genprotocol` 都在服务端；根 `package.json:22` 的 `gen:protocol` 随壳删除，而设计 §5 把它归服务端、**服务端仓却没有 package.json** → 两仓都没这个入口。
生成物头注释本身就写着这条指令：`generate.go:385` → `// GENERATED — 手改无效，跑 pnpm gen:protocol`，已写进三份产物第 1 行。
**改法**：`check-protocol-sync.mjs` 加 `--fix`（拉 golden 直接覆盖 `web/src/protocol/generated/`），客户端 AGENTS 写「协议红 → `node scripts/check-protocol-sync.mjs --fix` → 跑 typecheck」。
**关键实测**：golden 与 generated 目前**字节完全相同**（`diff -r app/server/internal/genprotocol/testdata/golden app/web/src/protocol/generated` → 无差异，退出 0），所以覆盖即修复，客户端不需要 Go 工具链。

**P3. 逐字节对比服务端 `release` HEAD 不 hermetic，与自托管多版本现实冲突。**
计划写「用 `gh api .../contents/...` 拉 golden 比对」，没有 `ref` 参数 → 取的是移动 HEAD。而客户端与服务端独立发版（客户端 1.4.0-rc.1 / 服务端 1.3.0-rc.1），GOSpeak 是自托管产品，客户端连的是用户自己的 server。
**后果**：服务端任意一次协议变更，第二天所有客户端 PR 无关变红；反之客户端为兼容旧 server 保留字段时也会误红。
**改法**：给脚本 `--server-ref`，默认钉在客户端声明的兼容基线（如客户端仓 `server-compat.json` 或最近一个 `gospeak-server-v*` tag）；PR 用 pinned ref 阻塞，另加每日 cron 对 HEAD 做预警（不阻塞）。若目标是协议**兼容**而非产物一致，改成子集校验（客户端 generated 必须覆盖服务端 golden 中全部 c2s/s2c 事件）更贴合自托管语义。

**P4. `gh api` 在 CI 里拿不到另一个仓的内容；应改用 raw 直链。**
本地 `gh auth status` 已登录且有 `repo` scope，所以本地跑得通；但客户端仓 CI 的默认 `GITHUB_TOKEN` 是只覆盖本仓的 installation token，打服务端仓会 404。
**实测**：公开仓免 token 成立 —— `curl -s -o /dev/null -w '%{http_code}' https://raw.githubusercontent.com/NoelOrin/GOSpeak/release/package.json` → **200**；`api.github.com/repos/.../contents/VERSION` → **200**。
base64 细节：`gh api --jq .content` 返回 60 列换行 base64（`events.ts` 得 69 行），Node `Buffer.from(s,"base64")` 宽容解码正确（实测 3050 字节）。
**路径易错**：Go 模块在仓内第二层，正确路径是 `server/internal/genprotocol/testdata/golden/events.ts`；照抄 Go 测试里的 `internal/genprotocol/...` 会 404。
**改法**：改用 `https://raw.githubusercontent.com/GOSpeak-org/GOSpeak-Server/<ref>/server/internal/genprotocol/testdata/golden/<file>` 直取原始字节（免 token、无 base64、无编码坑）；或 `gh api -H "Accept: application/vnd.github.raw"`。

**P5. `-out` 改必填后三个消费方全部失效，而 `-update-golden` 与它语义冲突。**

| 调用点 | 现在 | 拆仓后 |
|---|---|---|
| `package.json:22` `gen:protocol`（无 `-out`） | 可用 | 随壳删除；服务端仓无 package.json → 入口不存在 |
| `lefthook.yml:36` | 可用 | `lefthook.yml` 被删，两仓未接手 |
| `ci.yml:242-243`（无参） | 可用 | 无参 → 报错退出，job 红；Task 9 未给命令 |

且 `main.go:14` 的帮助文本说明 `-update-golden` 就是「不指定输出路径」的形态；照字面实现「缺 `-out` 报错」，会让 `generate_test.go:25` 与计划 Step 1 自己的报错文案变成错误路径。
**改法**：明确 `-update-golden` 与 `-out` 互斥、`-update-golden` 走固定路径；服务端仓建最小 `package.json`（`"gen:protocol": "go run ./cmd/gen-protocol -out ..."`）或在 AGENTS 改成纯 go 命令；同步改 `main.go:14` 帮助文本与 `generate.go:385` 生成头；Task 9 的服务端 `protocol-check` 明确写 `go test ./internal/genprotocol ./internal/protocol`（等价且不依赖 `-out`）。

**P6. 目录少一层，`buildinfo` 找不到 `VERSION` → 版本静默退化 `dev` → 更新提示永久失效。**

`server/internal/buildinfo/buildinfo.go:64-86` 的候选路径只有：cwd `VERSION`、`../../VERSION`、exe 附近 `dir`、`dir/../..`、`dir/../../..`。
现在 module 根是 `app/server`，`../../VERSION` 正好命中仓库根；**移到 `server/` 后 VERSION 在 `../VERSION`，不在候选里**。

受影响命令（都在 `app/server/package.json`，cwd = 模块根）：
- `:10` `prod` = `go run main.go server -e prod` → exe 在 `/var/folders` 下，exe 链不可用 → 落到 `dev`
- `:11` `build` = `go build -o gospeak main.go` → exe 在 `server/gospeak`，候选里同样没有 `server/../VERSION` → 落到 `dev`

影响面（已读代码确认）：`system_update_service.go:120-125` 用 `compareVersion(latest, buildinfo.Version)`；`:198-203` 对不可解析版本返回 0 → `Available` 恒 false，**更新提示永久静默**；`version` 命令也输出 `dev`。

兜底缺失：`buildinfo_test.go:66-84`（`TestResolveVersionFindsRepositoryRootFile`）模拟的正是 `root/app/server` 旧层级，拆仓后**照样绿**。
Docker 不受影响（cwd=`/app`，`VERSION` 是第一个候选）；air 也不受影响（`.air.toml` 的 `tmp/main` 命中 `dir/../..`）。

**改法**：`readInfoFile` 补 `filepath.Join("..", name)` 候选（或改成逐级上溯最多 3 层）；把该测试改成同时覆盖 `root/server` 结构；Task 7 增加一条 `cd server && go run main.go version` 期望输出 `1.3.0-rc.1`。

### 中

**P7. 服务端仓缺最小 JS 脚手架，Task 7 的验证命令跑不了；`test/turbo.json` 路径未改。**
服务端结构里没有 `package.json` / `pnpm-workspace.yaml` / `.gitignore`，而 Task 2 的 tree-filter 正是删掉它们的；Task 7 Step 2 却写 `cd server && pnpm test:server`。
`test/turbo.json:8-10` 的缓存输入仍是 `"$TURBO_ROOT$/app/server/**/*.go"` 等，Task 7 只改 `globalSetup.ts:23`。
忽略规则缺口：`test/.runtime/`（`.gitignore:126`）、`app/server/gospeak`（`:77`）、`app/server/uploads/`（`:109,112`）、`internal/webui/dist/*` + 占位反忽略（`:21-22,139-144`）在服务端仓都没有对应规则 → 二进制、上传目录、运行时文件会被误提交。
**改法**：建最小根 `package.json`（`test:server` / `version:check` / `gen:protocol`）、`pnpm-workspace.yaml`（`packages: [test]`）、`.gitignore`（`/gospeak`、`test/.runtime/`、`server/uploads/`、`server/internal/webui/dist/*` + `!EMBED_PLACEHOLDER` 反忽略）；Task 7 Step 2 改成 `pnpm install && pnpm test:server`，并同步改或删 `test/turbo.json`。

**P8. Task 6 Step 1 重复造轮子；`gen_contract_test.go` 删完只剩空壳。**
`generate_test.go:13-28` 的 `TestGolden` 已经在做完全等价的事（`Generate("../protocol")` 与 `testdata/golden/*.ts` 逐字节比对，报错文案也指向 `-update-golden`）。计划新加的 `TestGoldenMatchesGenerated` 逻辑等价、无新增覆盖。`gen_contract_test.go` 全文 29 行只有 `TestGeneratedArtifactsFresh` 一个测试，Step 2 删掉后该文件只剩重复测试。
**改法**：Task 6 只**删除** `gen_contract_test.go`（连带 `os`/`filepath` import），golden 新鲜度交给已有 `TestGolden`；若坚持在 protocol 包内留一道，加注释说明分工，避免双份维护。

**P9. 协议流程文档/注释全线指向已不存在的命令，Task 10 的两次 grep 都抓不到。**
`AGENTS.md:127/188/206`、单一事实源索引 `:76`、`app/server/internal/protocol/AGENTS.md:26/35`、`app/server/internal/genprotocol/AGENTS.md:10,15`；代码里 `generate.go:385`（生成头）、`main.go:14`（帮助文本）仍写 `app/web/src/protocol/generated`。
Task 10 Step 2 的 grep 只抓仓名；Step 4 的 grep `app/web|app/desktop|app/mobile|app/shared` **抓不到 `pnpm gen:protocol`**（不含 `app/` 前缀）→ 四步流程会原样留在客户端仓 AGENTS 里继续误导人。
**改法**：Task 10 增加显式「协议章节重写」——客户端写「golden → generated 同步检查 + `--fix` 修复」，服务端写「改 `protocol` → `go run ./cmd/gen-protocol -update-golden` → 提交 golden → 触发客户端同步」；同时改 `generate.go:385` 与 `main.go:14`。

### 该角度已确认无问题
- **`module GOSpeak` 与 import 路径**：`go.mod:1` 是 `module GOSpeak`，无文件系统成分，目录 `app/server` → `server/` **不改变任何 `GOSpeak/internal/...` import`**；服务端不依赖 proxyrules（`grep` 无命中）。唯二跨目录引用是 `gen_contract_test.go:19`（P8）与 `cmd/gen-protocol/main.go:19`（P5）——**真问题在相对路径深度（P6），不在模块名**。
- **`test/globalSetup.ts` 的 `repoRoot` 推导**：`setupDir = <repo>/test`、`repoRoot = resolve(setupDir,"..")`，新结构 `join(repoRoot,"server")` 正是 Go 模块根，改动正确；`config.go:169-181` 的 `findRepoDeployEnv` 从 cwd 逐级上溯找 `deploy/.env`，`server/` → 命中 `<repo>/deploy/.env`，配方不变。残留问题只在命令入口（P7）。
- **`app/desktop/go.mod` 的 module 名**：`app/shared/proxyrules/go.mod:1` 的 module 就是 `gospeak/proxyrules`，无需改 module 名，只改 replace 路径深度。

---

## 可执行性与客户端构建 review 补丁（exec-order + client-build，2026-11-26）

> **本节的 E1–E5 会让计划「照着做也跑不通」**，属于结构性问题，不能靠打补丁绕过。

### 阻塞

**E1. 壳从来没被 push，gitlink 从来没被 bump —— Task 12 验证的不是拆分后的真实状态。**

计划全文的 `git push` 只出现在 tag 与三个子仓，**没有任何一步 push 壳仓**；Task 5 Step 3 只 `git commit`。而默认分支是 `release`，当前分支是 `feature/multiplatform-repo-restructure`，Task 5 的壳提交落在这条分支上。

后果链：
- Task 12 Step 1 的 `git clone --recursive https://github.com/GOSpeak-org/GOSpeak.git` 拿的是远端 `release` = **拆分前的单体仓** → `ls server client docs` 直接失败；
- 即便补了 push，`git submodule add` 把 gitlink 钉在 Task 2/3/4 那三个提交，Task 6–11 之后子仓的新提交**没有任何 Task 回壳 bump**；
- Task 12 Step 3 断言 `desktop/go.mod` 含 `../proxyrules`，但 Task 3 的 tree-filter 只移目录不改 go.mod，`app/desktop/go.mod:18` 至今是 `=> ../shared/proxyrules` → clone 里必是旧值。

**改法**：Task 5 增加 `git push origin HEAD:release`；在 Task 11 后新增 **Task 11.5「bump 壳指针」**：
```bash
for d in server client docs; do
  git -C $d fetch origin release && git -C $d checkout release && git -C $d reset --hard origin/release
done
git add server client docs
git commit -m "chore: bump submodules to post-split state"
git push
```
Task 12 Step 1 追加断言：`git -C client rev-parse HEAD` 与 `git ls-remote origin refs/heads/release` 一致。

**E2. 鸡生蛋：Task 2 Step 3 的验收依赖 Task 6 才产出的改动。**

实测复现（把 server 目录复制成单仓形状，模拟 tree-filter 后无 `app/web`）：
```
--- FAIL: TestGeneratedArtifactsFresh
    gen_contract_test.go:23: read committed events.ts: open ../../../../app/web/src/protocol/generated/events.ts: no such file or directory
```
而协议解耦写在 Task 6。**Task 2 的验收要等 Task 6 完成**。

**改法**：把协议解耦**前置到 Task 0 之后、Task 2 之前**（新增 Task 1.5：原仓内先改 `gen_contract_test.go` 只比 golden + `gen-protocol -out` 必填），让 tree-filter 直接带走；或退一步，Task 2 Step 3 只跑 `go build ./... && go test ./internal/genprotocol/ -count=1`，并在 Expected 里显式写「`internal/protocol` 契约测试在 Task 6 前预期失败」。

**E3. Task 7 的 Dockerfile 改造跑不通，而且验错了东西。**

三处硬伤：
1. **漏改**：`Dockerfile:30` 是 `COPY app/server/ ./`，计划只提了把 `:28` 的 `COPY app/server/go.mod` 改为 `server/go.mod`；
2. **引用已删阶段**：`Dockerfile:32` 的 `COPY --from=web-builder /src/app/web/dist/ ./internal/webui/dist/` 指向被删除的 `web-builder`；
3. **阶段错了**：前端不是运行时挂盘，而是**编译期嵌入**——`app/server/internal/webui/embed.go:13-14` 是 `//go:embed all:dist`，由 go-builder 阶段在 `RUN go build` 之前填充。设计里的 `RUN mkdir -p /app/static && tar -xzf ...` 落在**最终 alpine 阶段**，对二进制内的资产毫无影响：镜像里 `/app/static/index.html` 存在，而服务实际返回的还是 `EMBED_PLACEHOLDER`。Task 7 的 Expected「镜像内 `/app/static/index.html` 存在」**恰好会通过** —— 典型的错断言。

**改法**：改到 **go-builder 阶段**：
```dockerfile
ARG CLIENT_WEB_URL
ADD $CLIENT_WEB_URL /tmp/web-dist.tar.gz
RUN mkdir -p ./internal/webui/dist && tar -xzf /tmp/web-dist.tar.gz -C ./internal/webui/dist \
 && test -f ./internal/webui/dist/index.html
```
再 `go build`。断言换成「起容器后 `curl -fsS localhost:8998/ | grep -q '<div id=\"root\"'`」+ 单测 `webui.HasAssets() == true`。若决定去 embed，必须单独加一个 Task，不能一步跨过去。

**E4. 没有任何步骤产出 `web-dist.tar.gz`，且 web/desktop 共用 `dist/` 会让该产物可能是桌面目标包。**
（合并 client-build #4 与上文 B5）

- `web-dist` 只出现在 Task 7 的 Consumes 与 Dockerfile 片段，**没有任何 Task 生成或发布它**；产出它的 `release.yml:274-299` 随 tree-filter 一起消失；
- `app/web/vite.config.ts:160-163`：`outDir: gospeakTarget === "mobile" ? "dist-mobile" : "dist"` → **web 与 desktop 共用 `dist`**；`desktop/mygo.config.ts:39-44` 的 buildCommand 又会跑一次 `GOSPEAK_TARGET=desktop` 覆盖 `web/dist`。

后果：Task 7 的 `--build-arg CLIENT_WEB_URL=<某 URL>` 无源可指；若与 desktop 构建混在一个 job，服务端会拿到 `__GOSPEAK_DESKTOP__=true` 的包。

**改法**：客户端新增**独立**打包 job：`GOSPEAK_TARGET=web pnpm --filter @gospeak/web build` → `tar -czf gospeak-web-v<ver>.tar.gz -C web dist`，明确「不得与 desktop 打包或 `pnpm -r build` 同 job」，并固定产物名供 Task 7 引用。建议同时把 `vite.config.ts` 的 desktop 目标改成独立 `dist-desktop/`，彻底消除覆盖。

**E5. `pnpm-workspace.yaml` 漏掉 `desktop` 与 `mobile`，两条构建链静默不执行且退出 0。**

现仓靠 `app/*` 才把这两个纳入 workspace；计划只写 `packages: ["web", "packages/*"]`。

实测（`/tmp` 同构 workspace）：
```
pnpm --filter @p/mobile hi  → No projects matched the filters，exit=0
pnpm -r hi                  → 只输出 web，mobile 被静默跳过
pnpm install --lockfile-only → Done in 37ms（不报错，直接不装）
```

后果：`mygo-cli`、Capacitor 依赖永不安装；`mygo build` / `cap sync` 变成空跑+成功；Task 8 Step 5 与 Task 12 Step 2 的「`pnpm -r build` 全量通过」会漏掉桌面与移动两条链却报绿。

**改法**：`packages` 增加 `"desktop"`、`"mobile"`；客户端 CI gate 加显式存在性断言（`pnpm --filter @gospeak/desktop-app exec mygo --version`、`pnpm --filter @gospeak/mobile exec cap --version`）。

### 高

**H1. `mobile/package.json` 的 `node ../../scripts/*.mjs` 断链，且该文件不在任何 Task 的 Files 里。**
旧布局 `app/mobile` → `../../` = 仓根 `/scripts` ✓；新布局 `client/mobile` → `../../` = **壳根**，那里没有 `scripts/`。
后果：`pnpm --filter @gospeak/mobile sync / sync:ios / sync:android`（移动端唯一构建入口）全部 ENOENT；而 Task 12 只跑 `pnpm -r build`（mobile 无 build 脚本），不会暴露。
**改法**：改为 `../scripts/…`；把 `client/mobile/package.json` 写进 Task 8 Files；Task 8 Step 5 加冒烟 `pnpm --filter @gospeak/mobile exec node ../scripts/mobile-version.mjs`。

**H2. `desktop/webui/dist` 无任何跟踪文件，`go:embed all:webui/dist` 在新仓首次 `go build` 必失败。**
`app/desktop/proxyserver.go:51` 有 `//go:embed all:webui/dist`；`git ls-files | grep -ci 'desktop/webui'` → **0**。实测缺目录时 `pattern all:webui/dist: no matching files found`。
对照：服务端有 `EMBED_PLACEHOLDER` 被跟踪，所以服务端新仓能干净构建，桌面端没有等价占位。
**改法**：二选一 —— (a) Task 3 抽取后补 `desktop/webui/dist/.gitkeep`（.gitignore 配套反忽略）；(b) Task 8 加 seed 步骤 `pnpm --filter @gospeak/web build:desktop && mkdir -p desktop/webui/dist && cp -R web/dist/. desktop/webui/dist/`（复刻 `ci.yml:416-425`）。

**H3. 三个子仓都从 `release` 抽取，丢掉当前分支的 24 个提交（含设计声称要保留的前置成果）。**

实测：`git log --oneline release..HEAD | wc -l` → **24**；`git diff --stat release...HEAD` → 28 files, +2696/−1120，含 `ci: create one release per component...` 等，而设计 §8 明确「保留其已交付的前置成果：一组件一 Release」。

三个抽取命令都是 `git clone --no-local --branch release`，**不含 Task 0 在 feature 分支上的 docs 提交** → 两份新文档不会进任何子仓，Task 12 Step 5「更新 plans/README.md」无路径可写（拆完后该文件实际属于 `server/docs/`）。

**改法**：Task 0 增加「把当前分支并入 `release`」（`git switch release && git merge --ff-only feature/... && git push`），或三条 clone 改 `--branch feature/multiplatform-repo-restructure` 并在 Expected 校验设计文档在树内；Task 12 Step 5 路径改为 `server/docs/superpowers/plans/README.md`。

**H4. 服务端仓没有 `.gitignore`，public 仓会直接暴露真实 `.env` 与数据库。**

根 `.gitignore` 承担全部规则，其中 `:56-58`（`app/server/.env*`）、`:66-68`（`*.db*`）、`:71`（`app/server/logs/`）、`:112`（`app/server/uploads/`）、`:131`（`deploy/.env`）、`:136`（`deploy/env/*.local.env`）在 server 仓化为根路径后**全部失效**；Server 结构里不含 `.gitignore`，也没有 Task 重建。而 Task 10 Step 5 就是各仓 `git add -A`，Task 7 还要在 server 里跑集成测试（产生 `test/.runtime`、临时 DB）。

**改法**：新增「仓级 .gitignore」步骤 —— server 用原文件裁剪（保留通用段 + 后端段，去掉 `app/` 前缀）；断言改成构建后验：`go build -o gospeak ./main.go && pnpm --dir test test && git check-ignore -q deploy/.env && test -z "$(git status --porcelain)"`。

**H5. 客户端根配置被删且无人重建：`biome.json` / `lefthook.yml` / `commitlint.config.js` / `turbo.json` / `package.json` 内容未定义。**
`biome.json:7-24` 的 `files.includes` 全是旧前缀；`turbo.json:7-17` 的 `globalDependencies` 同样；`lefthook.yml` 的 7 道 pre-push 门禁全部 `cd app/...`；`packages/*/package.json` 的 lint 依赖仓根配置；根 `package.json:41-53` 的 `pnpm.overrides`（含 `peerDependencyRules.typescript: 7.x`）不复制会直接影响安装。
**改法**：Task 8 增加「重建 `client/biome.json`（includes 改 `web/src/**`、`desktop/*.ts`、`mobile/*.ts`、`scripts/**`）、`client/lefthook.yml`、`client/commitlint.config.js`、`client/turbo.json`」，并显式写出 `client/package.json` 的 devDependencies + `pnpm.overrides` + `peerDependencyRules` 来源。

**H6. 版本单一事实源被删，Task 11 的断言命令本身无效。**
`.github/versions/*.yaml` 是唯一事实源（`check-version.mjs:103-105`、`mobile-version.mjs:26`），被 tree-filter 删除。
且 `mobile-version.mjs` **没有 flag 解析**：`argv[2] ?? resolve(dirname(import.meta.url), "..")`（`:95-96`），`--check` 会被当成 rootDir 拼成 `--check/.github/versions/mobile.yaml` → ENOENT；它本身还是**写入器**（`replaceAll` + `writeFileSync`），不是校验器。
**改法**：客户端仓新建 `.github/versions/{desktop,mobile}.yaml`；Task 11 改为 `node scripts/mobile-version.mjs && node scripts/check-version.mjs . "" desktop && node scripts/check-version.mjs . "" mobile`，把「检出 3 处一致」换成 `git diff --exit-code` 断言。

**H7. Capacitor 生成物写死 `node_modules` 相对层级，拆仓后多退一级。**
被跟踪文件：`app/mobile/android/capacitor.settings.gradle:3` 的 `new File('../../../node_modules/...')`（旧布局上溯 3 级 = 仓根 ✓，新布局 = **壳根** ✗）；`app/mobile/ios/App/CapApp-SPM/Package.swift:15-18` 的 `../../../../../node_modules/...` 同理。
后果：不在客户端仓内先 `cap sync` 重写，gradle/Xcode 会去壳根找 capacitor 模块。而计划验证链一次都跑不到移动原生构建。
**改法**：Task 8 增加「客户端仓内跑 `pnpm --filter @gospeak/mobile sync:no-bind`，让 Capacitor 按新位置重写并提交」；Task 9 客户端 CI 加 grep 断言这两处的相对层级。

**H8. `check-mobile-bridge.mjs` 被漏掉，移动端桥接契约门禁会整体消失。**
Task 9 Step 1 写「七个脚本」但只列 6 个，设计 §5 明确归客户端。该脚本 `:14-16` 硬编码 `app/mobile/ios/App/App`、`app/mobile/ios/App/App.xcodeproj/project.pbxproj`、`app/mobile/android/app/src/main/java/dev/gospeak/mobile`。
**改法**：清单补第 7 个，常量改 `mobile/...` 前缀，保留其 `.test.mjs`。

### 中

**M1. Task 9 要迁移的资产已被 tree-filter 自己删掉。**
Task 2 L210 / Task 3 L266 的 `rm -rf app packages scripts agent_test_logs artifacts .github node_modules` 让**两个仓都没有 `scripts/` 和 `.github/`**。于是 Task 9 Step 1「复制七个脚本到 client/scripts/」源不存在、Step 2 `cd server && node --test scripts/check-version.test.mjs` 路径不存在、Step 3 客户端 `lint(biome)` 没有 `biome.json`。
**改法**：tree-filter 保留 `scripts/`、`.github/`、`.gitignore`、`biome.json`、`lefthook.yml`、`turbo.json`、子目录 `AGENTS.md`，再在各仓内做「删无关部分」的独立提交；若坚持先删，则 Task 9 加 Step 0：`git archive pre-shell-split scripts .github | tar -x -C /tmp/scripts-src`。

**M2. Task 7 Step 2 的验证命令在 server 仓跑不起来。**
server 仓根无 `package.json`/`pnpm-workspace.yaml`，`test:server` 定义在被删的根 `package.json:19`。实测 `pnpm test:server` → `ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL × Command "test:server" not found`。而 `test/package.json` 自带 `"test": "vitest run"`。
**改法**：改为 `cd server/test && pnpm install && pnpm test`；或建最小 server `package.json`（`{"private":true,"scripts":{"test:server":"pnpm --dir test test"}}`）+ workspace 保留 `- "test"`。

**M3. Task 12 Step 4 的回滚预案不完整（实测 `reset --hard` 回不干净）。**
`/tmp` 复现「普通目录 → submodule」形状：
```
git reset --hard HEAD~1 → warning: unable to rmdir 'docs': Directory not empty
ls docs → d.txt legacy.md    # 子仓文件残留
git status --short → ?? docs/d.txt
```
且：唯一备份在 `/tmp`（macOS 会定期清理）、从未 `git bundle verify`；`git bundle --all` **不含未跟踪/被忽略文件**，而 Task 5 的 `rm -rf` 会连带删掉 `deploy/.env`（真实密钥）、本地 DB、`node_modules`。
**改法**：写成可执行四步 —— ① `git submodule deinit -f --all && rm -rf server client docs .git/modules/*`；② `git reset --hard pre-shell-split`；③ `git push --force-with-lease origin release`（注明回滚窗口内冻结推送）；④ 恢复本地状态（Task 0 增加 `tar czf ~/gospeak-local-state.tgz deploy/.env app/server/.env *.db app/server/uploads` + 解包校验），并把 bundle 复制到仓库外 + `git bundle verify`。

### 低

**L1. 一批 Expected 判定不了或必然误判。**
- Task 5 Step 1 Expected「`ls` 只剩 `.git`、`LICENSE`、`README.md`」与自己的命令矛盾：删除清单没删 `.gitignore`、`.dockerignore`、`.npmrc`、`.agents`、`.superpowers`、`.turbo`、`.worktrees`、`.tmp-visual-proposal`（`ls -a` 实测均存在），Step 2 还会新增 `.gitmodules`；
- Task 10 Step 4 Expected「`grep app/web|app/desktop|... --include='*.md'` 无命中」必然误判：`docs/` 下 27 个 md、`app/web` 278 行命中（历史档案）；
- Task 6 Step 2 的 `grep -c 'app/web'` 在计数为 0 时退出码是 1，`&&` 链会判失败；
- Task 0 Step 2 Expected「bundle > 5MB」没有任何大小或完整性校验。
**改法**：换成可判定断言 —— `test -f` + `stat -f%z` 阈值、`git bundle verify`、`[ -z "$(grep -rl … README.md AGENTS.md docs/AGENTS.md)" ]`、`git status --porcelain` 精确对比。

**L2. 三处硬编码 `app/...` 提示串不在任何替换清单里，Task 10 的 grep 也不覆盖 `.mjs`。**
`scripts/mobile-bind-proxyrules.mjs:33`（`"cd app/shared/proxyrules && go mod download ..."`）、`:358`（错误修复指引）、`:303`（注释）；`app/web/scripts/capture-storybook-baseline.mjs:3`（注释）。
**改法**：Task 9 Step 1 顺带改前缀；Task 10 Step 2 的 include 追加 `*.mjs`（否则 `*.mjs`、`*.gradle`、`*.swift` 里的旧路径永远清不掉）。

**L3. 客户端 `.gitignore` 漏 `go.work`/`go.work.sum`（客户端有两个 Go module）、`.turbo/`、`artifacts/`、`*.log`、`.DS_Store`、`*.local`。**
**改法**：补这 6 条。

**L4. 设计文档里有一条错误的备选写法。**
计划 L514 给的 `replace github.com/GOSpeak-org/GOSpeak/app/shared/proxyrules => ../proxyrules` 与实际 `require gospeak/proxyrules` **不匹配**，照抄会直接 build 失败。**只保留 `replace gospeak/proxyrules => ../proxyrules`**，删掉备选。

### 两个 reviewer 均确认无问题的角度
- `desktop/go.mod` 的 `replace => ../proxyrules`：**成立**（`client/desktop` 上溯一级 = `client/` = 新 `proxyrules/`）。
- `mobile-bind-proxyrules.mjs` 的 `MODULE_DIR`/`OUTPUT_DIR`：计划写对了；`root = resolve(dirname, "..")` 在 `client/scripts/` 时自动 = `client/`，**无需改**。gradle 的 `../../build/proxyrules/proxyrules.aar` 与新布局一致。
- `web/tailwind.config.ts` 改 `../packages/...`：**正确**（基准是 cwd/config 目录均已验证），且 Task 8 Step 3 自带 `orientation=vertical` 实证门禁。
- `capture-storybook-baseline.mjs` 的 `repoRoot` 改 `../..`：**正确**。
- Docs 仓独立性：`app/docs/package.json` 只依赖 `vitepress`，配置无跨仓引用，可独立 `pnpm install && pnpm build`。
- `git branch -m release` 在已处于 release 时 `exit 0`；`git submodule add -b release` 落地为本地跟踪分支（不会踩 detached HEAD）；`gh repo create --confirm` 在 gh 2.96 仍有效。

---

## Git 操作实测 review 补丁（git-ops，2026-11-26）

> 本节全部为在 `/tmp` 原样执行计划脚本的实测结果。**G1/G2 与 E1/H3 同源，此处只补实测数据。**

### 阻塞（补充实测）

**G1. 默认分支是 `release`，而壳的改写提交全在 feature 分支上。**

`gh repo view --json defaultBranchRef` → `{"name":"release"}`；Task 12 的 `git clone --recursive` 不带 `-b`，拿的就是 `release`。而 `git rev-list --count release..HEAD` → **24**。

Task 0 Step 2 的 `pre-shell-split` tag 也打在 feature 分支提交上 → Task 12 Step 4 的 `git reset --hard pre-shell-split` 只在当前分支仍是该 feature 分支时有效。

**改法**：Task 5 末尾 `git push origin HEAD:release`（或先 `git switch release` 再做壳化）；`gh repo edit GOSpeak-org/GOSpeak --default-branch release`；Task 12 Step 1 后断言 `test -f .gitmodules && [ "$(git submodule status | wc -l)" -eq 3 ]`。

**G2. 三条抽取命令都用 `--branch release`，而 24 个提交只在 feature 分支。**

实测 `git diff --stat release...HEAD` 涉及：`system_update_service.go`(15)、`router.go`(6)、`updater.go`(4)、`updater_release.go`(88)、`updater_release_test.go`(48)、`ClientDownload.vue`(67)、`release-notes.mjs`(105)、`release.yml`(85)、`version-bump.yml`(99)、`build-mobile.yml`(79)、`.github/versions/*.yaml`。

设计 §8 声称「保留一组件一 Release 的前置成果」，但这些改造**只存在于 feature 分支**。后果：Server 仓一出生就缺 `system_update_service.go`/`router.go` 的修复，Client 仓缺组件化 updater，而 Task 10 又要「改 updater 指向 Client」—— 等于在旧代码上重做，且 Task 6/7/9 的补丁基线全部偏旧。

**改法**：抽取前 `git switch release && git merge --ff-only feature/multiplatform-repo-restructure`；Task 0 把「抽取源 ref」记进 `/tmp/baseline-*.txt`，Task 2/3/4 只引用该 ref。

### 高

**G3. 实跑 Task 2/3 后仓根都没有 `scripts/`。**（与 E-M1 同源）
Task 2 后 `git ls-tree --name-only HEAD | grep scripts` → 无输出；Task 3 后根树为 `.agents .dockerignore .npmrc .tmp-visual-proposal LICENSE desktop mobile packages proxyrules web` —— 与设计 §2「客户端仓有 scripts/」矛盾。原仓 `git ls-files scripts` 有 13 个文件。

**G4. Docs 仓永远拿不到 `.github/workflows/docs.yml`。**（与 B2/H2 同源）
`--subdirectory-filter app/docs` 只能带走 `app/docs` 内部内容，仓根的 `.github` 进不去；实测 Docs 仓 `git ls-files | grep -c '^.github'` → **0**。

**G5. GitHub 官方明确：Pages 不随仓转移重定向。**

`gh api repos/NoelOrin/GOSpeak/pages` → `{"build_type":"workflow","source":{"branch":"release","path":"/"},"html_url":"https://noelorin.github.io/GOSpeak/"}`；environments 里还有 `github-pages`（带 branch_policy）。

官方《Transferring a repository》原文：**"If the transferred repository contains a GitHub Pages site, then links to the Git repository on the Web and through Git activity are redirected. However, we don't redirect GitHub Pages associated with the repository."**

同时 `app/docs/.vitepress/config.ts:4` 是 `const base = isProd ? '/GOSpeak/' : '/'` —— 搬到 Docs 仓后必须是 `'/GOSpeak-Docs/'`，全篇 14 个 Task 都没提这一行。

**改法**：Task 0.5 记录 Pages 现状；Task 9 在 Docs 仓 `gh api -X POST repos/GOSpeak-org/GOSpeak-Docs/pages -f build_type=workflow` + 重建 docs workflow；Task 10 把 `base` 改 `'/GOSpeak-Docs/'`；Task 12 加 `curl -sI https://gospeak-org.github.io/GOSpeak-Docs/ | head -1`。

### 中

**G6. Task 2/3 的提交数验收阈值与实测不符，执行者会误判为抽取失败。**

同一环境、同一脚本、`--prune-empty -- --all`：

| 指标 | 计划写的 | 实测 |
|---|---|---|
| 抽取源 `git rev-list --count release` | — | **87** |
| Task 2 后 `git rev-list --count HEAD` | `> 50` | **47** ← 判定失败 |
| Task 3 后 同一指标 | `> 100` | **53** ← 判定失败 |

但树内容是完整的：客户端 `git ls-files web | wc -l` = **442**（与 `git ls-tree -r release -- app/web` 的 442 完全一致）；`proxyrules/` 4/4；服务端 `git ls-files server | wc -l` = **468**；`test docs deploy` 合计 80。monorepo 各方向提交被 `--prune-empty` 大量丢弃属正常。

**改法**：Expected 直接写实测值（Task 2 `= 47`、Task 3 `= 53`，并注明「在 release 未合并 feature 的前提下」），或换相对判据（Task 0 记基线，Task 2/3 断言 `≥ 基线 × 0.5` 且 `git log --oneline -- web` 非空）。**注意若采纳 G2 先合 feature，这两个数值会变，必须重测后再写死。**

**G7. submodule 里的 detached HEAD 实测：Task 5 碰巧能过，重新 clone 就会失败。**

- Task 5 的 `git submodule add -b release` 会留下 `## release...origin/release`（本地分支 + upstream），所以紧接着的 push 成功；
- 但 `git clone --recursive` 后子模块是 `## HEAD (no branch)`，`git submodule status` = ` cb7bede… (remotes/origin/release)` —— `branch = release` 对 `update --init` **不起作用**，只被已禁用的 `--remote` 消费；
- 实测 detached HEAD 下 `git push` → `fatal: You are not currently on a branch.`

**改法**：Task 5 Step 2 后加 `git -C server switch release && git -C client switch release && git -C docs switch release`；Task 6/7/9/10/11 每次 push 前断言 `git -C <dir> symbolic-ref -q HEAD`（空输出即 detached，中止）。

**G8. 三个新仓的默认分支完全依赖「首个 push 自动设默认」，无任何校验。**

本地复现（bare 仓 HEAD→不存在的 `refs/heads/main`，只推 `release`）：`git clone` **退出码 0**，但 stderr `warning: remote HEAD refers to nonexistent ref, unable to checkout`，工作区只有 `.git`。而 `git clone --recursive` 反而成功（子模块按 pin SHA checkout）→ **Task 12 查不出问题**。

与 Task 10 README「开发请直接 clone 对应子仓」直接冲突。

**改法**：Task 1 建仓后与 Task 2/3/4 首次 push 后，各加 `gh api repos/GOSpeak-org/GOSpeak-<X> --jq .default_branch` 断言 `release`；不满足则 `gh repo edit ... --default-branch release`。

**G9. 删除清单两个方向都不准。**

**删多了（bundle 救不回）**：`rm -rf app packages test deploy docs scripts agent_test_logs artifacts .github node_modules` 会连带删掉被忽略的本地数据。实测存在 `app/server/db/app.db`（425984 字节）与 `app/server/db/nats/`；`git status --ignored --short | wc -l` = **76** 条。而 `git bundle create --all` 的 `list-heads` 只含 refs，**不含未跟踪/被忽略文件** —— 设计 §7「先 git bundle + tag」对此无效。

**删少了（留垃圾）**：实跑 Task 2 后根树 = `.agents .dockerignore .npmrc .tmp-visual-proposal CHANGELOG.md Dockerfile LICENSE VERSION deploy docs server test`；`.agents/` 有 **24 个 tracked 文件**（含 `room-voice-e2e` 技能，里面硬编码 `app/` 路径），`.tmp-visual-proposal/index.html` 是临时提议页，`.dockerignore` 里写死 `app/server`。

**忽略规则丢失**：实跑后 Server 仓 `git ls-files | grep -c 'gitignore\|gitattributes'` = **0**；Client 仓只有嵌套的 6 个，根级靠 Task 8 手写的 12 行，而原根 `.gitignore` 有 155 行，其中 `**/dist/` + `!app/server/internal/webui/dist/` 的「先忽略再反忽略」组合（`go:embed` 占位符依赖它）会随路径变化失效。

**改法**：Task 5 之前加 `rsync -a --exclude node_modules app/server/db app/server/uploads app/server/.env* deploy/.env* artifacts/ /tmp/gospeak-local-backup/`；tree-filter 末尾补 `rm -rf .agents .tmp-visual-proposal`；Task 2 增加「生成 `server/.gitignore`（重写 `webui/dist` 反忽略）与 `server/.dockerignore`」；Task 8 的 `.gitignore` 以原 155 行为基线平移，而不是手写子集。

### 实测确认无问题的部分

1. **`for x in _s/*` 的分词/空目录/隐藏文件行为**：filter-branch 的 tree-filter 在独立工作树里跑，**工作目录内没有 `.git`**；实跑全历史后 `git log --all --name-only | grep -E '^_s/|^_c/'` 无残留。
   **风险注记（不占条目）**：`git-filter-branch` 没有 `set -e`，脚本以 `true` 结尾，任何 `mv` 失败会被静默吞掉，随后 `rm -rf app` 就把源目录删了。建议每段末尾加 `[ -d web ] && [ -d packages ] || exit 1` 之类断言。
2. **`rm -rf app` 未连带误删已搬出内容**：HEAD 与根提交的 `app/` 路径数均为 0；`proxyrules/` 4 文件全在；`web/` 442 文件与原仓一致。
3. **`transfer` 的连带行为**：官方文档明确 issues/PR/wiki/stars/watchers 随转移、webhooks/secrets/deploy keys 保持、`git clone/fetch/push` 到旧地址会重定向、旧名被 retire 不可复用。实测本仓 `gh api .../actions/secrets` = `{"total_count":0}`、`variables` = 0、`branches/release/protection` = 404 → 无需重建密钥/变量/保护规则；`tags` 22、`releases` 9 与期望一致。**现有仓没有 `.gitmodules`、`git ls-files -s` 中 0 条 mode 160000** → 之前所有 submodule 讨论都是纯设计，从未落地。
4. **`git branch -m release`**：在已处于 `release` 的仓里退出码 0，不报 "already exists"。
5. **`git rm --cached` → `rm -rf` → `submodule add` 顺序**：git 层面无数据丢失；风险只在被忽略文件（见 G9）。

---

## 六角度全链路 review 总汇总（2026-11-26）

> 6 个独立 reviewer，共 **59 条**发现，全部经过实读代码或 `/tmp` 实跑取证。本节做交叉验证与开工决策。

### 交叉验证：被 2 个以上 reviewer 独立命中的问题

| 问题 | 独立发现方 | 判定 |
|---|---|---|
| **壳从未 push、gitlink 从未 bump** → Task 12 验的是拆分前状态 | exec-order E1、git-ops G1 | **确认，阻塞** |
| **三个子仓从 `release` 抽取，丢 24 个提交** | exec-order H3、git-ops G2 | **确认，阻塞** |
| **`rm -rf .github` 删掉 8 个 workflow + 2 action + versions/*，只重建 ci.yml** | ci-ops C1、org-compat #3、git-ops G4 | **确认，阻塞** |
| **`rm -rf scripts` 导致 Task 9 无源可复制** | exec-order M1、client-build #6、git-ops G3 | **确认，高** |
| **没有步骤产出 `web-dist.tar.gz`，web/desktop 共用 `dist/`** | ci-ops C4、client-build #4、exec-order E4 | **确认，阻塞** |
| **Dockerfile 前端放错阶段（embed 是编译期）** | exec-order E3、ci-ops C3 | **确认，阻塞** |
| **GHCR 镜像名迁移无人负责** | ci-ops C5、org-compat #2、git-ops #8 | **确认，阻塞** |
| **Pages 不随仓转移、`base` 写死 `/GOSpeak/`** | org-compat #5、git-ops G5 | **确认，高** |
| **Task 10 grep 匹配的是新仓名，旧引用全漏** | org-compat #4、git-ops #8 | **确认，高** |
| **版本单一事实源 `.github/versions/*` 被删** | org-compat #3、client-build #5、exec-order H6 | **确认，高** |
| **`mobile-version.mjs --check` 不存在该参数** | client-build #5、exec-order H6 | **确认，高** |
| **服务端缺 `package.json`/`.gitignore`** | ci-ops C6、server-proto P7、exec-order H4、git-ops G9 | **确认，高** |
| **submodule 内 detached HEAD** | git-ops G7 | **确认，中** |

### 六个最致命的、原设计里没有的

| # | 问题 | 后果 |
|---|---|---|
| 1 | **已装机客户端更新通道冻死** | `updater_release.go:33` 的仓名编译在二进制里，改不了；壳仓冻结后老客户端永远看不到新版 |
| 2 | **版本静默退化 `dev`** | `buildinfo` 候选路径无 `../VERSION`，目录少一层 → `Available` 恒 false |
| 3 | **壳从未 push** | 计划做完 Task 5–11，远端 `release` 仍是单体仓 |
| 4 | **workspace 漏 desktop/mobile** | `pnpm -r build` 静默跳过两条链却 **exit 0 报绿** |
| 5 | **没人为 `web-dist.tar.gz` 负责** | Dockerfile 的 `ADD $CLIENT_WEB_URL` 无源可指 |
| 6 | **Dockerfile 前端放 final stage** | 前端是编译期 `go:embed`，放 final stage 对二进制无效；旧断言恰好会通过 |

### 已被实测推翻的计划断言

| 计划写的 | 实测 | 处理 |
|---|---|---|
| Task 2 后 `git rev-list --count HEAD > 50` | **47** | 改实测值（G6） |
| Task 3 后 `> 100` | **53** | 改实测值（G6） |
| `replace github.com/GOSpeak-org/GOSpeak/app/shared/proxyrules => ../proxyrules` | 实际是 `require gospeak/proxyrules` | **删掉该备选**（L4） |
| Task 2 Step 3 `go test ./internal/protocol/` 能过 | 依赖 Task 6 的改动，鸡生蛋 | 前置 Task 1.5（E2） |
| `gh api` 拉 golden | CI 的 `GITHUB_TOKEN` 跨仓 404 | 改 `raw.githubusercontent.com`（P4） |
| `mobile-version.mjs --check` | 无此参数，且是写入器 | 改 `check-version.mjs`（H6） |

### 开工前必须解决的最小清单（12 项）

**结构性（必须重构计划，打补丁绕不过去）**
1. **壳的 push 与 bump**：Task 5 加 `push origin HEAD:release`；新增 **Task 11.5 bump 指针**（E1/G1）
2. **抽取源 ref**：先用 `--ff-only` 把 feature 合并进 `release`，再抽取（G2）
3. **协议解耦前置**：新增 **Task 1.5**，在原仓内先改 `gen_contract_test` + `gen-protocol -out`（E2）
4. **发布编排迁移**：新增 **Task 9.5**，逐项分配 8 个 workflow + 2 action + 3 个 versions.yaml（C1/B2/G4）
5. **web 产物与三 target 分离**：`vite.config.ts` 让 desktop 走 `dist-desktop/`；客户端独立 job 产出 `gospeak-web-v*.tar.gz`（C4/E4）

**阻塞修复**
6. **桥接 Release**：壳仓保留 `gospeak-desktop-v1.4.0-rc.1`（B1）
7. **`buildinfo` 补 `../VERSION`** 候选 + 测试覆盖 `root/server` 结构（P6）
8. **Dockerfile 改到 go-builder 阶段** + 补 `COPY server/ ./` + 断言改 `curl` 验 `<div id="root"`（E3）
9. **workspace 加 `desktop`/`mobile`** + CI 加 filter 命中断言（E5）
10. **服务端建 `package.json` + `pnpm-workspace.yaml` + `.gitignore` + `.dockerignore`**（H3/H4/P7）

**迁移正确性**
11. **`check-protocol-sync` 接进 gate** + `--fix` + pinned ref + raw 直链（P1/P2/P3/P4）
12. **全局替换正则改 `grep -rni 'noelorin'`**，Files 补 `updater.go`/`ClientDownload.vue`/`deploy/*`（H1/G8）

### 数字基线（供执行者对照）

```
release 提交数            87
release..HEAD（feature）  24
Task 2 抽取后             47
Task 3 抽取后             53
app/web 文件数            442（抽取后一致）
app/server 文件数         468
现有 tag                  22（server 11 + desktop 11）
现有 release              9
现有 mode 160000          0（无 .gitmodules，submodule 从未落地）
.gitignore               155 行
git status --ignored      76 条（含 app/server/db/app.db 425984 字节）
```

---

## Task 0: 备份与前置确认

**Files:** 无（准备任务）

**Interfaces:**
- Produces: 回滚锚点与状态快照，供 Task 12 使用。

- [ ] **Step 1: 记录工作区状态**

```bash
cd /Users/noelorin/Projects/GOSpeak
git status --short --ignored > /tmp/pre-shell-split-status.txt
git log --oneline -1
```
Expected: 只有计划/设计文档为 modified 或 untracked。

- [ ] **Step 2: 提交文档改动并建立回滚锚点**

```bash
git add -A
git commit -m "docs: add shell-plus-three-repos split design and plan"
git bundle create /tmp/gospeak-pre-shell-split.bundle --all
git tag -a pre-shell-split -m "anchor before shell + three repos split"
git push origin pre-shell-split
```
Expected: bundle 存在且 > 5MB；tag 已推送；工作区干净。

- [ ] **Step 3: 记录基线数字**

```bash
git rev-list --all --count > /tmp/baseline-commits.txt
git tag -l | wc -l > /tmp/baseline-tags.txt
cat /tmp/baseline-commits.txt /tmp/baseline-tags.txt
```
Expected: 记录提交数与 tag 数（供 Task 3/4 抽取后比对）。

---

## Task 0.5: 把现有仓迁入 GOSpeak-org 组织

**Files:** 无本地文件（仅 remote 更新）

**Interfaces:**
- Consumes: Task 0 的备份锚点。
- Produces: `GOSpeak-org/GOSpeak` 可访问；本地 remote 指向组织路径。

- [ ] **Step 1: 确认组织与权限**

```bash
gh api orgs/GOSpeak-org --jq '.login'
gh api user/memberships/orgs/GOSpeak-org --jq '.role + " " + .state'
```
Expected: 输出 `GOSpeak-org`；角色为 `admin active`（否则无转移权限，停下询问）。

- [ ] **Step 2: 转移仓库**

```bash
gh api -X POST repos/NoelOrin/GOSpeak/transfer -f new_owner=GOSpeak-org
```
Expected: 返回 `"message": "Transfer initiated"`；几秒后 `gh repo view GOSpeak-org/GOSpeak` 成功。

- [ ] **Step 3: 更新本地 remote**

```bash
cd /Users/noelorin/Projects/GOSpeak
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak.git
git remote -v
git fetch origin --tags
```
Expected: remote 指向 `GOSpeak-org/GOSpeak.git`；`git rev-list --all --count` 与 Task 0 基线一致。

- [ ] **Step 4: 验证 tag 与 Release 跟随迁移**

```bash
git tag -l | wc -l
gh release list --repo GOSpeak-org/GOSpeak | head -3
```
Expected: tag 数与基线一致（22）；Release 列表可见。

- [ ] **Step 5: 确认旧 URL 重定向**

```bash
gh api repos/NoelOrin/GOSpeak --jq .full_name 2>&1 | head -2
```
Expected: 返回 `GOSpeak-org/GOSpeak`（GitHub 自动重定向）。

---

## Task 1: 创建三个子仓

**Files:** 无本地改动

**Interfaces:**
- Produces: 三个空 public 仓，供 Task 2/3/4 推送。

- [ ] **Step 1: 建仓**

```bash
gh repo create GOSpeak-org/GOSpeak-Server --public --confirm
gh repo create GOSpeak-org/GOSpeak-Client --public --confirm
gh repo create GOSpeak-org/GOSpeak-Docs   --public --confirm
# 注意：在组织下建仓需要 admin/owner 权限；已确认当前账号为 GOSpeak-org admin
```
Expected: 三个 `gh repo view` 成功，`visibility` 均为 `PUBLIC`。

- [ ] **Step 2: 验证可写**

```bash
for r in Server Client Docs; do git ls-remote "https://github.com/GOSpeak-org/GOSpeak-$r.git" && echo "$r OK"; done
```
Expected: 三条均成功（空仓无输出或仅 HEAD）。

---

## Task 2: 抽取服务端仓历史

**Files:** 临时目录 `/tmp/gospeak-server-extract`

**Interfaces:**
- Consumes: 现有仓历史；Task 1 的空仓。
- Produces: `GOSpeak-Server` 的 `release` 分支。

- [ ] **Step 1: 抽取并重排**

```bash
rm -rf /tmp/gospeak-server-extract
git clone --no-local --branch release /Users/noelorin/Projects/GOSpeak /tmp/gospeak-server-extract
cd /tmp/gospeak-server-extract
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --tree-filter '
  mkdir -p _s
  [ -d app/server ] && mv app/server _s/server
  for d in test deploy docs; do [ -d "$d" ] && mv "$d" _s/; done
  for f in Dockerfile VERSION metadata.yaml CHANGELOG.md LICENSE; do [ -f "$f" ] && mv "$f" _s/; done
  rm -rf app packages scripts agent_test_logs artifacts .github node_modules
  for x in _s/*; do [ -e "$x" ] && mv "$x" .; done
  rmdir _s 2>/dev/null
  rm -rf .gitignore pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md README.md
  true
' -- --all
```
Expected: 仓根含 `server/`、`test/`、`deploy/`、`docs/`、`Dockerfile`、`VERSION`。

- [ ] **Step 2: 清理 tag 与备份 ref**

```bash
git tag -d $(git tag -l) 2>/dev/null || true
git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d 2>/dev/null || true
git branch -m release
```
Expected: `git tag -l | wc -l` 为 0。

- [ ] **Step 3: 验证历史与可构建**

```bash
git rev-list --count HEAD
cd server && go build ./... && go test ./internal/protocol/ -count=1
```
Expected: 提交数 > 50（非单提交）；Go 构建通过。

- [ ] **Step 4: 推送**

```bash
cd /tmp/gospeak-server-extract
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak-Server.git
git push origin release --no-tags
```
Expected: 推送成功。

---

## Task 3: 抽取客户端仓历史

**Files:** 临时目录 `/tmp/gospeak-client-extract`

**Interfaces:**
- Consumes: 现有仓历史。
- Produces: `GOSpeak-Client` 的 `release` 分支，含 `web/ packages/ desktop/ mobile/ proxyrules/`。

- [ ] **Step 1: 抽取并重排（proxyrules 提到根）**

```bash
rm -rf /tmp/gospeak-client-extract
git clone --no-local --branch release /Users/noelorin/Projects/GOSpeak /tmp/gospeak-client-extract
cd /tmp/gospeak-client-extract
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --tree-filter '
  mkdir -p _c
  for d in web desktop mobile; do [ -d "app/$d" ] && mv "app/$d" "_c/$d"; done
  [ -d app/shared/proxyrules ] && mv app/shared/proxyrules _c/proxyrules
  [ -d packages ] && mv packages _c/
  rm -rf app test deploy docs scripts agent_test_logs artifacts .github node_modules
  for x in _c/*; do [ -e "$x" ] && mv "$x" .; done
  rmdir _c 2>/dev/null
  rm -rf .gitignore pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md README.md Dockerfile VERSION metadata.yaml CHANGELOG.md
  true
' -- --all
```
Expected: 仓根含 `web/`、`desktop/`、`mobile/`、`packages/`、`proxyrules/`。

- [ ] **Step 2: 清理 tag 并验证**

```bash
git tag -d $(git tag -l) 2>/dev/null || true
git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d 2>/dev/null || true
git branch -m release
git rev-list --count HEAD
git log --oneline -- web | head -3
```
Expected: 提交数 > 100；`web` 下有真实提交。

- [ ] **Step 3: 推送**

```bash
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak-Client.git
git push origin release --no-tags
```
Expected: 推送成功。

---

## Task 4: 抽取文档仓历史

**Files:** 临时目录 `/tmp/gospeak-docs-extract`

**Interfaces:**
- Produces: `GOSpeak-Docs` 的 `release` 分支。

- [ ] **Step 1: 抽取并推送**

```bash
rm -rf /tmp/gospeak-docs-extract
git clone --no-local --branch release /Users/noelorin/Projects/GOSpeak /tmp/gospeak-docs-extract
cd /tmp/gospeak-docs-extract
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --subdirectory-filter app/docs HEAD
git tag -d $(git tag -l) 2>/dev/null || true
git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d 2>/dev/null || true
git branch -m release
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak-Docs.git
git push origin release --no-tags
```
Expected: 仓根含 `.vitepress/`、`guide/`、`api/`、`package.json`。

- [ ] **Step 2: 验证可独立构建**

```bash
cd /tmp/gospeak-docs-extract
pnpm install && pnpm build
ls -d .vitepress/dist
```
Expected: 构建产物存在。

---

## Task 5: 现有仓变成聚合壳

**Files:**
- Delete: 全部代码目录
- Create: `.gitmodules`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 2/3/4 已推送的三个子仓。
- Produces: `GOSpeak` 只剩 `.gitmodules` + `README.md` + 三个 gitlink。

- [ ] **Step 1: 清空代码目录**

```bash
cd /Users/noelorin/Projects/GOSpeak
git rm -r --cached app packages test deploy docs scripts agent_test_logs artifacts .github 2>/dev/null || true
rm -rf app packages test deploy docs scripts agent_test_logs artifacts .github node_modules
git rm --cached Dockerfile VERSION metadata.yaml CHANGELOG.md pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md 2>/dev/null || true
rm -f Dockerfile VERSION metadata.yaml CHANGELOG.md pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md
```
Expected: `ls` 只剩 `.git`、`LICENSE`、`README.md`。

- [ ] **Step 2: 加三条 submodule**

```bash
git submodule add -b release https://github.com/GOSpeak-org/GOSpeak-Server.git server
git submodule add -b release https://github.com/GOSpeak-org/GOSpeak-Client.git client
git submodule add -b release https://github.com/GOSpeak-org/GOSpeak-Docs.git   docs
git config -f .gitmodules --get-regexp 'submodule\..*\.path'
```
Expected: 三条 path 精确为 `server`、`client`、`docs`；`git submodule status` 三行无前缀。

- [ ] **Step 3: 提交**

```bash
git add -A
git commit -m "refactor: turn shell repo into aggregate of server client docs"
```
Expected: 提交成功；`git ls-files -s server client docs` 三行 mode `160000`。

---

## Task 6: 修协议生成跨仓断点

**Files:**
- Modify: `server/internal/protocol/gen_contract_test.go`
- Modify: `server/cmd/gen-protocol/main.go`
- Create: `client/scripts/check-protocol-sync.mjs`

**Interfaces:**
- Consumes: golden 目录 `server/internal/genprotocol/testdata/golden/`。
- Produces: `gen_contract_test` 只依赖 golden；客户端能校验同步。

- [ ] **Step 1: 写 golden 一致性测试**

```go
func TestGoldenMatchesGenerated(t *testing.T) {
	files, err := genprotocol.Generate(".")
	if err != nil { t.Fatal(err) }
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join("..", "genprotocol", "testdata", "golden", name))
		if err != nil { t.Fatalf("golden %s: %v", name, err) }
		if string(got) != want { t.Errorf("%s 与 golden 不一致，请运行 gen-protocol -update-golden 并提交", name) }
	}
}
```
Run: `cd server && go test ./internal/protocol/ -run TestGoldenMatchesGenerated`
Expected: PASS。

- [ ] **Step 2: 去掉对 app/web 的硬依赖**

删除 `gen_contract_test.go` 里指向 `../../../../app/web/src/protocol/generated` 的读取与比对。

Run: `cd server && go test ./internal/protocol/ && grep -c 'app/web' internal/protocol/gen_contract_test.go`
Expected: 测试通过；grep 输出 0。

- [ ] **Step 3: gen-protocol 的 -out 必填**

去掉默认输出路径，缺 `-out` 时报错退出，保留 `-update-golden`。

Run: `cd server && go run ./cmd/gen-protocol 2>&1 | head -2; go run ./cmd/gen-protocol -out /tmp/gen && ls /tmp/gen`
Expected: 无参报错；带参生成三份文件。

- [ ] **Step 4: 客户端同步校验脚本**

`client/scripts/check-protocol-sync.mjs`：读取本地 `web/src/protocol/generated/*.ts`，用 `gh api repos/GOSpeak-org/GOSpeak-Server/contents/...` 拉 golden 逐字节比对，不一致退出 1 并提示服务端版本。

Run: `cd client && node scripts/check-protocol-sync.mjs`
Expected: 一致时退出 0；人为改动 generated 后退出 1。

- [ ] **Step 5: 两仓提交**

```bash
cd /Users/noelorin/Projects/GOSpeak/server && git add -A && git commit -m "refactor: decouple protocol generation from client repo" && git push
cd ../client && git add -A && git commit -m "feat: add protocol sync check" && git push
```

---

## Task 7: 重写 Dockerfile 与集成测试路径

**Files:**
- Modify: `server/Dockerfile`
- Modify: `server/test/globalSetup.ts`

**Interfaces:**
- Consumes: 客户端 Release 的 `web-dist.tar.gz`。
- Produces: 镜像不构建前端。

- [ ] **Step 1: Dockerfile 改为直链下载前端**

删除所有 `COPY app/web` / `packages/*` / `pnpm --filter @gospeak/web` 阶段，改为：

```dockerfile
ARG CLIENT_WEB_URL
ADD $CLIENT_WEB_URL /tmp/web-dist.tar.gz
RUN mkdir -p /app/static && tar -xzf /tmp/web-dist.tar.gz -C /app/static \
 && test -f /app/static/index.html
```
并把 `COPY app/server/go.mod` 改为 `COPY server/go.mod`。

Run: `cd server && docker build --build-arg CLIENT_WEB_URL=<某 web-dist.tar.gz URL> -t gospeak-test .`
Expected: 镜像内 `/app/static/index.html` 存在。

- [ ] **Step 2: 修集成测试路径**

`test/globalSetup.ts` 的 `join(repoRoot, "app/server")` → `join(repoRoot, "server")`。

Run: `cd server && pnpm test:server`
Expected: 集成测试通过。

- [ ] **Step 3: 提交**

```bash
cd /Users/noelorin/Projects/GOSpeak/server
git add Dockerfile test/globalSetup.ts && git commit -m "refactor: build container from client release artifacts" && git push
```

---

## Task 8: 客户端仓装配

**Files:**
- Create: `client/pnpm-workspace.yaml`、`client/.gitignore`、`client/package.json`
- Modify: `client/desktop/go.mod`、`client/web/tailwind.config.ts`、`client/web/scripts/capture-storybook-baseline.mjs`

**Interfaces:**
- Produces: `pnpm install && pnpm -r build` 在客户端仓可跑通。

- [ ] **Step 1: 写 workspace 与忽略规则**

`pnpm-workspace.yaml`：
```yaml
packages:
  - "web"
  - "packages/*"
onlyBuiltDependencies:
  - esbuild
  - lefthook
supportedArchitectures:
  os: [current, linux, darwin, win32]
  cpu: [current, x64, arm64]
  libc: [current, glibc, musl]
```

`.gitignore`：
```
node_modules/
**/dist/
**/dist-desktop/
**/dist-mobile/
.tanstack/
*.tsbuildinfo
web/src/routeTree.gen.ts
desktop/mygo.ts
desktop/gospeak-desktop
mobile/www/
mobile/build/
web/visual-baseline/*.png.diff.png
```

Expected: `git status --short` 为空。

- [ ] **Step 2: proxyrules 提根后的 go.mod 修正**

`client/desktop/go.mod`：`replace github.com/GOSpeak-org/GOSpeak/app/shared/proxyrules => ../proxyrules`（若 module 路径未改则保持 `gospeak/proxyrules => ../proxyrules`）。

Run: `cd client/desktop && go build ./... && go test ./...`
Expected: 通过（能从 `../proxyrules` 解析）。

- [ ] **Step 3: 修两处跨层相对路径**

`client/web/tailwind.config.ts`：`"../../packages/components/src/**/*.{ts,tsx}"` → `"../packages/components/src/**/*.{ts,tsx}"`

`client/web/scripts/capture-storybook-baseline.mjs`：`../../..` → `../..`

Run: `cd client/web && pnpm build && grep -c 'orientation=vertical' dist/assets/*.css`
Expected: 输出 > 0。

- [ ] **Step 4: 验证三条平级路径**

```bash
cd client
grep -n 'cp -R ../web/dist' desktop/mygo.config.ts
grep -n 'webDir' mobile/capacitor.config.ts
grep -n 'replace.*proxyrules' desktop/go.mod
```
Expected: `../web/dist`、`"../web/dist-mobile"`、`=> ../proxyrules` 三处正确。

- [ ] **Step 5: 全量验证并提交**

```bash
cd client
pnpm install && pnpm -r build
(cd web && pnpm test)
(cd desktop && go build ./...)
git add -A && git commit -m "refactor: configure client workspace after split" && git push
```
Expected: 全部通过。

---

## Task 9: 脚本与 CI 归属迁移

**Files:**
- Move（→ client）: 七个门禁脚本及测试
- Keep（→ server）: `check-version.mjs`（收窄）、`release-*.mjs`、`build-changelog.mjs`
- Modify: 两仓 `.github/workflows/*`

**Interfaces:**
- Consumes: Task 8 的客户端 workspace。
- Produces: 每仓 CI 独立。

- [ ] **Step 1: 迁移客户端脚本并改常量**

七个脚本复制到 `client/scripts/`，改路径常量：
- `check-desktop-commands.mjs`：`DESKTOP_DIR = "desktop"`、`TS_DIR = "packages/desktop/src/capabilities"`
- `check-ui-tokens.mjs` / `check-radius-tokens.mjs`：`SCAN_DIRS = ["web/src", "packages/components/src"]`
- `check-hook-deps.mjs`：`./web`、`./packages`
- `mobile-bind-proxyrules.mjs`：`MODULE_DIR = "proxyrules"`、`OUTPUT_DIR = "mobile/build/proxyrules"`
- `mobile-version.mjs`：`mobile/`、`web/package.json`

Run: `cd client && node --test scripts/*.test.mjs`
Expected: 全部通过。

- [ ] **Step 2: 服务端收窄 check-version.mjs**

删除 `checkDesktop` / `checkMobile` 分支与相关路径，只校验 `VERSION` 与 `server/`。

Run: `cd server && node --test scripts/check-version.test.mjs`
Expected: 通过（fixture 同步删减）。

- [ ] **Step 3: 两仓各建 ci.yml**

服务端：`changes → lint(go vet) → build-server → protocol-check → test-server → gate`，全部 `working-directory: server`。
客户端：`changes → lint(biome) → typecheck → test-web → test-packages → desktop-go → mobile-bind → gate`。

Run: `actionlint .github/workflows/`（两仓各跑）
Expected: 无错误。

- [ ] **Step 4: 提交**

```bash
cd /Users/noelorin/Projects/GOSpeak/server && git add -A && git commit -m "ci: scope server workflows" && git push
cd ../client && git add -A && git commit -m "ci: add client workflows" && git push
```

---

## Task 10: 仓名引用与文档同步

**Files:**
- Modify: `server/internal/service/system_update_service.go`
- Modify: `client/desktop/updater_release.go`、`client/web/src/components/modal/settingModal/tab_item/about.tsx`
- Modify: 三仓 `README.md` / `AGENTS.md`
- Modify: 壳 `README.md`

**Interfaces:**
- Produces: 所有 GitHub 引用指向正确仓名；壳 README 讲清「壳只用于归档」。

- [ ] **Step 1: 更新检测的仓库指向**

服务端 → `repos/GOSpeak-org/GOSpeak-Server`，tag 前缀 `gospeak-server-v`。
客户端 → `repos/GOSpeak-org/GOSpeak-Client`，tag 前缀按组件（`gospeak-desktop-v` / `gospeak-mobile-v` / `gospeak-web-v`）。

Run: `cd server && go test ./internal/service/ -run TestSystemUpdate`
Expected: PASS。

- [ ] **Step 2: 全局替换旧仓名引用**

```bash
grep -rn 'GOSpeak-org/GOSpeak[^-]' --include='*.md' --include='*.go' --include='*.ts' --include='*.tsx' --include='*.yml' /Users/noelorin/Projects/GOSpeak/{server,client,docs} 2>/dev/null | grep -v node_modules
```
逐条改为对应子仓名。

- [ ] **Step 3: 壳 README 重写**

内容要点：这是聚合壳；`git clone --recursive` 一次拿全部；**开发请直接 clone 对应子仓**；壳里改代码会落在 detached HEAD。

- [ ] **Step 4: 子仓 AGENTS/README 目录树重写**

服务端写 `server/ test/ deploy/ docs/ scripts/`；客户端写 `web/ packages/ desktop/ mobile/ proxyrules/`，并注明 proxyrules 已提到根。

Run: `grep -rn 'app/web\|app/desktop\|app/mobile\|app/shared' --include='*.md' /Users/noelorin/Projects/GOSpeak/{server,client,docs}`
Expected: 无命中。

- [ ] **Step 5: 提交**

```bash
# 各仓分别提交
```

---

## Task 11: 版本基线对齐

**Files:**
- Modify: `client/desktop/package.json`、`client/mobile/package.json`、`client/web/package.json`
- Modify: `server/VERSION`

**Interfaces:**
- Consumes: 现网版本（server 1.3.0-rc.1、desktop 1.3.0-rc.1、mobile 1.1.0-rc.2）。
- Produces: 客户端首个版本 ≥ 现网。

- [ ] **Step 1: 定客户端首个版本**

客户端统一 `1.4.0-rc.1`，写入 `desktop/package.json`、`mobile/package.json`、`web/package.json`。

Run: `cd client && node scripts/mobile-version.mjs --check`
Expected: 三处一致。

- [ ] **Step 2: 服务端版本保持**

`server/VERSION` 维持 `1.3.0-rc.1`，下次发版再 bump。

- [ ] **Step 3: 提交**

```bash
cd /Users/noelorin/Projects/GOSpeak/client && git add -A && git commit -m "chore: set client baseline version 1.4.0-rc.1" && git push
```

---

## Task 12: 全量验证与回滚预案

**Files:** 无（验证任务）

- [ ] **Step 1: 全新 clone 复现**

```bash
rm -rf /tmp/verify-gospeak
git clone --recursive https://github.com/GOSpeak-org/GOSpeak.git /tmp/verify-gospeak
cd /tmp/verify-gospeak
ls server client docs
git submodule status
```
Expected: 三个目录都有内容；`git submodule status` 三行无 `+/-` 前缀。

- [ ] **Step 2: 三仓门禁**

```bash
cd /tmp/verify-gospeak
(cd server && go build ./...)
(cd client && pnpm install && pnpm -r build && (cd desktop && go build ./...))
(cd docs && pnpm install && pnpm build)
```
Expected: 三条链全部通过。

- [ ] **Step 3: 相对路径回归**

```bash
cd /tmp/verify-gospeak/client
grep -n 'cp -R ../web/dist' desktop/mygo.config.ts
grep -n 'replace.*proxyrules' desktop/go.mod
test -d proxyrules && echo "proxyrules 在仓根 OK"
```
Expected: 三处均正确。

- [ ] **Step 4: 回滚预案**

```bash
cd /Users/noelorin/Projects/GOSpeak
# 壳回滚
git reset --hard pre-shell-split
# 子仓保留（只读归档）或删除（不可逆）
# 兜底：/tmp/gospeak-pre-shell-split.bundle
```
Expected: 壳回到拆分前状态。

- [ ] **Step 5: 更新计划索引**

在 `docs/superpowers/plans/README.md` 把本计划标为已交付，并移除或标注废止旧 submodule 计划。

---

