# Release 产物分离与独立发版 Implementation Plan

> 【状态】2026-11-27 核验：该独立发版路线已废弃（Submodule 拆分整体被「聚合壳 + 三子仓」取代），仅供历史参考；一组件一 Release 能力已在三仓拆分后的 server 仓独立发布中落地。

> **状态：已交付（作为 Submodule 拆分的前置子集）。** 本计划已完成主要改造并已合入 `release`：每个组件在自己的 tag 上建独立 Release、`desktop_included` 已删、`--latest` / `--latest=false` 分流、`docs.yml` 组件守卫、`mainTagPattern` 修正、`index.html` no-store 均已落地。
>
> **路线关系**：本计划是 [`2026-11-26-submodule-split.md`](./2026-11-26-submodule-split.md)（四仓 Submodule 拆分，**当前执行路线**）的**严格前置子集**——拆仓后的子仓 Release 必须已经具备「一组件一 Release」能力，否则会建到错误的 tag 上。因此本计划已先落地，Submodule 计划在此基础上继续。
>
> 保留本文作为该前置改造的实现记录与回归基线。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 三个目标：(1) 让每个组件拥有独立的 GitHub Release 条目，产物不再混在一起——当前 `gospeak-server-v1.3.0-rc.1` 一个 Release 里塞了 20 个附件；(2) 允许任一组件独立发版，改自己的 `.github/versions/<component>.yaml` 即可；(3) 优化移动端 CI，把 Android 构建从 macOS runner 迁到 Linux，去掉重复步骤，减少不必要的阻塞。

**Architecture:** 不拆仓库、不动目录结构。根因是 `release.yml` 里 `create-release` 硬编码了 server tag（第 157 行），导致任何组件触发都建到 server 的 Release 上；同时 `desktop_included` 逻辑（第 204-214 行）把桌面下载表格注入 server Release 正文。改为「哪个组件触发，就在哪个组件的 tag 上建 Release，只上传该组件产物」。配套删除 `version-bump.yml` 的 lockstep 同版本约束——它正是「产物挤在一个 Release」逼出来的。

**Tech Stack:** GitHub Actions · Node（scripts/*.mjs，零依赖）· Go 1.27

**背景决策记录：** 原「四端拆成独立 Git 仓库」方案一度被判定性价比过低而暂缓，其三项净收益（Release 产物分离、独立发版、移动端 CI 优化）先在本计划落地。**后续已确认继续执行仓库级拆分**，见 [`2026-11-26-submodule-split.md`](./2026-11-26-submodule-split.md)；本计划完成的部分正是其 Task 12 的前置条件。

## Global Constraints

- 工作目录：`/Users/noelorin/Projects/GOSpeak`；主仓库 `https://github.com/NoelOrin/GOSpeak.git`。
- 分支策略：从最新 `origin/release` 签出短期工作分支；唯一常驻分支为 `release`。
- 不拆仓库、不改目录结构、不改包管理配置。
- 脚本零依赖（对齐 `scripts/` 现有 20 个 .mjs 的风格）；发布类脚本改动必须配同名 `.test.mjs`。
- 代码不加注释（复杂逻辑除外）、不用 emoji；提交遵循 Conventional Commits。
- 文档与代码同 PR。

## Review Focus

- **产物仍混在一起**：验证方式是分别对 server 和 desktop 各跑一次 `workflow_dispatch`，检查两个 Release 条目的附件清单是否各自干净；Task 4 Step 3 钉住。
- **正文渲染错误**：`release-notes.mjs` 的 `--component` 分支必须让 server Release 只含服务端表格、desktop Release 只含安装包表格；Task 3 Step 5 钉住。
- **latest 语义漂移**：desktop Release 不能让 `/releases/latest` 指向它（文档下载卡与桌面端「前往下载页」依赖该语义）；Task 3 Step 6 用 `--latest=false` 钉住。
- **文档站误部署**：`docs.yml` 由 release 事件触发，desktop Release 不应触发文档部署；Task 4 Step 2 钉住。
- **lockstep 删除后版本漂移**：删除约束后 server 与 desktop 可各自独立发版，需确认 `version-bump.yml` 的 tag 存在性判断仍按组件独立工作；Task 1 Step 2 钉住。
- **移动端 CI 未真正提速**：迁到 Linux 后需实测耗时对比，确认 macOS 分钟数归零；Task 2 Step 5 钉住。

---

## 文件结构

| 文件 | 改动 |
|---|---|
| `.github/workflows/release.yml` | `create-release` 按触发组件建 Release；删除 `desktop_included` 注入；3 处上传目标改为自身 tag |
| `.github/workflows/version-bump.yml` | 删除 `Require lockstep component versions` 步骤（第 78-89 行） |
| `.github/workflows/docs.yml` | release 事件加组件前缀判断，只有 server Release 触发文档部署 |
| `scripts/release-naming.mjs` + `.test.mjs` | `COMPONENTS` 扩到四组件 |
| `scripts/release-notes.mjs` + `.test.mjs` | 新增 `--component`，按组件只渲染本组件表格 |
| `app/server/internal/router/router.go` | `serveSPA` 为 `index.html` 补 `Cache-Control: no-store` |
| `app/server/internal/service/system_update_service.go` + `_test.go` | 修 `mainTagPattern` |
| `app/docs/guide/desktop.md` | 下载说明改为指向桌面端 Release |
| `AGENTS.md` / `scripts/AGENTS.md` | 发布模型描述同步 |

---
## Task 1: 独立发版（删除 lockstep + 纳入 mobile）

**Files:**
- Modify: `.github/workflows/version-bump.yml`（删 lockstep、清理 released 语义、纳入 mobile）
- Modify: `.github/workflows/build-mobile.yml`（加 tag 触发、正式/测试双路径，见 Step 5）
- Modify: `scripts/release-naming.mjs` + `.test.mjs`（COMPONENTS 加 mobile，见 Step 3）
- Modify: `.github/versions/mobile.yaml`（删除「未定义 mobile tag 前缀」的过时注释）

**Interfaces:**
- Consumes: 现状（server/desktop 强制同版本；mobile 完全不在 version-bump 流程内）。
- Produces: 任一组件改自己的 `.github/versions/<component>.yaml` 即可单独发版，其余组件不受影响。

**现状盘点（重要）：** 下游编排**已经**是组件独立的：

```yaml
release-server:  if: released == 'true' && server_tag  != ''
release-desktop: if: released == 'true' && desktop_tag != ''
docker:          if: released == 'true' && server_tag  != ''
```

所以只改 desktop 版本时，server_tag 为空 → release-server 与 docker 自动跳过。**独立发版机制已存在**，本 Task 只需清理障碍并补齐 mobile。

- [ ] **Step 1: 删除 lockstep 校验步骤**

删除 `.github/workflows/version-bump.yml` 中名为 `Require lockstep component versions` 的整个 step（当前第 78-89 行），连同其 `SERVER_VERSION` / `DESKTOP_VERSION` 环境变量与比对逻辑。该约束强制两端同版本，是独立发版的硬阻塞。

Run:
```bash
grep -n "lockstep" .github/workflows/version-bump.yml
```
Expected: 无输出。

- [ ] **Step 2: 清理 `released` 中间变量的同步语义**

当前判定：

```bash
if [ "$SERVER_EXISTS" = "false" ] || [ "$DESKTOP_EXISTS" = "false" ]; then
  echo "released=true"     # 语义上像"一次发版"
```

改为按组件输出，去掉误导性的 `released`（下游 job 实际只依赖 `<component>_tag` 是否非空）：

```bash
# 每个组件独立决定自己的 tag 是否要打
for c in server desktop mobile; do
  if [ "[object Object]_EXISTS" = "false" ]; then
    echo "[object Object]_tag=$(node scripts/release-naming.mjs tag "$c" "$VERSION")" >> "$GITHUB_OUTPUT"
  else
    echo "[object Object]_tag=" >> "$GITHUB_OUTPUT"
  fi
done
```

下游 `if:` 同步去掉 `released == 'true' &&` 前缀，只保留 `<component>_tag != ''`。

Expected: `grep -n "released" .github/workflows/version-bump.yml` 无输出（或仅注释）。

- [ ] **Step 3: 打通 mobile 的 tag 命名（前置阻塞）**

移动端当前**完全不在发布体系内**，有三个硬阻塞必须先解：

| 阻塞 | 现状 | 处理 |
|---|---|---|
| `release-naming.mjs` 无 mobile | `COMPONENTS = ["server", "desktop"]`，传 mobile 直接抛错 | 扩为 `["server", "desktop", "mobile"]`（web 见 Task 4） |
| `release.yml` 无 mobile 分支 | 全部构建 job 只有 `component == 'server'` / `'desktop'` | 见 Step 4 |
| `mobile.yaml` 注释自述 | 「release-naming.mjs 未定义 mobile tag 前缀，检查时须显式传 component=mobile」 | 修完后删除该注释 |

先改 `scripts/release-naming.mjs`：

```js
const COMPONENTS = ["server", "desktop", "mobile"];
```

补测试：`releaseTag("mobile", "1.2.0") === "gospeak-mobile-v1.2.0"`、`parseReleaseTag("gospeak-mobile-v1.2.0")` 返回 `{component:"mobile", version:"1.2.0"}`。

Run: `node --test scripts/release-naming.test.mjs`
Expected: PASS。

- [ ] **Step 4: 把 mobile 纳入 version-bump 编排**

1. `Read declared versions` 补读 `.github/versions/mobile.yaml`，输出 `mobile`；
2. `Detect published tags` 补 `check mobile "$MOBILE_VERSION"`；
3. 新增 `Tag mobile release if version changed` step（照抄 server/desktop，`if: steps.tags.outputs.mobile_exists == 'false'`）；
4. 新增下游 job：

```yaml
release-mobile:
  name: Publish mobile release
  needs: [bump-tags]
  if: needs.bump-tags.outputs.mobile_tag != ''
  permissions:
    contents: write
  uses: ./.github/workflows/build-mobile.yml
  with:
    tag: ${{ needs.bump-tags.outputs.mobile_tag }}
```

**注意**：mobile 调用的是 `build-mobile.yml`（Capacitor/gradle 构建），不是 `release.yml`（Go 二进制构建）。两者构建环境不同。

Expected: `grep -n "mobile" .github/workflows/version-bump.yml` 在版本读取、tag 检测、打 tag、下游 job 四处均有命中。

- [ ] **Step 5: build-mobile.yml 支持正式发版**

当前状态（第 16-20、78-122 行）：
- 只有 `workflow_dispatch` 触发，**没有 tag 触发**
- 只构建 `assembleDebug`（debug APK），**无 release 签名产物**
- 产物固定上传到 `gospeak-mobile-test` 并 `--clobber` 覆盖
- iOS 构建整体注释禁用（第 149-166 行）

改造：

1. **加 tag 触发**：
   ```yaml
   on:
     push:
       tags: ['gospeak-mobile-v*']
     workflow_dispatch:
       inputs:
         tag: { description: "发布 tag（留空=只做测试包）", required: false }
     workflow_call:
       inputs:
         tag: { type: string, required: true }
   ```

2. **区分正式/测试两条路径**：
   - 正式（有 tag）：`gh release create "$TAG"` 创建正式 Release，产物命名 `gospeak-mobile-v<version>-debug.apk`，**不 clobber**
   - 测试（无 tag）：沿用 `gospeak-mobile-test` + `--clobber`，行为不变

3. **产物说明**：当前只有 debug APK，正式 Release 需在正文明确标注「当前为 debug 构建，正式签名产物待补」。若要 release APK，需先配置 Android 签名密钥（keystore + secrets），这是独立前置工作。

4. **iOS 保持禁用**：注释保留，正式 Release 只出 Android 产物。

Expected:
```bash
gh release view gospeak-mobile-v<version> --json assets -q '.assets[].name'
# 输出形如 gospeak-mobile-v1.1.0-debug.apk
```

- [ ] **Step 6: mobile 版本同步链路确认**

`scripts/mobile-version.mjs` 已把 `mobile.yaml` 的 version 同步到四处：`app/mobile/package.json`、Android `build.gradle`（versionName + versionCode）、iOS `project.pbxproj`、以及 `app/web/package.json`（供 `__APP_VERSION__`）。

确认 bump mobile 版本时这条链路仍工作：

Run: `node scripts/mobile-version.mjs --check`（或对应校验命令，见脚本内 usage）
Expected: 四处版本一致，无漂移。

- [ ] **Step 7: 验证独立发版（三场景）**

依次验证（只改对应 yaml 的 version 后合并）：

| 场景 | 改哪个文件 | 期望创建的 tag | 期望 skipped 的 job |
|---|---|---|---|
| A 只发桌面端 | `desktop.yaml` | `gospeak-desktop-v*` | release-server、docker、release-mobile |
| B 只发服务端 | `server.yaml` | `gospeak-server-v*` | release-desktop、release-mobile |
| C 只发移动端 | `mobile.yaml` | `gospeak-mobile-v*` | release-server、release-desktop、docker |

Run: `gh run view <run-id>`
Expected: 三种场景的 skipped job 与上表一致；对应 Release 只含本组件产物。

- [ ] **Step 8: 提交**

```bash
git add .github/workflows/version-bump.yml .github/workflows/build-mobile.yml .github/versions/ scripts/release-naming.mjs scripts/release-naming.test.mjs
git commit -m "ci: allow independent per-component releases including mobile"
```

---

## Task 2: 移动端 CI 优化（减少阻塞）

**Files:**
- Modify: `.github/workflows/build-mobile.yml`
- Modify: `app/mobile/package.json`（新增 `sync:no-bind` 脚本）

**Interfaces:**
- Consumes: Task 1 的 mobile 发版链路。
- Produces: Android 构建从 macOS runner 迁到 Linux，耗时与成本大幅下降；iOS 保持独立可恢复。

**现状问题（已核实）：**

| 问题 | 证据 | 影响 |
|---|---|---|
| Android 跑在 `macos-latest` | `build-mobile.yml:36` | macOS runner 成本约为 Linux 10 倍且排队久；iOS 已整体注释禁用（145-166 行），无步骤真正需要 macOS |
| `sync` 重复 bind | `app/mobile/package.json:13` 的 `sync` 含 `bind:rules`，而 CI 第 73 行已单独跑 `mobile-bind-proxyrules.mjs` | gomobile 编译跑两遍 |
| `--no-daemon` 放弃增量 | `build-mobile.yml:79` | 配合 `setup-gradle@v6` 缓存时反而更慢 |
| 不取消旧构建 | `build-mobile.yml:31` `cancel-in-progress: false` | 连续触发时浪费 runner |

- [ ] **Step 1: 拆分 Android 与 iOS job**

把单 job 拆成两个：

```yaml
jobs:
  android:
    name: Build Android APK
    runs-on: ubuntu-latest          # ← 从 macos-latest 改过来
    steps:
      - uses: actions/checkout@v7
      - uses: ./.github/actions/pnpm-install
      - uses: actions/setup-java@v6
        with:
          distribution: temurin
          java-version: '21'
      - uses: gradle/actions/setup-gradle@v6
      - uses: actions/setup-go@v7
        with:
          go-version-file: app/shared/proxyrules/go.mod
          cache: false
      - uses: ./.github/actions/go-build-cache
        with:
          go-sum-path: app/shared/proxyrules/go.sum
          cache-prefix: mobile-bind-android
      - run: node scripts/mobile-bind-proxyrules.mjs --target=android
      - run: pnpm --filter @gospeak/mobile sync:no-bind
      - run: cd app/mobile/android && ./gradlew assembleDebug
      # upload + publish 步骤照旧

  ios:
    name: Build iOS (disabled)
    if: false                        # ← 保持禁用，解开时改条件
    runs-on: macos-latest
    steps: [... 原注释中的三段 ...]
```

**关键**：`gomobile bind --target=android` 在 Linux 上产 AAR，不需要 macOS。XCFramework 只在 iOS job 里需要。

Expected: `grep -n "runs-on" .github/workflows/build-mobile.yml` 显示 android 用 `ubuntu-latest`、ios 用 `macos-latest`。

- [ ] **Step 2: 新增 sync:no-bind 脚本**

`app/mobile/package.json` 增加：

```json
"sync:no-bind": "pnpm run version:sync && pnpm run build:web && cap sync"
```

CI 用 `sync:no-bind`，避免与第 73 行/Step 1 的显式 bind 重复。

Expected: `grep -n "sync:no-bind" app/mobile/package.json` 有命中。

- [ ] **Step 3: 去掉 --no-daemon**

```yaml
- run: cd app/mobile/android && ./gradlew assembleDebug
```

配合 `gradle/actions/setup-gradle@v6` 的缓存，保留 daemon 能复用增量编译结果。

Expected: `grep -c "no-daemon" .github/workflows/build-mobile.yml` 输出 0。

- [ ] **Step 4: 允许取消旧构建**

```yaml
concurrency:
  group: build-mobile-${{ github.ref }}
  cancel-in-progress: true
```

并在 publish 步骤加 `if: !cancelled()`，避免取消时留下半套 assets。

Expected: `grep -n "cancel-in-progress" .github/workflows/build-mobile.yml` 显示 `true`。

- [ ] **Step 5: 验证**

```bash
/tmp/actionlint -color .github/workflows/build-mobile.yml
gh workflow run build-mobile.yml
gh run view <run-id> --log
```

Expected:
- actionlint 无错误
- Android job 在 `ubuntu-latest` 上成功产出 APK
- 总耗时相比改造前下降（记录改造前后耗时对比）

- [ ] **Step 6: 提交**

```bash
git add .github/workflows/build-mobile.yml app/mobile/package.json
git commit -m "ci: move android build to linux runner and reduce redundant steps"
```

---

## Task 3: 一组件一 Release

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/release-notes.mjs` + `.test.mjs`

**Interfaces:**
- Consumes: Task 1 的独立版本。
- Produces: 每个组件在自己的 tag 上创建独立 Release，只含本组件产物。

- [ ] **Step 1: 改 create-release 的 tag 来源**

`release.yml` 第 157 行改为按触发组件取 tag：

```bash
# 改前
release_tag="$(node scripts/release-naming.mjs tag server "$RELEASE_VERSION")"

# 改后
release_tag="${{ needs.resolve-tag.outputs.tag }}"
```

**同时修正 Release 标题**（第 252 行当前写死 `--title "GOSpeak v$RELEASE_VERSION"`）：

```bash
# 按组件生成
case "$COMPONENT" in
  server)  title="GOSpeak Server v$RELEASE_VERSION" ;;
  desktop) title="GOSpeak Desktop v$RELEASE_VERSION" ;;
  mobile)  title="GOSpeak Mobile v$RELEASE_VERSION" ;;
esac
args=( "$release_tag" --target "$target_sha" --title "$title" --notes-file /tmp/release-notes.md )
```

Expected: 两个 Release 的标题分别显示 Server / Desktop，不再都是 "GOSpeak v..."。

- [ ] **Step 1b: 修正 resolve-tag 的默认回退**

`release.yml` 第 92-95 行在无 `INPUT_TAG` 时按创建时间取「最新 tag」，但 `grep -E '^gospeak-(server|desktop)-v[0-9]'` 会把两个组件混在一起取——可能取到 desktop tag 却按 server 流程构建。

改为显式要求 tag（手动触发必须传）：

```bash
# 手动触发必须传 tag，避免歧义
if [ -z "$tag" ]; then
  echo "::error::workflow_dispatch requires tag input; refusing to guess"
  exit 1
fi
```

保留 `push` 事件从 `GITHUB_REF_NAME` 取 tag 的路径（tag 推送天然明确）。

Expected: 手动触发不传 tag 时明确报错，不会误用其他组件的 tag。

- [ ] **Step 2: 删除 desktop_included 注入逻辑**

删除 `release.yml` 第 204-224 行中 `desktop_version` / `desktop_args` / `desktop_included` 的整段逻辑，以及 `--desktop-version` / `--desktop-included` / `--desktop-notes-file` 参数传递。

Expected: `grep -n "desktop_included" .github/workflows/release.yml` 无输出。

- [ ] **Step 3: 改 3 处上传目标**

`release.yml` 第 592 / 759 / 824 行的 `TAG:` 全部改为：

```yaml
TAG: ${{ needs.resolve-tag.outputs.tag }}
```

这样 server 产物传到 server tag、desktop 产物传到 desktop tag。

- [ ] **Step 4: release-notes.mjs 增加 --component**

**现状问题（已核实）：**

`renderReleaseBody`（第 310-340 行）里有两处会破坏一组件一 Release：

1. `const tag = releaseTagForVersion(version)` —— 硬编码为 **server tag**（第 46 行 `releaseTagForVersion` 只返回 `gospeak-server-v*`），导致 desktop Release 的下载链接指向 server tag；
2. 第 322-326 行的 TIP 文案写死「服务端二进制、容器镜像与桌面端安装包**同属** `${tag}`」，与拆分后的模型矛盾。

改造：

```js
export function renderReleaseBody({ component, version, repo, tag, serverNotes, desktopNotes }) {
  const parts = [];
  if (component === "server") {
    parts.push("## 下载 GOSpeak Server");
    parts.push(renderServerTable(version, repo, tag));
    parts.push(renderDeploySection(repo, tag, version));
  } else if (component === "desktop") {
    parts.push("## 下载 GOSpeak Desktop");
    parts.push(renderDesktopTable(version, repo, tag));
    parts.push(renderDesktopRequirements());   // 新增：系统要求说明
  }
  return parts.join("\n");
}
```

`parseArgs`（第 366 行）加 `--component`（必填）与 `--tag`（由 workflow 传入实际 tag，不再从 version 推导）。

删除 `releaseTagForVersion` 在 body 渲染中的使用；保留该函数供 `checksums` 命名使用。

Expected: `grep -n "同属" scripts/release-notes.mjs` 无输出（旧文案已删）。

- [ ] **Step 5: 写测试**

在 `scripts/release-notes.test.mjs` 增加：
- `--component server` 的输出不含 `.dmg` / `.deb` / `install.sh`
- `--component desktop` 的输出不含 `gospeak-server-` 二进制条目

Run: `node --test scripts/release-notes.test.mjs`
Expected: PASS。

- [ ] **Step 6: latest 语义保护**

`gh release create` 时：desktop 传 `--latest=false`，server 传 `--latest`。保证 `/releases/latest` 始终指向服务端版本（文档下载卡与桌面端「前往下载页」依赖它）。

Expected: `grep -n "latest" .github/workflows/release.yml` 能看到明确的 `--latest` / `--latest=false` 分支。

- [ ] **Step 7: 提交**

```bash
git add .github/workflows/release.yml scripts/release-notes.mjs scripts/release-notes.test.mjs
git commit -m "ci: create one release per component with isolated artifacts"
```

---
## Task 4: 扩展组件解析并验证产物分离

**Files:**
- Modify: `scripts/release-naming.mjs` + `.test.mjs`
- Modify: `.github/workflows/docs.yml`

**Interfaces:**
- Consumes: Task 3 的按组件 Release。
- Produces: 组件名解析完整；文档站不被桌面 Release 误触发。

- [ ] **Step 1: 扩展 COMPONENTS**

`scripts/release-naming.mjs` 第 22 行 `COMPONENTS = ["server", "desktop"]` 扩为 `["server", "web", "desktop", "mobile"]`，并补对应 `releaseTag` / `parseReleaseTag` 测试。

Run: `node --test scripts/release-naming.test.mjs`
Expected: PASS。

- [ ] **Step 2: docs.yml 加组件判断**

`.github/workflows/docs.yml` 的 release 触发步骤加：

```yaml
if: startsWith(github.event.release.tag_name, 'gospeak-server-v')
```

避免桌面 Release 触发文档部署。

Expected: `grep -n "gospeak-server-v" .github/workflows/docs.yml` 有命中。

- [ ] **Step 3: 实跑验证**

分别对 server 与 desktop 各触发一次：

```bash
gh workflow run release.yml -f tag=gospeak-server-v<当前版本>
gh workflow run release.yml -f tag=gospeak-desktop-v<当前版本>
```

然后检查：

```bash
gh release view gospeak-server-v<版本> --json assets -q '.assets[].name'
gh release view gospeak-desktop-v<版本> --json assets -q '.assets[].name'
```

Expected: server Release 只有 `gospeak-server-*` 二进制与 `SHA256SUMS-gospeak-server-*`；desktop Release 只有 `.dmg` / `.deb` / `.exe` / `install.sh` 与 `SHA256SUMS-gospeak-desktop-*`。

- [ ] **Step 4: 提交**

```bash
git add scripts/release-naming.mjs scripts/release-naming.test.mjs .github/workflows/docs.yml
git commit -m "ci: extend release components and guard docs deploy"
```

---

## Task 5: 修复两个现存 bug

**Files:**
- Modify: `app/server/internal/service/system_update_service.go` + `_test.go`
- Modify: `app/server/internal/router/router.go`

**Interfaces:**
- Consumes: 无（与 Task 1-4 独立）。
- Produces: 更新检测恢复工作；前端首页不被缓存。

- [ ] **Step 1: 修 mainTagPattern 与全部 TrimPrefix**

`system_update_service.go` 第 31 行：

```go
// 改前：匹配不到任何现网 tag（仓库里 0 个 v* tag，11 个 gospeak-server-v*）
var mainTagPattern = regexp.MustCompile(`^v[0-9]`)

// 改后
var mainTagPattern = regexp.MustCompile(`^gospeak-server-v[0-9]`)
```

**必须同步改全部 4 处 `TrimPrefix`**（漏一处就会出现 `Latest` 变成 `gospeak-server-v1.3.0` 导致 `parseVersion` 失败、`Available` 恒 false）：

| 行号 | 当前 | 改为 |
|---|---|---|
| 117 | `strings.TrimPrefix(release.TagName, "v")` | `strings.TrimPrefix(release.TagName, "gospeak-server-v")` |
| 171 | `strings.TrimPrefix(r.TagName, "v")` | 同上 |
| 174（两处） | `strings.TrimPrefix(r.TagName, "v")` / `strings.TrimPrefix(latest.TagName, "v")` | 同上 |

**保留不改**：第 225 行 `parseVersion` 里的 `TrimPrefix(v, "v")` 是通用解析，继续兼容带 v 前缀的输入。

Expected: `grep -n 'TrimPrefix.*"v"' app/server/internal/service/system_update_service.go` 只剩第 225 行一处。

- [ ] **Step 2: 写测试**

在 `system_update_service_test.go` 增加：`gospeak-server-v1.3.0` 能被识别、`gospeak-desktop-v1.3.0` 被忽略。

Run: `cd app/server && go test ./internal/service/ -run TestSystemUpdate`
Expected: PASS。

- [ ] **Step 3: serveSPA 补 no-store**

`router.go` 的 `serveSPA` 中，两个 `serveIndex` 实现（第 197 行外部目录分支、第 214 行 embed 分支）在返回前各加一行：

```go
c.Header("Cache-Control", "no-store")
```

只针对 `index.html`；带 hash 的静态资源保持长缓存。

- [ ] **Step 4: 提交**

```bash
git add app/server/internal/service/system_update_service.go app/server/internal/service/system_update_service_test.go app/server/internal/router/router.go
git commit -m "fix: update check tag pattern and index.html cache header"
```

---

## Task 6: 文档同步

**Files:**
- Modify: `app/docs/guide/desktop.md`
- Modify: `AGENTS.md` / `scripts/AGENTS.md`

- [ ] **Step 1: 更新桌面端下载说明**

`app/docs/guide/desktop.md` 第 17-18 行的下载链接改为指向 `gospeak-desktop-v*` 的 Release 条目，而不是 server Release。

- [ ] **Step 2: 更新 AGENTS.md 发布模型**

关于「一版一个 Release」的描述改为「一组件一 Release，各自独立」。

Run: `grep -rn "一版一个 Release\|统一 Release" AGENTS.md scripts/AGENTS.md app/docs/`
Expected: 无残留旧描述。

- [ ] **Step 3: 提交**

```bash
git add app/docs/guide/desktop.md AGENTS.md scripts/AGENTS.md
git commit -m "docs: reflect per-component release model"
```

---

## Task 7: 回滚预案

**Files:** 无（操作任务）

- [ ] **Step 1: 撤销 workflow 改动**

```bash
git revert <Task 1-5 commit sha>
```
Expected: 回到「一版一个 Release」的旧行为；已存在的 Release 条目不受影响。

- [ ] **Step 2: 清理多余 Release（可选）**

若需回到单一 Release，手工把 desktop Release 的附件移到 server Release 后删除前者：

```bash
gh release download gospeak-desktop-v<版本> -D /tmp/desktop-assets
gh release upload gospeak-server-v<版本> /tmp/desktop-assets/* --clobber
gh release delete gospeak-desktop-v<版本> --yes
```