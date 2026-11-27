# 多平台仓库结构调整（Submodule 拆分）Implementation Plan

> 【状态】2026-11-27 核验：该方案已被「聚合壳 + 三子仓」取代废弃，仅供历史参考；实际采用方案见 `specs/2026-11-26-two-repo-split-design.md`。

> **状态：已定稿，执行中（当前唯一路线）。** 用户已明确选择仓库级拆分，本计划为**当前执行路线**。其前置子集 [Release 产物分离与独立发版](./2026-11-26-per-component-release.md) 已先在 `release` 落地（一组件一 Release、无 lockstep、组件守卫等），满足本计划 Task 12 的前置条件。
>
> **执行起点（2026-11-26 核查）**：工作分支 `feature/multiplatform-repo-restructure` 已就位；Task 0 前置改造**尚未开始**；四个远端仓库**尚未创建**；主仓库无 `.gitmodules` 残留。
>
> **已知代价（执行中持续承担，不再作为否决理由）**：CI 从并行 13 job 变成跨仓串行（一次前端改动需 2 轮 CI + 人工 bump gitlink，慢 2-3 倍）、复合 action 需复制 4 份、跨仓依赖需额外机制（proxyrules module、gen-protocol 跨仓写入、mobile 的 workspace 依赖）。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `app/web`、`app/desktop`、`app/mobile`、`packages/` 四个目录从 GOSpeak 主仓库拆成独立 Git 仓库并以 submodule 接入，保留各自完整历史；各端拥有独立 Release（产物不再挤在同一个 Release 条目里）、独立 tag、独立 CI 与发版节奏。前端不再嵌入二进制，改为随发布包分发的外部静态目录并支持热更新；更新检测改为多仓库模型。

**Architecture:** 在临时 clone 中用 Git 内置 `filter-branch --subdirectory-filter` 从主仓库历史抽取每个目录为独立仓库（子仓库根即原目录内容），推送到四个新远端仓库的 `release` 分支；主仓库 `git rm -r --cached` 四个目录后 `git submodule add` 回相同路径。`GOSpeak-Web` 与 `GOSpeak-Mobile` 内部各自嵌套挂载 `packages/` submodule（`GOSpeak-Packages`），使 `workspace:*` 与源码直连保持不变；`proxyrules` 独立成第 5 个仓 `GOSpeak-Proxyrules`，desktop 与 mobile 各挂 submodule（desktop 用 `replace` 指向本仓内副本，不依赖 proxy）。CI/发布 workflow 按仓库归属拆分，各子仓库独立 CI 与 Release；主仓库只负责 server、packages 的 module tag 与整体协调。版本声明改为分层 `metadata.yaml`，版本 bump 采用「子仓库改 metadata → 主仓库更新 metadata 与 gitlink」两阶段流程；`server`、`web`、`desktop`、`mobile` 各自独立 SemVer 与 tag，各自创建独立 GitHub Release。前端不再 `go:embed`：服务端只通过 `STATIC_DIR` / `/app/static` / `./static` 消费外部前端；桌面安装包内含外部前端目录。更新检测改为多仓库模型：`server` 查 GOSpeak 主仓库 `v*`，`web` 查 GOSpeak-Web `gospeak-web-v*`，`desktop` 查 GOSpeak-Desktop `gospeak-desktop-v*`，`mobile` 查 GOSpeak-Mobile `gospeak-mobile-v*`，各自独立缓存与降级。`serveSPA` 对 `index.html` 补 `Cache-Control: no-store`。

**Tech Stack:** Git 2.x（内置 `filter-branch`，无需额外工具）· GitHub · pnpm 10 · Turbo · Biome · Vitest 5 · Go 1.27 · Gin（静态托管）

**Specs:**
- `docs/superpowers/specs/2026-11-26-submodule-split-design.md`（本计划的配套设计文档）
- `docs/superpowers/specs/2026-11-26-submodule-release-design.md`（Task 9 创建，含 web 外部更新规则）
- `docs/superpowers/specs/2026-11-26-release-pipeline-design.md`（Task 12 创建，一组件一 Release）
- `docs/superpowers/specs/2026-11-26-update-check-design.md`（Task 11 创建，多仓更新检测）

## Global Constraints

- 工作目录：`/Users/noelorin/Projects/GOSpeak`；主仓库远端：`https://github.com/NoelOrin/GOSpeak.git`；执行分支为从 `origin/release` 签出的 `feature/multiplatform-repo-restructure`（已就位）。
- 分支策略：从最新 `origin/release` 签出工作分支；唯一常驻分支为 `release`。
- 子仓库地址（可替换，但一旦确定必须全程一致）：
  - `https://github.com/NoelOrin/GOSpeak-Web.git`      → 主仓库 `app/web`
  - `https://github.com/NoelOrin/GOSpeak-Desktop.git`  → 主仓库 `app/desktop`
  - `https://github.com/NoelOrin/GOSpeak-Mobile.git`   → 主仓库 `app/mobile`
  - `https://github.com/NoelOrin/GOSpeak-Packages.git` → 主仓库 `packages`
- 子仓库默认分支：`release`。
- 子仓库结构：子仓库根即原目录内容，不保留 `app/web` / `packages` 这层前缀。
- **挂载方案已定稿：β + β1 + β-a（共 6 个挂载点）**。用户要求「尽量少挂」，在 α(3)/β(5)/γ(7) 中选定 β；两个门禁缺口再分别选 **β1（版本校验下沉）** 与 **β-a（desktop 挂 packages 保门禁）**：

  | 仓 | 挂载了什么 | 为什么 |
  |---|---|---|
  | 主仓 | `app/web`、`packages` | web：`gen-protocol` 原地写生成物 + `gen_contract_test` 比对；packages：`check-ui-tokens` / `check-radius-tokens` / `check-hook-deps` 的扫描根 |
  | GOSpeak-Web | `packages` | web 真实依赖全部 4 个包（components 123 处 / sfu-client 31 / mobile-capabilities 24 / desktop 23） |
  | GOSpeak-Desktop | `proxyrules`、`packages` | proxyrules：Go `replace => ./proxyrules`；packages：`check-desktop-commands.mjs` 迁入本仓后需读 `packages/desktop/src/**` |
  | GOSpeak-Mobile | `proxyrules` | `gomobile bind` 必须拿到源码（`cwd=proxyrules`、输出到 `build/proxyrules`） |
  | GOSpeak-Packages | 无 | — |
  | GOSpeak-Proxyrules | 无 | — |

  **主仓不挂 `app/desktop` 与 `app/mobile`**（β 相对 γ 省掉的两个）。`check-version.mjs` 的跨仓比对改为**下沉各子仓 CI + 主仓 `gh api` 读公开仓比对**（β1）。
  **mobile 不挂 `packages`**：它没有 `src/`，`@gospeak/mobile-capabilities` 在 `.ts/tsx/js/java/kt/swift/gradle/plist` 全仓零命中，是残留声明——直接删依赖行即可。
- **`app/shared/proxyrules` 独立成第 5 个仓 `GOSpeak-Proxyrules`**（不再留主仓）。module 路径 = `github.com/NoelOrin/GOSpeak-Proxyrules`（仓库根即 module 根，直接打 `v0.1.0`，不用子目录 tag）。desktop 与 mobile 各挂一份 submodule（见上表）；主仓不挂（server 零引用，已实测确认）。
- 历史重写统一用 Git 内置 `filter-branch`（`--subdirectory-filter` 抽取目录、`--tree-filter` 移动目录），**不依赖 `git filter-repo` 或 `git subtree`**（两者本机均不可用）；只在临时 clone 中运行，**绝不**在主仓库工作区直接运行。
- 文件沙箱为 danger-full-access；审批已禁用，不要请求 sandbox 提权。
- 代码不加注释（复杂逻辑除外）、不用 emoji；提交遵循 Conventional Commits。
- 文档与代码同 PR；计划索引只保留仍在跟进的条目。

## Review Focus

- **当前未提交改动丢失**：`app/web` 内多个已修改文件在拆分前必须有显式落点（提交或 stash），且执行 `git rm -r --cached` 前再次核对 `git status`；Task 1 用状态快照与 diff 存档钉住。
- **子仓库历史缺失**：`filter-branch` 后子仓库 `git log` 必须包含原目录中至少一次真实提交（非仅初始空提交）；Task 3 用 log 抽样钉住。
- **submodule 路径漂移**：主仓库 `.gitmodules` 在 β 方案下只有**两个** path——`app/web`、`packages`；`GOSpeak-Web` 的 `.gitmodules` 中 packages path 必须是 `packages`；desktop 与 mobile 的 `.gitmodules` 中 proxyrules path 必须是 `proxyrules`；Task 5 / Task 6B / Task 6C 用 `git config -f .gitmodules --get-regexp path` 钉住。
- **`workspace:*` 解析失败**：`GOSpeak-Web` 独立 clone 后 `pnpm install --frozen-lockfile` 必须成功，证明嵌套的 `packages/` 子模块已就位；Task 6B / Task 8 钉住。
- **挂载点数漂移**：`.gitmodules` 的 path 总数必须精确——主仓 **2**（`app/web`、`packages`）、web **1**、desktop **2**（`proxyrules`、`packages`）、mobile **1**；Task 5 / 6B / 6C 各用 `git config -f .gitmodules --get-regexp 'submodule\..*\.path' | wc -l` 钉住。
- **跨仓依赖断裂**：desktop / mobile 必须能在各自仓内找到 `proxyrules` submodule 并可 `go build` / `gomobile bind`；mobile 必须能拿到 web 的 `dist-mobile` 目标产物；Task 0 / 6C / Task 8 钉住。
- **工具链/CI 因路径变化失效**：`pnpm typecheck`、`pnpm lint`、三目标构建、`pnpm test`、Go 构建必须全部通过；Task 8 逐条验证。
- **主仓库残留跟踪文件**：`git ls-files app/web packages` 在提交后只能返回**两个** gitlink（mode 160000）；`app/desktop` 与 `app/mobile` 应已从主仓删除且不再挂载。Task 5 钉住。
- **clone 后不可复现**：全新 clone + `git submodule update --init --recursive` 后目录内容完整（含嵌套层）；Task 8 钉住。
- **各端 Release 仍挤在一起**：若 `create-release` 仍固定用 server tag，则拆分未达成目标；Task 12 钉住「一组件一 Release」。
- **cross-repo 凭据缺失导致 CI 全线 403**：私有子仓 checkout 必须带 `SUBMODULES_TOKEN` 或 GitHub App token；Task 2 Step 4 与 Task 7 Step 1/4 钉住。
- **Docker 上下文缺 submodule**：`Dockerfile` 的 `COPY app/web` / `COPY packages/*` 要求本地已 checkout；改用远程 BuildKit 会静默失败；Task 10 Step 3 钉住。
- **lockfile 双源漂移**：子仓必须生成自己的 `pnpm-lock.yaml` 并携带 `supportedArchitectures`，不能复用主仓 lockfile；Task 6B Step 3/4 钉住。
- **跨仓门禁脚本被误迁**：`check-desktop-commands.mjs` 等跨读两个子仓，只能留在主仓库；Task 7 Step 5 钉住。
- **server 包静默缺前端**：web Release 缺失时必须失败而非产出坏包；Task 12 Step 4 钉住。
- **gitlink 长期落后**：子仓前进但主仓不 bump，CI 仍绿；Task 8 Step 9 的漂移检测钉住。

## 跨仓执行风险登记（Review 结论，2026-11-26）

> 本节由对现存仓库事实的核对得出（`grep secrets`、`Dockerfile`、`pnpm-workspace.yaml`、`scripts/*.mjs`、`ci.yml` job 拓扑）。**R1–R4 是阻塞级**：不解决就开工必然失败，不要跳到 Task 2。

### 阻塞级（开工前必须定方案）

**R1. 私有子仓 checkout 拿不到凭据。**
主仓库 CI 的 `GITHUB_TOKEN` 是**单仓作用域**，无法读取 `GOSpeak-Web` 等私有子仓库。现状 `grep -rhoE 'secrets\.[A-Z_]+' .github/` 只有 `GITHUB_TOKEN`，因此 `submodules: recursive` 在 CI 上必然 403。
解决（三选一，执行前定稿）：
- **fine-grained PAT**（推荐起步）：创建一个对四仓 `contents: read` 的 PAT，存为主仓库 secret `SUBMODULES_TOKEN`，checkout 写 `token: ${{ secrets.SUBMODULES_TOKEN }}`；
- **GitHub App 安装 token**：更规范、可自动轮换，但要额外维护 action；
- **deploy key**：每仓一把只读 key，配 `ssh-key`，维护 4 把。

**无论选哪个**：所有 workflow 的 checkout 都要带 `token:`，且 Task 7 Step 1 的「只加 `submodules: recursive`」不成立，必须同步加凭据。另需在本机开发侧验证：`git clone --recurse-submodules` 对私有仓是否需要已配置的凭据助手。

**R2. Docker 构建上下文里 submodule 的可用性。**
现 `Dockerfile` 第 8–19 行直接 `COPY app/web/package.json`、`COPY app/web`、`COPY packages/components` 等。拆仓后这些路径仍是 submodule 挂载点——**只要构建上下文里已 checkout 内容就能 COPY**，但：
- `docker build` 不认 submodule，CI 必须先 `submodules: recursive` 再 build；
- `.dockerignore` 现有规则未排除也不加密 `app/web`、`packages`，需确认没有 `.git` 之类的误排除；
- 若改用 `docker/build-push-action` 的 git context 或远程 BuildKit 构建，**submodule 不会被拉取**，必须改为本地上下文 + 预 checkout。
Task 10 Step 3 需补这条前置校验。

**R3. pnpm lockfile 双源。**
现根 `pnpm-lock.yaml` 单锁覆盖 `packages/*`、`app/*`、`test`、`plugins/*`，且启用 `supportedArchitectures`（多平台可选二进制）。拆仓后：
- `GOSpeak-Web` 需要自己的 lockfile，覆盖 `web` + 嵌套 `packages/*`，**必须重新生成并提交**，不能直接复用根 lockfile；
- `GOSpeak-Packages` 若要有独立 CI（`pnpm -r test`），同样需要独立 lockfile 或改用 no-lock 安装；
- `supportedArchitectures` 的注释（macOS 解析、linux CI 缺包）必须一起带到子仓，否则会踩同样的坑。
Task 6B 只写了「`pnpm install --frozen-lockfile` 可解析」，未说明 lockfile 从哪来 —— 必须补「首次 `pnpm install` 不带 `--frozen-lockfile` 生成后提交」。

**R4. 跨 submodule 的门禁脚本不能迁走。**
`scripts/check-desktop-commands.mjs` 同时读 `app/desktop`（GOSpeak-Desktop）与 `packages/desktop`（GOSpeak-Packages）；`scripts/mobile-bind-proxyrules.mjs` 需要 `app/shared/proxyrules`（主仓）+ `app/mobile`（GOSpeak-Mobile）；`scripts/mobile-version.mjs` 需要 `app/mobile` + `app/web/package.json`。
**这些校验只能在主仓库跑**（唯一同时挂载多个 submodule 的地方）。Task 7 Step 5 写「迁移到对应子仓库，主仓库删除」是错的，改为**保留在主仓库**，并在子仓 CI 中不重复实现。

### 高优先级（执行中必须补）

**R5. 主仓库 checkout 内容是「钉住的旧版」。** 子仓 `release` 前进后，主仓库 gitlink 不变即不回更新，主仓 CI 永远绿但代码落后。需补一个定时检测：每日 job 比对四个 gitlink 与对应子仓 `release` HEAD，落后则开 issue 或告警。

**R6. secrets 与签名。** 当前 workflow 只用 `GITHUB_TOKEN`，mobile 只有 debug APK、desktop 无代码签名。**一旦要正式签名**，keystore / 证书 / Apple 密钥必须**在每个子仓库分别配置 secret**（GitHub 私有仓 secret 不跨仓共享，除非建 Organization 级 secret）。若走 org 级 secret，需确认仓库是否在同一个 org 下。

**R7. 复合 action 复制与漂移。** 现只有两个：`.github/actions/pnpm-install`、`go-build-cache`。拆仓后需复制到每个用到的子仓（不是计划早先以为的 4 份 × 多 action）。更优做法：建一个 `GOSpeak-CI` 公共仓存 reusable workflow 与 action，各仓 `uses: NoelOrin/GOSpeak-CI/.github/actions/pnpm-install@v1`；私有仓 reusable workflow 需要显式权限配置，执行时评估。

**R8. 依赖机器人。** 现 `.github/dependabot.yml` 只覆盖主仓路径。拆仓后每个子仓需要自己的 `dependabot.yml`（npm / gomod / gradle / pub 各自），否则依赖更新静默停摆。

**R9. 发布故障域。** server 包要拉 `gospeak-web-v*` 的 Release 附件；若 web Release 不存在、被删或附件缺失，server 发版直接失败。Task 12 Step 4 必须定义降级：`static/` 缺失时明确失败并提示「先发 web」，或退化为不带前端（仅 API）并显式告警。**不要静默产出缺前端的 server 包。**

**R10. 分支保护与 required checks。** 四个子仓新建后默认无保护。需为每个仓的 `release` 分支配置：required status checks（各自 CI 的 gate）、禁止直推、合并后删分支。主仓库现有 gate 聚合语义不会自动延伸到子仓。

### 中优先级（记录，执行时按需处理）

**R11. Actions 成本。** 私有仓 Actions 分钟数按仓独立计。现有 13 job 并行 → 拆成 5 个仓的独立流水线，单次改动可能触发 2 轮（子仓 + 主仓），总分钟数上升。Android 迁 Linux（前置计划已做）是主要的成本对冲。

**R12. 工具链三件套要复制。** 每个子仓需要各自的 `lefthook.yml`、`biome.json`、`tsconfig` 基座、`turbo.json`（若用）；否则子仓没有本地门禁。

**R13. protocol 生成的半边门禁。** 主仓 `protocol-check` 只能校验 gitlink 未动，看不到子仓内生成物变化。需在 `GOSpeak-Web` CI 里增加「`generated/` 与协议源一致」的反向校验（web 仓没有协议源，实际做法：由主仓 CI 在生成后 push 到 web 仓或开 PR，或降级为仅主仓校验）。Task 7 Step 3 的「已知限制」需升级为**明确的补偿措施**。

**R14. `test/` 集成测试依赖。** `test/globalSetup.ts` 有 `internal/webui/dist` 相关排除。Task 10 去 embed 后需复核：集成测试是否依赖前端产物、是否需要 `STATIC_DIR` 指向子仓产物。

---

## 执行顺序与阻塞点

```
Task 0  前置改造（module 路径 / 公开依赖 / gen-protocol -out / 产物来源本地化）
   │     ↑ 必做；proxyrules 先独立成第 5 仓并打 v0.1.0
   ▼
Task 1  状态快照与决策确认（工作区干净则可直接进 Task 2）
   ▼
Task 2  建五仓（全 public）+ 分支保护（R10，推送后回填）；公开仓无需跨仓凭据
   ▼
Task 3  从临时 clone 抽取历史并推送五仓 release（含 proxyrules + module 路径改造 + v0.1.0）
   ▼
Task 4  主仓移除目录跟踪
   ▼
Task 5  加 2 个 submodule（app/web + packages）并锁定指针
   ▼
Task 6 / 6B / 6C  文档 + web 挂 packages + desktop 挂 proxyrules+packages + mobile 挂 proxyrules（R3 lockfile 与 β-a 门禁迁移在此完成）
   ▼
Task 7  CI 按仓拆分（★R1 token / ★R4 脚本留主仓 / R7 action / R8 dependabot / R12 工具链）
   ▼
Task 8  全量验证 + 全新 clone 复现 + gitlink 漂移检测（R5）+ test 依赖复核（R14）
   ▼
Task 9  metadata 版本管理（一致性三重校验）
   ▼
Task 10 去 embed（★R2 Docker 上下文）
   ▼
Task 11 多仓更新检测
   ▼
Task 12 发布链路（★R9 故障域降级）
   ▼
Task 13 rc 进入 latest
   ▼
Task 14 回滚预案（仅失败时）
```

**★ = 有阻塞级风险（R1–R4）或需显式降级策略（R9）的步骤，跳过会直接失败或静默产出坏包。**

**可以并行/后置**：Task 9 与 Task 10 之间无强依赖；Task 11 只依赖 Task 9 的命名规则；Task 13 依赖 Task 11/12。

---

## 文件结构

**新建**

| 文件 | 职责 |
|---|---|
| `docs/superpowers/specs/2026-11-26-submodule-split-design.md` | 设计文档（已创建） |
| `docs/superpowers/plans/2026-11-26-submodule-split.md` | 本计划（已存在，执行时只更新状态块） |
| `docs/superpowers/specs/2026-11-26-submodule-ci-design.md` | 子仓库 CI 策略说明（Task 7） |
| `docs/superpowers/specs/2026-11-26-submodule-release-design.md` | 版本与发布规则说明（Task 9） |
| `metadata.yaml`（主仓库 + 四个子仓库各一份） | 版本与元数据的分层单一事实源（Task 9）；含 `server`/`web`/`desktop`/`mobile` 四组件 |
| `scripts/metadata.mjs` + `.test.mjs` | metadata.yaml 解析与一致性校验（Task 9） |
| `.gitmodules`（主仓库） | 四个 submodule 的 path/url/branch 声明 |
| `.gitmodules`（GOSpeak-Web） | 嵌套 `packages` submodule 声明（Task 6B） |
| `.gitmodules`（GOSpeak-Mobile） | 嵌套 `proxyrules` submodule 声明（Task 6C） |
| `.gitmodules`（GOSpeak-Desktop） | 嵌套 `proxyrules` 与 `packages` submodule 声明（Task 6C Step 3b） |
| `pnpm-workspace.yaml`（GOSpeak-Packages） | 四包 workspace 声明（Task 6C） |
| `pnpm-workspace.yaml`（GOSpeak-Mobile） | 本仓 + `packages/*`（Task 6C） |

**修改**

| 文件 | 改动 |
|---|---|
| `docs/superpowers/plans/README.md` | 索引新增本计划条目 |
| `AGENTS.md` | 目录骨架说明三端为 submodule；单一事实源索引补一句「子仓库地址见 `.gitmodules`」 |
| `docs/superpowers/specs/2026-11-26-submodule-split-design.md` | 决策点确认后回填最终选择（如子仓库地址、未提交改动处理方式） |
| `.github/workflows/ci.yml` 等 7 个 workflow | checkout 加 submodules；路径过滤器适配 gitlink；protocol-check 调整（Task 7） |
| `scripts/check-version.mjs` + `.test.mjs` | 改读 metadata.yaml + submodule 守卫（Task 9） |
| `scripts/release-naming.mjs` + `.test.mjs` | 增加 mobile 与 web 组件（Task 9） |
| `scripts/build-changelog.mjs` / `release-notes.mjs` + 各自 test | 按组件从 metadata 生成（Task 9） |
| `.github/versions/*.yaml` | 删除，迁移到 metadata.yaml（Task 9） |
| `.github/workflows/version-bump.yml` / `release.yml` | 两阶段版本更新与 tag 事件适配（Task 9）；release.yml 增加 web 静态压缩包构建与发布；服务端与桌面产物改为「二进制 + 外部前端」打包，不再 embed |
| `scripts/AGENTS.md` | 版本脚本描述补 submodule 守卫（Task 9） |
| `app/server/internal/router/router.go` | `serveSPA` 为 `index.html` 响应补 `Cache-Control: no-store`（Task 9） |
| `Dockerfile` | 移除把前端产物 COPY 进 `internal/webui/dist` 的逻辑；final stage 增加 `/app/static` 并把前端产物 COPY 进去（Task 11） |
| `deploy/docker-compose.yml` / `deploy/docker-compose.standard.yml` | `STATIC_DIR=/app/static` 挂载点随发布包前端目录调整（Task 11） |
| `app/server/internal/webui/embed.go` / `stub.go` | 移除 `go:embed`，`webui` 包改为仅提供外部目录探测与 `HasAssets()` 兼容层（Task 11） |
| `app/desktop/mygo.config.ts` / `app/desktop/updater*.go` | 桌面宿主改为加载外部前端目录；updater 改查 GOSpeak-Desktop 仓库（Task 11/12） |
| `app/server/internal/service/system_update_service.go` + `_test.go` | 更新检测抽象为多仓库模型，新增 web 检测，desktop / mobile 各自查对应子仓库（Task 12） |
| `app/server/internal/handler/system_update_handler.go` + `app/server/internal/router/routes/system/routes.go` | 新增按组件查询更新端点（Task 12） |
| `app/web/src/api/system.ts` + `app/web/src/components/modal/settingModal/tab_item/about.tsx` | 前端新增 web 前端更新检测入口与展示（Task 11） |
| `app/shared/proxyrules/go.mod` | module 路径改为 `github.com/NoelOrin/GOSpeak-Proxyrules`（Task 0） |
| `app/desktop/go.mod` | `replace github.com/NoelOrin/GOSpeak-Proxyrules => ./proxyrules`（Task 0） |
| `app/desktop/*.go` | import 路径改为新 module 路径（Task 0） |
| `scripts/mobile-bind-proxyrules.mjs` | 迁入 mobile 仓，`MODULE_DIR`→`proxyrules`、`OUTPUT_DIR`→`build/proxyrules`、root 参数化（Task 6C） |
| `scripts/check-desktop-commands.mjs` + `.test.mjs` | 迁入 desktop 仓，`DESKTOP_DIR`→`.`（Task 6C Step 3b） |
| `scripts/check-version.mjs` | 拆分：主仓只校验 server + metadata；desktop/mobile 各仓自校验 + 主仓 `gh api` 交叉比对（β1） |
| `app/server/cmd/gen-protocol/main.go` | 生成目标路径参数化，跨仓写入改为输出到指定目录（Task 0） |
| `app/mobile/capacitor.config.ts` | `webDir` 改为本地相对路径（mobile 内自取产物，Task 0） |
| `app/desktop/mygo.config.ts` | 前端产物来源改为本地相对路径（Task 0） |
| `scripts/check-desktop-commands.mjs` / `check-mobile-bridge.mjs` / `mobile-version.mjs` | 路径按仓库归属拆分或参数化（Task 7） |
| `pnpm-workspace.yaml` / `turbo.json` / `lefthook.yml` / `biome.json` | 移除或适配被拆出目录的路径（Task 7） |

**删除（从主仓库 index 移除，文件由 submodule checkout 重建）**

| 路径 | 说明 |
|---|---|
| `app/web/**` | 由 `git rm -r --cached app/web` 移除跟踪，改由 submodule 提供 |
| `app/desktop/**` | 同上 |
| `app/mobile/**` | 同上 |
| `packages/**` | 同上（远端 `GOSpeak-Packages`） |
| `app/shared/proxyrules/**` | 同上（远端 `GOSpeak-Proxyrules`） |

---

## Task 0 开工序列（β 定稿后可直接执行）

> 本序列是 Task 0 的**可复制执行版**。每步都有「不做会怎样」的说明，按顺序执行，每步末尾有验证。
> 前置确认：工作分支 `feature/multiplatform-repo-restructure` 已就位；主仓库 public；五仓尚未创建。

### 0-A. 建立回滚锚点（先做，5 分钟）

```bash
cd /Users/noelorin/Projects/GOSpeak
git status --ignored --short > /tmp/gospeak-pre-split-status.txt
git bundle create /tmp/gospeak-pre-split.bundle --all
git tag -a pre-submodule-split -m "rollback anchor before submodule split"
git push origin pre-submodule-split
```
**为什么先做**：后续有 filter-branch 与 force-push 类操作，出错时这是唯一对照物（补丁 G10 / D3）。
Expected: bundle 文件存在且 > 1MB；tag 已推送。

### 0-B. proxyrules 独立仓先行（Task 2 局部 + Task 3 Step 6b）

**顺序很关键**：desktop 与 mobile 的 `replace` 依赖 proxyrules 仓存在且 `v0.1.0` 已打。

```bash
gh repo create NoelOrin/GOSpeak-Proxyrules --public --confirm
# 抽取（Task 3 Step 6b 原命令）
```
Expected: 新仓 `go build ./... && go test ./...` 通过；`v0.1.0` tag 已推。

### 0-C. 改 module 路径与 import（Task 0 Step 1）

```bash
# 主仓内先改通，再拆
sed -i '' 's|^module gospeak/proxyrules$|module github.com/NoelOrin/GOSpeak-Proxyrules|' app/shared/proxyrules/go.mod
# desktop 三处 import + go.mod
```
同步改：`app/desktop/proxyserver.go`、`server_config.go`、`proxyserver_security_test.go` 的 import；`app/desktop/go.mod` 的 require + replace。
Expected: `cd app/shared/proxyrules && go test ./...` 通过；`cd app/desktop && go build ./...` 通过（此时仍用本地 replace）。

### 0-D. gen-protocol 加 `-out`（Task 0 Step 3）

```bash
# main.go 增加 -out 参数，默认仍是 ../web/src/protocol/generated
cd app/server && go test ./internal/protocol/... && go run ./cmd/gen-protocol -out /tmp/gen-test
```
Expected: 能写到指定目录；golden 比对仍通过。

### 0-E. 前端产物来源本地化（Task 0 Step 4）

- `app/mobile/capacitor.config.ts` 的 `webDir` → `www`
- `app/desktop/mygo.config.ts` 的 `buildCommand` → 消费本地 `webui/dist`
- `app/web/vite.config.ts` 让 desktop 走 `dist-desktop/`（补丁 F1）

Expected: 三目标构建分别产出 `dist/`、`dist-desktop/`、`dist-mobile/`；断言互不覆盖。

### 0-F. 提交前置改造

```bash
git add app/shared app/desktop app/mobile app/server/cmd/gen-protocol app/web/vite.config.ts
git commit -m "refactor: make cross-repo dependencies resolvable before submodule split"
```

### 进入 Task 1 的前置条件（逐条打勾）

- [ ] `pre-submodule-split` tag 已推送
- [ ] GOSpeak-Proxyrules 仓已建，`v0.1.0` 已推，`go test` 通过
- [ ] `app/desktop` 能在新 module 路径下构建
- [ ] `gen-protocol -out` 可用
- [ ] 三个 target 产物目录互不覆盖
- [ ] 主仓工作区干净（除计划文档）

---

## Task 0: 前置依赖改造（拆仓前必须先做）

**Files:**
- Modify: `app/shared/proxyrules/go.mod`（module 路径）
- Modify: `app/desktop/go.mod` + `app/desktop/*.go`（依赖方式与 import）
- Modify: `scripts/mobile-bind-proxyrules.mjs`（路径适配）
- Modify: `app/server/cmd/gen-protocol/main.go`（输出路径参数化）
- Modify: `app/mobile/capacitor.config.ts` / `app/mobile/package.json`（前端产物来源）
- Modify: `app/desktop/mygo.config.ts`（前端产物来源）
- Modify: `AGENTS.md`（记录 proxyrules module 新路径）

**Interfaces:**
- Consumes: 当前同仓结构。
- Produces: 跨仓依赖在拆分前已可独立成立；供 Task 3/5/6b 直接使用。

**为什么先做：** 这三项是拆仓后的硬断点，先在同仓内改成「可跨仓」形态并验证通过，拆仓才不会连锁失败。

- [ ] **Step 1: 改 proxyrules module 路径**

把 `app/shared/proxyrules/go.mod` 的 `module gospeak/proxyrules` 改为 `module github.com/NoelOrin/GOSpeak/app/shared/proxyrules`。同步改 `app/desktop` 三处 import（`proxyserver.go`、`server_config.go`、`proxyserver_security_test.go`）与 `scripts/mobile-bind-proxyrules.mjs` 的 module 引用。

Run:
```bash
cd app/shared/proxyrules && go build ./... && go test ./...
cd ../../desktop && go build ./... && go test ./...
node --test scripts/mobile-bind-proxyrules.test.mjs
```
Expected: 全部通过。

- [ ] **Step 2: desktop 改用公开 module 依赖**

先在主仓库为 proxyrules 打 module 子目录 tag（Go 子模块版本规范）：
```bash
git tag app/shared/proxyrules/v0.1.0
git push origin app/shared/proxyrules/v0.1.0
```
然后把 `app/desktop/go.mod` 的 `require gospeak/proxyrules v0.0.0` 改为 `require github.com/NoelOrin/GOSpeak/app/shared/proxyrules v0.1.0`，删除 `replace` 行，执行 `go mod tidy`。

**关于下载体积（已实测确认）**：Go 的 module proxy 对子目录 module **只打包该子目录**，不会拉整个仓库。以 `golang.org/x/tools/gopls`（x/tools 的子目录 module）为例，其 zip 为 2.4MB / 1204 文件，内容全部位于 `gopls@v<ver>/` 下，不含 x/tools 的其他目录。因此 `proxyrules`（4 个文件）的下载量可忽略。

Run: `cd app/desktop && go mod tidy && go build ./...`
Expected: 能从 `proxy.golang.org` 解析到该 module；构建通过。

**已验证前提**：主仓库 `NoelOrin/GOSpeak` 是 **public**（`gh repo view` 确认 `visibility=PUBLIC`），因此公开 proxy 拉取成立；这也意味着 `app/shared/proxyrules` 的源码是公开的——若不可接受，需改为 private 主仓 + `GOPRIVATE` + 子仓凭据（会牵出额外配置，执行前确认）。

- [ ] **Step 2b: 确认 GOTOOLCHAIN 兼容**

`proxyrules` 声明 `go 1.26.0`，desktop 声明 `go 1.27.4`，本机默认可能是更低版本。验证自动工具链切换生效：

```bash
go env GOTOOLCHAIN
cd app/desktop && go version
```
Expected: `GOTOOLCHAIN` 为 `auto`（或 `local+auto`）；`go version` 在 desktop 目录下报告 ≥ 1.27。若不是，需在 CI 与本地显式设置 `GOTOOLCHAIN=auto` 并在设计文档记录。

- [ ] **Step 3: gen-protocol 输出路径参数化**

给 `app/server/cmd/gen-protocol` 增加 `-out` 参数（默认保持当前 `../web/src/protocol/generated`），使跨仓 CI 可指定输出目录；契约测试 `protocol/gen_contract_test.go` 相应读取该参数或保持 golden 比对。

Run: `cd app/server && go test ./internal/protocol/...`
Expected: 通过；`go run ./cmd/gen-protocol -out /tmp/gen-test` 能写到指定目录。

- [ ] **Step 4: 前端产物来源本地化**

把 `app/mobile/capacitor.config.ts` 的 `webDir: "../web/dist-mobile"` 改为 `"www"`（mobile 仓库内目录），`package.json` 的 `sync` 脚本改为消费外部传入的产物（拆仓后由 CI 把 web 产物放进 `www/`）。同理 `app/desktop/mygo.config.ts` 的 `buildCommand` 改为消费本地 `webui/dist`（由 CI 预置），不再 `cp ../web/dist`。

Run: `pnpm --filter @gospeak/web build:mobile && pnpm --filter @gospeak/mobile sync`
Expected: 通过（此时仍在同仓，用脚本先拷产物到本地目录再 sync）。

- [ ] **Step 5: 提交前置改造**

```bash
git add app/shared app/desktop app/mobile app/server/cmd/gen-protocol scripts/mobile-bind-proxyrules.mjs AGENTS.md
git commit -m "refactor: make cross-repo dependencies resolvable before submodule split"
```
Expected: 提交成功。

---

## Task 1: 执行前状态快照与决策点确认

**Files:**
- Modify: `docs/superpowers/specs/2026-11-26-submodule-split-design.md`（回填决策点）
- Test: 无（本任务产出状态快照文件与确认结果）

**Interfaces:**
- Consumes: 当前工作区状态。
- Produces: 决策点结论（子仓库地址、未提交改动处理方式、子仓库结构）；供 Task 2/3/4 使用。

- [ ] **Step 1: 记录当前工作区状态**

Run:
```bash
cd /Users/noelorin/Projects/GOSpeak
git status --short > /tmp/gospeak-submodule-pre-status.txt
git diff --stat >> /tmp/gospeak-submodule-pre-status.txt
```
Expected: 输出到文件且包含 `app/web` 的修改条目。

- [ ] **Step 2: 确认决策点（三项已有定论，只剩第 2 项待按实况处理）**

| 决策点 | 结论 |
|---|---|
| 子仓库地址 | 采用默认值 `NoelOrin/GOSpeak-{Web,Desktop,Mobile,Packages}`（见 Global Constraints） |
| 子仓库结构 | 根即原目录内容，不保留 `app/web` / `packages` 前缀 |
| 未提交改动处理 | 执行时若工作区干净则直接进入 Task 2（无需推送）；若有未提交改动，先在本分支提交，再从该提交抽取，**不要 push 到 `release`**（避免污染常驻分支） |

把上述结论回填到 `docs/superpowers/specs/2026-11-26-submodule-split-design.md` 的「决策点」章节，删除未采纳的候选。

- [ ] **Step 3: 按决策处理未提交改动并签出工作分支**

> **前置校验**：若 `git status --short` 只有计划文档修改，建议先提交这些文档（`docs: ...`）并**不要**执行方式 A 的推送步骤——方式 A 仅在确实需要把这些改动写进子仓库历史时使用。

**方式 A（改动进入子仓库历史，需推送到 `release`，须授权）**：把未提交改动先提交到 `origin/release` 的本地跟踪分支，再从该分支签出工作分支：
```bash
git fetch origin
git switch -C release-tmp origin/release
git add -A
git commit -m "chore: snapshot worktree before submodule split"
git push -u origin release-tmp:release   # 需授权推送到共享分支；若不允许则停下询问
git switch -c feature/multiplatform-repo-restructure release-tmp
```
Expected: snapshot 提交位于 `release-tmp`，工作分支从它签出；后续 Task 3 抽取时会包含该提交。

**方式 B（改动不进子仓库历史）**：当前分支提交或 stash，再从 `origin/release` 签出：
```bash
git add -A
git commit -m "chore: snapshot worktree before submodule split"
git fetch origin
git switch -c feature/multiplatform-repo-restructure origin/release
```
Expected: snapshot 提交留在原分支（如 `dev-1124`），子仓库从干净的 `origin/release` 抽取，**不含这些改动**；执行者须明确接受这一后果。

Ruling: 默认采用方式 A 以保证 `app/web` 改动进入子仓库历史；若你不授权推送到 `release`，退化为方式 B 并在设计文档标注。代价：方式 B 会丢失子仓库中的这些改动历史。

---

## Task 2: 创建五个远端空仓库（全部 public）

**Files:** 无本地文件（GitHub 远端操作）

**Interfaces:**
- Consumes: Task 1 确认的子仓库地址。
- Produces: 四个可推送的空仓库；供 Task 3 推送。

- [ ] **Step 1: 创建空仓库**

在 GitHub 上创建四个仓库（用 `gh` CLI 或网页），均不初始化 README/LICENSE/.gitignore：
```bash
gh repo create NoelOrin/GOSpeak-Web       --public --confirm
gh repo create NoelOrin/GOSpeak-Desktop   --public --confirm
gh repo create NoelOrin/GOSpeak-Mobile    --public --confirm
gh repo create NoelOrin/GOSpeak-Packages  --public --confirm
gh repo create NoelOrin/GOSpeak-Proxyrules --public --confirm
```
Expected: **五个** `gh repo view` 均成功；`gh repo view <name> --json visibility` 全为 `PUBLIC`。

**子仓一律 public**（已决策）。连带收益：submodule checkout **不需要任何 token**（R1 消失）、子仓嵌套 checkout 无需凭据（CI5 消失）、fork PR 门禁不受影响（CI3 消失）、公开仓 Actions 分钟数免费（R11 缓解）。
**连带约束**：proxyrules 的反代规则、packages 的组件源码将公开可见——这是接受该决策的前提。

- [ ] **Step 2: 验证远端可写**

```bash
for r in Web Desktop Mobile Packages; do git ls-remote "https://github.com/NoelOrin/GOSpeak-$r.git"; done
```
Expected: 四条命令均成功（空仓库无输出或仅 HEAD）。

- [ ] **Step 3: 配置分支保护与 required checks（风险 R10）**

四个子仓库建好后立即为 `release` 分支设保护，避免后续推 tag/合并时被误直推破坏：

分支保护必须用 JSON body（`gh api -f` 的扁平键写不出嵌套结构）：

```bash
for r in Web Desktop Mobile Packages Proxyrules; do
  gh api -X PUT "repos/NoelOrin/GOSpeak-$r/branches/release/protection" \
    --input - <<'JSON'
{"required_status_checks":null,"enforce_admins":false,"required_pull_request_reviews":null,"restrictions":null,"allow_force_pushes":false,"allow_deletions":false}
JSON
done
```

（先只设「禁止强推 / 禁止删除」，required status checks 待 Task 7 子仓 CI 落地后回填。**注意 check 名取的是 job 的 `name`，不是 job id**——主仓 `gate` 的 name 是 `CI Gate`；子仓要么给 gate job 不写 `name:`（则 id 即 check 名），要么写全名，否则 required check 会永远停在 Expected。）
**注意**：首次推送 `release` 分支前必须先建分支（保护不能作用于不存在的分支），所以本 Step 放在 Task 3 推送完成后再回填 required checks。此时只需确认 `enforce_admins` 与 required checks 的占位；各子仓 CI 的 gate job 名称待 Task 7 Step 6 落地后再补进保护规则。

Expected: 四仓 `release` 分支保护已开启；`gh api repos/NoelOrin/GOSpeak-Web/branches/release/protection -q .required_status_checks` 有输出。

- [ ] **Step 4: 确认「无需跨仓凭据」并登记签名类 secret 的归属（R1 / R6 已因 public 消除）**

子仓全部 **public** 后：

```bash
# 公开仓的 submodule checkout 不需要 token，验证一下
git ls-remote https://github.com/NoelOrin/GOSpeak-Web.git >/dev/null && echo "匿名可读 OK"
```

- **不需要** `SUBMODULES_TOKEN`、不需要 fine-grained PAT、不需要 GitHub App token；
- **不需要** `persist-credentials: false` 之类的规避（CI3 的 403 风险不存在）；
- fork PR 也能正常 `submodules: recursive`（CI3 的门禁问题不存在）；
- `go mod download` 若走 proxy 也无需 `GOPRIVATE`（不过 β 方案下 desktop/mobile 都用 submodule + replace，本就不走 proxy）。

**仍需登记**：未来 desktop 代码签名证书、mobile keystore **不是 submodule 凭据**，而是发布签名材料——GitHub secret 不跨仓共享，必须各自子仓配置或下沉组织级（需确认五仓同属一个 org）。

Expected: 匿名 `git ls-remote` 成功；设计文档记录「公开仓无需跨仓凭据」与「签名材料按仓配置」。

---

## Task 3: 抽取五个目录历史到子仓库

**Files:** 临时目录 `/tmp/gospeak-submodule-extract/{web,desktop,mobile,packages,proxyrules}`（由 filter-branch 创建）

**Interfaces:**
- Consumes: Task 1 的工作分支基准；Task 2 的五个远端。
- Produces: 五个远端 `release` 分支，各自包含对应目录历史；供 Task 4 接入。

> **proxyrules 是新增的第 5 个抽取对象**（β 方案定稿）。抽取 `app/shared/proxyrules` → `GOSpeak-Proxyrules`，其仓库根即 Go module 根，抽取后需把 `go.mod` 的 module 路径改为 `github.com/NoelOrin/GOSpeak-Proxyrules` 并作为首提交推送。

- [ ] **Step 1: 确认使用 Git 内置的 filter-branch（不装额外工具）**

**不依赖 `git filter-repo`**（本机实测未安装），也不依赖 `git subtree`（实测无该命令）。统一使用 Git 自带的 `git filter-branch --subdirectory-filter`，已实测可用（Git 2.50.1）：

```bash
git --version          # 需 >= 2.x，Apple Git 2.50.1 已验证
git filter-branch -h   # 确认存在
```
Expected: 输出 `git version 2.50.1` 与 filter-branch 用法提示。

**注意事项：**
- filter-branch 会打印 gotchas 警告，用 `FILTER_BRANCH_SQUELCH_WARNING=1` 抑制；
- **必须在临时 clone 中运行**，绝不在主仓库工作区执行；
- 它重写历史，运行后原 remote 引用会被改写，用 `-- --all` 覆盖所有 ref。

- [ ] **Step 2: 抽取 app/web**

```bash
rm -rf /tmp/gospeak-submodule-extract/web
git clone --no-local --branch feature/multiplatform-repo-restructure /Users/noelorin/Projects/GOSpeak /tmp/gospeak-submodule-extract/web
cd /tmp/gospeak-submodule-extract/web
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --subdirectory-filter app/web -- --all
git branch -m release
git remote set-url origin https://github.com/NoelOrin/GOSpeak-Web.git
git push -u origin release
```
Expected: 推送成功；子仓库根出现 `src/`、`package.json` 等原 `app/web` 内容。

- [ ] **Step 3: 验证 app/web 历史**

```bash
cd /tmp/gospeak-submodule-extract/web
git log --oneline -5
git ls-tree -d HEAD --name-only
```
Expected: log 中至少有 3 条真实历史提交（非唯一初始提交）；根目录含 `src` 目录。

- [ ] **Step 4: 抽取 app/desktop**

对 `app/desktop` 重复 Step 2/3，远端 `GOSpeak-Desktop.git`，临时目录 `/tmp/gospeak-submodule-extract/desktop`。

Expected: 子仓库根含原 `app/desktop` 的 `*.go`、`README.md` 等；历史非空。

- [ ] **Step 5: 抽取 app/mobile**

对 `app/mobile` 重复 Step 2/3，远端 `GOSpeak-Mobile.git`，临时目录 `/tmp/gospeak-submodule-extract/mobile`。

Expected: 子仓库根含 `package.json`、`capacitor.config.ts`、`ios/`、`android/`；历史非空。

- [ ] **Step 6: 抽取 packages**

```bash
rm -rf /tmp/gospeak-submodule-extract/packages
git clone --no-local --branch feature/multiplatform-repo-restructure /Users/noelorin/Projects/GOSpeak /tmp/gospeak-submodule-extract/packages
cd /tmp/gospeak-submodule-extract/packages
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --subdirectory-filter packages -- --all
git branch -m release
git remote set-url origin https://github.com/NoelOrin/GOSpeak-Packages.git
git push -u origin release
```

Expected: 子仓库根出现 `components/`、`sfu-client/`、`desktop/`、`mobile-capabilities/`；历史非空。

- [ ] **Step 6b: 抽取 app/shared/proxyrules**

```bash
rm -rf /tmp/gospeak-submodule-extract/proxyrules
git clone --no-local --branch feature/multiplatform-repo-restructure /Users/noelorin/Projects/GOSpeak /tmp/gospeak-submodule-extract/proxyrules
cd /tmp/gospeak-submodule-extract/proxyrules
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --subdirectory-filter app/shared/proxyrules HEAD
git branch -m release
# 仓库根即 module 根：改 module 路径
sed -i '' 's|^module gospeak/proxyrules$|module github.com/NoelOrin/GOSpeak-Proxyrules|' go.mod
git commit -am "refactor: set module path to GOSpeak-Proxyrules"
git tag v0.1.0
git remote set-url origin https://github.com/NoelOrin/GOSpeak-Proxyrules.git
git push origin release --no-tags
git push origin v0.1.0
```

Expected: 子仓根含 `go.mod` / `go.sum` / `proxyrules.go` / `proxyrules_test.go`；`go build ./... && go test ./...` 通过（**必须在此确认**，因为之后 desktop/mobile 都依赖它）；`v0.1.0` 已推送。

- [ ] **Step 7: 验证 packages 历史**

```bash
cd /tmp/gospeak-submodule-extract/packages
git log --oneline -5
git ls-tree -d HEAD --name-only
```
Expected: 至少 3 条真实历史提交；根目录含四个包目录。

- [ ] **Step 8: 清理临时 clone**

```bash
rm -rf /tmp/gospeak-submodule-extract
```
Expected: 目录不存在。

---

## Task 4: 处理主仓库工作区并移除目录跟踪

**Files:**
- Modify: `app/web/**`, `app/desktop/**`, `app/mobile/**`, `packages/**`, `app/shared/proxyrules/**`（从 index 移除）

**Interfaces:**
- Consumes: Task 1 的干净工作区与分支；Task 3 的五个子仓库已就绪。
- Produces: 主仓库 index 中五个路径无普通文件；供 Task 5 添加 submodule（β 方案只挂回 2 个）。

- [ ] **Step 1: 再次核对工作区干净**

```bash
cd /Users/noelorin/Projects/GOSpeak
git status --short
```
Expected: 输出为空或仅含计划/设计文档修改。

- [ ] **Step 2: 从 index 移除五个目录（保留本地文件用于比对）**

```bash
git rm -r --cached app/web app/desktop app/mobile packages app/shared/proxyrules
```
Expected: 命令成功；`git status --short` 显示大量 `D  app/web/...` 等删除条目。

- [ ] **Step 3: 删除本地目录内容**

```bash
rm -rf app/web app/desktop app/mobile packages app/shared/proxyrules
rmdir app/shared 2>/dev/null || true   # proxyrules 迁出后 app/shared 为空
```
Expected: 五个目录不存在。

- [ ] **Step 4: 验证 index 无普通文件残留**

```bash
git ls-files app/web app/desktop app/mobile packages app/shared | head -20
```
Expected: 输出为空。

---

## Task 5: 添加两个 submodule 并锁定指针（β 方案）

> **β 方案下主仓只挂 2 个**：`app/web`（供 `gen-protocol` 原地写 + `gen_contract_test`）与 `packages`（供 `check-ui-tokens` / `check-radius-tokens` / `check-hook-deps` 扫描）。
> `app/desktop` 与 `app/mobile` **不挂**——它们只作为独立仓存在，由各自的 CI 与开发者在主仓之外 checkout。
> `app/shared/proxyrules` **不挂**——server 零引用，desktop/mobile 各挂自己的副本。

**Files:**
- Create: `.gitmodules`
- Modify: gitlink `app/web`、`packages`（mode 160000）

**Interfaces:**
- Consumes: Task 3 的子仓库；Task 4 的空路径。
- Produces: `.gitmodules` 与**两个** gitlink；供 Task 6 提交。

- [ ] **Step 1: 添加 app/web submodule**

```bash
cd /Users/noelorin/Projects/GOSpeak
git submodule add -b release https://github.com/NoelOrin/GOSpeak-Web.git app/web
```
Expected: 命令成功；`app/web` 目录重新出现且**根目录**即 `src/`、`package.json`、`vite.config.ts`（无 `web/` 子层）。

- [ ] **Step 1b: 添加 packages submodule**

```bash
git submodule add -b release https://github.com/NoelOrin/GOSpeak-Packages.git packages
```
Expected: `packages` 目录重新出现且含四个包目录（components / desktop / mobile-capabilities / sfu-client）。

- [ ] **Step 2: 验证 .gitmodules 只有两条 path**

```bash
git config -f .gitmodules --get-regexp 'submodule\..*\.path'
git config -f .gitmodules --get-regexp 'submodule\..*\.url'
```
Expected: **两条** path 精确为 `app/web`、`packages`；**不得**出现 `app/desktop`、`app/mobile`、`app/shared/proxyrules`。
```bash
git config -f .gitmodules --get-regexp 'submodule\..*\.path' | wc -l   # 必须为 2
```

- [ ] **Step 3: 验证 gitlink 模式**

```bash
git ls-files -s app/web packages
```
Expected: **两行** mode 均为 `160000`。

- [ ] **Step 4: 提交 submodule 接入**

```bash
git add .gitmodules app/web packages
git commit -m "refactor: split web desktop mobile packages and proxyrules into separate repos"
```
Expected: 提交成功；`git status --short` 干净。

---

## Task 6: 文档与索引更新

**Files:**
- Modify: `docs/superpowers/plans/README.md`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 5 已提交的 submodule 结构。
- Produces: 文档反映三端为 submodule；供 Task 7 验证。

- [ ] **Step 1: 更新 plans/README.md**

在「进行中 / 待跟进」表格中新增一行：
```
| [多平台仓库结构调整（Submodule 拆分）](./2026-11-26-submodule-split.md) | 执行中 | 四仓 submodule 接入与全量门禁验证 |
```
Expected: 表格新增该行。

- [ ] **Step 2: 更新 AGENTS.md**

在「目录骨架」代码块中把 `app/web` / `app/desktop` / `app/mobile` / `packages` 注释加上「（git submodule）」；在「单一事实源索引」表新增一行，指向 `.gitmodules` 作为子仓库地址事实源。

**同时必须重写的段落**（只加注释不够，会留下错误命令）：
- §6 验证与门禁：`pnpm typecheck` / `pnpm lint` / `pnpm test` 在主仓已不覆盖三端，需改为「主仓跑 server/scripts，前端与 packages 在各自仓跑」，并给出跨仓验证顺序；
- §4.4 桌面条件编译边界：`pnpm --filter @gospeak/web build:desktop` 等命令在子仓内执行，路径需注明；
- CI 描述节：13 job 并行 → 五仓独立流水线。

`CLAUDE.md`（速查版）与 `ARCHITECTURE.md`（架构图）中的目录树与命令同步更新，否则新人按旧命令执行会全部失败。

Expected: `grep -n "submodule" AGENTS.md` 至少两处命中；`grep -n "pnpm typecheck" AGENTS.md` 附近有「已拆仓，需在子仓执行」的说明。

- [ ] **Step 3: 提交文档**

```bash
git add docs/superpowers/plans/README.md AGENTS.md
git commit -m "docs: update submodule layout and plan index"
```
Expected: 提交成功。

---

---

## Task 6B: GOSpeak-Web 嵌套挂载 packages

**Files:**
- Modify: `GOSpeak-Web` 仓库：新增 `.gitmodules`、`pnpm-workspace.yaml`、`package.json`
- Modify: `GOSpeak-Web` 的 `web/package.json`（`workspace:*` 保持不变，仅确认可解析）

**Interfaces:**
- Consumes: Task 3 的 `GOSpeak-Packages` 远端；Task 5 的主仓库 submodule 结构。
- Produces: `GOSpeak-Web` 独立 clone 后能 `pnpm install` 并构建；供 Task 7/8 验证。

**为什么需要：** 拆仓后 web 与 mobile 都必须能解析 `workspace:*`。**实测消费矩阵**（`grep -rn 'workspace:' app/*/package.json`）：

| 仓 | 依赖的 packages 包 | 实际情况 |
|---|---|---|
| `app/web` | components、desktop、mobile-capabilities、sfu-client | **真实使用**（components 123 处 / sfu-client 31 / mobile-capabilities 24 / desktop 23） |
| `app/mobile` | mobile-capabilities | **仅在 package.json 声明，全仓零使用** |
| `app/desktop`（Go 宿主） | 无 | 只有 `mygo-cli` |

**关于 mobile 的关键事实**：`app/mobile` **没有 JS/TS 源码目录**（无 `src/`，只有 `android/`、`ios/`、`capacitor.config.ts`、`assets/`）。`@gospeak/mobile-capabilities` 在 `package.json:30` 声明为 `workspace:*`，但在 `*.ts/tsx/js/java/kt/swift/gradle/plist` 全仓搜索**零命中**——它是一条残留声明，不是真实依赖。这意味着 **mobile 仓可以不挂 packages，直接删掉该依赖行即可**。

命名陷阱：`@gospeak/desktop` 是 **web 用的 TS 能力 SDK**，不是 desktop Go 仓的依赖——别因为名字就往 desktop 仓里挂 packages。

**四个有效包互不依赖**，各自 `tsc` 构建、`main` 直指 `./src/index.ts`、tsconfig **无 `extends`**（自包含），因此 `GOSpeak-Packages` 仓不需要从主仓复制 tsconfig 基座。

- [ ] **Step 1: 保持「根即 app/web 内容」布局（不要引入 `web/` 前缀）**

**早期草案曾用 `filter-branch --tree-filter` 把所有文件移入 `web/`，已废弃**：那会让 web 源码实际位于 `app/web/web/src`，与 Global Constraints「子仓根即原目录内容」矛盾，并连带打断 Dockerfile、gen-protocol、gen_contract_test、mygo.config.ts 等至少 6 处硬路径。

**定稿方案（与 mobile 仓同构）**：GOSpeak-Web 仓根 = 原 `app/web` 内容，`packages/` 作为子仓根下的一层 submodule。这样主仓挂载后 `app/web/src`、`app/web/dist`、`../web/src/protocol/generated`、`COPY app/web` 全部**保持不变**。

因此本 Step 只需确认 Task 3 的抽取结果即可，**不重写历史**：

```bash
git clone https://github.com/NoelOrin/GOSpeak-Web.git /tmp/gospeak-web-setup
cd /tmp/gospeak-web-setup
ls   # 应直接看到 src/ package.json vite.config.ts tailwind.config.ts ...
```

Expected: 仓库根直接是 web 源码（**无** `web/` 子目录）；`test ! -d web` 成立。

**唯一需要改的相对路径**：`tailwind.config.ts` 的 content glob。现状写 `../../packages/components/src/**`（从 `app/web` 上溯两级到主仓根）；新布局下 web 仓根下直接就是 `packages/`，应改为 `./packages/components/src/**`，否则共享组件的独有变体类会静默缺失（实测 `dist/assets/*.css` 里 `orientation=vertical` 出现 5 次，全部来自 `packages/components/src/components/slider.tsx`）。

- [ ] **Step 2: 添加 packages 嵌套 submodule**

```bash
git submodule add -b release https://github.com/NoelOrin/GOSpeak-Packages.git packages
```
Expected: `packages/` 出现且含四个包目录；`.gitmodules` 中 path 为 `packages`。

- [ ] **Step 3: 配置 workspace 与根 package.json**

`pnpm-workspace.yaml`（**必须带上 `supportedArchitectures`，见风险 R3**）：
```yaml
packages:
  - "."
  - "packages/*"
onlyBuiltDependencies:      # 注意：pnpm 10.11.0 不支持 allowBuilds（见下）
  - esbuild
  - lefthook
supportedArchitectures:
  os: [current, linux, darwin, win32]
  cpu: [current, x64, arm64]
  libc: [current, glibc, musl]
```

**`allowBuilds` 是死配置**：实测 pnpm **10.11.0**（根 `package.json` 的 `packageManager` 锁定版本）的 bundle 里只有 `onlyBuiltDependencies`，没有 `allowBuilds`（该键 10.26.2 才引入）。而 `lefthook` 有 `postinstall` 脚本、pnpm 10 默认拦截生命周期脚本，不放行会导致钩子不可用。四仓必须**统一**写 `onlyBuiltDependencies`（或统一把 `packageManager` 升到 ≥10.26 再继续用 `allowBuilds`）。

根 `package.json` 保留 turbo/biome 等工具链依赖，scripts 指向 `web`。把原主仓库的 `turbo.json`、`biome.json`、`tsconfig` 基座同步过来。

Expected: `pnpm install --frozen-lockfile` 可解析全部依赖。

- [ ] **Step 4: 生成子仓专属 lockfile 并验证独立构建**

**不能直接复用主仓库根 `pnpm-lock.yaml`**（风险 R3）：它覆盖 `app/*`、`packages/*`、`test`、`plugins/*`，与子仓 workspace 构成不同。首次必须不带 `--frozen-lockfile` 生成，再固化：
```bash
cd /tmp/gospeak-web-setup
pnpm install                 # 生成 web 仓自己的 pnpm-lock.yaml
git add pnpm-lock.yaml
pnpm install --frozen-lockfile   # 验证锁定可复现
pnpm --filter @gospeak/web build
pnpm --filter @gospeak/web test
```
**同时把主仓库 `pnpm-workspace.yaml` 的 `supportedArchitectures` 段落复制过来**（多平台可选二进制：os/cpu/libc），否则 macOS 生成的 lockfile 到 linux CI 会缺包。

Expected: 子仓根出现自己的 `pnpm-lock.yaml`；`--frozen-lockfile` 通过；构建与测试通过，证明 `workspace:*` 已正确解析。

- [ ] **Step 5: 提交并推送**

```bash
git add .gitmodules packages pnpm-workspace.yaml package.json turbo.json biome.json web
git commit -m "refactor: nest packages submodule and configure workspace"
git push origin release
```
Expected: 推送成功；`git submodule status` 显示 packages 指针。

- [ ] **Step 6: 更新主仓库 gitlink**

回到主仓库，把 `app/web` 的 gitlink 指向新的提交（目录布局变化后 commit 不同）：
```bash
cd /Users/noelorin/Projects/GOSpeak
git submodule update --remote app/web
git add app/web && git commit -m "chore: bump web submodule to nested packages layout"
```
Expected: 主仓库 `app/web` gitlink 更新。

---

## Task 6C: 三个子仓的自身装配（Mobile / Desktop / Packages）

**Files:**
- `GOSpeak-Packages`：新增 `pnpm-workspace.yaml`、根 `package.json`、`turbo.json`（可选）
- `GOSpeak-Mobile`：新增 `pnpm-workspace.yaml`、嵌套挂 `packages/` submodule

**为什么单列：** Task 6B 只覆盖了 web 仓。实测还有三处装配未定义：Packages 仓自己的 workspace、Mobile 仓的 proxyrules 挂载、Desktop 仓的 proxyrules + packages 双挂载（β-a）。

- [ ] **Step 1: 清理 `packages/plugin-sdk` 空壳**

`packages/` 下实际有 5 个目录，但 `plugin-sdk/` **没有 `package.json`**，只剩 `node_modules/`、`.turbo/`、`.DS_Store`（插件平台计划残留）。抽取到 `GOSpeak-Packages` 前先删除，避免污染仓库根与 workspace glob。

```bash
rm -rf packages/plugin-sdk
```
Expected: `packages/` 只剩 components / desktop / mobile-capabilities / sfu-client 四个目录。

- [ ] **Step 2: GOSpeak-Packages 仓配置自己的 workspace**

拆出后包目录直接位于子仓根（`components/` 而非 `packages/components/`），因此：

`pnpm-workspace.yaml`：
```yaml
packages:
  - "components"
  - "desktop"
  - "mobile-capabilities"
  - "sfu-client"
```

根 `package.json` 需含 `turbo`、`typescript`、`vitest` 等工具链依赖与 `test`/`build`/`typecheck` 聚合脚本（对齐主仓现有 `pnpm -r` 用法）。**注意：包之间无依赖、tsconfig 无 extends，所以不需要迁移任何基座文件。**

Run:
```bash
cd /tmp/gospeak-packages-setup
pnpm install            # 生成子仓自己的 lockfile
pnpm -r build && pnpm -r test
```
Expected: 四包构建与测试通过；子仓根出现独立 `pnpm-lock.yaml`。

- [ ] **Step 3: GOSpeak-Mobile 挂 proxyrules（**不挂 packages**）**

**β 方案定稿：mobile 不需要 packages。** 实测 `app/mobile` 没有 `src/`，`@gospeak/mobile-capabilities` 在 `.ts/tsx/js/java/kt/swift/gradle/plist` 全仓**零命中**，是残留声明。因此：

```bash
cd /tmp/gospeak-mobile-setup
# 1) 删掉残留依赖，而不是为它挂一个整仓
#    package.json 中删除: "@gospeak/mobile-capabilities": "workspace:*"
# 2) 挂 proxyrules（gomobile bind 必须拿到源码）
git submodule add -b release https://github.com/NoelOrin/GOSpeak-Proxyrules.git proxyrules
```

`pnpm-workspace.yaml`（mobile 本身就是一个包，不需要额外 workspace 成员）：
```yaml
packages:
  - "."
onlyBuiltDependencies:      # pnpm 10.11.0 不支持 allowBuilds
  - lefthook
supportedArchitectures:
  os: [current, linux, darwin, win32]
  cpu: [current, x64, arm64]
  libc: [current, glibc, musl]
```

**同时修正 `app/mobile/package.json` 的两处相对路径**（拆仓后 `../../scripts` 与 `../web` 都不存在）：
- `bind:rules*` 的 `node ../../scripts/mobile-bind-proxyrules.mjs` → 脚本迁入本仓，改为 `node scripts/mobile-bind-proxyrules.mjs`；
- `build:web` 的 `pnpm --filter @gospeak/web build:mobile` → 改为消费外部传入的 web 产物（见 Task 0 Step 4 与补丁 F1 的三 target 产物）。

**脚本随之改造**（不能只改 node 路径，见补丁 B3）：`MODULE_DIR` 改为 `proxyrules`、`OUTPUT_DIR` 改为 `build/proxyrules`、root 用 `--root` 或 `process.cwd()`；`mobile-bind-proxyrules.test.mjs` 一并迁入。

Run:
```bash
cd /tmp/gospeak-mobile-setup
pnpm install                 # 生成子仓 lockfile
pnpm install --frozen-lockfile
git add .gitignore pnpm-lock.yaml pnpm-workspace.yaml package.json proxyrules .gitmodules
git commit -m "refactor: mount proxyrules and drop unused packages dependency"
git push origin release
```
Expected: 无 `packages/` 目录；`proxyrules` 可 `go build`；`git status` 干净。

- [ ] **Step 3b: GOSpeak-Desktop 挂 proxyrules + packages**

desktop 需要两份挂载：`proxyrules`（Go 编译）+ `packages`（`check-desktop-commands.mjs` 迁入后要读 `packages/desktop/src/**`）。

```bash
cd /tmp/gospeak-desktop-setup
git submodule add -b release https://github.com/NoelOrin/GOSpeak-Proxyrules.git proxyrules
git submodule add -b release https://github.com/NoelOrin/GOSpeak-Packages.git packages
```

`go.mod` 改为指向本仓内副本：
```
require github.com/NoelOrin/GOSpeak-Proxyrules v0.0.0
replace github.com/NoelOrin/GOSpeak-Proxyrules => ./proxyrules
```

**迁入门禁**：把主仓的 `scripts/check-desktop-commands.mjs` 与 `.test.mjs` 复制到 desktop 仓的 `scripts/`，改三处常量：
```js
const DESKTOP_DIR = ".";                                  // 原 "app/desktop"
const TS_DIR = "packages/desktop/src/capabilities";       // 原同名（本仓内相对路径）
const TS_EVENTS_FILE = "packages/desktop/src/events.ts";
```
并在该仓 `ci.yml` 里挂成必过步骤。主仓删除这两个脚本及其在 `test-scripts` 里的引用。

Run:
```bash
cd /tmp/gospeak-desktop-setup
go build ./... && go test ./...
node scripts/check-desktop-commands.mjs
```
Expected: Go 构建通过（能从 `./proxyrules` 解析 module）；`check-desktop-commands` 通过；主仓 `git ls-files scripts/check-desktop-commands.mjs` 为空。

- [ ] **Step 4: gitlink 同步约定（β 方案共 6 个挂载点）**

| 挂载点 | 谁 bump |
|---|---|
| 主仓 `app/web` | 主仓（协议生成物变更时） |
| 主仓 `packages` | 主仓（仅供 UI token 类门禁，可滞后） |
| web 仓 `packages` | web 仓（真实消费方） |
| desktop 仓 `proxyrules` | desktop 仓 |
| desktop 仓 `packages` | desktop 仓（供命令门禁） |
| mobile 仓 `proxyrules` | mobile 仓 |

约定：改 packages → 至少 bump **web 仓**（真实消费方）；主仓那份只影响门禁，可滞后但需在 PR 说明。改 proxyrules → desktop 与 mobile 各自按需 bump（互不强制同步）。

Expected: `git -C app/web submodule status` 显示 packages 指针；`git -C app/desktop submodule status` 与 `git -C app/mobile submodule status` 各自显示 proxyrules 指针。

---

## Task 7: CI 按仓库归属拆分（各端 CI 迁回各自子仓库）

**Files:**
- Modify: `.github/workflows/ci.yml`（checkout submodules + 路径过滤器 + protocol-check；移除三端专属 job）
- Modify: `.github/workflows/release.yml`（移除 build-web / build-desktop* / desktop-checksums；仅保留 server 与协调）
- Delete: `.github/workflows/build-mobile.yml`（迁移到 GOSpeak-Mobile）
- Modify: `.github/workflows/docker.yml`、`.github/workflows/docs.yml`、`.github/workflows/security.yml`、`.github/workflows/version-bump.yml`（checkout 加 submodules；移除三端相关步骤）
- Create: `docs/superpowers/specs/2026-11-26-submodule-ci-design.md`（CI 归属矩阵与触发规则）
- 在四个子仓库中 Create: `.github/workflows/ci.yml`（各自独立 CI）与 `.github/workflows/release.yml`（各自独立 Release；由 Task 3 的子仓库提交落地）

**Interfaces:**
- Consumes: Task 5 的 submodule 结构；现有 workflows 内容。
- Produces: 每端 CI 在各自仓库运行；主仓库 CI 只负责 server 与跨仓协调；供 Task 8 验证。

**CI 归属矩阵（写入设计文档）：**

| 工作流 | 所属仓库 | 职责 |
|---|---|---|
| CI（server / Go / scripts / protocol / 集成测试） | GOSpeak 主仓库 | 只跑 `app/server`、`app/shared`、`scripts`、`test`、`deploy`；不再构建三端 |
| CI（lint / typecheck / test / build web） | GOSpeak-Web | 前端与嵌套 packages 的质量门禁；生成物 `dist/` 由本仓 release 打包 |
| CI（packages 单元测试） | GOSpeak-Packages | 四个 workspace 包自身的 test / typecheck / build |
| CI（Go / mygo build / 安装包） | GOSpeak-Desktop | 桌面宿主自身门禁与安装包构建 |
| CI（Capacitor sync / gradle / xcode） | GOSpeak-Mobile | 移动壳自身门禁与 APK/IPA 构建 |
| CI（Go 单测 + gobind 绑定面） | **GOSpeak-Proxyrules** | `go vet ./... && go test ./...` + `bind-surface --surface-only`（原主仓 `desktop-go` 里的两条门禁迁到这里） |
| Release（server / Docker） | GOSpeak 主仓库 | 只发布 server 产物；消费 web 压缩包打进 `static/` |
| Release（web 静态包） | GOSpeak-Web | 独立 Release `gospeak-web-v*`，产物为前端压缩包 |
| Release（desktop 安装包） | GOSpeak-Desktop | **独立 Release** `gospeak-desktop-v*`，产物为 dmg/deb/exe/install.sh |
| Release（mobile 安装包） | GOSpeak-Mobile | **独立 Release** `gospeak-mobile-v*`，产物为 APK/IPA |
| Release（proxyrules module） | GOSpeak-Proxyrules | 打 `v*` module tag；无 Release 产物（packages 同样无独立 Release） |

- [ ] **Step 1: 改造 ci.yml 的 checkout（公开子仓，无需 token）**

**子仓全部 public，无需 `token`**（R1/CI3/CI5 均已消除）。只需加 `submodules: recursive`：
```yaml
- uses: actions/checkout@v7
  with:
    submodules: recursive
    fetch-depth: 0
```
注意 `test-scripts` job 已有 `fetch-depth: 0`，只加 `submodules` 即可。

**只给需要子仓内容的 job 加**（β 方案下主仓只有 2 个 submodule，但仍有 job 不需要）：`build-server`、`protocol-check`、`lint`（跨仓脚本）必加；`changes`、`gate`、`workflow-lint` 不加（减少 checkout 时间）。

Expected:
```bash
grep -c "checkout@v7" .github/workflows/ci.yml
grep -c "submodules: recursive" .github/workflows/ci.yml
grep -c "secrets.SUBMODULES_TOKEN" .github/workflows/ci.yml   # 必须为 0
```
前两者按 job 需求匹配，第三个为 **0**。

- [ ] **Step 2: 改造 paths-filter 过滤器**

在 `js` 与 `go` 过滤器中加入 gitlink 路径本身（因为主仓库 PR 只改 gitlink，`app/web/**` 不会显示变更）：
```yaml
go:
  - 'app/server/**'
  - 'app/server/go.mod'
  - 'app/server/go.sum'
  - 'app/desktop'           # gitlink
  - 'app/shared/**'
js:
  - 'app/web'               # gitlink
  - 'app/mobile'            # gitlink
  - 'app/desktop/mygo.config.ts'   # 删除：该文件已在子仓库，主仓库不再有
  - 'app/desktop/package.json'     # 删除：同上
  - 'packages'             # gitlink（拆仓后主仓不再有 packages/** 文件模式）
  - 'test/**'
  ...
```
注意 `app/desktop/**/*.go` 等具体文件模式在主仓库已失效，统一替换为 `app/desktop` gitlink。

Expected: 过滤器包含三项 gitlink 路径，不含 `app/web/**` / `app/desktop/**` / `app/mobile/**` 文件模式。

- [ ] **Step 3: 改造 protocol-check job 并补补偿措施（风险 R13）**

原命令 `git diff --exit-code -- ../web/src/protocol/generated` 在主仓库看不到子仓库内变化。改为**两者都校验**：

```bash
# 在主仓库根运行（去掉 working-directory: app/server）
git diff --exit-code -- app/web      # gitlink 未被就地修改
```

**仅校验 gitlink 不足以防止漏提交**：生成物在子仓内变了、但没人 push 到 GOSpeak-Web，主仓的 gitlink 也没动，此检查照样通过。必须补一条真正的补偿措施，二选一：

- **方案 A（推荐）**：`protocol-check` 在生成后检查子仓工作区是否 dirty，dirty 即失败并提示「协议生成物已变，请在 GOSpeak-Web 提交并回填 gitlink」：
  ```bash
  (cd app/web && git status --porcelain -- src/protocol/generated | grep . ) && { echo '::error::generated/ 有未提交变更'; exit 1; } || true
  ```
- **方案 B**：主仓 CI 用可写 PAT 直接把生成物 push 到 GOSpeak-Web 的 PR 分支，再更新 gitlink（需要跨仓写权限，复杂）。

Expected: 命令在主仓库运行退出码 0；故意修改 `internal/protocol` 而未重新生成时，job 失败并给出明确指引。

- [ ] **Step 4: 改造其余 workflows checkout**

对 `release.yml`、`build-mobile.yml`、`docker.yml`、`docs.yml`、`security.yml`、`version-bump.yml` 中所有 `actions/checkout@v7` 加 `with: submodules: recursive`（已有 `with` 的合并进去）。**公开仓不加 `token`。**

需要写子仓的 workflow（protocol 生成物同步、跨仓 tag）**不在本步处理**——那是跨仓写权限问题，见 CI4 的定稿（β 下推荐主仓只探测+提示，不跨仓写）。

Expected: 
```bash
grep -rc "submodules: recursive" .github/workflows/ | grep -v ':0'
grep -rc "SUBMODULES_TOKEN" .github/workflows/ | grep -v ':0'
```
两者覆盖的 workflow 集合一致。

- [ ] **Step 5: 从主仓库 CI 移除三端专属 job**

从 `ci.yml` 删除 `build-js`、`test-web`、`test-packages`、`desktop-go` 三端专属 job 及 `gate` 中对应 needs；`lint` 的 biome 路径去掉 `./app/web` / `./app/desktop` / `./app/mobile` / `./packages`；`typecheck` 只保留主仓库 `scripts`。从 `release.yml` 删除 `build-web`、`build-desktop-macos`、`build-desktop-others`、`desktop-checksums` 及所有向 `webui/dist` 拷贝前端产物的步骤；删除 `build-mobile.yml`。

同时清理主仓库工具链配置中对已拆出目录的引用：
- `pnpm-workspace.yaml`：**不能简单改成「只剩 test」**。`app/docs` 是 `@gospeak/docs` 的 workspace 成员，`docs.yml` 的 `pnpm --filter @gospeak/docs build` 依赖它；把 `app/*` 整个删掉会让文档站构建直接 `No projects matched` 失败。正确写法是显式列：
  ```yaml
  packages:
    - "app/server"   # 根 pnpm dev / start:dev / dev:server 依赖 @gospeak/server
    - "app/docs"     # docs.yml 的 pnpm --filter @gospeak/docs build 依赖它
    - "test"
    - "plugins/*"
  ```
  **注意**：`turbo run dev --filter=<不存在的包>` 会 `exit 1`（会红），但 `pnpm --filter <不存在的包>` 只打印 `No projects matched` 并 `exit 0`（**静默绿**）。误删成员时 `pnpm dev:web` 会「成功退出但什么都没启动」，必须用上面的显式清单。
  同时验证 `test/` 不依赖 `@gospeak/*` 工作区包（`grep -rn '@gospeak/' test/`），若依赖则需保留 `packages/*` 或改为 npm 依赖。
  Expected 追加一条：`pnpm --filter @gospeak/docs build` 仍通过。
- `turbo.json`：`globalDependencies` 移除 `app/*/tsconfig*.json`、`packages/*/tsconfig*.json`
- `lefthook.yml` / `biome.json`：移除 `./app/web`、`./app/desktop`、`./app/mobile`、`./packages` 路径
- `scripts/check-desktop-commands.mjs`、`check-mobile-bridge.mjs`、`mobile-version.mjs`：**保留在主仓库，不要迁到子仓库**。理由见风险 R4：它们跨读 `app/desktop`（Desktop 仓）与 `packages/desktop`（Packages 仓），或跨读 `app/shared/proxyrules`（主仓）与 `app/mobile`（Mobile 仓）；子仓单独 checkout 时这些路径不存在，迁移后会直接失效。子仓 CI 不重复实现这些校验。

Expected: `grep -rn "app/web\|app/desktop\|app/mobile\|packages/" .github/workflows/ pnpm-workspace.yaml turbo.json lefthook.yml biome.json` 仅剩 gitlink 路径与 checkout 配置。

- [ ] **Step 6: 在四个子仓库建立独立 CI / Release**

各子仓库建立**独立 CI 与独立 Release**（这是本次拆分的核心目标：产物分开管理）：

- **GOSpeak-Web**：`ci.yml`（lint / typecheck / vitest / build）；`release.yml` 由 tag `gospeak-web-v*` 触发，构建并发布前端静态压缩包到**自己的 Release**。
- **GOSpeak-Packages**：`ci.yml`（四包 test / typecheck / build），无 Release。
- **GOSpeak-Desktop**：`ci.yml`（go vet / go test / mygo build）；`release.yml` 由 tag `gospeak-desktop-v*` 触发，发布安装包到**自己的 Release**。
- **GOSpeak-Mobile**：`ci.yml`（capacitor sync / gradle / xcode）；`release.yml` 由 tag `gospeak-mobile-v*` 触发，发布 APK/IPA 到**自己的 Release**。

**发布 tag 归属（修订）**：每个子仓库打自己的发布 tag（`gospeak-web-v*` / `gospeak-desktop-v*` / `gospeak-mobile-v*`）并创建自己的 Release。主仓库只保留 server 的 tag 与 Release。各仓 tag 格式统一为 `gospeak-<component>-v<semver>`，由 `release-naming.mjs` 派生。这样 Releases 页面按仓库天然分开，不再挤在一起。

---

- [ ] **Step 6b: 分发复合 action 与工具链三件套（风险 R7 / R12）**

现主仓库只有两个复合 action：`.github/actions/pnpm-install`、`.github/actions/go-build-cache`。按用途复制：

| 子仓库 | 需要的 action | 需要的工具链配置 |
|---|---|---|
| GOSpeak-Web | `pnpm-install` | `biome.json`、`tsconfig` 基座、`turbo.json`（若用）、`lefthook.yml` |
| GOSpeak-Packages | `pnpm-install` | 同上 |
| GOSpeak-Desktop | `pnpm-install`、`go-build-cache` | `biome.json`（若含 TS 配置）、`lefthook.yml` |
| GOSpeak-Mobile | `pnpm-install`、`go-build-cache` | `lefthook.yml` |

**避免复制漂移**：若后续 action 逻辑还要改，优先建 `GOSpeak-CI` 公共仓统一维护（私有仓 reusable workflow 需额外权限配置，任务执行时评估）；短期先复制并在各仓 README 注明「源头在主仓库 `.github/actions`」。

`lefthook.yml` 的路径必须按子仓库目录重写（例：web 仓用 `./web`，不再是 `./app/web`），否则 pre-commit 直接报路径不存在。

Expected: 每个子仓 `.github/actions/` 与 `lefthook.yml`/`biome.json` 存在；`cd <子仓> && npx lefthook run pre-commit` 通过。

- [ ] **Step 6c: 按仓拆分 dependabot（风险 R8）**

主仓库现有 `.github/dependabot.yml` 只覆盖主仓路径。拆仓后逐仓新建：

- GOSpeak-Web / GOSpeak-Packages：`npm`（`/` 或 `/web`）
- GOSpeak-Desktop：`gomod`（`/`）
- GOSpeak-Mobile：`npm`（`/`）、`gradle`（`/android`）、`pub`（`/ios` 若启用）

主仓库的 `dependabot.yml` 同步删除已拆出目录的条目（`app/web`、`app/desktop`、`app/mobile`、`packages`），只保留 `gomod`/`npm` 的主仓部分。

Expected: `grep -rn "app/web\|app/desktop\|app/mobile\|packages/" .github/dependabot.yml` 无命中；四仓各有自己的 `dependabot.yml`。

- [ ] **Step 7: 建立 CI 归属设计文档**

创建 `docs/superpowers/specs/2026-11-26-submodule-ci-design.md`，写入「CI 归属矩阵」、触发规则（子仓库 push/tag 触发自身 CI/Release；主仓库靠 gitlink 更新触发自身 CI）、主仓库不再承担三端构建、**各仓独立 Release 使产物天然分开**。

Expected: 文档存在且包含归属矩阵全部核心行，明确写出「各端 Release 独立、产物不再混在同一 Release 条目」。

- [ ] **Step 8: 提交 CI 改造**

```bash
git add .github/workflows/ docs/superpowers/specs/2026-11-26-submodule-ci-design.md
git commit -m "ci: move per-platform workflows into submodule repos"
```
Expected: 提交成功。

- [ ] **Step 9: 验证 workflow 语法**

```bash
curl -sSfL https://github.com/rhysd/actionlint/releases/download/v1.7.7/actionlint_1.7.7_linux_amd64.tar.gz | tar xz -C /tmp actionlint
/tmp/actionlint -color .github/workflows/
```
Expected: 无错误输出。

---

## Task 8: 全量验证与全新 clone 复现

**Files:** 无文件改动（验证任务）

**Interfaces:**
- Consumes: Task 5/6 的提交。
- Produces: 验证结论；若失败回到对应任务修复。

- [ ] **Step 1: 校验 submodule 状态（含嵌套层）**

```bash
cd /Users/noelorin/Projects/GOSpeak
git submodule status
cd app/web && git submodule status
```
Expected: 主仓库**两行**（`app/web`、`packages`）、web 内一行（`packages`），均以空格开头（无 `+` / `-` 前缀）。

- [ ] **Step 2: 主仓库门禁**

```bash
pnpm test:scripts
node --test scripts/*.test.mjs
```
Expected: 全部退出码 0。

- [ ] **Step 3: GOSpeak-Web 独立验证**

```bash
cd /tmp/gospeak-web-verify
pnpm install --frozen-lockfile
pnpm --filter @gospeak/web typecheck
pnpm --filter @gospeak/web test
pnpm --filter @gospeak/web build
pnpm --filter @gospeak/web build:desktop
pnpm --filter @gospeak/web build:mobile
```
Expected: 独立 clone 后 `workspace:*` 正常解析，三个目标均构建成功。

- [ ] **Step 4: GOSpeak-Packages 独立验证**

```bash
cd /tmp/gospeak-packages-verify
pnpm install --frozen-lockfile
pnpm -r test
pnpm -r typecheck
```
Expected: 四包测试与类型检查通过。

- [ ] **Step 5: Go 门禁与服务端测试**

```bash
cd /Users/noelorin/Projects/GOSpeak
(cd app/server && go vet ./...)
(cd app/web && git log -1 --oneline)   # 确认子仓就位
pnpm test:server:go
```
**注意**：不能在仓库根跑 `go vet ./app/server/...`——根目录没有 `go.mod`，实测报 `directory prefix app/server does not contain main module`。proxyrules 的单测已迁到 GOSpeak-Proxyrules 仓 CI（β 下主仓不再有该目录）。
Expected: 全部成功。

- [ ] **Step 6: 跨仓依赖验证**

```bash
cd /tmp/gospeak-desktop-verify && go mod download && go build ./...
```
Expected: desktop 能从公开 proxy 拉到 `github.com/NoelOrin/GOSpeak/app/shared/proxyrules`；构建通过。

- [ ] **Step 7: 全新 clone 复现**

```bash
rm -rf /tmp/gospeak-submodule-verify
git clone https://github.com/NoelOrin/GOSpeak.git /tmp/gospeak-submodule-verify
cd /tmp/gospeak-submodule-verify
git submodule update --init --recursive
ls app/web/src app/desktop app/mobile/android
```
Expected: 目录内容完整；主仓 `git submodule status` **两行**（`app/web` / `packages`）无前缀异常；`app/desktop` 与 `app/mobile` **不存在**（β 不挂）。

- [ ] **Step 8: 清理验证 clone**

```bash
rm -rf /tmp/gospeak-submodule-verify
```
Expected: 目录不存在。

- [ ] **Step 9: 建立 gitlink 漂移检测（风险 R5）**

子仓 `release` 前进后主仓 gitlink 不会自动跟随，主仓 CI 会长期显示绿色但实际落后。新增一个定时 job（如 `.github/workflows/submodule-drift.yml`，`cron: '0 2 * * *'`）：

```bash
for p in app/web:Web app/desktop:Desktop app/mobile:Mobile packages:Packages; do
  path="${p%%:*}"; name="${p##*:}"
  locked=$(git ls-tree HEAD "$path" | awk '{print $3}')
  remote=$(git ls-remote "https://github.com/NoelOrin/GOSpeak-$name.git" refs/heads/release | cut -f1)
  [ "$locked" = "$remote" ] || echo "::warning::$path 落后于子仓 release"
done
```

也可直接用 `git submodule status` 看 `+` 前缀。漂移时开 issue 或 `::warning::`，**不要自动 bump gitlink**（避免绕过评审）。

Expected: 手动把某子仓 `release` 推一个空提交后，该 job 在次日（或手动触发时）输出漂移告警。

- [ ] **Step 10: 复核 `test/` 集成测试对前端产物的依赖（风险 R14）**

`test/globalSetup.ts` 现有 `internal/webui/dist` 相关排除。Task 10 去 embed 后复核：
- 集成测试是否要求前端产物存在；
- 若需要，`STATIC_DIR` 应指向 `app/web/dist`（submodule 挂载点，path 不变）还是发布包 `static/`；
- 更新 `globalSetup.ts` 的排除列表。

Run: `pnpm test:server`
Expected: 集成测试通过，且不依赖 `go:embed` 的前端产物。

- [ ] **Step 11: 更新计划状态**

在本计划顶部加 Status 块，记录验证结果与日期。

Expected: 计划状态块存在。

---

---

## Task 9: 通过 metadata.yaml 做版本与元数据管理（独立发布）

**Files:**
- Create: `metadata.yaml`（主仓库总控）
- Create: `metadata.yaml`（四个子仓库各自一份，通过 Task 3 的子仓库提交落地；packages 无 release 但保留 version）
- Modify: `scripts/check-version.mjs` + `scripts/check-version.test.mjs`（改读 metadata.yaml + submodule 守卫）
- Modify: `scripts/release-naming.mjs` + `scripts/release-naming.test.mjs`（mobile 与 web 组件）
- Modify: `scripts/build-changelog.mjs` / `release-notes.mjs` + 各自 test（按组件生成）
- Modify: `.github/workflows/version-bump.yml` / `release.yml`（metadata 驱动、按组件 bump；release.yml 增加 web 静态压缩包构建与发布）
- Delete: `.github/versions/{server,desktop,mobile}.yaml`（迁移到 metadata.yaml）
- Create: `docs/superpowers/specs/2026-11-26-submodule-release-design.md`（含 web 外部更新规则）
- Modify: `app/server/internal/router/router.go`（`serveSPA` 补 `index.html` no-store 响应头）
- Modify: `scripts/AGENTS.md`、根 `AGENTS.md`

**Interfaces:**
- Consumes: Task 5 的 submodule 结构；Task 7 改造后的 CI workflows。
- Produces: 以 metadata.yaml 为核心的版本与元数据管理；独立发布规则得到落地。

**元数据模型（builder call，已记录）：**

采用方案 C：主仓库总控 + 子仓库各自一份，分层聚合。

`metadata.yaml`（主仓库根）结构：
```yaml
schema_version: 1
components:
  server:
    version: 1.2.0
    source: app/server
    tag: gospeak-server-v1.2.0
    release: true
  web:
    version: 1.0.0
    source: app/web            # git submodule
    metadata_ref: app/web/metadata.yaml
    tag: gospeak-web-v1.0.0
    release: true              # 产物为 dist/ 静态压缩包，供外部 STATIC_DIR 部署
  desktop:
    version: 1.1.0
    source: app/desktop        # git submodule
    metadata_ref: app/desktop/metadata.yaml
    tag: gospeak-desktop-v1.1.0
    release: true
  mobile:
    version: 0.9.0
    source: app/mobile         # git submodule
    metadata_ref: app/mobile/metadata.yaml
    tag: gospeak-mobile-v0.9.0
    release: true              # 产物由 GOSpeak-Mobile 自己的 release.yml 发布
  packages:
    version: 1.0.0
    source: packages           # git submodule
    metadata_ref: packages/metadata.yaml
    release: false             # 仅作为 web 的 workspace 依赖，不单独发 Release
```

**Release 归属（四仓模型）：**

| 组件 | 发布仓库 | Release tag | 产物 |
|---|---|---|---|
| server | GOSpeak | `gospeak-server-v*` | 二进制 + `static/` |
| web | GOSpeak-Web | `gospeak-web-v*` | 前端静态压缩包 |
| desktop | GOSpeak-Desktop | `gospeak-desktop-v*` | dmg / deb / exe / install.sh |
| mobile | GOSpeak-Mobile | `gospeak-mobile-v*` | APK / IPA |
| packages | —（不发 Release） | — | 仅 workspace 依赖 |

`metadata.yaml`（子仓库根，以 desktop 为例）：
```yaml
schema_version: 1
component: desktop
version: 1.1.0
metadata:
  name: GOSpeak Desktop
  description: GOSpeak 桌面客户端
  platforms: [darwin/amd64, darwin/arm64, windows/amd64, linux/amd64, linux/arm64]
  artifact_type: installer
  changelog: CHANGELOG.md
```

`metadata.yaml`（web 子仓库根）：
```yaml
schema_version: 1
component: web
version: 1.0.0
metadata:
  name: GOSpeak Web
  description: GOSpeak 前端静态产物
  platforms: [any]
  artifact_type: static-archive
  changelog: CHANGELOG.md
```

规则：
- 主仓库总控的 `components.<name>.version` 是最终事实源；子仓库 `version` 必须与之一致，由 CI 校验。
- `metadata_ref` 指向子仓库 metadata 文件，check-version 会读取并比对版本。
- 元数据字段（name/description/platforms/artifact_type/changelog）供 release-notes 与 build-changelog 消费。
- **版本号不统一发布**：server / web / desktop / mobile 四组件各自独立 SemVer，互不跟随；packages 有独立 version 但不发布。
- 版本 bump 只针对发生变更的组件：先改该子仓库 `metadata.yaml` 与版本文件并 commit/push，再更新主仓库总控 `metadata.yaml` 的 `version` 与 `tag`，最后更新 gitlink 并打 tag。
- 每个组件独立 tag / **独立 Release（在自己的仓库里）** / 独立 changelog；彻底消除「一版一个 Release、产物挤在一起」的旧模型。
- 子仓库 commit message 约定：`chore(<component>): bump version to <semver>`，gitlink bump 与版本发布解耦。

- [ ] **Step 1: 写 release-naming 增加 mobile 与 web 组件的测试**

在 `scripts/release-naming.test.mjs` 加：
```js
test("releaseTag mobile", () => {
  assert.equal(releaseTag("mobile", "1.2.0"), "gospeak-mobile-v1.2.0");
  assert.deepEqual(parseReleaseTag("gospeak-mobile-v1.2.0"), { component: "mobile", version: "1.2.0" });
});
test("releaseTag web", () => {
  assert.equal(releaseTag("web", "1.0.0"), "gospeak-web-v1.0.0");
  assert.deepEqual(parseReleaseTag("gospeak-web-v1.0.0"), { component: "web", version: "1.0.0" });
});
```
Run: `node --test scripts/release-naming.test.mjs`
Expected: FAIL（mobile 与 web 不在 COMPONENTS 中）。

- [ ] **Step 2: 实现 mobile 与 web 组件**

把 `scripts/release-naming.mjs` 的 `COMPONENTS` 改为 `["server", "web", "desktop", "mobile"]`。

Run: `node --test scripts/release-naming.test.mjs`
Expected: PASS。

- [ ] **Step 3: 写 metadata 读取与一致性校验测试**

新建 `scripts/metadata.test.mjs`，实现 `readMetadata(rootDir, component)` 的测试：
- 主仓库总控存在 `components.<component>.version`。
- 子仓库 `metadata.yaml` 的 `version` 与总控一致；不一致时返回 issue。
- 子仓库未 checkout 时返回「submodule not initialized」明确错误。

Run: `node --test scripts/metadata.test.mjs`
Expected: FAIL（模块不存在）。

- [ ] **Step 4: 实现 metadata 读取模块**

新建 `scripts/metadata.mjs`：零依赖解析 `metadata.yaml`（用正则，风格对齐现有 check-version.mjs 的 `readDeclaredVersion`），导出 `readMetadata(rootDir, component)` 与 `checkMetadataConsistency(rootDir, component)`。

Run: `node --test scripts/metadata.test.mjs`
Expected: PASS。

- [ ] **Step 5: 改造 check-version.mjs 读 metadata.yaml**

把 `.github/versions/*.yaml` 的读取改为 `scripts/metadata.mjs`，删除旧 yaml 文件。保留原有「VERSION / package.json / mygo.config.ts / build.gradle / pbxproj」一致性校验，但声明源改为 metadata。

Run: `node --test scripts/check-version.test.mjs`
Expected: PASS（测试 fixture 改为 metadata.yaml）。

**metadata ↔ tag 协同规则（新增）：**
- **tag 由 metadata 派生，不手写**：`gospeak-<component>-v<metadata.components.<component>.version>`，一律经 `release-naming.mjs tag` 生成。
- **tag 存在性驱动发布**：version-bump 对每个组件检测远端 tag 是否存在；不存在则打 annotated tag 并 `workflow_call` 调 release.yml；已存在则跳过（幂等）。
- **metadata ↔ tag 双向一致性**：
  - 正向：`check-version.mjs` 校验 `metadata.version` 与 tag 解析出的 version 一致（tag 事件时）。
  - 反向：`metadata.yaml` 中显式写 `tag` 字段，CI 校验它等于 `release-naming.mjs tag <component> <version>` 的派生结果，防止手工写错。
- **子仓库 metadata 协同**：子仓库 `metadata.yaml` 的 `version` 必须与主仓库总控一致；不一致时 version-bump 拒绝打 tag（CI 门禁）。
- **lockstep 约束已删除**：`version-bump.yml` 的 `Require lockstep component versions` 已由前置计划 [`2026-11-26-per-component-release.md`](./2026-11-26-per-component-release.md) 移除；此处只需复核「每组件独立判断 tag 存在性」在拆仓后仍成立。
- **tag 指向的 commit**：annotated tag 打在主仓库「更新 metadata + gitlink 的那次提交」上，保证 tag 对应的 tree 中 submodule 指针就是该组件版本的文件状态。
- **发布 tag 归属（定稿）**：**各组件在自己的仓库打 tag、建 Release。** server 在主仓库（`gospeak-server-v*`）；web / desktop / mobile 各自在 GOSpeak-Web / GOSpeak-Desktop / GOSpeak-Mobile 内打 `gospeak-<component>-v*` 并创建本仓 Release。tag 唯一来源仍是 `release-naming.mjs` 派生，不在任一仓库手写。
- **为什么不用「只在主仓库打 tag + workflow_call 触发子仓 Release」**：跨仓 `workflow_call` 需要子仓暴露 reusable workflow 并配置跨仓 token 授权，链路脆弱且失败点不透明；而「各仓自打 tag」与 Submodule 的核心收益（各端独立发版节奏、Releases 页面天然分开）一致。
- **双源一致性怎么保证**：主仓库 `metadata.yaml` 的 `components.<name>.version` 与 `tag` 是总控事实源；`check-version.mjs` 校验 (a) 总控 `tag` 等于 `release-naming.mjs` 派生结果、(b) 子仓库 `metadata.yaml` 的 `version` 与总控一致、(c) 子仓 gitlink 所指 commit 上确有该 tag。三者不一致则 CI 拒绝发布。
- **发布顺序**：先子仓 bump metadata → 打 tag → 子仓 Release 出产物；再主仓 bump gitlink 与总控 `metadata.yaml` → 打 server tag（若 server 也变）。server 包依赖的 web 产物固定取 gitlink 指向的 web tag，不用浮动分支。

- [ ] **Step 6: 写 metadata ↔ tag 协同测试**

在 `scripts/metadata.test.mjs` 增加：
- `checkTagConsistency(metadata, component)`：metadata 中 `tag` 字段必须等于 `release-naming.mjs tag` 的派生结果；不等时返回 issue。
- 子仓库 `metadata.version` 与主仓库总控不一致时返回 issue。

Run: `node --test scripts/metadata.test.mjs`
Expected: FAIL（函数未实现）。

- [ ] **Step 7: 实现 tag 协同校验**

在 `scripts/metadata.mjs` 实现 `checkTagConsistency` 与子仓库版本一致性检查，从 `release-naming.mjs` 导入 `releaseTag`。

Run: `node --test scripts/metadata.test.mjs`
Expected: PASS。

- [ ] **Step 8: 改造 version-bump.yml 为 metadata 驱动的按组件 bump + tag 协同**

流程：
```
checkout（with submodules + fetch-depth: 0）
→ 读取主仓库 metadata.yaml，逐组件取 version/tag
→ 对每个组件：校验 metadata.tag === release-naming 派生 tag；
   校验子仓库 metadata.version === 总控 version（不一致则 fail，不打 tag）
→ 检测远端 tag 是否已存在（git ls-remote --tags origin）
→ 仅对「版本变化且 tag 不存在」的组件：
   打 annotated tag gospeak-<component>-v<version> 到当前 HEAD
   （HEAD 必须是更新 metadata + gitlink 的那次提交）
   → 推送 tag → workflow_call 调 release.yml 仅发布该组件
→ 其他组件 tag 已存在则跳过（幂等）
```
关键：lockstep 约束已在前置计划中删除，本步只做拆仓后的复核；**不 bump 其他组件**；tag 与 metadata 双向一致才允许打 tag。

Expected: actionlint 通过；dry-run 验证：
- 两组件版本不同时不再报错;
- metadata.tag 写错时 CI 失败;
- 子仓库版本不一致时 CI 失败;
- tag 已存在时幂等跳过。

- [ ] **Step 9: 改造 release.yml 为一组件一 Release**

去掉「统一 Release」逻辑，按 component 条件构建与发布；Release 正文由 `metadata.yaml` 的 `metadata` 字段驱动（name/description/platforms/artifact_type/changelog）。mobile 产物由独立 `build-mobile.yml` 发布，不在本 workflow 构建。

Expected: `/tmp/actionlint -color .github/workflows/` 无错误。

- [ ] **Step 10: 改造 build-changelog.mjs 与 release-notes.mjs**

按组件从 metadata 读取 changelog 路径与元数据，从子仓库 git log（或 gitlink bump commit）提取该组件变更；Release 正文只含该组件下载表格与说明。

Run: `node --test scripts/build-changelog.test.mjs scripts/release-notes.test.mjs`
Expected: 旧「一版含两端」断言改为按组件断言后 PASS。

- [ ] **Step 11: 落地子仓库 metadata.yaml**

在 Task 3 抽取后、Task 5 接入前，把子仓库 `metadata.yaml` 作为子仓库首提交的一部分（或单独提交）推送。四个子仓库各自文件见上方结构。

Expected: 每个子仓库根目录有 `metadata.yaml` 且 `component` 字段正确。

- [ ] **Step 12: 编写版本与元数据设计文档**

创建 `docs/superpowers/specs/2026-11-26-submodule-release-design.md`，覆盖：方案 C 分层模型、metadata 字段结构、独立发布规则、metadata ↔ tag 双向一致性、tag 存在性驱动发布（幂等）、删除 lockstep 约束、两阶段按组件 bump、子仓库 commit 约定、mobile 独立构建原因、gitlink 与版本解耦。

Expected: 文档存在且包含上述七点。

- [ ] **Step 13: 更新 AGENTS.md 文档**

`scripts/AGENTS.md`：新增 `metadata.mjs` 行；`check-version.mjs` 描述改为读 metadata；`release-naming.mjs` 改四组件（含 web）；新增设计文档指针。根 `AGENTS.md` §6 补「版本与元数据以 metadata.yaml 为单一事实源，四组件独立发布」。设计文档 `2026-11-26-submodule-release-design.md` 必须包含 web 静态包发布与外部 `STATIC_DIR` 更新规则、embed 兜底关系。

Expected: `grep -n "metadata.yaml" scripts/AGENTS.md AGENTS.md` 有命中；`grep -n "STATIC_DIR" docs/superpowers/specs/2026-11-26-submodule-release-design.md` 有命中。

- [ ] **Step 14: web 静态压缩包构建落在 GOSpeak-Web 自己的 release.yml**

**不在主仓库构建 web 产物。** 在 `GOSpeak-Web` 仓库的 `release.yml` 中：由 tag `gospeak-web-v*` 触发，`pnpm install --frozen-lockfile` → `pnpm --filter @gospeak/web build`，把 `web/dist/` 打包为 `gospeak-web-v<version>.tar.gz`，上传到**该仓库自己的 Release**。产物命名走 `release-naming.mjs` 派生。

主仓库的 `release.yml` 只在构建 server 产物时需要前端：从 `gospeak-web-v<对应版本>` 的 Release 下载压缩包，解压进 `static/`。

Expected: GOSpeak-Web 打 tag 后，**在 GOSpeak-Web 仓库的 Releases 页**出现 `gospeak-web-v<version>` 条目与压缩包附件；主仓库 Releases 页无 web 条目。

- [ ] **Step 15: serveSPA 补 index.html no-store 响应头**

在 `app/server/internal/router/router.go` 的 `serveSPA` 中，定位 `serveIndex` 的两个实现（外部目录分支与 embed 分支），在返回 `index.html` 前设置 `c.Header("Cache-Control", "no-store")`。只针对首页，带 hash 的静态资源仍可长缓存。

Run: `go test ./internal/router/...`（在 `app/server` 下）
Expected: 测试通过；`grep -n "no-store" app/server/internal/router/router.go` 至少两处命中。

- [ ] **Step 16: 全量脚本测试**

```bash
node --test scripts/*.test.mjs
```
Expected: 全部通过。

- [ ] **Step 17: 提交**

```bash
git add metadata.yaml scripts/ .github/workflows/version-bump.yml .github/workflows/release.yml docs/superpowers/specs/2026-11-26-submodule-release-design.md scripts/AGENTS.md AGENTS.md app/server/internal/router/router.go
git commit -m "feat: metadata-driven versioning with independent releases"
```
Expected: 提交成功。

## Task 10: 前端去 embed，与二进制一起分发

**Files:**
- Modify: `app/server/internal/webui/embed.go` / `stub.go`（移除 go:embed，保留外部目录探测兼容层）
- Modify: `app/server/internal/router/router.go`（仅保留外部目录路径，移除 webui.FS() 兜底分支）
- Modify: `Dockerfile`（final stage 增设 `/app/static` 并 COPY 前端产物）
- Modify: `deploy/docker-compose.yml` / `deploy/docker-compose.standard.yml`（前端目录挂载点）
- Modify: `.github/workflows/ci.yml` / `release.yml`（去掉往 `internal/webui/dist` 与 `app/desktop/webui/dist` 的拷贝，改为随发布包输出 `static/`）
- Modify: `app/desktop/mygo.config.ts` / `app/desktop/*.go`（桌面宿主读外部前端目录）
- Modify: `app/server/internal/webui/AGENTS.md` / 根 `AGENTS.md` / `app/web/AGENTS.md`（文档同步）

**Interfaces:**
- Consumes: Task 9 的 web 静态压缩包产物；Task 5 的 submodule 结构。
- Produces: 服务端与桌面产物均为「二进制 + 外部前端目录」；无 embed 前端。

- [ ] **Step 1: 写移除 embed 的测试**

为 `webui` 包新增测试：`HasAssets()` 在外部 `static` 目录不存在时为 false；`FS()` 恒返回 nil（或删除该 API 后测试路由仅走外部目录）。为 `serveSPA` 补测试：无外部目录时返回 404/空响应而非 embed 内容。

Run: `go test ./internal/webui/... ./internal/router/...`（在 `app/server` 下）
Expected: FAIL（embed 逻辑仍在）。

- [ ] **Step 2: 移除 go:embed 与兜底分支**

删除 `embed.go` 的 `//go:embed all:dist` 与 `embedded` 变量，`webui` 包只保留 `HasAssets()`（探测外部目录）与必要的路径常量；`stub.go` 相应简化。删除 `router.go` 中 `webui.FS()` 回退分支，仅保留 `STATIC_DIR > /app/static > ./static` 三级外部目录。

Run: `go test ./internal/webui/... ./internal/router/...`
Expected: PASS。

- [ ] **Step 3: 改造 Dockerfile 与 compose**

`Dockerfile` 删除 `COPY --from=web-builder /src/app/web/dist/ ./internal/webui/dist/`；final stage 加 `COPY --from=web-builder /src/app/web/dist/ /app/static/`，`VOLUME` 增加 `/app/static`（或由 compose 挂载覆盖）。`deploy/docker-compose*.yml` 确认 `STATIC_DIR=/app/static` 并把前端产物挂载到该路径。

**前置校验（风险 R2，缺一不可）**：
1. 依赖 `COPY app/web` / `COPY packages/*` 的 builder 阶段要求构建上下文里子模块**已 checkout**；本地 `docker build .` 若子模块未 init 会失败，先 `git submodule update --init --recursive`；
2. CI 的 docker 构建 job 必须在 build 之前完成 `submodules: recursive` + 凭据 checkout（见风险 R1）；
3. **不要改用远程 BuildKit / git context 构建**——submodule 不会被自动拉取，会静默产出缺前端或构建失败的镜像；
4. 复核 `.dockerignore` 未把 `app/web`、`packages` 下必需文件排除（当前无相关排除，改动后需回归）。

Run:
```bash
git submodule update --init --recursive
docker build -t gospeak-test .
```
Expected: 镜像内 `/app/static/index.html` 存在，二进制不含前端资源。

- [ ] **Step 4: 改造 CI / release 产物布局**

删除 `.github/workflows/ci.yml` 与 `release.yml` 中所有向 `app/server/internal/webui/dist` 与 `app/desktop/webui/dist` 拷贝前端产物的步骤；服务端 release 产物改为 `gospeak-server-v<version>.tar.gz`（含二进制 + `static/` 目录），web 产物仍是纯 `static/` 压缩包。桌面端改为安装包内含外部前端目录，不再 `go:embed`。

Expected: actionlint 通过；release 产物列表含 server 与 web 两个可独立下载的压缩包。

- [ ] **Step 5: 桌面宿主改为外部前端目录**

`app/desktop/mygo.config.ts` 去掉 `webui/dist` 拷贝与 embed 相关注释，`buildCommand` 输出前端到安装包 `static/` 目录；`app/desktop` 中读取前端的 Go 代码改为指向该目录，缺失时给出明确错误提示（不再静默空页面）。

Run: `pnpm --filter @gospeak/desktop-app build`（或对应构建命令）
Expected: 桌面产物包含外部 `static/index.html`；启动能加载页面。

- [ ] **Step 6: 文档同步与提交**

更新 `app/server/internal/webui/AGENTS.md`（描述改为外部目录探测）、根 `AGENTS.md`（内嵌前端描述改为外部前端目录）、`app/web/AGENTS.md`（产物说明去掉 embed）、`deploy/DEPLOY.md`（补充前端独立更新与目录布局）。

```bash
git add app/server/internal/webui app/server/internal/router Dockerfile deploy .github/workflows app/desktop docs
 git commit -m "refactor: ship frontend outside binary and enable static dir updates"
```
Expected: 提交成功；`grep -r "go:embed" app/server app/desktop` 无命中。

## Task 11: 更新检测链路改为多仓库模型并新增 web 检测

**Files:**
- Modify: `app/server/internal/service/system_update_service.go` + `_test.go`
- Modify: `app/server/internal/handler/system_update_handler.go`
- Modify: `app/server/internal/router/routes/system/routes.go`
- Modify: `app/web/src/api/system.ts` + `app/web/src/api/system.test.ts` + `app/web/src/api/backgroundRequests.test.ts`
- Modify: `app/web/src/components/modal/settingModal/tab_item/about.tsx`
- Modify: `app/desktop/updater_release.go` + `app/desktop/updater.go` + `app/desktop/updater_release_test.go`
- Create: `docs/superpowers/specs/2026-11-26-update-check-design.md`

**Interfaces:**
- Consumes: Task 9 的四组件 metadata 与 tag 命名；Task 10 的去 embed 产物布局。
- Produces: server / web / desktop / mobile 各自从对应仓库检查最新 Release；前端提供 web 更新检测入口。

**仓库与 tag 映射（单一事实源，写入设计文档）：**

| 组件 | 仓库 | Release tag 前缀 | 当前版本来源 |
|---|---|---|---|
| server | `NoelOrin/GOSpeak` | `gospeak-server-v`（如 `gospeak-server-v1.2.3`） | `buildinfo.Version` |
| web | `NoelOrin/GOSpeak-Web` | `gospeak-web-v`（如 `gospeak-web-v1.0.0`） | 前端构建期常量或服务端下发 |
| desktop | `NoelOrin/GOSpeak-Desktop` | `gospeak-desktop-v` | 桌面宿主版本 |
| mobile | `NoelOrin/GOSpeak-Mobile` | `gospeak-mobile-v` | 移动壳版本 |

- [ ] **Step 1: 写多组件更新检测测试**

重写 `system_update_service_test.go`：为四个组件分别提供 stub GitHub API 响应，验证各自 tag 前缀过滤正确、互不影响；新增 web 组件检测用例；验证 desktop 旧逻辑（从主仓库读 `desktop-v*`）不再使用。

Run: `go test ./internal/service/... -run TestSystemUpdate`
Expected: FAIL（服务仍只支持单仓库）。

- [ ] **Step 2: 实现多仓库检测服务**

把 `SystemUpdateService` 改造为组件感知：配置表含仓库 API URL 与 tag 前缀；`Check(ctx, component)` 返回该组件的 `UpdateInfo`（Current/Latest/Available/Notes/URL）；每个组件独立缓存与负缓存，避免互相污染。web 的当前版本由服务端下发的 `web_version`（从 metadata 或独立版本文件读取）提供。

Run: `go test ./internal/service/... -run TestSystemUpdate`
Expected: PASS。

- [ ] **Step 3: 扩展 handler 与路由**

`CheckUpdate` 接受 `?component=server|web|desktop|mobile`（默认 server）；`CurrentVersion` 同样按组件返回。`routes.go` 保持两个端点不变，仅通过 query 参数区分组件。保留 admin 防御性校验。

Run: `go test ./internal/handler/... ./internal/router/...`
Expected: PASS。

- [ ] **Step 4: 桌面 updater 改查独立仓库**

`app/desktop/updater_release.go` 的 `githubAPILatest` 改为 `https://api.github.com/repos/NoelOrin/GOSpeak-Desktop/releases/latest`，tag 过滤改 `gospeak-desktop-v*`，资产名前缀同步；`releasePageURL` 改为 GOSpeak-Desktop releases 页面。更新测试 fixture。

Run: `go test ./app/desktop/...`（或对应包路径）
Expected: PASS；`grep -r "NoelOrin/GOSpeak/releases" app/desktop` 无命中（除注释）。

- [ ] **Step 5: 前端新增 web 更新检测入口**

`app/web/src/api/system.ts` 增加 `checkSystemUpdate(component)` 与 `getCurrentVersion(component)`；`about.tsx` 在非桌面环境下展示「前端版本」与「检查前端更新」按钮，调用 `component=web`；桌面环境下更新检查走宿主 updater（已改为查 GOSpeak-Desktop）。更新相关测试。

Run: `pnpm --filter @gospeak/web test`
Expected: PASS；`pnpm --filter @gospeak/web typecheck` PASS。

- [ ] **Step 6: 编写设计文档与提交**

创建 `docs/superpowers/specs/2026-11-26-update-check-design.md`，覆盖仓库/tag 映射表、四组件独立缓存、web 检测入口、桌面 updater 迁移、降级策略（仓库不可达时展示 releases 页面链接）。

```bash
git add app/server/internal/service app/server/internal/handler app/server/internal/router app/web/src/api app/web/src/components/modal/settingModal/tab_item app/desktop docs/superpowers/specs/2026-11-26-update-check-design.md
 git commit -m "feat: multi-repo update checks with web frontend detection"
```
Expected: 提交成功。

## Task 12: 发布链路改造为一组件一 Release（各在自己仓库）

> **前置已完成**：主仓库「一组件一 Release」的通用部分（`create-release` 按组件取 tag、删除 `desktop_included`、`--latest` 分流、`docs.yml` 组件守卫）已由 [`2026-11-26-per-component-release.md`](./2026-11-26-per-component-release.md) 落地。本 Task 只处理**子仓库侧**（三仓各自的 release.yml、产物上传与正文渲染）与主仓库 server-only 收窄。

**Files:**
- Modify: `.github/workflows/release.yml`（resolve-tag 解析四组件；统一 Release 入口；按组件消费子仓库产物）
- Modify: `.github/workflows/version-bump.yml`（发布顺序 / 依赖编排）
- Modify: `scripts/release-naming.mjs` + test（四组件解析；子仓库 tag 命名与主仓库同一派生规则）
- Modify: `scripts/check-version.mjs` + test（tag 事件校验发布 tag 只来自主仓库）
- Create: `docs/superpowers/specs/2026-11-26-release-pipeline-design.md`

**Interfaces:**
- Consumes: Task 7（CI 归属）、Task 9（metadata + tag 规则）、Task 10（去 embed 产物布局）、Task 11（更新检测 tag 映射）。
- Produces: 四仓各自可追溯的发布入口与产物版本绑定；主仓库只发布 server，其余组件在各自仓库发布。

**已完成（由前置计划修掉，勿重复处理）：**
- 「一个版本只创建一个 GitHub Release」的注释与统一 Release 逻辑——已删除；
- `create-release` 硬编码 `tag server`——已改为 `needs.resolve-tag.outputs.tag`；
- `desktop_included` 注入桌面下载表格——已删除；
- `version-bump.yml` 的 lockstep 同版本约束——已删除；
- `docs.yml` 对桌面 Release 的误触发——已加 `gospeak-server-v` 守卫。

**仍需本计划处理（拆仓相关）：**
- 产物上传目标仍指向主仓库 tag（`release.yml` 的 3 处 `TAG:`），拆分后 desktop/mobile 产物要上传到**各自仓库**的 tag；
- 主仓库 `release.yml` 需收窄为 server-only（删除 desktop / mobile 构建与发布 job，整体迁移到子仓库）；
- `build-mobile.yml` 的固定 pre-release `gospeak-mobile-test` + `--clobber` 仍存在，迁移到 GOSpeak-Mobile 后改为版本化正式 Release；

**目标模型：一组件一 Release，各在自己仓库。**

```
主仓库（GOSpeak）
  server tag gospeak-server-v*  → 主仓库 release.yml → 主仓库 Release（二进制 + static/）
  proxyrules module tag          → 仅 Go module，无 Release

GOSpeak-Web
  web tag gospeak-web-v*        → 本仓 release.yml → 本仓 Release（前端压缩包）

GOSpeak-Desktop
  desktop tag gospeak-desktop-v* → 本仓 release.yml → 本仓 Release（dmg/deb/exe/install.sh）

GOSpeak-Mobile
  mobile tag gospeak-mobile-v*  → 本仓 release.yml → 本仓 Release（APK/IPA）
```

**产物版本绑定：**
- server 包的 `static/` 来自 tag commit 上的 `app/web` gitlink 所指版本；release.yml 读 gitlink 的 commit，去 GOSpeak-Web 找对应 tag 的 Release 附件（`gospeak-web-v<version>.tar.gz`）下载解压，禁止浮动引用分支。
- web 版本未变而 server 变：server 包复用当前 gitlink 对应的 web 版本，Release 正文标注 `bundled-web: <version>`。
- web 变而 server 未变：只发 web 独立 Release；运维可单独热更新前端，无需动 server。

**预发布 / rc 规则：**
- 四组件 tag 允许 `-rc.N` 预发布段，**rc 直接进入 latest 推荐**（调整策略，见 Task 13）：`system_update_service` 不过滤 rc；`release.yml` 不传 `--prerelease`；`build-mobile.yml` 去掉 `--prerelease`。
- SemVer 比较规则不变：`compareVersion` 仍判定同版本正式版 > rc（`1.2.3 > 1.2.3-rc.1`）；当 rc 是最高版本（如 `2.0.0-rc.1` > `1.9.0`）时即成为 latest，正式版发布后自动让位。
- 同一组件的 rc 与正式版可并存于 Releases 页，但 tag 唯一；正式版发布不删除 rc。
- 文档站部署是否跟随 rc latest 由 `docs.yml` 独立决定（当前 rc 不部署文档，保持与推荐版本解耦）。

- [ ] **Step 1: 写发布链路测试**

在 `scripts/release-naming.test.mjs` / `check-version.test.mjs` 增加：四组件 tag 解析（server/web/desktop/mobile）、不带组件前缀的裸 tag（如 `v1.0.0`）被拒绝识别为发布 tag、`resolve-tag` 默认取最新 tag 时四组件均被识别并按所属仓库路由。

Run: `node --test scripts/release-naming.test.mjs scripts/check-version.test.mjs`
Expected: FAIL（当前只识别两组件）。

- [ ] **Step 2: 主仓库 release.yml 收窄为 server-only**

第 1–3 项（统一 Release 注释、硬编码 `tag server`、`desktop_included`）已由前置计划完成，本步复核即可。仍需执行：
1. 主仓库 `release.yml` 只保留 `if: needs.resolve-tag.outputs.component == 'server'` 的构建分支；web/desktop/mobile 的构建与发布**整体迁移到各自仓库的 `release.yml`**；
2. `resolve-tag` 的 `grep -E '^gospeak-(server|desktop)-v[0-9]'` 改为只匹配 `^gospeak-server-v[0-9]`（主仓库只管 server）；
3. 产物上传目标由主仓库 tag 改为按组件所属仓库的 tag。

Expected: 主仓库 `release.yml` 中不再出现 desktop/mobile 构建 job；`grep -n "desktop_included" .github/workflows/release.yml` 无命中。

- [ ] **Step 3: 子仓库各自建 Release**

在三个有 Release 的子仓库（Web / Desktop / Mobile）的 `release.yml` 中分别实现：
- 触发：`on.push.tags: ['gospeak-web-v*']` / `['gospeak-desktop-v*']` / `['gospeak-mobile-v*']`
- 创建 Release：`gh release create "$TAG" --title "..." --notes-file ...`（tag 即本仓 tag）
- 上传产物：web 传 `.tar.gz`，desktop 传安装包 + SHA256SUMS，mobile 传 APK/IPA

Expected: 三个子仓库各自 Releases 页出现本组件的独立条目，产物不再混入主仓库 Release。

- [ ] **Step 3b: 复核 version-bump 的按组件判定**

`Require lockstep component versions` 步骤已在前置计划中删除。本步只需确认 `Detect published tags` 对每个组件独立判断 tag 是否存在，且拆仓后（tag 分散在四个仓库）仍按所属仓库分别查询。

Run: `grep -n "lockstep" .github/workflows/version-bump.yml`
Expected: 无命中。

- [ ] **Step 3c: 迁移 build-mobile 到正式 Release**

把 `build-mobile.yml` 的「固定 pre-release `gospeak-mobile-test` + `--clobber` 覆盖」改为：由 `gospeak-mobile-v*` tag 触发，`gh release create "$TAG"` 创建正式 Release，产物以版本化文件名上传（不覆盖）。该 workflow 迁移到 GOSpeak-Mobile 仓库。

Expected: 移动产物出现在 `gospeak-mobile-v<version>` 的独立 Release 中；不再有 `gospeak-mobile-test` 固定 tag 写入。

- [ ] **Step 4: server 包从 web Release 取前端产物**

主仓库 `release.yml` 构建 server 时：从 tag commit 读 `app/web` gitlink 的 commit SHA → 查 GOSpeak-Web 中该 commit 对应的 `gospeak-web-v<version>` tag → 用 `gh release download` 拉压缩包 → 解压进 `static/` 后打进 server 产物。

**失败处理（风险 R9，必须显式）**：
- web Release 不存在 / 附件缺失 / 下载失败 → **job 直接失败并输出「先发布 gospeak-web-v<version>」**，不要产出缺前端的 server 包；
- 若确实需要「只发 API 不带前端」的应急包，必须走独立的手动开关（如 `workflow_dispatch` 的 `include_web: false`）并在 Release 正文显式标注 `bundled-web: none`；
- 校验 `static/index.html` 解压后存在，否则失败。

Expected: server Release 附件 `static/index.html` 与该 web 版本产物一致；发布说明含 `bundled-web: <version>`。

- [ ] **Step 5: 编写发布链路设计文档与提交**

创建 `docs/superpowers/specs/2026-11-26-release-pipeline-design.md`，覆盖：**一组件一 Release（各在自己仓库）**、四个 Release 的触发与产物清单、server 包的前端产物绑定方式、rc 规则、以及「Releases 页面按仓库天然分开」的验证方法。

```bash
git add .github/workflows/release.yml .github/workflows/version-bump.yml scripts/release-naming.mjs scripts/release-naming.test.mjs scripts/check-version.mjs scripts/check-version.test.mjs docs/superpowers/specs/2026-11-26-release-pipeline-design.md
 git commit -m "ci: one release per component in its own repository"
```
Expected: 提交成功。

## 发布链路 review 补丁（release-reviewer 结论，2026-11-26）

> 本节 10 条均已对照真实代码取证。**标记为阻塞的 3 条会让发布链路上线即坏**，必须在 Task 12 之前处理。

### 阻塞

**P1. web 存在性校验位置错误 → 会留下「标着 latest 的空 Release」，并永久锁死该版本。**

现状：`release.yml` 的 `create-release` 只 `needs: [resolve-tag, version-check]`，创建时就写 `--latest`，此时一个附件都没有；真正构建在 `build-release-binaries`（`needs: [create-release, ...]`）。计划 Task 12 Step 4 把 web 校验放在构建阶段 = 先发空 Release 标 latest 再失败。

更严重的是 tag 先于发布：`version-bump.yml` 先 `git push` tag，之后每次都以「tag 存在」判定已发布 → `emit_tag` 输出空 → `release-server` 永不执行，`workflow_dispatch` 也跳过，只能人工救。

改法：
- 新增 `preflight-web` job（`needs: [resolve-tag]`，在 `create-release` 之前），`gh release view/download` 校验 `gospeak-web-v<ver>` 存在且 `index.html` 可解压；`create-release` 增加 `needs: [..., preflight-web]`；
- 幂等键从「tag 存在」改为「Release 存在且必备资产齐全」，否则任何一次发布失败都会让该版本在自动化里永久失效。

**P2. 「gitlink 所指 commit 的 web tag」不成立——gitlink 推进没有任何「必须指向已打 tag 的 commit」约束。**

计划写「server 包前端来自 gitlink 所指版本」，但 gitlink bump 用的是 `git submodule update --remote`（跟随分支 HEAD）与「改 packages 就三处 bump」，**没有任何步骤要求 gitlink 只指向打了 `gospeak-web-v*` 的 commit**。实测 web 侧 tag 数为 **0**，抽取后从零开始，Task 6B/6C 那几次 bump 的 commit 必然没有 tag。

改法：
- server 包前端绑定源从 gitlink 改为 metadata 显式声明 `web.bundled_version`（配 `web.commit` 校验），**server 发布只看 metadata，不看 gitlink**；
- gitlink bump 加 preflight：`git ls-remote --tags <web> | grep <target-sha>`，无 tag 则要求先发 web 或拒绝 bump。

**P3. 历史 tag 零迁移方案 + metadata 示例版本低于现网 → 桌面端对已装客户端永久失效。**

实测：22 个 tag 全在主仓（server 11 / desktop 11），`gospeak-mobile-*` 与 `gospeak-web-*` 均为 **0**。现网版本：`desktop.yaml`=`1.3.0-rc.1`、`mobile.yaml`=`1.1.0-rc.2`、`VERSION`=`1.3.0-rc.1`；而计划 metadata 示例写 desktop `1.1.0`、mobile `0.9.0`、server `1.2.0` —— **全部低于现网，照抄即回退**。

后果链：新 GOSpeak-Desktop 仓 tag/release 为 0 → `updater_release.go` 返回「无更新不报错」；首个新仓 Release 若按示例发 `1.1.0` → `compareReleaseVersion("1.1.0","1.3.0-rc.1") < 0` → **永远不提示更新**，直到版本号超过 `1.3.0-rc.1`。

改法：
- 新增「tag 迁移表」：抽取后在各子仓对**当前已发布版本**补打 annotated tag（至少 `gospeak-desktop-v1.3.0-rc.1`、`gospeak-server-v1.3.0-rc.1`、`gospeak-mobile-v1.1.0-rc.2`）；
- metadata 初始值**直接取现网值**，不要用示例值；
- `checkMetadataConsistency` 加硬约束 `version >= max(远端已有 tag 版本)`，防止版本回退。

### 高

**P4. Task 9 自相矛盾：tag 到底在哪个仓打。**
计划同时写「annotated tag 打在**主仓库**更新 metadata+gitlink 的那次提交」（L1295）与「各组件在自己的仓库打 tag」（L1296），二者不可能同时成立；且 Task 9 Step 8 的流程仍是主仓单仓模型（`git tag → workflow_call release.yml`），与 Task 12 Step 3「子仓 `on.push.tags` 触发」直接冲突。
改法：删掉「主仓打 tag」分支；server tag 留主仓，web/desktop/mobile 的 tag 由各子仓自己的版本门禁打；Step 8 的 dry-run 必须覆盖「两个仓各打一次 tag」的真实路径。

**P5. 子仓 Release 完全没有版本门禁，且 `check-version.mjs` 在子仓跑不通。**
`check-version.mjs` 的 `declaredPathFor` 写死 `<root>/.github/versions/<component>.yaml`（Task 9 要删），`checkDesktop` / `checkMobile` 读 `<root>/app/desktop/...`、`<root>/app/mobile/...` —— 拆仓后子仓根就是原目录内容，这些路径全不存在。且 `normalizeComponent` 白名单只有 `server|desktop|mobile`，**web 连参数都不收**。
改法：
- `check-version.mjs` 加 `--repo-root` 组件路径映射表（子仓根 ↔ `app/<component>`）；
- 把 `release-naming.mjs` + `check-version.mjs` + `metadata.mjs` 明确列入「随仓分发脚本」（R12 只列了 lefthook/biome/tsconfig/turbo，漏了脚本）；
- 每个子仓 `release.yml` 首步加 `node scripts/check-version.mjs . "$TAG" <component>`。

**P6. 两个 `compareVersion` 实现，桌面端按字典序 → `rc.10` 不会推荐给桌面端。**
服务端 `comparePrerelease` 按 SemVer 数值比较（正确处理 `rc.10 > rc.9`）；桌面端 `compareReleaseVersion` 最后一行是 `strings.Compare(pa.pre, pb.pre)` 纯字典序，`"rc.10" < "rc.4"`。Task 13 只改服务端。
改法：桌面端换成数值比较并补 `rc.10 vs rc.9` 单测；Task 13 Step 1 增加跨端一致性测试（同一组 tag 序列在两侧结论必须相同）。

**P7. Task 13 Step 3 删错了对象。**
`build-mobile.yml` **已经有**版本化正式路径（`gh release create "$TAG" --latest=false`，还带 `check-version.mjs . "$tag" mobile` 门禁），Task 12 Step 3c 描述的「改成正式 Release」其实已完成。`--prerelease` 只出现在**测试包路径**（固定 tag `gospeak-mobile-test` + `--clobber`），删它会让被反复覆盖的 debug 包去抢 latest，是净负面。真正挡住「rc 可 latest」的是 official 路径的 `--latest=false` 与 `release.yml` 里非 server 组件一律 `--latest=false`。
另：`gospeak-mobile-v*` 实测 **0 个** tag，而 `version-bump.yml` 的 mobile 打 tag 逻辑早已存在 → 该路径**从未成功产出过 tag**，拆仓前必须先演练并排查失败原因。
改法：official 路径去掉 `--latest=false`；test 路径保留 `--prerelease` 但固定 tag 改为 `mobile-test-<date|commit>`；验收改 `grep -n 'latest=' build-mobile.yml release.yml` 人工逐行确认；Task 12 Step 3c 改写为「复核 official 路径 + 修头注释 + 先在主仓演练一次 mobile tag 全链路」。

**P8. `mobile-version.mjs` 会写 `app/web/package.json` → 跨组件污染。**
该脚本 `replaceAll('<root>/app/web/package.json', ...)` 把 mobile 版本写进 web（注释说供 `__APP_VERSION__`）。现网已漂移：web `package.json`=`1.3.0-rc.1` vs `mobile.yaml`=`1.1.0-rc.2`，**今天跑一次 `version:sync` 就会把 web 版本改成 `1.1.0-rc.2`**。拆仓后它在主仓运行会改 submodule 工作区 → 主仓 dirty，与 R13 的 dirty 校验、Task 8 的 clean 验收打架。
改法：删除该写入；web 版本由 web 仓 `package.json` = metadata.web 独立维护；`check-version.mjs` 增加 web 校验 `app/web/package.json == metadata.web.version`；Task 7 脚本清单注明它是 **writer** 而非 reader。

### 中

**P9. web 更新检测的「当前版本」来源从头到尾没落地，且天然永久误报。**
`__APP_VERSION__` 是构建期常量（取自 web 仓 `package.json`）；Task 10 后前端可热更新，版本与实际部署 dist 不一致 → `Available` 永远 true 或永远 false。计划只写「从 metadata 或独立版本文件读取」，metadata 是开发期文件不进 static 包，等于没定。
另：`/update/current` 与 `/update/check` 都是 `RequireRole("admin")` 且有二层 admin 校验；Task 11 Step 5 要求非桌面环境展示「检查前端更新」→ 普通用户点下去就是 403。
改法：web 构建产出 `dist/version.json`（`{version, commit, builtAt}`），server 读 `STATIC_DIR/version.json`，缺失返回 `unknown` 而非猜；端点拆成公开只读 `/system/version` 与 admin-only 的 update check。

**P10. `release-notes.mjs` / `build-changelog.mjs` 仍是单仓硬编码，web 会静默渲染成 server 正文。**
`renderReleaseBody` 只有 desktop/mobile 两个分支，其余**一律 fall through 到 server**；CLI 白名单 = `["server","desktop","mobile"]`（实测确认无 web）。`build-changelog.mjs` 的 `REPO` 写死主仓，在子仓生成 CHANGELOG 的 compare 链接会 404；且只有 `--server`/`--desktop`。
改法：`release-notes.mjs` 未知组件直接 `throw`，补 web 分支；`build-changelog.mjs` 的 `REPO` 改为 `process.env.GITHUB_REPOSITORY ?? git remote get-url origin` 派生，`--component` 参数化；两处补 web/未知组件测试。

### 顺带修正的低优先级事实错误

- Task 12 Step 2 第 2 项要求改 `grep -E '^gospeak-(server|desktop)-v[0-9]'`，但**该 grep 已不存在**（实测无命中），属陈旧描述，删除即可；
- Task 9 Step 1 期望「mobile 与 web 都不在 COMPONENTS 中」，实际 `release-naming.mjs` 已是 `["server","desktop","mobile"]`，只有 web 会 FAIL，修正措辞。

---

## CI 与 Actions review 补丁（ci-reviewer 结论，2026-11-26）

> 与「发布链路 review 补丁」互补：上一节管发布语义，本节管 workflow 级正确性。**CI1 / CI2 为阻塞**。

### 阻塞

**CI1. 主仓 `version-bump.yml` 只认 `origin`=主仓，拆仓后 desktop/mobile/web 永远不会产出 Release。**

证据：`version-bump.yml:88` `git ls-remote --tags origin`、`:143/:155/:167` `git push origin refs/tags/...`、`:209-237` `workflow_call` 只调本地 workflow。而 Task 12 Step 2 已把主仓 release 收窄成 server-only → 那些 job 全被 `if` 跳过；子仓 `on.push.tags` 因没人往子仓推 tag 也永不触发。计划 `:1626` 只说「按所属仓库分别查询」，**没说往哪个仓库推 tag**。

改法（与发布链路补丁 P4 合并处理）：
- 主仓 `version-bump.yml` 把四组件分两类：`server` 走主仓 tag + 本仓 release；`web/desktop/mobile` **只做只读探测**（`git ls-remote --tags https://x-access-token:$TOKEN@github.com/NoelOrin/GOSpeak-Web.git`），缺失就 fail 并提示「到 GOSpeak-Web 发版」，**不在主仓打 tag**；
- tag 创建者写死一处（推荐：子仓 `release.yml` 用 `workflow_dispatch(inputs.version)` 打 tag 并建 Release）；
- 删掉「tag 打在主仓提交上」这条与「各仓自打」冲突的规则。

**CI2. `ci.yml` 删 4 个 job 后的引用点没列全 → workflow 非法或 gate 常红。**

证据（计划只写「删除 job 及 gate 中对应 needs」）：
```
ci.yml:188  needs: [changes, build-js]          # build-server 依赖被删的 build-js → GitHub 判 workflow 非法，整个 CI 不启动
ci.yml:204-205  Download web dist / download-artifact(name: web-dist)   # 产物来自 build-js
ci.yml:457  needs: [..., build-js, ..., test-web, test-packages, ..., desktop-go, ...]
ci.yml:464-472  BUILD_JS_RESULT / TEST_WEB_RESULT / TEST_PACKAGES_RESULT / DESKTOP_GO_RESULT
ci.yml:481  for result in ... "$BUILD_JS_RESULT" ...; case success|skipped) ;; *) exit 1
```
即使只改 needs，`gate` 的 `env` 与 for 循环仍引用被删 job → `needs.X.result` 为空串 → 落 `*` 分支 → `exit 1` → **gate 永远红**。另：`gate` 的 job 名是 `CI Gate`（`ci.yml:453`），不是 `gate`。

改法：Step 5 补三条明确条目——① `build-server` 改 `needs: [changes]`，并按 Task 10 删 `Download web dist` / `Embed frontend into server`；② `gate.needs` 删四项；③ `gate.steps[0].env` 删四个 `*_RESULT` 并同步删 for 循环变量。Expected 写 `grep -n "needs.build-js\|needs.test-web\|needs.desktop-go" .github/workflows/ci.yml` 无命中。

### 高

**CI3. `SUBMODULES_TOKEN` 作用域过宽会打断主仓 checkout 与 tag 推送。**
- fine-grained PAT 若只给四子仓 `contents: read`，用它 clone **主仓自身**会 404；
- `persist-credentials` 默认 true 会把 token 写进 `.git/config`，之后 `git push origin refs/tags/...` 全用它 → 403，workflow 级 `permissions: contents: write` 失效；
- fork PR 里 secret 为空串 → `changes` job 第一个 checkout 就 403，**外部贡献者门禁全红**。

改法：① 只给真正需要子仓内容的 job 加（拆仓后实际是 `build-server`、`protocol-check`、`lint` 的跨仓脚本、`docker.yml`、`release.yml`），`changes`/`gate`/`workflow-lint`/`docs.yml`/`security.yml`/`version-bump.yml` **不加**；② `version-bump.yml` 显式 `persist-credentials: false`；③ PAT 必须同时含主仓与四子仓 `Contents: read`；④ fork PR 用 `if: github.event.pull_request.head.repo.full_name == github.repository` 守卫，否则退回 `submodules: false` 并明确 skip。

**CI4. 跨仓 `workflow_call` 权限模型不成立（计划未定稿）。**
被调 workflow 的 `GITHUB_TOKEN` 仍是**调用方**仓库、权限只能降级不能提升；私有被调仓还需在被调仓 Settings→Actions 显式放行调用方，且 `secrets: inherit` 仅同 org。结果：主仓调 `GOSpeak-Web/release.yml` 时 `gh release create` 拿主仓 token → 上传资产 403。
改法：二选一并写死——**A（推荐）**不做跨仓调用，子仓只由本仓 tag/dispatch 触发，主仓只探测+提示；**B** 若跨仓调用，则写明可写 `RELEASE_TOKEN`、被调仓 Access policy、`secrets: inherit`、被调 workflow 内 `GH_TOKEN` 显式注入，并加 `gh release list --repo` smoke。

**CI5. 子仓 CI 要 checkout 嵌套 `packages`，但凭据只在主仓配了（R1 未覆盖）。**
`GOSpeak-Packages` 是 private，被嵌套挂在 GOSpeak-Web 与 GOSpeak-Mobile；而 `gh secret set` 只写进了主仓。
改法：Task 2 Step 4 改为**四仓各配一份** `SUBMODULES_TOKEN`；Task 7 Step 6 的 Web/Mobile 条目补 `checkout with: submodules: recursive, token: ${{ secrets.SUBMODULES_TOKEN }}`；Expected 写成「在 GOSpeak-Web 开一个只改 README 的 PR，CI 能完成 `pnpm install --frozen-lockfile`」。

**CI6. 删 `desktop-go` 会连带删掉 proxyrules 绑定面阻断门禁。**
`mobile-bind-proxyrules.mjs` 在 CI 的**唯一阻断性调用**就是 `ci.yml:407-414`（注释明写「阻断性校验必须留在这里，否则门禁形同虚设」），另一处在 `build-mobile.yml:85`（随 workflow 一起删）。删 `desktop-go` 后主仓与子仓都不会再跑 `--surface-only`，绑定面漂移变静默。
改法：在 `test-scripts`（或新建轻量 `proto-bind-check` job）保留 `node scripts/mobile-bind-proxyrules.mjs --target=all --surface-only` 并进 `gate.needs`；Task 6C 里「脚本迁入 mobile 仓」改为「mobile 仓只保留 gradle/sync，绑定面校验仍回主仓 CI」，并把脚本归属写进 CI 归属矩阵。

**CI7. `docs.yml` 的 sfu-client 触发路径会失效。**
`docs.yml:25-29` 与 `:90` 的 `docs_path_regex` 以 `packages/sfu-client/**` 为触发条件，拆仓后该路径只剩 gitlink，改 sfu-client 不再触发文档重建。
改法：改为 gitlink `packages`，或明确「sfu-client 文档改动不再自动部署 docs」并写进文档。

### 中

**CI8. 安全扫描覆盖面丢失。**
`security.yml` 的 `pnpm audit --prod` 只在主仓根 workspace 运行 → 拆仓后 web/mobile/packages 依赖完全不被审计；`govulncheck` 只覆盖 `app/server`，desktop Go module 扫描随 `desktop-go` 消失。
改法：CI 归属矩阵加一行 `Security（audit / govulncheck / dependency-review）`，每仓 `ci.yml` 加 `pnpm audit --prod`；`security.yml` 的 paths 删前端条目只留 server。

**CI9. `.gitmodules` 不在任何 paths-filter 里，只改它时门禁全绿。**
`ci.yml:45-74` 三个过滤器（go/js/ci）均无 `.gitmodules`，而 Task 5/6B 的核心改动恰恰是它。仅改 `.gitmodules` 时 go/js/ci 全 false → `gate` 按 skipped 接受 → 绿。
改法：`ci` 过滤器加 `- '.gitmodules'`（顺带 `- '.github/dependabot.yml'`），Expected 加 `grep -n gitmodules .github/workflows/ci.yml`。

**CI10. 计划自带的 actionlint 步骤在 macOS 上跑的是 Linux 二进制。**
`plan:1018` 下载 `actionlint_1.7.7_linux_amd64.tar.gz` 直接执行 → 本机 macOS 会 `exec format error`。
改法：按 `uname -sm` 选包，或直接 `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/`（Go 已就绪），Expected 写「退出码 0」。

### 顺带修正与确认

- **gitlink 的 paths-filter 写法本身正确**（已实测）：gitlink 变更以**无斜杠**路径出现，`app/web` 能匹配、`app/web/**` 不匹配；仅 Expected 里「三项 gitlink」应改「四项」（`app/desktop` 在 `go` 过滤器）。
- **复合 action 可直接复制**（已核实无写死主仓路径），但 `go-build-cache` 的 `go-sum-path` 在子仓要改成 `go.sum` / `web/go.sum`。
- **actions 版本全部可用**：checkout v7.0.1 / setup-go v7.1.0 / setup-java v6.3.0 / gradle-actions v6.4.1 / upload-artifact v7.0.2 / download-artifact v8.0.2 / paths-filter v4.0.3 / pnpm-action-setup v6.1.1。
- **带 checkout 的 workflow 只有 6 个**（ci 13 处、release 9、docker 2、docs 2、security 2、version-bump 1），Task 7 Step 4 清单文件级无遗漏。
- **分支保护的 required check 名不是 job id**：GitHub 取 job 的 `name`（主仓 `gate` 的 name 是 `CI Gate`）。Task 2 Step 3 里 `contexts:["gate"]` 照抄会永远停在 Expected；应改为「子仓 gate job 不写 `name:`（则 id 即 check 名）或写全名」。

---

## Git 操作 review 补丁（git-reviewer 结论，2026-11-26）

> 本节全部为**真实仓库实测复现**（`/tmp` 临时 clone 跑计划里的原命令）。**G1/G2 为阻塞，且 G1 会让 Task 3 的四个推送全部失效。**

### 先纠正三处事实前提（影响判断，但不改结论）

- 全仓 **169** 个提交（HEAD 分支 111），不是大仓；`app/web` 目录相关提交 **34** 个；
- `git tag | wc -l` = **22**（11 `gospeak-server-v*` + 11 `gospeak-desktop-v*`）；
- `git-filter-repo` **是装了的**（`/opt/homebrew/bin/git-filter-repo`），但 Python 解释器缺失不可用（`bad interpreter: /opt/homebrew/opt/python@3.14/...`）。修好解释器后可优先用它（能正确处理 tag、支持多路径一次抽取）；filter-branch 作兜底。**性能担忧不成立**：169 提交量级下 `--subdirectory-filter` 实测 4.65s。

### 阻塞

**G1. Task 3 的 `git remote add origin` 必然失败 → 四个子仓推送全部失效。**

`git clone` 已把 `origin` 占给本地路径，`git remote add origin <新仓>` 直接 `error: remote origin already exists`（exit 3）。若执行者跳过该行直接 `git push -u origin release`，推的是**本地主仓路径**而非新子仓。

改法（已落进计划）：`git remote set-url origin <新仓>`；并在 Expected 加硬校验：
```bash
git remote get-url origin | grep -q 'GOSpeak-Web.git'   # 四条都要过
```

**G2. Task 6B 重写 web 历史后 `git push origin release` 必被拒（非快进），且主仓 gitlink 会指向「子仓已不存在的 commit」。**

`--tree-filter` 重写后每个提交 SHA 都变，与 Task 3 已推的 `release` 无祖先关系（实测 `git push` → `! [rejected] (non-fast-forward)`）。更糟：Task 5 已把主仓 gitlink 提交到**重写前**的 SHA，force-push 后该 commit 在任何 ref 上都不再可达 → 之后 `git checkout <Task5 提交> && git submodule update --init` 会 `fatal: ... did not contain <sha>`，bisect 与旧 commit 重跑 CI 全废。

改法（二选一）：
- **推荐**：把目录重排**并进 Task 3 的同一次抽取**——在同一个临时 clone 里连跑两遍 rewrite（先 `--subdirectory-filter app/web`，再 `--tree-filter` 移进 `web/`）后再首次 push，主仓永远不会引用被重写的 SHA；
- 若坚持分两步：Step 5 必须 `git push --force-with-lease origin release`，并在设计文档写明「web 仓在首次被主仓 gitlink 引用前允许 force-push，之后禁止重写历史」；且分支保护必须在 Task 6B 完成后再开。

### 高

**G3. 子仓没有任何 `.gitignore`，Task 6B Step 5 的 `git add web` 会把 `node_modules`、`dist`、`routeTree.gen.ts` 提交进去。**

`app/web/.gitignore` **不存在**；忽略规则全在主仓根 `.gitignore`（`node_modules`、`**/dist/`、`.turbo/`、`app/web/src/routeTree.gen.ts`、`app/desktop/mygo.ts` 等），`--subdirectory-filter` 不会把它们带进子仓。而 Step 4 正好在 Step 5 之前跑 `pnpm install` + `build`，工作区必然有 `web/node_modules` 与 `web/dist`。实测 `git add -n web` 会列出这三类文件。

改法：Task 6B/6C 各加前置 Step——按子仓布局**先写子仓根 `.gitignore`**（从主仓根裁剪，**不要**带 `app/web/pnpm-lock.yaml`），再提交；Expected 断言 `git ls-files web | grep -c node_modules` 为 0。

**G4. Task 6C / Task 8 引用的四个临时目录从来没有 `git clone`，且 6C 全程没有 commit/push。**

全文只有 Task 3 与 Task 6B 出现 `git clone`；`/tmp/gospeak-packages-setup`、`/tmp/gospeak-mobile-setup`、`/tmp/gospeak-web-verify`、`/tmp/gospeak-packages-verify` 全部无创建来源。6C 产出的正是 Packages 的 `pnpm-workspace.yaml`/根 `package.json`/lockfile 与 Mobile 的 workspace 配置，却没有一条 `git commit`/`git push` → Task 8 Step 4 的 `pnpm install --frozen-lockfile` 在远端必然失败。

改法：6C 两个 Step 开头补 `git clone <url> <dir>`；各加 `git add`（含新 `.gitignore`）+ `git commit` + `git push origin release`；Task 8 的三个 verify 目录统一加 `rm -rf` + `git clone --recursive`。

**G5. `.gitmodules` 的 `branch = release` + `submodule update --remote` 会让 gitlink 漂到无 tag 的 commit，击穿「gitlink 上必有对应 tag」前提。**

实测：packages 仓 `release` 前进一格后 `git submodule update --remote packages` → gitlink 改成远端 HEAD。这与「三处 gitlink 显示同一 commit」和「server 包取 gitlink 所指 web commit 上的 tag」直接冲突（与发布补丁 P2 同根因）。

改法：Task 6B Step 6 改为确定性写法 `git -C app/web fetch --tags && git -C app/web checkout "gospeak-web-v<ver>" && git add app/web`（或 `git update-index --cacheinfo 160000,<sha>,app/web`）；文档明文**禁止 CI/脚本使用 `submodule update --remote`**；`check-version.mjs` 增加断言 `gitlink sha == git -C <sub> rev-parse <component tag>`。

### 中

**G6. `-- --all` 让 22 个 tag 指向未过滤的原始提交（含完整主仓树），存在 `push --tags/--mirror` 泄漏整仓历史的通道。**

实测重写后 `tags after: 22`，`git log -1 gospeak-server-v1.3.0-rc.1` 仍指向主仓提交且 `git ls-tree` 含 `.github`、`packages`、`AGENTS.md` 等全仓文件；`refs/original/*` 备份 ref 仍在。

改法：push 前清 ref——`git tag -d $(git tag -l)`、`git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d`；push 用 `git push origin release:release --no-tags`；建议把命令降为 `... --subdirectory-filter app/web HEAD`（只重写 HEAD，避免 tag 警告与备份 ref）。

**G7. 历史重写后 tag/Release 断代：11 个 `gospeak-desktop-v*` 留在主仓，新仓 Releases 为空 → 已装机桌面端更新检测立刻失联。**

实测 `gh release list -R NoelOrin/GOSpeak | grep -i desktop` **无命中**，桌面资产目前挂在 server Release 里（`gospeak-server-v1.3.0-rc.1` 含 7 个 dmg 等）。拆仓后新仓 `/releases` 返回空数组，1.3.0-rc.1 的在装客户端「检查更新」永久无更新。（与发布补丁 P3 同一问题的另一半：P3 讲版本号回退，这条讲 Release 记录缺失。）

改法：Task 3 或 Task 12 增加「子仓基线 Release」步骤——在 GOSpeak-Desktop 用**当前版本 1.3.0-rc.1** 建 tag + Release（可只重传已有机型资产或空附件+说明），保证首个自建 Release 版本 ≥ 1.3.0-rc.1；并让 updater 在新仓 `/releases` 为空时降级为「打开主仓 releases 页」而非报错。

**G8. Task 9 Step 11 的 metadata 落地时序与执行顺序自相矛盾。**

Step 11 要求「Task 3 抽取后、Task 5 接入前」落子仓 `metadata.yaml`，但 Task 9 在 Task 8 之后执行——此时 gitlink 已指向**不含** metadata 的 commit，再推一个提交会让 gitlink 立刻落后（拆分当天就触发 R5 的漂移），且 Task 10/11 读 `app/web/metadata.yaml` 会文件不存在。

改法：把 metadata 落地并入 Task 3/6B 的同一次子仓提交；Step 11 改为「复核存在 + 断言 `git -C <sub> cat-file -e <gitlink sha>:metadata.yaml` 成功」。

### 低

**G9. 技术依据写错**（已在上面「事实前提」纠正）：`filter-repo` 不是未安装，是解释器坏了；「1700+ 提交」应改为 169。

**G10. Task 1 的快照与备份不足以支撑历史重写。**

`git status --short` 不含 ignored 文件；全计划没有一次 `git bundle`。改法：Task 1 Step 1 补 `git status --ignored --short` 与 `git bundle create /tmp/gospeak-pre-split.bundle --all`，并 `git tag backup/pre-split-20261126 && git push origin backup/pre-split-20261126`。

### 实测确认无问题的角度

- `--prune-empty` 行为正确：web 子仓实测得到 **34** 个提交，与 `git rev-list --count HEAD -- app/web` 完全相等，无空提交、无丢失；
- 抽取后各子仓根内容符合预期（web/desktop/mobile/packages 均正确）；
- 嵌套 submodule 在 `clone --recursive` / `update --init --recursive` 下机制正确（按 gitlink SHA 检出、不读 `branch`）；
- Task 6B 的 `for f in $(ls -A …)` 分词隐患目前不触发（历史路径无空格、工作目录无 `.git`）；
- `git rm -r --cached` → `rm -rf` → `submodule add` 的顺序本身可执行，无不可恢复损失。

---

## 文档与回滚 review 补丁（docs-reviewer 结论，2026-11-26）

> 本节区分于 R4/R13：那些讲**门禁与校验**，本节讲**文档还在教人做拆仓后物理上做不到的事**，以及回滚命令本身跑不通。**D1–D3 为阻塞。**

### 阻塞

**D1. `pnpm-workspace.yaml` 收窄会连带踢掉 `@gospeak/server`（不只是 docs）。**

`app/*` 同时覆盖 **`app/server`** 与 **`app/docs`**，这两个目录**不拆走**。根 `package.json` 的 `dev`/`start:dev`/`dev:server`/`build:server`/`prod:server` 全部依赖 `@gospeak/server`。

两种失败模式不一致（实测）：
- `turbo run dev --filter=@gospeak/doesnotexist` → `x No package found`，**exit 1**（会红）；
- `pnpm --filter @gospeak/doesnotexist exec true` → `No projects matched`，**exit 0**（**静默绿**）。

即 `pnpm dev:web` 拆仓后会「成功退出但什么都没启动」。改法（已落进计划）：显式列 `app/server`、`app/docs`、`test`、`plugins/*`；Expected 加真实回归 `pnpm dev:server` 能起 air、`pnpm --filter @gospeak/docs build` 能出 dist。

**D2. Task 9/10/11/12 的 Files 与 `git add` 仍在主仓处理子仓文件 → 改动静默丢失。**

这些 Task 的 Files 写 `Modify: app/web/src/api/system.ts`、`app/desktop/updater_release.go`，`git add` 路径也是 `app/web/...`。拆仓后实测：
```
git add app/web/index.ts  → fatal: Pathspec 'app/web/index.ts' is in submodule 'app/web'
git commit -am "try"      → nothing to commit, working tree clean
```
即「提交成功」会通过（server 侧文件进了库），但 about.tsx / system.ts / updater 的改动**完全没提交**。

改法：给这些 Task 统一加**两条腿模板**——
```text
文件（子仓 GOSpeak-Web）：web/src/api/system.ts ...
文件（主仓）：app/server/...、scripts/...
步骤：① cd app/web && git switch release && ... && git commit && git push
      ② 回主仓 git add app/web && git commit   # 仅 gitlink
Expected：git -C app/web log --oneline -1 有新提交；git submodule status 中 app/web 无 +/- 前缀
```

**D3. Task 14 回滚在命令层面跑不通。**

实测三处：
1. **revert 冲突**：撤销「`git rm -r --cached` + 删目录」那条提交时，工作树里 `app/web` 已是子模块 checkout（同名文件为 untracked）→ `error: The following untracked working tree files would be overwritten by merge ... fatal: revert failed`（exit 128）；
2. **deinit 顺序反了**：revert 掉 submodule 引入提交后 `.gitmodules` 已不存在，此时再 `git submodule deinit -f <path>` → `error: pathspec 'app/web' did not match any file(s)`；且 `.git/config` 残留 `submodule.*`、`.git/modules/app` 仍在；
3. **Step 1 清单会误伤**：`git log --oneline origin/release..HEAD` 当前是 **20 条**，其中 `89705271 / 55483a29 / f46ce7b5 / 843cda72 / d3d4d68c / 32f08eb1 / ea8eb38b / 8ad449fc` 这 **8 条属于已交付的前置计划（per-component release）**，按「逆序整理成 revert 清单」会把它们连带回滚。

改法：Task 1 增加 `git tag -a pre-submodule-split <拆仓前 commit>`；Task 14 重写为固定序列：
```bash
git submodule deinit -f --all
rm -rf .git/modules/app .git/modules/packages
git config --remove-section submodule.app/web   # 其余三个同样处理
rm -rf app/web app/desktop app/mobile packages
git revert --no-commit pre-submodule-split..HEAD
git commit -m "revert: roll back submodule split"
```
Expected 改成可判定三条：`test ! -e .gitmodules`、`git config --get-regexp '^submodule\.'` 无输出、`git status --short` 为空 + `pnpm typecheck && pnpm lint && pnpm test` 通过。并明确「**不要** revert 前置计划的 8 个提交」。

### 高

**D4. Task 6 的文档重写清单严重不足：主仓内还有 13 处失效命令。**

实测清单（已排除 node_modules 与 .agents/worktrees）：`README.md:77-83`（onboarding）、`README.md:63`（「前端已内嵌」）、`docs/deployment-guide.md:34/54/62-63/233`、`deploy/DEPLOY.md:214`、`app/docs/AGENTS.md:20-21`、`app/docs/guide/desktop.md:142-148`、`app/docs/guide/faq.md:140`、`docs/e2e-test-spec.md:59`、`AGENTS.md:127/188`（「改协议四步」）、`app/server/internal/protocol/AGENTS.md:26`、`app/server/internal/genprotocol/AGENTS.md:15`、`app/server/internal/sfu/providers/AGENTS.md:20`、`app/server/internal/signal/AGENTS.md:25`。

这条与 R13 不同：R13 管「CI 有没有补偿校验」，这条是**文档仍在教人「一个 PR 内改两端」**，而拆仓后该动作物理上不可在一个仓完成。

改法：Task 6 增「失效文档清单」+ 可复跑门禁：
```bash
grep -rn "pnpm --filter @gospeak/\(web\|desktop-app\|mobile\|sfu-client\|components\|docs\)" \
  --include=*.md . | grep -v node_modules | grep -v '^./docs/superpowers/'
```
（白名单外应为 0；跨仓命令只允许出现在「跨仓开发回路」章节。）

**D5. 子仓自身的 AGENTS.md / README 在 root 化后路径与 module 名全错。**

`app/desktop/AGENTS.md:53/55` 要求「与 `packages/desktop` 命令一致」「补 `packages/desktop/src/capabilities/` fallback」——但 desktop 仓**不挂 packages**；`:22` 让去 `app/shared/proxyrules` 跑测试——本仓没该目录。`app/mobile/AGENTS.md:32` 说 desktop「用 `replace` 指向本地目录」——`replace` 在 Task 0 已删。`packages/*/AGENTS.md` 引用 `app/desktop/`、`scripts/check-desktop-commands.mjs`。`app/web/AGENTS.md:10-16` 命令写成裸 `pnpm dev`（实际应为 `pnpm --filter @gospeak/web`）。

改法：Task 6 新增 Step「四仓 doc 归一」，映射规则写死：`app/desktop/*` → `./*`；`packages/<pkg>/*` → `<pkg>/*`；`app/web/*`、`app/shared/proxyrules/*`、`scripts/*` → 一律改成「跨仓，见主仓 URL」。Expected 用 `grep -rn "app/\(web\|desktop\|mobile\|shared\)/\|packages/\|scripts/" --include=*.md` 为空。

**D6. 回滚只 revert 主仓 → 所有子仓侧工作静默作废，且 `gh repo delete` 不可逆。**

Task 6B/6C/7/9/11/12 在子仓内的提交（布局、workspace、CI、metadata、updater、release.yml）**没有任何一步会被主仓 revert 撤销**；而 Step 5 把 `gh repo delete` 放在「保留/删除」二选一里，全计划无备份步骤。且因路径被 filter-branch 改过（`web/src/x` vs `app/web/src/x`），保留仓也无法直接 `git am` 回主仓。

改法：Task 14 增 **Step 0 冻结与导出**（在 Step 2 之前）：
```bash
for r in Web Desktop Mobile Packages; do
  git clone --mirror https://github.com/NoelOrin/GOSpeak-$r.git /tmp/rollback-$r.git
  git -C /tmp/rollback-$r.git bundle create /tmp/gspeak-$r-rollback.bundle --all
done
```
落到 `docs/superpowers/rollback/` 或 Release 附件，并写死路径映射表；Step 5 改为「bundle 存在且可 clone 通过」才允许 delete。

**D7. 根 `package.json` 的 scripts 一行没改，新人命令会静默失败。**

根 `package.json` 的 `dev`/`dev:all`/`dev:web`/`build:web`/`test:web`/`format`/`gen:protocol` 全部按单仓 workspace 写。计划 Task 6/7 都没提改它。
改法：对已拆出的目标**删掉或改成显式失败**（如 `dev:web` → `echo "前端已拆到 GOSpeak-Web，请在该仓运行 pnpm dev" && exit 1`），写进 Task 7 Step 5；Task 6 增「拆仓后本地开发回路」三仓命令表。

### 中

**D8. 跨仓调试回路缺失，且子模块默认 detached HEAD 是丢提交陷阱。**

计划只写了维护者 bump 仪式，没有开发者本地联调方案。实测 `git submodule update --init` 后 `git -C app/web status` → `HEAD detached at <sha>`；开发者在 `app/web/packages/` 直接 commit 会挂空引用，下次 `submodule update` 直接覆盖。
改法：Task 6C 增 Step——必须先 `git switch release && git switch -c feat/xxx` 再提交；本地 `pnpm install` 后 workspace 软链直接吃工作树改动（`pnpm-lock.yaml` 里记的是 `link:`）。

**D9. `docs.yml` 的 sfu-client 触发正则跨仓失效（与 CI7 同源，此处补充影响面）。**

另：在跟计划里共 **163 处**路径将失效（`2026-11-16-app-mobile.md` 65、`2026-11-25-audio-output-strategy.md` 33、`2026-11-24-screen-share-on-demand-subscribe.md` 31、其余 34），它们都还标着「待执行/进行中」。`agent_test_logs/*.md` 里约 20 处属历史记录**不该回填**。
改法：Task 7 给 `docs_path_regex` 加 `^packages$`、`^app/web$`；Task 6 增「在跟计划审计」（逐计划改命令归属仓，或登记为子仓新建计划）；在 AGENTS.md §4.6 落一句口径：`agent_test_logs/` 与已交付计划的命令快照是历史记录，不随拆仓回填。

### 低

**D10. 计划自身不自洽**（部分已修）：
- `Task 6C` 曾定义在 Task 7 内部（**已在本轮移到 Task 6B 之后**）；但 Task 7 里的 `Step 6b/6c` 与 Task 6B/6C 编号体系仍冲突，建议合并为 `Task 7.1–7.4` 或整体顺延；
- Task 8 Step 7 的 Expected 写「三行」但主仓有**四**个 gitlink，应为四行；
- 设计文档 `2026-11-26-submodule-split-design.md:101` 仍写「当前分支 dev-1124」，应同步为执行分支；
- Task 6 Step 2 的 Expected 用 `grep -n "pnpm typecheck" AGENTS.md` 作判据，而该段正是要被重写的内容 → 会假失败；应改成断言句子本身。

### 实测确认无问题的角度

- **proxyrules / packages 挂载矩阵**：实测与计划 L670-676 的消费矩阵、L34-38 的三处挂载结论**完全一致**；
- **`packages/plugin-sdk` 空壳**：Task 6C 的清理判断正确（且它本无被跟踪文件，不会进历史）；
- **Task 3 的 filter-branch 命令、Task 5 的 `.gitmodules`/gitlink 断言、Task 8 的验证命令集**：与仓库事实一致；
- **`test/` 集成测试**：只依赖 vitest/ws，**不** import `@gospeak/*`，所以「test 是否依赖工作区包」的担心实际不存在。

---

## 前端构建链 review 补丁（frontend-reviewer 结论，2026-11-26）

### 阻塞

**F1. 三个构建目标共用 `dist/`，而 web Release 只发一个包 → desktop/mobile 永远拿不到自己 target 的产物。**

证据：`app/web/vite.config.ts:160-163` 只有 mobile 走独立目录——`outDir: gospeakTarget === "mobile" ? "dist-mobile" : "dist"`，**web 与 desktop 写同一个 `dist/`**。现状靠 `release.yml:273-299` 为 desktop 再构建一次（注释原文「两个组件共用：desktop 以条件编译目标构建」），`mygo.config.ts:39-44` 也自己跑 `GOSPEAK_TARGET=desktop pnpm --filter @gospeak/web build`。

拆仓后 desktop / mobile 仓**都没有 web 源码**（Global Constraints 明确 desktop 不挂 packages 也不挂 web），无法自己产出对应 bundle；而计划只把 web Release 写成「前端静态压缩包」（单数、无 target 维度）。桌面独享分支若被默认 web 产物替代，会被条件编译**摇树剔除**（AGENTS.md §4.4）。

更隐蔽的是 Task 8 Step 3 在同一 clone 里顺序跑 `build` → `build:desktop` → `build:mobile` 却只断言「成功」：desktop 目标会覆盖 `dist/`，跑完再打包 `dist/` 就发错包。

改法：
1. `vite.config.ts` 让 desktop 也走独立目录：`outDir: gospeakTarget === "desktop" ? "dist-desktop" : gospeakTarget === "mobile" ? "dist-mobile" : "dist"`，并加进 `.gitignore` 与 turbo `outputs`；
2. GOSpeak-Web 的 `release.yml` 产出**三份按 target 命名**的产物：`gospeak-web-v<ver>-web.tar.gz` / `-desktop.tar.gz` / `-mobile.tar.gz`；
3. desktop / mobile 的 `release.yml` 用 `gh release download gospeak-web-v<ver> -p '*-desktop.tar.gz' -R NoelOrin/GOSpeak-Web` 解压到 `webui/dist` / `www/`，缺失直接失败（与 R9 同原则）；
4. Task 8 Step 3 增加**产物级断言**（如 `dist-desktop/assets/*.js` 含 desktop-only 哨兵、`dist/assets/*.js` 不含），否则 target 混淆会静默通过。

### 高

**F2. `tailwind.config.ts` 的 content glob 拆仓后越出仓库根 → 共享组件类名静默缺失。**

现状 `app/web/tailwind.config.ts:7-10`：`content: ["./src/**/*.{html,tsx}", "../../packages/components/src/**/*.{ts,tsx}"]`，文件头注释自己写明「漏扫会让共享组件里独有的变体类永远不生成，表现为样式莫名失效」。该配置是生效链路的一部分（`styles.css:11` 有 `@config "../tailwind.config.ts"`）。

实测这条 glob 确实在喂产物：`dist/assets/*.css` 里 `orientation=vertical` 出现 **5** 次，而该变体只出现在 `packages/components/src/components/slider.tsx:25,35,42`（web 自身源码不写）。

改法：新布局下改为 `"./packages/components/src/**/*.{ts,tsx}"`（已在 Task 6B Step 1 写明）；GOSpeak-Web CI 增加产物哨兵——构建后断言 `dist/assets/*.css` 含 `orientation=vertical`。

**F3. 主仓四个前端门禁拆仓后只校验 pinned 副本，其中一个还会静默放行。**

`ci.yml` 的 biome ci 扫 `./app/web ./packages ...`；`check-ui-tokens.mjs:29` 与 `check-radius-tokens.mjs:7` 的 `SCAN_DIRS = ["app/web/src", "packages/components/src"]`；`check-hook-deps.mjs:19-26` 直接 `biome lint ./app/web ./packages`。

拆仓后主仓这些路径**存在但内容只是 gitlink 指向的旧提交**，真正改动发生在 GOSpeak-Web 的 PR 里 → 主仓四道门禁依然全绿；且 `check-radius-tokens.mjs:38-45` 是 `try { walk } catch { continue }`，目录不存在时**静默放行**。R4 只点名了另外三个脚本，未覆盖这四项。

改法：把 `check-ui-tokens.mjs`、`check-radius-tokens.mjs`、`check-hook-deps.mjs` 迁入 **GOSpeak-Web**（它同时挂 `packages/`，正好覆盖两个扫描根），scan 根改 `src` / `packages/components/src`；主仓 biome 缩小为 `./scripts ./test`。同时把 `check-radius-tokens` 的 `catch → continue` 改成「目录缺失即失败」，否则迁移后仍会静默。

**F4. `biome.json` 的 includes 与计划描述不符；照搬会让子仓 biome 零匹配 exit 1。**

`biome.json:11-29` 用的是**无 `./` 前缀**的 exact 子路径（`app/web/src/**/*.{ts,tsx,...}`、`packages/components/**/*`、`!app/web/src/routeTree.gen.ts`、`!app/desktop/mygo.ts`）；而计划 Task 7 Step 5 按 `./app/web` 形式描述要「移除」，两者对不上（按 `./app/web` sed 匹配不到）。直接复制到子仓后所有 `app/web/**` include 全部不匹配，实测 biome 零匹配是**硬失败** `exit=1`（只有 `--no-errors-on-unmatched` 才放行）。

改法：每个子仓写**独立** `biome.json`，不要 verbatim 复制；GOSpeak-Web 用 `src/**` + `packages/*/**`，排除 `src/routeTree.gen.ts`；GOSpeak-Packages 用四个包目录。验收写成 `cd <子仓> && pnpm exec biome ci .` 且必须真实处理到文件。

### 中

**F5. 根 `package.json` 完全不在修改清单，拆仓后 `lint`/`typecheck` 会「空跑变绿」。**

根 `package.json` 的 `dev`/`build:web`/`test:web`/`format` 全部指向已拆出的包。实测 turbo 空任务行为：
```
pnpm turbo run typecheck --filter=@gospeak/test → WARNING No tasks were executed ... exit=0
pnpm turbo run lint --filter=@gospeak/test      → 同上，exit=0
```
而 `lefthook.yml:44` 的 pre-push 恰好调 `pnpm typecheck` → **本地门禁静默失效**。反向也有硬失败：`turbo run build:desktop` → `x Could not find task`（`turbo.json` 没有该任务）。

另：计划说「typecheck 只保留主仓 scripts」，但根目录没有 `tsconfig.json`，`scripts/*.mjs` 无类型检查入口，该要求无法照字面落地。

改法：修改表加入根 `package.json`——删除指向已拆包的脚本，`lint`/`check` 改 `biome ci ./scripts ./test`，`typecheck` 删除或改 `node --check`；`turbo.json` 可整体删除（主仓再无 turbo 任务）。

**F6. 本地 pre-push 的 protocol gen-diff 拆仓后**永远通过**（R13 只补了 CI）。**

`lefthook.yml:34-36`：`pnpm gen:protocol && git diff --exit-code -- app/web/src/protocol/generated`。拆仓后 `app/web` 是 gitlink，主仓 `git diff` 对子仓内部路径不返回任何内容 → 恒为 0；生成物其实写进了子仓工作区，只在 `git status` 里表现为 `app/web` modified content。

改法：钩子改成与 CI 同构：`pnpm gen:protocol && (cd app/web && git status --porcelain -- src/protocol/generated | grep . && exit 1 || true)`；同时 biome ci 行缩到 `./scripts ./test`。

### 低

**F7. web 仓内两个脚本的 `repoRoot` 推导会指向仓库外。**

`app/web/scripts/capture-storybook-baseline.mjs:9-19` 用 `../../..`（现在三级上 = 仓库根）推导 `repoRoot`，输出到 `packages/components/storybook-*`。新布局下 `scripts/` 的 `../..` 才是仓库根，`../../..` 会跑到仓库外并 mkdir 写文件。
改法：`../../..` → `../..`，输出目录改 `packages/components/storybook-*`（相对路径不变，因为 packages 就在仓根下）。

### 实测确认无问题的角度

- **web 确实用到全部 4 个 packages**：`grep -rl '@gospeak/<pkg>' app/web/src | wc -l` → components 123 / sfu-client 31 / mobile-capabilities 24 / desktop 23；四个包之间**零** `@gospeak/*` 互引；四个 tsconfig **都无 `extends`**（「无需迁移 tsconfig 基座」成立）。
- **`test/` 包**：只依赖 vitest/ws，不 import `@gospeak/*` → 「主仓 workspace 只留 test」的顾虑可关闭（但**不能**连 `app/server`/`app/docs` 一起删，见文档补丁 D1）。
- **`plugins/*`**：根目录**不存在** `plugins/`，workspace 里的 glob 是死条目。
- **`routeTree.gen.ts` 生成链路**：靠 `tsr.config.json` + devDep `@tanstack/router-cli` + vite 插件，不依赖主仓路径，拆仓不断链（唯一风险是它在新仓不再被忽略，已并入 G3/F8）。
- **web 仓建议不要 turbo**：workspace 里除 web 外都是 `main → ./src/index.ts` 源码包，`pnpm --filter` 足够；且 turbo 的 `outputs: dist/**` 与 F1 的三目标目录直接冲突。

---

### 子仓 `.gitignore` 必须补建（G3 / backend #10 / F3 三方共同确认）

四个子仓**都没有** `.gitignore`（实测 `app/web`/`app/mobile`/`app/desktop`/`packages` 均为 0，只有子层目录各自有）。忽略规则全在根 `.gitignore`：`node_modules`(L4)、`**/dist/`(L20)、`.tanstack`(L25)、`*.tsbuildinfo`(L27)、`app/web/pnpm-lock.yaml`(L30)、`app/web/dist-mobile/`(L33)、`app/web/src/routeTree.gen.ts`(L39)、`app/desktop/mygo.ts`(L42)、`app/desktop/webui/dist/`(L46)、`build/`(L123)。`--subdirectory-filter` 不会带走它们。

必须补建的清单：

| 子仓 | 需要的忽略项 |
|---|---|
| GOSpeak-Web | `node_modules/`、`dist/`、`dist-desktop/`、`dist-mobile/`、`.tanstack/`、`src/routeTree.gen.ts`、`*.tsbuildinfo`（**删除** `pnpm-lock.yaml` 例外，子仓要提交 lockfile） |
| GOSpeak-Desktop | `webui/dist/`、`mygo.ts`、`gospeak-desktop`、`dist/` |
| GOSpeak-Mobile | `node_modules/`、`www/`、`build/` |
| GOSpeak-Packages | `node_modules/`、`*/dist/` |

另：`biome.json:22` 的 `!app/desktop/mygo.ts` 排除项也要带进 Desktop 仓（否则生成物会被 format）。

验收：`cd <子仓> && git status --porcelain` 为空。

---

## 后端与跨语言依赖 review 补丁（backend-reviewer 结论，2026-11-26）

> 第 1、2 条已在本轮直接改掉（proxyrules 立场、web 布局），此处保留结论与残留待办。

### 阻塞（已修，保留证据）

**B1. proxyrules 立场表述曾与第 5 仓方案冲突** —— 计划 L39 / Architecture / 索引表 / Task 0 Step 1–2 曾写「留主仓 + 子目录 tag + 公开 proxy」，全仓 `grep 'GOSpeak-Proxyrules'` 命中 0。**已改为**：module 路径 `github.com/NoelOrin/GOSpeak-Proxyrules`（仓根即 module 根，直接打 `v0.1.0`）；desktop 用 `replace => ./proxyrules` 指向本仓内 submodule；mobile 挂同一 submodule。
顺带消除一笔隐性凭据：**`SUBMODULES_TOKEN` 只解决 submodule checkout，解决不了 `go mod download`**——若走 proxy 且仓私有，还需 `GOPRIVATE` + `insteadOf` 带 token。改用 submodule + replace 后这笔凭据不再需要。

**B2. `GOSpeak-Web` 的 `web/` 前缀与「子仓根即原目录内容」矛盾** —— 原 Task 6B Step 1 用 `--tree-filter` 把所有文件移入 `web/`，导致源码实际在 `app/web/web/src`，并打断至少 6 处硬路径：`Dockerfile:8/15/20`、`gen-protocol/main.go:20`（`../web/src/protocol/generated`）、`gen_contract_test.go:20`、`mygo.config.ts:41`（`cp -R ../web/dist`）、`ci.yml` 的 `Upload web dist path`、以及 Task 8 的 `ls app/web/src` 验证。**已改为**：GOSpeak-Web 仓根即 web 内容，`packages/` 挂仓根下，workspace 用 `[".", "packages/*"]`（与 mobile 同构）——所有原路径保持不变。

**B3. `mobile-bind-proxyrules.mjs` 迁移不是「改一行 node 路径」。** 残留待办：
- 脚本三个根路径全部以主仓根为基准：`:326` `root = resolve(dirname(...), "..")`、`:20` `MODULE_DIR = "app/shared/proxyrules"`、`:22` `OUTPUT_DIR = "app/mobile/build/proxyrules"`。落到 mobile 仓后 root 变成 mobile 仓根，`MODULE_DIR` 会解析到不存在的路径并 `::error::找不到共享模块` 退 1，`OUTPUT_DIR` 也会错成 `mobile仓/app/mobile/build/...`（消费方 `android/app/build.gradle:30` `file('../../build/proxyrules/proxyrules.aar')` 与 `ios/.../project.pbxproj:42` 都是**相对 mobile 仓根**，正确值应为 `build/proxyrules`）。
- 计划 Task 6C 只给 mobile 挂 `packages`，**没挂 proxyrules** → 脚本在 mobile 仓里没有共享模块可 bind。
- 口径冲突：R4 说本脚本「只能留主仓」，Task 6C 说「迁入本仓」。

**定稿改法**：脚本归 mobile 仓，并做四件事——① mobile 仓 `.gitmodules` 增挂 proxyrules（`shared/proxyrules`），R4 里删掉该脚本的保留理由；② `MODULE_DIR = process.env.GOSPEAK_PROXYRULES_DIR ?? "shared/proxyrules"`、`OUTPUT_DIR = "build/proxyrules"`、root 用 `--root` 参数或 `process.cwd()`，并断言 `git -C moduleDir rev-parse` 成功；③ `mobile-bind-proxyrules.test.mjs` 一并迁入（主仓 `test-scripts` 的 `node --test scripts/*.test.mjs` 会失去它，需一并交代）；④ android/ios 的相对路径**不动**（mobile 仓根 = app/mobile 内容）。

### 高

**B4. gobind 绑定面门禁与 proxyrules 自身 `go test` 拆仓后没有归属。**
现状两条门禁都在主仓 `ci.yml` 的 `desktop-go` job：`:403` `Test shared proxyrules`（`working-directory: app/shared/proxyrules`）、`:407-414` `Verify proxyrules binding surface`。计划删 `desktop-go`，而 Desktop/Mobile 仓 CI 清单都没这条；唯一还有命令的 `build-mobile.yml:85` 自述「不进 CI Gate」。
改法（与 CI6 合并）：门禁归属 **GOSpeak-Proxyrules 仓**（源与脚本同仓、唯一稳定挂载点）：CI 跑 `go vet ./... && go test ./... -count=1` + `node scripts/bind-surface.mjs --surface-only`，设为 required check。

**B5. Dockerfile 前端阶段依赖「主仓根 workspace + 根 lockfile」，与 workspace 收缩及 R3 冲突。**
`Dockerfile:7-20` 是根 workspace 安装（`COPY package.json pnpm-workspace.yaml pnpm-lock.yaml` → `COPY app/web/package.json` + 4 个 `packages/*/package.json` → `pnpm install --filter @gospeak/web...`）。workspace 收缩后根 lockfile 不再有 `app/web` importer（现在 `pnpm-lock.yaml:33` 有 `app/desktop:`、`:78` 有 `app/web:`），`--filter @gospeak/web...` 会 `No projects matched` 退非 0。计划 Task 10 Step 3 只改 stage 2/3 的 `COPY --from`，默认 stage 1 原样保留——内部矛盾。
改法：stage 1 重写为「在 web 子仓自己的 workspace 里装」：`COPY app/web ./web` → `WORKDIR /src/web && pnpm install --frozen-lockfile && pnpm build`；删掉根依赖拷贝与 `--filter @gospeak/web`。或按 Task 12 的「不在主仓构建 web 产物」彻底删 web-builder，改为 `ARG WEB_TARBALL_URL` + `ADD` 下载并校验 sha256。

**B6. 桌面仓 CI 会因 `//go:embed all:webui/dist` 直接编译失败。**
`app/desktop/proxyserver.go:51` 有 `//go:embed all:webui/dist`，而 `webui/dist` 被 `.gitignore:46` 整目录忽略且**无占位文件**（server 侧 `.gitignore:142-144` 保留了 `EMBED_PLACEHOLDER`，desktop 没保留）。现主仓靠 `ci.yml:392-402` `Seed embed placeholder` 现场种占位（注释原话说明原因）。Desktop 仓 CI 清单没有这个种子步骤 → 全新 clone 上 `go vet`/`go test` 直接 `pattern all:webui/dist: no matching files found`。
另一面：Task 10 去 embed 后，`proxyserver_test.go:220-237` 的 `requireBuiltDist` 是「失败即 `t.Skipf`」→ 反代/安全测试全部变 skip，`go test` 依旧绿。
改法：① Desktop 仓提交 `webui/dist/.gitkeep` 并在该仓 `.gitignore` 加例外（或 CI 保留等价 seed 步骤）；② 去 embed 后把 `requireBuiltDist` 改成读仓内 `testdata/dist/index.html` 夹具，**不要**靠「缺文件就 skip」，否则桌面反代覆盖面归零。

### 中

**B7. R3 漏了 desktop 仓的 lockfile。** `app/desktop/package.json` 有 `mygo-cli` 依赖，根 `pnpm-lock.yaml:33` 有它的 importer，但 `app/desktop/pnpm-lock.yaml` 不存在。Desktop 仓 CI 要跑 `mygo build` 就必须先 `pnpm install`。改法：R3 扩成三仓（web / packages / **desktop**）：首次 `pnpm install` 生成并提交 lockfile，CI 用 `--frozen-lockfile`。

**B8. Go 工具链路径与门禁命令没跟着改。**
- `build-mobile.yml:72/78/85` 的 `go-version-file: app/shared/proxyrules/go.mod`、`go-sum-path`、`node scripts/mobile-bind-proxyrules.mjs` 迁到 Mobile 仓后全部不存在；
- 计划 Task 8 的 `go vet ./app/server/... ./app/shared/...` 从**仓库根**运行实测失败：`pattern ./app/server/...: directory prefix app/server does not contain main module`（根目录无 `go.mod`）。应改为 `(cd app/server && go vet ./...)`；
- Task 0 Step 2b 的 `GOTOOLCHAIN=auto` 没落到 workflow env；proxyrules 声明 `go 1.26.0`、desktop `go 1.27.4`，runner 版本偏低时会报 toolchain 错误。
改法：Desktop 仓用 `go.mod`/`go.sum`，Mobile 仓用 `shared/proxyrules/go.mod`；workflow env 显式 `GOTOOLCHAIN: auto`。

### 已核实无问题的部分

- **server 零依赖 proxyrules / packages**：`grep -rn 'gospeak/proxyrules' --include='*.go' .` 除 `app/desktop` 与 proxyrules 自身外 **0 命中**；`app/server/go.mod` 无 `replace`。server 与 web 的唯一耦合是 `internal/webui` 的 `//go:embed all:dist`，Task 10 去 embed 后彻底解耦。
- **proxyrules 换 module 路径不影响 gomobile 生成的符号名**：`x/mobile` 的 `JavaPkgName` 用 `pkg.Name()`（Go 包名 `proxyrules`）而非 import path，所以 `proxyrules.Proxyrules.validateBaseURL` 与 `#import <Proxyrules/Proxyrules.objc.h>` 在改名后依然成立——前提是**包名保持 `proxyrules`** 且不传 `-prefix`/`-javapkg`。
- **`gen_contract_test.go` 的路径算术本身没错**（上溯四级 = 仓根），失效只来自 `web/` 布局与「子模块未 init」两种情形；后者的解法是改用已提交的 `internal/genprotocol/testdata/golden`（实测存在 events.ts / payloads.ts / envelope.ts，且 `gen-protocol -update-golden` 具备写入能力）。

---

## 多 agent review 总汇总（2026-11-26，6 个角度 / 59 条）

> 六个独立 reviewer（Git / CI / 前端 / 后端 / 发布 / 文档）各自只读取证，结论已按角度分别落到上列五节补丁。本节做**交叉验证**与**开工决策**。

### 交叉验证：被 2 个以上 reviewer 独立命中的问题（最高可信度）

| 问题 | 独立发现方 | 判定 |
|---|---|---|
| 子仓没有 `.gitignore`，`git add` 会吞 node_modules/dist/二进制 | git G3、backend B10、frontend F3 | **确认**，已附完整补建清单 |
| 三个构建目标共用 `dist/`，desktop/mobile 拿不到自己 target 的产物 | backend B6、frontend F1 | **确认**，阻塞级 |
| gobind 绑定面门禁随 `desktop-go` 删除而消失 | ci CI6、backend B4 | **确认**，需归属新仓 |
| 历史 tag/Release 断代，已装机桌面端更新检测失联 | release P3、git G7 | **确认**（同一问题两半：版本号回退 + Release 记录缺失） |
| 「tag 到底在哪个仓打」自相矛盾，没人往子仓推 tag | release P4、ci CI1 | **确认**，阻塞级 |
| `pnpm-workspace` 收窄会踢掉仍在主仓的成员 | ci CI6、docs D1 | **确认**，且不止 docs，还有 `app/server` |
| 计划把子仓文件放在主仓 `git add` | docs D2、git（实证） | **确认**，静默丢失 |
| 根 `package.json` scripts 未改，`pnpm --filter` 静默 exit 0 | docs D7、frontend F5 | **确认** |
| `docs.yml` 的 `packages/sfu-client/**` 触发失效 | ci CI7、docs D9 | **确认** |

### 已在本轮直接改掉的命令级/设计级错误

| 改动 | 原错误 | 不修的后果 |
|---|---|---|
| `git remote add` → `git remote set-url` | clone 已占用 origin | 四个子仓推送全部失效 |
| `pnpm-workspace` 显式列 `app/server` + `app/docs` + `test` + `plugins/*` | 「只剩 test」 | 根 `pnpm dev`/`dev:server` 直接失败 |
| GOSpeak-Web 放弃 `web/` 前缀布局 | 源码实际在 `app/web/web/src` | Dockerfile / gen-protocol / mygo 至少 6 处硬路径断 |
| proxyrules 独立第 5 仓（统一 Global Constraints / Architecture / 索引 / Task 0 / Task 8） | 正文仍写「留主仓 + proxy」 | Task 0 产出错误终态 |
| `allowBuilds` → `onlyBuiltDependencies` | pnpm 10.11.0 不认识 `allowBuilds` | 四仓 lefthook 钩子装不上（死配置） |
| Task 8 的 `git submodule status` 「三行」→「四行」 | 主仓有 4 个 gitlink | 验收断言自相矛盾 |
| Task 6C 归位到 Task 6B 之后 | 曾被误插在 Task 7 内部 | 编号体系混乱 |

### 开工前必须解决的最小清单（8 项，按依赖顺序）

1. **定凭据方案**：四仓各配 `SUBMODULES_TOKEN`，PAT 必须同时含主仓与四子仓 `Contents: read`；`version-bump.yml` 显式 `persist-credentials: false`；fork PR 用 `head.repo.full_name == github.repository` 守卫。
2. **定 tag 创建者**（唯一口径）：server 在主仓；web/desktop/mobile 在各自仓。主仓 version-bump 对这三个**只做只读探测 + 失败提示**，不推 tag、不呼跨仓 workflow_call（可用性已否证）。
3. **定 proxyrules 归属**：独立第 5 仓 `GOSpeak-Proxyrules`；desktop/mobile 各挂 submodule；主仓不挂。
4. **定 web 产物形态**：`vite.config.ts` 让 desktop 走 `dist-desktop/`；web 仓 Release 出三份按 target 命名的 tar.gz；desktop/mobile 下载对应包并断言 `index.html`。
5. **补 `.gitignore` 与子仓 lockfile**：四仓各自的忽略清单 + web/packages/**desktop** 三仓独立 `pnpm-lock.yaml`（含 `supportedArchitectures`）。
6. **处理 `web/` 布局残留**：确认 Task 6B 不再 tree-filter；同步改 `tailwind.config.ts` 的 glob 与 `capture-storybook-baseline.mjs` 的 `repoRoot`。
7. **处理 `ci.yml` 删除后的引用点**：`build-server` 的 `needs`、`gate` 的 `needs` + 四个 `*_RESULT` 环境变量 + for 循环、`Download web dist`/`Embed frontend` 步骤。
8. **建立 `pre-submodule-split` 备份点**：`git bundle --all` + tag，供 Task 14 回滚与偏差对照。

### 剩余风险（接受并登记，不再阻塞）

- **CI 分钟数上升**（R11）：五仓独立流水线 + 子仓→主仓两轮触发；Android 迁 Linux 已部分对冲。
- **gitlink 漂移**（R5）：靠定时检测 job + PR 评审，不做自动 bump。
- **跨仓调试体验下降**（docs D8）：靠 `git switch -c` 避免 detached HEAD 丢提交 + `link:` 软链吃工作树改动。
- **在跟计划 163 处命令失效**（docs D9）：登记为拆仓后的 follow-up，不随本次回填。

### 结论

计划的**目标与分层设计成立**（proxyrules 独立仓、packages 三处挂载、各仓独立 Release 均被独立复核为正确），但**执行命令层存在大量「照抄即失败」的缺陷**（remote add、tree-filter 布局、workspace 成员、ci.yml 引用点、git add 子仓路径、回滚序列）。上述 8 项解决后，计划可进入 Task 0。

---

## 挂载方案（**已定稿：β**）

> 你要求「尽量少挂」，在 α(3)/β(5)/γ(7) 三案中选定 **β**。本节省略决策过程，只留结论与残留分支。

**β 最终形态（含 β1 + β-a）**：主仓挂 `app/web` + `packages`（2）；web 挂 `packages`（1）；desktop 挂 `proxyrules` + `packages`（2）；mobile 挂 `proxyrules`（1）；Packages / Proxyrules 仓无挂载 → **共 6 个挂载点**。

**残留分支已定稿**：缺口 1 → **β1**（版本校验下沉子仓 + 主仓 `gh api` 交叉比对）；缺口 2 → **β-a**（desktop 挂 packages，`check-desktop-commands` 迁入 desktop 仓）。

### 主仓的真实依赖清单（拆完 server/scripts/test/deploy/docs 之后）

| 主仓还需要子仓的什么 | 在哪里用 | 能否脱离 |
|---|---|---|
| `app/web/src/protocol/generated` | `gen-protocol/main.go`（默认输出）、`gen_contract_test.go`（比对） | 可——golden 已存在（`internal/genprotocol/testdata/golden/` 三份文件），`-update-golden` 已可写 |
| `app/web/package.json` | `mobile-version.mjs` **写入**版本 | 应删（P8：跨组件污染） |
| `app/web/src` + `packages/components/src` | `check-ui-tokens.mjs`、`check-radius-tokens.mjs`、`check-hook-deps.mjs` | 可迁 web 仓（它挂了 packages） |
| `app/desktop/package.json`、`mygo.config.ts` | `check-version.mjs` 版本门禁 | 可改 GitHub API 读远端，或下沉子仓 |
| `app/mobile/package.json`、`build.gradle`、`pbxproj` | `check-version.mjs` 版本门禁 | 同上 |
| `app/desktop`（Go Bind）+ `packages/desktop`（TS 命令串） | `check-desktop-commands.mjs` | 难以脱离——必须同时看到两个仓 |

### 三个方案（挂载总数 / 代价）

（下表保留供复核）

| 方案 | 主仓 | web 仓 | desktop 仓 | mobile 仓 | **总数** | 必须做的改造 |
|---|---|---|---|---|---|---|
| **α 最少** | 0 | packages | proxyrules | proxyrules | **3** | 4 个门禁脚本全部下沉/改造；`gen_contract_test` 改比 golden；协议生成物靠主仓 CI 推 PR 到 web 仓（需可写 token）；`check-desktop-commands` 无处可跑 → 放弃或另建仓 |
| **β 平衡 ← 已选定（+β1 +β-a → 6）** | web、packages | packages | proxyrules、packages | proxyrules | **6** | 版本门禁下沉各仓（β1）；`check-desktop-commands` 迁 desktop 仓（β-a）；`gen-protocol` 原地写；UI token 类门禁留主仓 |
| **γ 原稿** | web、desktop、mobile、packages | packages | proxyrules | proxyrules | **7** | 门禁零改造，但 4 个 gitlink 每次改动都要 bump |

**mobile 的 packages 已确认可删**：mobile 无 `src/`，`@gospeak/mobile-capabilities` 全仓零使用，是残留声明。三个方案都已把它去掉（原稿是 7 而非 8，就是扣掉这一处）。

### β 残留分支：主仓不挂 desktop/mobile 带来的两处门禁缺口

β 省掉了 `app/desktop` 与 `app/mobile` 两个 gitlink，代价是主仓有两个脚本失去读端：

**缺口 1：`check-version.mjs` 读不到 desktop / mobile 的版本文件。**
它现在读 `app/desktop/package.json`、`app/desktop/mygo.config.ts`、`app/mobile/package.json`、`app/mobile/android/app/build.gradle`、`app/mobile/ios/...pbxproj`（`check-version.mjs:130/140/158/167/189`）。β 下这些路径在主仓不存在。

两个子选项（执行时二选一，需再次确认）：
- **β1（推荐）下沉**：版本一致性校验改由**子仓自己的 CI** 执行——desktop 仓校验「本仓版本文件 == 主仓下发的 metadata」；mobile 仓同理。主仓只保留 server 版本校验。主仓与子仓的 metadata 比对通过 `gh api` 读公开仓 raw 文件完成（public 仓无需 token）。
  - 代价：每个子仓 CI 多一步拉主仓 metadata；主仓失去「一处看全部版本」的视角。
- **β2 挂回来**：主仓改为挂 4 个（即 γ），放弃 β 的收益。——**这违背你「少挂」的目标，不建议**。

**缺口 2：`check-desktop-commands.mjs` 读不到 desktop 的 Go 源码。**
它需要 `app/desktop/*.go`（`mygo.Bind` 声明）与 `app/desktop/preload_*.js`，加上 `packages/desktop/src/capabilities/*.ts` 与 `packages/desktop/src/events.ts`（`check-desktop-commands.mjs:14-16`）。β 下主仓有 packages 但没有 desktop。

三个子选项：
- **β-a 迁到 desktop 仓**：desktop 仓再挂一份 `packages`（+1 挂载 → 总 6）。门禁在 desktop 仓跑，逻辑不变。
- **β-b 两步导出比对**：desktop 仓 CI 导出 `bind-list.json`（解析 `mygo.Bind`），packages 仓 CI 导出 `ts-commands.json`，由主仓或一个轻量 job 比对两者。无新增挂载（总 5），但要多写两份导出脚本 + 一个比对步骤。
- **β-c 放弃该门禁**（总 5）：接受「Go Bind ↔ TS 命令串逐字一致」不再自动校验，只靠 code review。这是历史上真实防住过拼写漂移的护栏。

**已定稿：缺口 1 → β1；缺口 2 → β-a（总 6 个挂载点）。**

具体落地：
- **β1**：`check-version.mjs` 拆成两半——主仓侧只校验 `VERSION`/`metadata.yaml` 与 server；desktop/mobile 的子仓 CI 各自校验「本仓版本文件 == 主仓 metadata」；主仓再跑一次 `gh api repos/NoelOrin/GOSpeak-<X>/contents/...` 读公开仓文件做交叉比对（无 token）。
- **β-a**：desktop 仓挂 `packages`，并把 `scripts/check-desktop-commands.mjs` 与其 `.test.mjs` 迁入 desktop 仓，`DESKTOP_DIR` 改为 `.`、`TS_DIR`/`TS_EVENTS_FILE` 改为 `packages/desktop/src/...`，主仓删除。

---

### 我的判断

- 只追求数字最小选 **α（3 个）**，但代价是**门禁体系推倒重写**：`check-desktop-commands` 在主仓没挂载时无处可跑（它跨 desktop 仓与 packages 仓），只能放弃这道「Go Bind ↔ TS 命令串必须逐字一致」的护栏——而这条护栏正是历史上防住桥接拼写漂移的机制。
- **β（5 个）** 只砍掉 desktop/mobile 两个 gitlink，门禁基本不动，`gen-protocol` 仍能原地写。相对原稿省 2 个，且不牺牲保障。
- **γ（7 个）** 就是当前 A1+B1 的写法。

**需要你选：α / β / γ。** 选定后我据此定稿 Task 0、3、5、6B、6C、7、8、9、11 的相关段落，并同步更新「多 agent review 总汇总」里的开工清单。

---

## （旧版待决策记录，已由上方挂载方案取代）

> 以下为决策 A（proxyrules）与决策 B（packages）的原记录，保留供对照；最终以「挂载方案 α/β/γ」为准。


> 这两项此前被我按「推荐方案」写成了定稿，现按要求退回待决策状态。下面是已核实的事实与可选方案。

### 决策 A：proxyrules 怎么放

**已核实事实**
- 全仓只有**两个**消费方：`app/desktop`（Go import，3 个文件）、`app/mobile`（`gomobile bind` 产物 AAR/XCFramework）。
- `app/server` **零引用**（`grep -rn 'gospeak/proxyrules' --include='*.go'` 除 desktop 与模块自身外 0 命中）。
- `gomobile bind` 的实现是 `cwd = app/shared/proxyrules` + `-o <root>/app/mobile/build/proxyrules/proxyrules.aar` + 包参数 `"."` —— **硬假设 proxyrules 与 mobile 同仓**。
- AAR/XCFramework **不入库**（`build/` 被 gitignore），Android gradle 缺文件直接 fail → mobile CI **必须自己能产出**。
- 主仓库是 **public**，子仓已决定 **public**。

| 方案 | 做法 | 优点 | 代价 | 可行性 |
|---|---|---|---|---|
| **A1 独立第 5 仓** | 新建 `GOSpeak-Proxyrules`；desktop 挂 submodule + `replace => ./proxyrules`；mobile 挂 submodule 供 bind；主仓不挂 | mobile 能自产 AAR；无需 Go proxy；改动 proxyrules 不用 tag→push→bump 三步；公开仓无凭据问题 | 多一个仓；desktop 与 mobile 各要 bump 一次 gitlink | ✅ 成立 |
| **A2 留主仓 + 公开 proxy** | module 路径 `.../GOSpeak/app/shared/proxyrules`；desktop 删 replace 改 proxy | 不增加仓库 | **mobile 拿不到源码做 bind**，必须额外让 mobile CI 跨仓 sparse checkout 主仓；proxyrules 每次改动要走 tag→push→bump；proxy 缓存不可变（发错版本只能靠 retract） | ⚠️ mobile 侧需补机制 |
| **A3 留主仓 + mobile 跨仓 checkout** | A2 + mobile CI 额外 `actions/checkout repository: NoelOrin/GOSpeak, sparse-checkout: app/shared/proxyrules`，并用文件记录 pin 的 commit | 不增加仓库；公开仓无需 token | 隐式跨仓依赖、版本漂移难发现；mobile CI 两份 checkout；pin 靠约定无强校验 | ✅ 可跑，但脆 |

**我的推荐：A1**。主要理由是 mobile 的 bind 必须拿到源码，A2 的 proxy 方案解决不了；A3 能跑但引入隐式依赖，与「各仓独立可复现」的目标相悖。

### 决策 B：packages 挂几处

**已核实事实（与先前结论有出入）**
- web 真实使用全部 4 个包；
- **mobile 零使用**：没有 `src/`，`mobile-capabilities` 只在 `package.json` 声明，全仓源码零命中 → 可直接删该依赖行；
- desktop（Go 宿主）不用；
- 主仓只剩一个消费者：`scripts/check-desktop-commands.mjs`，它同时读 desktop 仓的 `mygo.Bind` 与 packages 仓的 TS 命令串。

| 方案 | 挂载点 | 优点 | 代价 |
|---|---|---|---|
| **B1 两处** | `GOSpeak-Web` + 主仓 | 门禁脚本 `check-desktop-commands.mjs` 仍能在主仓跑；mobile 无需挂载 | 主仓多一个 gitlink 要维护 |
| **B2 一处（仅 web）** | 只有 `GOSpeak-Web` | 最少维护 | `check-desktop-commands.mjs` 失效——它跨读 desktop 仓与 packages 仓，在任一子仓都跑不了；除非放弃该门禁或改成跨仓 action |
| **B3 三处（原稿）** | web + mobile + 主仓 | 与早期文档一致 | **mobile 那处是无效挂载**（零消费），纯维护负担 |

**我的推荐：B1**。先删掉 mobile 的残留依赖声明，再把 packages 挂在 web 与主仓两处。

### 已定（无需再决策）

- **子仓可见性：public**（已确认）。因此 R1 凭据、CI3 token 作用域、CI5 子仓嵌套 checkout、fork PR 门禁**全部不再是问题**，公开仓的 Actions 分钟数也免费，R11 成本压力显著缓解。

---

## Task 13: rc 进入 latest 推荐

**Files:**
- Modify: `app/server/internal/service/system_update_service.go` + `_test.go`（去掉 rc 过滤）
- Modify: `.github/workflows/release.yml`（删除误导性 latest 注释，确认不传 `--prerelease`）
- Modify: `.github/workflows/build-mobile.yml`（去掉 `--prerelease`）
- Modify: `docs/superpowers/specs/2026-11-26-submodule-release-design.md`（策略改为 rc 可 latest）

**Interfaces:**
- Consumes: Task 11（多仓库更新检测）、Task 12（发布链路统一）。
- Produces: rc 版本可被更新检测推荐；GitHub latest 可指向 rc。

**策略（与旧行为相反）：**
- 更新检测：移除 `fetchLatest` 中 `pre != ""` 的过滤；rc 按 SemVer 参与比较。
- 比较规则不变：`compareVersion` 仍保持「同版本正式版 > rc」，所以 `2.0.0-rc.1` 只在高于所有正式版（如 `1.9.0`）时成为 latest，`2.0.0` 发布后自动让位。
- 发布：`release.yml` 不传 `--prerelease`（最新 tag 自动成为 GitHub latest），删除旧注释；`build-mobile.yml` 删除 `--prerelease`。
- 文档站部署仍由 `docs.yml` 独立判断（rc 不部署文档），与 latest 推荐解耦。

- [ ] **Step 1: 写 rc 检测测试**

在 `system_update_service_test.go` 增加：只有 rc（如 `v2.0.0-rc.1`）时 `info.Latest` 应为 `2.0.0-rc.1`、`Available` 为 true；rc 与更高正式版并存时取最高正式版；同版本 rc 与正式版并存时取正式版。

Run: `go test ./internal/service/... -run TestSystemUpdate`
Expected: FAIL（当前过滤 rc）。

- [ ] **Step 2: 移除 rc 过滤**

删除 `fetchLatest` 中的 `if _, pre, ok := ...; !ok || pre != "" { continue }`，改为只校验版本可解析（`parseVersion` 返回 ok），保留 `compareVersion` 原逻辑。

Run: `go test ./internal/service/... -run TestSystemUpdate`
Expected: PASS。

- [ ] **Step 3: 调整发布 workflow 的 prerelease 标记**

`release.yml` 删除「Latest 只按发布先后决定」注释，改为明确「rc 与正式版都允许成为 latest」；确认 `gh release create` 未传 `--prerelease` / `--latest=false`。`build-mobile.yml` 删除 `--prerelease` 参数与相关注释。

Expected: `grep -r "prerelease" .github/workflows/release.yml .github/workflows/build-mobile.yml` 无有效参数命中（仅注释允许）。

- [ ] **Step 4: 更新文档并提交**

在 `2026-11-26-submodule-release-design.md` 写明 rc/latest 新策略；更新 `AGENTS.md` 若有相关描述。

```bash
git add app/server/internal/service .github/workflows/release.yml .github/workflows/build-mobile.yml docs/superpowers/specs/2026-11-26-submodule-release-design.md
 git commit -m "feat: allow release candidates to be recommended as latest"
```
Expected: 提交成功。

## Task 14: 回滚预案（仅在失败时执行）

**Files:** 无（操作任务）

**Interfaces:**
- Consumes: Task 1 的状态快照与 Task 5 的提交。
- Produces: 主仓库恢复到拆分前状态。

> **回滚必须按提交逆序逐项执行**，只 revert Task 5 的 submodule 接入提交不足以恢复：Task 7（CI 拆分）、Task 9（metadata 与版本脚本）、Task 10（去 embed）、Task 12（发布链路）都改动了主仓库文件，且 Task 6B 改动了子仓库结构。

- [ ] **Step 1: 记录本次拆分的全部提交**

```bash
cd /Users/noelorin/Projects/GOSpeak
git log --oneline origin/release..HEAD
```
Expected: 列出 Task 0 / 4 / 5 / 6 / 7 / 9 / 10 / 11 / 12 / 13 各自的提交；把它们按时间**逆序**整理成 revert 清单。

- [ ] **Step 2: 逆序 revert 主仓库改动**

```bash
git revert --no-commit <最后一项 sha>   # 从最新开始
# ... 依次到最旧
git revert --continue
```
Expected: `.gitmodules` 消失，`app/web` / `app/desktop` / `app/mobile` / `packages` 恢复为普通文件；`release.yml` / `version-bump.yml` / `metadata.yaml` / 脚本回到拆分前形态。

- [ ] **Step 3: 处理回滚冲突（预期高发点）**

- `app/web` 等目录：revert 会把 submodule gitlink 换回普通文件，但工作区里仍是 submodule checkout → 先 `git submodule deinit -f <path>` 再 `git rm --cached` 后 checkout；
- `metadata.yaml`：若后续计划已依赖它，revert 会删除文件，需与依赖方确认；
- `.github/workflows/ci.yml`：revert 后主仓库 CI 需重新承担三端构建。

Expected: `git status` 干净，`pnpm typecheck && pnpm lint && pnpm test` 在主仓库全部通过。

- [ ] **Step 4: 恢复 pre-split 工作区状态**

若 Task 1 采用提交方案，确认该提交仍在历史中；若采用 stash，`git stash pop`。

Expected: 工作区恢复到拆分前状态。

- [ ] **Step 5: 子仓库处置（谨慎，不可逆）**

- **保留**：四个子仓库可作为只读历史档案保留，主仓库不再引用；
- **删除**：仅在确认不再需要历史时执行，且需负责人确认：
```bash
gh repo delete NoelOrin/GOSpeak-Web --yes
gh repo delete NoelOrin/GOSpeak-Desktop --yes
gh repo delete NoelOrin/GOSpeak-Mobile --yes
gh repo delete NoelOrin/GOSpeak-Packages --yes
```
Expected: 远端清理完成（或按决定保留）。