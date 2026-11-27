# 三仓拆分与装配 Implementation Plan

> 【状态】2026-11-27 核验：该计划已交付（聚合壳 + 三子仓已落地），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在迁移完成的前提下，把单体仓拆成「聚合壳 `GOSpeak` + 三个平级子仓」，壳挂 3 条 submodule，客户端内部零 submodule。

**Architecture:** 三个子仓在临时 clone 里用 `filter-branch` 抽取历史——**保留 `scripts/` 与 `.github/`**，避免「先删再重建」的断链。抽取后把壳清空、加 `.gitmodules`、push 到 `release`。客户端把 `app/{web,desktop,mobile}` 与 `packages` 提到仓根、`app/shared/proxyrules` 提到仓根 `proxyrules/`。

**Tech Stack:** Git 2.50 · pnpm 10.11 · Go 1.27 · GitHub Actions · VitePress

**Spec:** `docs/superpowers/specs/2026-11-26-two-repo-split-design.md`
**前置计划:** `docs/superpowers/plans/2026-11-26-org-migration.md`（**必须先完成**）
**Review 证据:** 6 角度 59 条发现见 [附录](./2026-11-26-three-repo-split.review-appendix.md)

## Global Constraints

- 工作目录 `/Users/noelorin/Projects/GOSpeak`；四仓全 public，同在 `GOSpeak-org`。
- **只有一层父子**：壳挂 3 条 submodule；客户端仓内部 0 submodule。
- 客户端目录在仓内**平级**：`web/`、`packages/`、`desktop/`、`mobile/`、`proxyrules/`。
- 服务端目录扁平：`server/`、`test/`、`deploy/`、`docs/`、`scripts/`、`.github/`。
- **禁止**在 CI/脚本里用 `git submodule update --remote`。
- 客户端首个版本 `1.4.0-rc.1`（必须 > 现网 `1.3.0-rc.1` / `1.1.0-rc.2`）。
- 代码不加注释（复杂逻辑除外）、不用 emoji；Conventional Commits。

### 抽取后预期数字（实测，取自 2026-11-26）

```
release 提交数（合并 feature 后）   约 111
Task 2 抽取后（服务端仓）           47 → 合并后会变，必须重测
Task 3 抽取后（客户端仓）           53 → 同上
app/web 文件数                      442
app/server 文件数                   468
```

**注意**：47/53 是在「`release` 未合并 feature」前提下的实测值。迁移计划已把 feature 合并进 `release`，所以**必须在 Task 2/3 里先实测一次再写死断言**，不要照抄。

## Review Focus

- **壳从未 push** → Task 5 Step 4 强制 `push origin HEAD:release`。
- **gitlink 从未 bump** → Task 13 Step 1 显式 bump 后 clone 才是真实状态。
- **协议生成物跨仓不同步** → Task 0 前置解耦 + Task 8 接入 gate。
- **已装机客户端更新通道** → `updater_release.go:33` 仓名编译在二进制里，Task 10 留桥接 Release。
- **版本静默退化 `dev`** → `buildinfo` 缺 `../VERSION` 候选，Task 9 Step 1 钉住。
- **Dockerfile 前端阶段** → embed 是编译期，必须填在 go-builder 阶段；Task 9 Step 3 用 `curl` 验 `<div id="root"`。
- **workspace 漏 desktop/mobile** → `pnpm -r build` 静默跳过且 exit 0；Task 7 Step 1 加存在性断言。
- **Pages 不随仓转移** → `base` 写死 `/GOSpeak/`，Task 6 Step 4 与 Task 7 钉住。
- **submodule detached HEAD** → `clone --recursive` 后是游离 HEAD，Task 5 Step 3 切回 `release`。
- **服务端缺 `.gitignore`** → public 仓会暴露真实 `.env` 与 DB，Task 6 Step 1 钉住。

---

## 文件结构

**`GOSpeak`（壳）**：`.gitmodules`、`README.md`、`.github/workflows/shell-drift.yml`、`scripts/check-shell-drift.mjs`

**`GOSpeak-Server`**：`server/`、`test/`、`deploy/`、`docs/`、`scripts/`、`.github/`、`Dockerfile`、`.dockerignore`、`.gitignore`、`package.json`、`pnpm-workspace.yaml`、`VERSION`、`metadata.yaml`、`CHANGELOG.md`、`LICENSE`

**`GOSpeak-Client`**：`web/`、`packages/`、`desktop/`、`mobile/`、`proxyrules/`、`scripts/`、`.github/`、`pnpm-workspace.yaml`、`.gitignore`、`package.json`、`biome.json`、`lefthook.yml`、`turbo.json`、`commitlint.config.js`

**`GOSpeak-Docs`**：仓根即原 `app/docs` + `.github/workflows/docs.yml` + `LICENSE`

---

## Task 0: 协议解耦前置（仍在原仓内做）

**Files:**
- Delete: `app/server/internal/protocol/gen_contract_test.go`
- Modify: `app/server/cmd/gen-protocol/main.go`、`app/server/internal/genprotocol/generate.go:385`
- Modify: `package.json`（`gen:protocol`）、`lefthook.yml:34-36`

**Interfaces:**
- Produces: `gen-protocol -out` 可用；契约测试只依赖 golden。后续 tree-filter 直接带走，子仓开箱即通。

**为什么前置**：Task 1 的验收要跑 `go test ./internal/protocol/`，而该测试读 `../../../../app/web/src/protocol/generated`——抽取后路径不存在，实测报 `no such file or directory`。放后面会造成「Task 1 验收依赖 Task 4」的鸡生蛋。

- [ ] **Step 1: 确认 golden 与 generated 一致**

```bash
cd /Users/noelorin/Projects/GOSpeak
diff -r app/server/internal/genprotocol/testdata/golden app/web/src/protocol/generated && echo "byte-identical"
```
Expected: `byte-identical`（实测当前完全一致，这是后面 `--fix` 设计的基础）。

- [ ] **Step 2: 删除读 app/web 的契约测试**

```bash
rm app/server/internal/protocol/gen_contract_test.go
cd app/server && go test ./internal/protocol/ ./internal/genprotocol/
```
Expected: 通过；`grep -rn 'app/web' internal/protocol/` 无命中。golden 新鲜度由 `genprotocol/generate_test.go` 的 `TestGolden` 覆盖（已存在，逻辑等价，勿重复实现）。

- [ ] **Step 3: `-out` 必填、与 `-update-golden` 互斥**

`main.go`：去掉默认 `outDir`；缺 `-out` 且未给 `-update-golden` 时报错退出；`-update-golden` 固定写 `internal/genprotocol/testdata/golden`。同步改 `:14` 帮助文本与 `generate.go:385` 生成头（把 `pnpm gen:protocol` 换成新命令，否则每次生成都把过期指令写进产物）。

```bash
cd app/server
go run ./cmd/gen-protocol 2>&1 | head -2                    # 报错
go run ./cmd/gen-protocol -out /tmp/gen && ls /tmp/gen       # 三份
go run ./cmd/gen-protocol -update-golden
git diff --exit-code -- internal/genprotocol/testdata/golden # 幂等
```
Expected: 三条均符合。

- [ ] **Step 4: 更新入口脚本**

`package.json`：`"gen:protocol": "cd app/server && go run ./cmd/gen-protocol -out ../web/src/protocol/generated"`
`lefthook.yml:34-36` 保持不变（仍写 `app/web/...`，Task 7 再改到子仓）。

```bash
pnpm gen:protocol && git diff --exit-code -- app/web/src/protocol/generated
```
Expected: 退出 0。

- [ ] **Step 5: 提交**

```bash
git add -A
git commit -m "refactor: decouple protocol generation from client directory"
git push origin release
```

---

## Task 1: 抽取服务端仓

**Files:** 临时目录 `/tmp/extract-server`

**Interfaces:**
- Consumes: 迁移计划产出（`release` 已含 24 个 feature 提交）。
- Produces: `GOSpeak-Server` 的 `release` 分支，含 `server/ test/ deploy/ docs/ scripts/ .github/`。

- [ ] **Step 1: 在临时 clone 里重排目录（保留 scripts 与 .github）**

```bash
rm -rf /tmp/extract-server
git clone --no-local --branch release /Users/noelorin/Projects/GOSpeak /tmp/extract-server
cd /tmp/extract-server
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --tree-filter '
  mkdir -p _s
  [ -d app/server ] && mv app/server _s/server
  for d in test deploy docs scripts .github; do [ -d "$d" ] && mv "$d" _s/; done
  for f in Dockerfile VERSION metadata.yaml CHANGELOG.md LICENSE; do [ -f "$f" ] && mv "$f" _s/; done
  rm -rf app packages agent_test_logs artifacts node_modules .agents .tmp-visual-proposal
  for x in _s/* _s/.[!.]*; do [ -e "$x" ] && mv "$x" .; done
  rmdir _s 2>/dev/null
  rm -f .gitignore pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md README.md .dockerignore .npmrc
  [ -d server ] && [ -d test ] || exit 1
  true
' -- --all
```
Expected: 仓根含 `server/ test/ deploy/ docs/ scripts/ .github/` + `Dockerfile VERSION LICENSE`。

**关键**：`scripts/` 与 `.github/` **必须保留**——Task 6/7 要按归属拆分它们，若此处删除，Task 6 就没有可复制的源（实测原仓 `scripts/` 有 13 个文件、`.github/` 有 23 个 tracked 文件）。

结尾的 `[ -d server ] && [ -d test ] || exit 1` 是防御：`git-filter-branch` 没有 `set -e`，`mv` 失败会被静默吞掉，随后 `rm -rf` 就把源删了。

- [ ] **Step 2: 清理 tag / 备份 ref，并实测提交数**

```bash
git tag -d $(git tag -l) 2>/dev/null || true
git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d 2>/dev/null || true
git branch -m release
echo "commits=$(git rev-list --count HEAD)"
echo "server_files=$(git ls-files server | wc -l)"
echo "scripts=$(git ls-files scripts | wc -l)"
```
Expected: **先记录实测值**，再写进 Expected。参考：未合并 feature 时是 47；合并后应更高。`server_files` 应为 `468`（与基线的 `app/server` 一致）；`scripts` 应 > 0。

- [ ] **Step 3: 验证可构建**

```bash
cd server && go build ./... && go test ./internal/protocol/ ./internal/genprotocol/ -count=1
```
Expected: 通过（Task 0 已解耦，不再依赖 `app/web`）。

- [ ] **Step 4: 推送并锚定默认分支**

```bash
cd /tmp/extract-server
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak-Server.git
git push origin release --no-tags
gh repo edit GOSpeak-org/GOSpeak-Server --default-branch release
gh api repos/GOSpeak-org/GOSpeak-Server --jq .default_branch
```
Expected: 输出 `release`。

---

## Task 2: 抽取客户端仓

**Files:** 临时目录 `/tmp/extract-client`

**Interfaces:**
- Produces: `GOSpeak-Client` 的 `release` 分支，含 `web/ packages/ desktop/ mobile/ proxyrules/ scripts/ .github/`。

- [ ] **Step 1: 重排目录（proxyrules 提到仓根）**

```bash
rm -rf /tmp/extract-client
git clone --no-local --branch release /Users/noelorin/Projects/GOSpeak /tmp/extract-client
cd /tmp/extract-client
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --tree-filter '
  mkdir -p _c
  for d in web desktop mobile; do [ -d "app/$d" ] && mv "app/$d" "_c/$d"; done
  [ -d app/shared/proxyrules ] && mv app/shared/proxyrules _c/proxyrules
  [ -d packages ] && mv packages _c/
  for d in scripts .github; do [ -d "$d" ] && mv "$d" _c/; done
  rm -rf app test deploy docs server agent_test_logs artifacts node_modules .agents .tmp-visual-proposal
  for x in _c/* _c/.[!.]*; do [ -e "$x" ] && mv "$x" .; done
  rmdir _c 2>/dev/null
  rm -f .gitignore pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md README.md Dockerfile VERSION metadata.yaml CHANGELOG.md .dockerignore .npmrc
  [ -d web ] && [ -d proxyrules ] && [ -d scripts ] || exit 1
  true
' -- --all
```
Expected: 仓根含 `web/ packages/ desktop/ mobile/ proxyrules/ scripts/ .github/`。

- [ ] **Step 2: 清理并实测**

```bash
git tag -d $(git tag -l) 2>/dev/null || true
git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d 2>/dev/null || true
git branch -m release
echo "commits=$(git rev-list --count HEAD)"
echo "web_files=$(git ls-files web | wc -l)"          # 应为 442
echo "proxyrules=$(git ls-files proxyrules | wc -l)"  # 应为 4
git log --oneline -3 -- web
```
Expected: `web_files=442`、`proxyrules=4`；`web` 下有真实历史提交。提交数**先实测再写断言**。

- [ ] **Step 3: 推送并锚定默认分支**

```bash
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak-Client.git
git push origin release --no-tags
gh repo edit GOSpeak-org/GOSpeak-Client --default-branch release
```
Expected: 推送成功；默认分支为 `release`。

---

## Task 3: 抽取文档仓

**Files:** 临时目录 `/tmp/extract-docs`

**Interfaces:**
- Produces: `GOSpeak-Docs` 的 `release` 分支。

- [ ] **Step 1: 抽取并补 LICENSE 与 workflow**

```bash
rm -rf /tmp/extract-docs
git clone --no-local --branch release /Users/noelorin/Projects/GOSpeak /tmp/extract-docs
cd /tmp/extract-docs
FILTER_BRANCH_SQUELCH_WARNING=1 git filter-branch -f --prune-empty --subdirectory-filter app/docs HEAD
git tag -d $(git tag -l) 2>/dev/null || true
git for-each-ref --format='%(refname)' refs/original | xargs -n1 git update-ref -d 2>/dev/null || true
git branch -m release
# LICENSE 不会随 subdirectory-filter 带过来，站点 footer 却声明 Apache 2.0
cp /Users/noelorin/Projects/GOSpeak/LICENSE .
```
Expected: 仓根含 `.vitepress/ guide/ api/ package.json LICENSE`。

- [ ] **Step 2: 改 Pages base（否则线上资源全 404）**

```bash
grep -n "const base" .vitepress/config.ts
sed -i '' "s|isProd ? '/GOSpeak/' : '/'|isProd ? '/GOSpeak-Docs/' : '/'|" .vitepress/config.ts
grep -n "const base" .vitepress/config.ts
```
Expected: 改为 `'/GOSpeak-Docs/'`。**为什么**：Pages 地址会变成 `https://gospeak-org.github.io/GOSpeak-Docs/`，且 GitHub 明确不重定向 Pages。

- [ ] **Step 3: 补 docs workflow（原 `.github` 进不来）**

`--subdirectory-filter` 只能带走 `app/docs` 内部，仓根的 `.github/workflows/docs.yml` 进不了 Docs 仓（实测 `git ls-files | grep -c '^.github'` = 0）。新建 `.github/workflows/docs.yml`：

```yaml
name: Docs
on:
  push:
    branches: [release]
  workflow_dispatch:
permissions:
  contents: read
  pages: write
  id-token: write
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: github-pages
    steps:
      - uses: actions/checkout@v7
      - uses: pnpm/action-setup@v6
      - uses: actions/setup-node@v6
        with: { node-version: 22, cache: pnpm }
      - run: pnpm install --frozen-lockfile
      - run: pnpm build
      - uses: actions/upload-pages-artifact@v5
        with: { path: .vitepress/dist }
      - uses: actions/deploy-pages@v5
```

- [ ] **Step 4: 验证独立构建**

```bash
pnpm install && pnpm build
ls -d .vitepress/dist
grep -o '/GOSpeak-Docs/guide' .vitepress/dist/index.html | head -1
```
Expected: 产物存在，且链接前缀为 `/GOSpeak-Docs/`。

- [ ] **Step 5: 推送**

```bash
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak-Docs.git
git add -A && git commit -m "docs: add license, pages base and deploy workflow"
git push origin release --no-tags
gh repo edit GOSpeak-org/GOSpeak-Docs --default-branch release
```
Expected: 推送成功。Pages 站点需在 GitHub 上手动启用（Settings → Pages → Source: GitHub Actions），Task 10 验收时确认。

---

## Task 4: 现有仓变聚合壳

**Files:**
- Delete: 全部代码目录
- Create: `.gitmodules`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1/2/3 已推送的三个子仓。
- Produces: 壳只剩 `.gitmodules` + `README.md` + 3 个 gitlink，且**已 push 到 release**。

- [ ] **Step 1: 清空代码目录（保留 `.git` 与 `LICENSE`）**

```bash
cd /Users/noelorin/Projects/GOSpeak
git rm -r --cached app packages test deploy docs scripts agent_test_logs artifacts .github 2>/dev/null || true
rm -rf app packages test deploy docs scripts agent_test_logs artifacts .github node_modules .agents .tmp-visual-proposal
git rm --cached Dockerfile VERSION metadata.yaml CHANGELOG.md pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md .dockerignore .npmrc 2>/dev/null || true
rm -f Dockerfile VERSION metadata.yaml CHANGELOG.md pnpm-workspace.yaml package.json pnpm-lock.yaml turbo.json biome.json lefthook.yml commitlint.config.js ARCHITECTURE.md CLAUDE.md AGENTS.md .dockerignore .npmrc
ls -a
```
Expected: 只剩 `.git .gitignore LICENSE README.md`。

**注意**：`.agents/` 有 24 个 tracked 文件（含硬编码 `app/` 路径的技能），实测会随抽取进入子仓，此处必须显式删除。

- [ ] **Step 2: 加三条 submodule**

```bash
git submodule add -b release https://github.com/GOSpeak-org/GOSpeak-Server.git server
git submodule add -b release https://github.com/GOSpeak-org/GOSpeak-Client.git client
git submodule add -b release https://github.com/GOSpeak-org/GOSpeak-Docs.git   docs
git config -f .gitmodules --get-regexp 'submodule\..*\.path'
```
Expected: 三条 path 精确为 `server`、`client`、`docs`。

- [ ] **Step 3: 切回 release 分支（避免 detached HEAD）**

`git submodule add -b` 会留下本地跟踪分支，但**重新 clone 后是游离 HEAD**（实测 `git clone --recursive` 后 `git -C server symbolic-ref HEAD` 返回空，`git push` 会 `fatal: You are not currently on a branch.`）。

```bash
for d in server client docs; do
  git -C $d switch release
  printf '%s: ' "$d"; git -C $d branch --show-current
done
```
Expected: 三个都输出 `release`。

- [ ] **Step 4: 提交并 push 到 release**

```bash
git add -A
git commit -m "refactor: turn shell repo into aggregate of server client docs"
git push origin HEAD:release
git ls-remote origin refs/heads/release | cut -c1-8
git rev-parse --short HEAD
```
Expected: 远端 SHA 与本地 HEAD 一致。

**这一步是整份计划的关键**：没有它，Task 9 的 `git clone --recursive` 拿到的仍是拆分前单体仓。

- [ ] **Step 5: 写壳 README**

```markdown
# GOSpeak

这是**聚合壳仓库**，只用 submodule 把三个子仓锚定在一起，方便一次 `git clone --recursive` 拿全部。

| 目录 | 仓库 | 说明 |
|---|---|---|
| `server/` | GOSpeak-Server | Go 后端、部署、文档 |
| `client/` | GOSpeak-Client | Web / 桌面 / 移动 / 共享规则 |
| `docs/` | GOSpeak-Docs | VitePress 文档站 |

## 开发请直接 clone 对应子仓

```bash
git clone https://github.com/GOSpeak-org/GOSpeak-Client.git
```

**不要在壳里改代码**：三个目录是 submodule，默认处于游离 HEAD，直接提交会丢。
```

```bash
git add README.md && git commit -m "docs: explain shell repo usage" && git push origin HEAD:release
```

---

## Task 5: 服务端仓整理

**Files:**
- Create: `GOSpeak-Server/package.json`、`pnpm-workspace.yaml`、`.gitignore`
- Modify: `scripts/`（只留服务端脚本）、`scripts/check-version.mjs`

**Interfaces:**
- Consumes: Task 1 抽取的服务端仓（已含 `scripts/`、`.github/`）。
- Produces: 可在服务端仓内跑 `pnpm test:server`、`go build`。

- [ ] **Step 1: 建最小 JS 脚手架**

没有它，`check-version.mjs` 会 ENOENT（它读 `package.json` 三方互比），`pnpm test:server` 也不存在（实测报 `Command "test:server" not found`）。

`GOSpeak-Server/package.json`：
```json
{
  "name": "@gospeak/server-root",
  "private": true,
  "version": "1.3.0-rc.1",
  "packageManager": "pnpm@10.11.0",
  "scripts": {
    "test:server": "pnpm --dir test test",
    "gen:protocol": "cd server && go run ./cmd/gen-protocol -out /tmp/gen",
    "version:check": "node scripts/check-version.mjs . \"\" server"
  }
}
```

`pnpm-workspace.yaml`：
```yaml
packages:
  - "test"
onlyBuiltDependencies:
  - esbuild
```

```bash
cd /Users/noelorin/Projects/GOSpeak/server
pnpm install
ls test/package.json
```
Expected: `pnpm install` 成功；`test/package.json` 存在（含 `"test": "vitest run"`）。

- [ ] **Step 2: 建 `.gitignore`（以原 155 行为基线平移）**

```gitignore
node_modules/
.superpowers/
.turbo/
*.log
.DS_Store
*.local

/server/gospeak
/server/uploads/
/server/logs/
/server/.env*
/server/db/
*.db
*.db-shm
*.db-wal
/test/.runtime/
/deploy/.env
/deploy/env/*.local.env

/server/internal/webui/dist/*
!/server/internal/webui/dist/EMBED_PLACEHOLDER
```

```bash
git add .gitignore package.json pnpm-workspace.yaml
git commit -m "chore: add server repo scaffolding"
```
Expected: `git check-ignore -q deploy/.env && echo "ignored OK"` 输出 `ignored OK`。

**为什么**：公开仓没有 `.gitignore` 会直接暴露真实 `.env`、本地 DB 与上传目录；原规则里的 `app/server/.env*` 在扁平化后全部失效。

- [ ] **Step 3: 按归属裁剪 scripts**

保留：`check-version.mjs`（收窄）、`release-naming.mjs`、`release-notes.mjs`、`build-changelog.mjs`、`dev-livekit.sh` 及各自 `.test.mjs`。
删除：`check-desktop-commands.mjs`、`check-hook-deps.mjs`、`check-mobile-bridge.mjs`、`check-radius-tokens.mjs`、`check-ui-tokens.mjs`、`mobile-bind-proxyrules.mjs`、`mobile-version.mjs`（进客户端仓，见 Task 6）。

```bash
node --test scripts/*.test.mjs
```
Expected: 全通过。

- [ ] `Step 4: 收窄 check-version.mjs``

```bash
node --test scripts/check-version.test.mjs
node scripts/check-version.mjs . "" server
```
Expected: 通过，版本一致。

- [ ] **Step 5: 提交**

```bash
git add -A && git commit -m "refactor: scope server scripts and version check" && git push
```

---

## Task 6: 客户端仓整理

**Files:**
- Create: `pnpm-workspace.yaml`、`package.json`、`.gitignore`、`biome.json`、`lefthook.yml`、`turbo.json`、`commitlint.config.js`
- Modify: `web/tailwind.config.ts`、`web/scripts/capture-storybook-baseline.mjs`、`mobile/package.json`

**Interfaces:**
- Produces: `pnpm -r build` 能覆盖 web/packages/desktop/mobile 四条链。

- [ ] **Step 1: 写 workspace（必须显式含 desktop 与 mobile）**

```yaml
packages:
  - "web"
  - "packages/*"
  - "desktop"
  - "mobile"
onlyBuiltDependencies:
  - esbuild
  - lefthook
supportedArchitectures:
  os: [current, linux, darwin, win32]
  cpu: [current, x64, arm64]
  libc: [current, glibc, musl]
```

```bash
cd /Users/noelorin/Projects/GOSpeak/client
pnpm install
pnpm --filter @gospeak/desktop-app exec mygo --version
pnpm --filter @gospeak/mobile exec cap --version
```
Expected: 两条 filter 都命中（不是 `No projects matched`）。

**为什么必须显式列**：实测漏登记时 `pnpm --filter <pkg>` 打印 `No projects matched` 并 **exit 0**，`pnpm -r build` 静默跳过这两条链却报绿。

- [ ] **Step 2: 写 `.gitignore`（原 155 行基线平移）**

```gitignore
node_modules/
.superpowers/
.turbo/
*.log
.DS_Store
*.local
test-results/
artifacts/

**/dist/
**/dist-desktop/
**/dist-mobile/
.tanstack/
*.tsbuildinfo
web/src/routeTree.gen.ts
web/visual-baseline/*.diff.png

desktop/mygo.ts
desktop/gospeak-desktop
desktop/webui/dist/*
!desktop/webui/dist/.gitkeep
mobile/www/
mobile/build/

go.work
go.work.sum
```

- [ ] **Step 3: 修两处跨层相对路径**

```bash
sed -i '' 's|"../../packages/components/src|"../packages/components/src|' web/tailwind.config.ts
sed -i '' 's|"../../.."|"../.."|' web/scripts/capture-storybook-baseline.mjs
grep -n 'packages/components' web/tailwind.config.ts
grep -n 'resolve(' web/scripts/capture-storybook-baseline.mjs | head -2
```
Expected: 两处各减一级。

- [ ] **Step 4: 修 mobile 的 `../../scripts` 断链**

`client/mobile` → `../../` = **壳根**，那里没有 `scripts/`。

```bash
sed -i '' 's|node \.\./\.\./scripts/|node ../scripts/|g' mobile/package.json
grep -n 'scripts/' mobile/package.json
```
Expected: 四条 `bind:rules*` / `version:sync` 变成 `../scripts/`。

- [ ] **Step 5: 迁移七个客户端脚本并改常量**

从 `pre-shell-split` tag 取这七个 + 各自 `.test.mjs`：
`check-desktop-commands.mjs`、`check-hook-deps.mjs`、`check-mobile-bridge.mjs`、`check-radius-tokens.mjs`、`check-ui-tokens.mjs`、`mobile-bind-proxyrules.mjs`、`mobile-version.mjs`。

```bash
git -C /Users/noelorin/Projects/GOSpeak archive pre-shell-split scripts | tar -x -C /tmp/scripts-src
ls /tmp/scripts-src/scripts | head
```

常量改法：

| 脚本 | 改动 |
|---|---|
| `check-desktop-commands.mjs` | `DESKTOP_DIR="desktop"`、`TS_DIR="packages/desktop/src/capabilities"`、`TS_EVENTS_FILE="packages/desktop/src/events.ts"` |
| `check-ui-tokens.mjs` / `check-radius-tokens.mjs` | `SCAN_DIRS=["web/src","packages/components/src"]` |
| `check-hook-deps.mjs` | `./web`、`./packages` |
| `check-mobile-bridge.mjs` | `IOS_DIR="mobile/ios/App/App"`、`IOS_PROJECT="mobile/ios/App/App.xcodeproj/project.pbxproj"`、`ANDROID_DIR="mobile/android/app/src/main/java/dev/gospeak/mobile"` |
| `mobile-bind-proxyrules.mjs` | `MODULE_DIR="proxyrules"`、`OUTPUT_DIR="mobile/build/proxyrules"`；`root` 不用改；三处提示串 `app/shared/proxyrules`→`proxyrules`、`app/mobile`→`mobile` |
| `mobile-version.mjs` | 五处路径：`mobile/package.json`、`mobile/android/app/build.gradle`、`mobile/ios/.../project.pbxproj`、`web/package.json`、`.github/versions/mobile.yaml` |

```bash
node --test scripts/*.test.mjs
```
Expected: 全通过。

- [ ] **Step 6: 重建根配置**

`biome.json`：`files.includes` → `web/src/**`、`packages/*/**`、`desktop/*.{ts,json,jsonc}`、`mobile/*.{ts,json,jsonc}`、`scripts/**`，排除 `web/src/routeTree.gen.ts`、`desktop/mygo.ts`。
`turbo.json`：`globalDependencies` 去掉 `app/*` 前缀。
`lefthook.yml`：7 道 pre-push 门禁的 `cd app/...` 全改新路径；`protocol gen-diff` 改成 `(cd web && git status --porcelain -- src/protocol/generated | grep . && exit 1 || true)`。
`commitlint.config.js`：原样复制。
`package.json`：复制原根 devDependencies、`pnpm.overrides`、`peerDependencyRules`。

```bash
pnpm exec biome ci . 2>&1 | tail -3
```
Expected: 真实处理到文件（不是零匹配 exit 1）。

- [ ] **Step 7: 处理 Capacitor 生成物的 node_modules 层级**

`mobile/android/capacitor.settings.gradle` 与 `mobile/ios/App/CapApp-SPM/Package.swift` 写死 `../../../node_modules`（旧布局上溯 3 级 = 仓根；新布局 = **壳根**）。

```bash
pnpm --filter @gospeak/mobile sync:no-bind 2>/dev/null || true
grep -n 'node_modules' mobile/android/capacitor.settings.gradle | head -2
```
Expected: 层级按新位置重写（`cap sync` 不可用则手工改正层数并提交）。

- [ ] **Step 8: 补桌面端 `webui/dist` 占位**

`desktop/proxyserver.go:51` 有 `//go:embed all:webui/dist`，而该目录**从未被跟踪**（实测 0 个文件）。缺目录时 `go build` 报 `pattern all:webui/dist: no matching files found`。

```bash
mkdir -p desktop/webui/dist && touch desktop/webui/dist/.gitkeep
git add -f desktop/webui/dist/.gitkeep
cd desktop && go build ./...
```
Expected: 编译通过。

- [ ] **Step 9: 全量验证并提交**

```bash
cd /Users/noelorin/Projects/GOSpeak/client
pnpm install && pnpm -r build
(cd web && pnpm test)
(cd desktop && go build ./... && go test ./...)
git add -A && git commit -m "refactor: configure client workspace after split" && git push
```
Expected: 四链全过。

---

## Task 7: 发布编排迁移

**Files:**
- 服务端：`release.yml`（去 desktop 分支）、`version-bump.yml`（收窄）、`docker.yml`、`versions/server.yaml`、`actions/*`、`security.yml`、`pr-title.yml`、`dependabot.yml`
- 客户端：新增 `release.yml`（desktop + web 产物）、`build-mobile.yml`、`versions/{desktop,mobile,web}.yaml`、`actions/pnpm-install`、`security.yml`、`pr-title.yml`、`dependabot.yml`
- 文档：Task 3 已建 `docs.yml`

**Interfaces:**
- Consumes: Task 1/2 抽取时保留的 `.github/`。
- Produces: 四仓各有完整发布链。

**为什么单列**：Task 1/2 的 tree-filter 保留了 `.github/`，所以两个子仓现在各有一份**完整**的旧 workflow；本 Task 按归属裁剪并补缺失部分。

- [ ] **Step 1: 服务端仓按归属裁剪**

保留并改路径：
- `release.yml`：删除 `build-desktop-*`、`desktop-checksums`、`desktop_included` 相关全部 job/步骤；`build-release-binaries` 的前端来源改为从客户端 Release 下载（见 Task 9 Step 4）
- `version-bump.yml`：只处理 `server`；`uses: ./.github/workflows/release.yml` 保留（同仓）；删除 mobile/desktop 分支
- `docker.yml`：保留；`images:` 改为 `ghcr.io/gospeak-org/gospeak-server`；verify/publish 两步补 `CLIENT_WEB_URL` build-arg
- `security.yml`、`pr-title.yml`：原样
- `versions/server.yaml`：保留
- `.github/actions/{pnpm-install,go-build-cache}`：保留
- `dependabot.yml`：`gomod /server` + `github-actions /`

```bash
cd /Users/noelorin/Projects/GOSpeak/server
grep -n 'app/server\|app/desktop' .github/workflows/*.yml | head -10
```
Expected: 逐条改成 `server/` 或删除。

- [ ] **Step 2: 客户端仓裁剪并补 `web` 产物链**

保留 `build-mobile.yml`；新增 `release.yml`：
- 触发：`push.tags: ['gospeak-desktop-v*','gospeak-mobile-v*','gospeak-web-v*']`
- `gospeak-web-v*` 分支：**独立 job**，`GOSPEAK_TARGET=web pnpm --filter @gospeak/web build` → `tar -czf gospeak-web-<ver>-dist.tar.gz -C web dist` → 上传到客户端 Release
- `gospeak-desktop-v*`：构建安装包
- `gospeak-mobile-v*`：调用 `build-mobile.yml`

**关键**：web 产物 job **不得**与 desktop 构建或 `pnpm -r build` 同 job——`desktop/mygo.config.ts` 会跑 `GOSPEAK_TARGET=desktop` 覆盖 `web/dist`。

同时把 `web/vite.config.ts` 的 desktop 目标改成独立 `dist-desktop/`：

```ts
outDir: gospeakTarget === "desktop" ? "dist-desktop"
      : gospeakTarget === "mobile"  ? "dist-mobile"
      : "dist"
```

```bash
(cd web && GOSPEAK_TARGET=web pnpm build && GOSPEAK_TARGET=desktop pnpm build && ls -d dist dist-desktop)
```
Expected: 两个目录并存，互不覆盖。

- [ ] **Step 3: 客户端 `release-naming` 加 web 组件**

`scripts/release-naming.mjs`（从原仓复制）的 `COMPONENTS = ["server","desktop","mobile"]` → 加 `"web"`，并按需补测试。

```bash
node -e "const m=require('./scripts/release-naming.mjs');" 2>/dev/null || true
node scripts/release-naming.mjs tag web 1.4.0-rc.1
```
Expected: 输出 `gospeak-web-v1.4.0-rc.1`。

- [ ] **Step 4: 补 `versions/` 与 dependabot**

客户端 `.github/versions/desktop.yaml`（`1.4.0-rc.1`）、`mobile.yaml`（`1.4.0-rc.1`）、`web.yaml`（`1.4.0-rc.1`）。
`dependabot.yml`：`npm /` + `gomod /desktop` + `github-actions /`。

- [ ] **Step 5: 提交（两仓）**

```bash
cd /Users/noelorin/Projects/GOSpeak/server && git add .github && git commit -m "ci: scope server release and version workflows" && git push
cd ../client && git add .github scripts package.json 2>/dev/null; git commit -m "ci: add client release pipeline with web artifact" && git push
```

---

## Task 8: 协议同步门禁

**Files:**
- Create: `GOSpeak-Client/scripts/check-protocol-sync.mjs`
- Modify: `GOSpeak-Client/.github/workflows/ci.yml`（新增 job）
- Modify: `GOSpeak-Client/lefthook.yml`（pre-push）

**Interfaces:**
- Consumes: 服务端 `release` 分支的 golden（`server/internal/genprotocol/testdata/golden/`）。
- Produces: 协议不同步会红。

- [ ] **Step 1: 写同步脚本（raw 直链 + `--fix`）**

**不要用 `gh api .../contents`**：客户端 CI 的 `GITHUB_TOKEN` 是只覆盖本仓的 installation token，跨仓会 404（本地 gh 登录了所以本地能跑，CI 跑不了）。用 raw 直链（公开仓免 token，实测 200）。

`check-protocol-sync.mjs` 要点：
- 默认 ref：读 `server-compat.json`（`{"serverRef":"gospeak-server-v1.3.0-rc.1"}`）；缺省时用 `release`
- URL：`https://raw.githubusercontent.com/GOSpeak-org/GOSpeak-Server/<ref>/server/internal/genprotocol/testdata/golden/<file>`
- 三份：`events.ts`、`payloads.ts`、`envelope.ts`
- 逐字节比对 `web/src/protocol/generated/<file>`
- `--fix`：直接用远端内容覆盖本地（实测 golden 与 generated **字节完全相同**，覆盖即修复，客户端不需要 Go 工具链）

```bash
cd /Users/noelorin/Projects/GOSpeak/client
node scripts/check-protocol-sync.mjs
node scripts/check-protocol-sync.mjs --fix
git diff --exit-code -- web/src/protocol/generated
```
Expected: 一致时退出 0；人为改一个字节后退出 1 并提示服务端版本；`--fix` 后恢复。

- [ ] **Step 2: 接进 CI 并设为必过**

`ci.yml` 加 `protocol-sync` job，**无条件运行**（不挂在 paths-filter 下，否则会变 skipped 卡死 PR），并入 `gate` 的 `needs` 且 `gate` 对它要求非 skipped。

```bash
actionlint .github/workflows/ci.yml
```
Expected: 无错误。

- [ ] **Step 3: 接进 lefthook pre-push**

`lefthook.yml` 加：

```yaml
protocol-sync:
  run: node scripts/check-protocol-sync.mjs
```

- [ ] **Step 4: 提交**

```bash
git add scripts .github lefthook.yml server-compat.json && git commit -m "ci: enforce protocol sync across repos" && git push
```

---

## Task 9: 服务端构建链修正

**Files:**
- Modify: `server/internal/buildinfo/buildinfo.go` + `_test.go`
- Modify: `Dockerfile`
- Modify: `server/internal/webui/`（断言）

**Interfaces:**
- Consumes: 客户端 Release 的 `gospeak-web-v*-dist.tar.gz`。
- Produces: 二进制能找到 `VERSION`；镜像内前端真实可用。

- [ ] **Step 1: 修 `buildinfo` 的 `VERSION` 候选路径**

`buildinfo.go:64-86` 的候选只有 cwd `VERSION`、`../../VERSION`、exe 附近几层。现在 module 根是 `app/server`，`../../VERSION` 正好命中仓库根；**移到 `server/` 后 VERSION 在 `../VERSION`，不在候选里**。

后果：`prod`/`build` 命令下版本退化成 `dev`，而 `system_update_service.go:198-203` 对不可解析版本返回 0 → `Available` 恒 false → **更新提示永久静默**。

```go
// 在候选列表中补一个：
filepath.Join("..", name),
```

并把 `buildinfo_test.go:66-84`（现模拟 `root/app/server`）改成同时覆盖 `root/server`。

```bash
cd /Users/noelorin/Projects/GOSpeak/server/server
go test ./internal/buildinfo/ -count=1
go run main.go version
```
Expected: 测试通过；`version` 输出 `1.3.0-rc.1`（**不是 `dev`**）。

- [ ] **Step 2: 确认 `go vet` 与集成测试入口**

```bash
cd /Users/noelorin/Projects/GOSpeak/server
(cd server && go vet ./...)
pnpm install && pnpm test:server
```
Expected: 都通过（`test/globalSetup.ts` 已改为 `join(repoRoot, "server")`）。

- [ ] **Step 3: Dockerfile 前端改到 go-builder 阶段**

**原设计的错误**：把 `ADD`/`tar` 放在最终 alpine 阶段。前端是**编译期**嵌入（`internal/webui/embed.go:13-14` 的 `//go:embed all:dist`），必须在 `go build` **之前**填进 `internal/webui/dist`，否则二进制里仍是 `EMBED_PLACEHOLDER`，而 `/app/static/index.html` 的存在会让旧断言错误通过。

另：`Dockerfile:30` 的 `COPY app/server/ ./` 也要改成 `COPY server/ ./`，`Dockerfile:32` 的 `COPY --from=web-builder` 整段删除。

改后的 go-builder 阶段：

```dockerfile
FROM golang:1.27-alpine AS go-builder
WORKDIR /src
ARG CLIENT_WEB_URL
ARG CLIENT_WEB_SHA256
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
ADD $CLIENT_WEB_URL /tmp/web-dist.tar.gz
RUN echo "$CLIENT_WEB_SHA256  /tmp/web-dist.tar.gz" | sha256sum -c - \
 && mkdir -p ./internal/webui/dist \
 && tar -xzf /tmp/web-dist.tar.gz -C ./internal/webui/dist \
 && test -f ./internal/webui/dist/index.html
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /gospeak main.go
```

```bash
cd /Users/noelorin/Projects/GOSpeak/server
docker build \
  --build-arg CLIENT_WEB_URL=<客户端 release 的 dist.tar.gz 直链> \
  --build-arg CLIENT_WEB_SHA256=<对应 sha256> \
  -t gospeak-test .
```
Expected: 构建成功。

- [ ] **Step 4: 断言改成「前端真的能用」**

原来的 `test -f /app/static/index.html` 会误通过。改成两项：

```bash
# 1) 单测层：embed 资产真实存在
cd server && go test ./internal/webui/ -run TestHasAssets -count=1

# 2) 运行层：起容器看 HTML
docker run -d --name gs -p 18998:8998 gospeak-test
sleep 5
curl -fsS localhost:18998/ | grep -q '<div id="root"' && echo "frontend OK"
docker rm -f gs
```
Expected: 两项都通过。

- [ ] **Step 5: `release.yml` 的二进制构建也改直链**

`build-release-binaries` 原来依赖同 run 的 `release-web-dist-server` workflow artifact（`retention-days: 3`）。改为从客户端 Release 下载：

```yaml
- name: Download client web dist
  run: |
    gh release download "$CLIENT_WEB_TAG" --repo GOSpeak-org/GOSpeak-Client \
      --pattern 'gospeak-web-*-dist.tar.gz' --dir /tmp/web
    mkdir -p server/internal/webui/dist
    tar -xzf /tmp/web/*.tar.gz -C server/internal/webui/dist
    test -f server/internal/webui/dist/index.html
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 6: 提交**

```bash
git add Dockerfile .github/workflows/release.yml server/internal/buildinfo
git commit -m "fix: resolve VERSION after flattening and embed web dist at build time" && git push
```

---

## Task 10: 壳仓 CI、drift 检测与桥接 Release

**Files:**
- Create: `GOSpeak/scripts/check-shell-drift.mjs`、`GOSpeak/.github/workflows/shell-drift.yml`

**Interfaces:**
- Produces: 壳的 gitlink 可达性检测；已装机客户端的更新桥接。

- [ ] **Step 1: 写 drift 检测脚本**

```js
#!/usr/bin/env node
// 只报告，不自动改。
import { execSync } from "node:child_process";
const SUBS = { server: "GOSpeak-Server", client: "GOSpeak-Client", docs: "GOSpeak-Docs" };
let drift = 0;
for (const [path, repo] of Object.entries(SUBS)) {
  const pinned = execSync(`git ls-tree HEAD ${path}`).toString().split(/\s+/)[2];
  const remote = execSync(`git ls-remote https://github.com/GOSpeak-org/${repo}.git refs/heads/release`)
    .toString().split(/\s+/)[0];
  if (pinned !== remote) { console.log(`::warning::${path} 落后于 ${repo} release`); drift++; }
}
process.exit(drift ? 1 : 0);
```

```bash
node scripts/check-shell-drift.mjs
```
Expected: 三个 gitlink 与远端 `release` 一致时退出 0。

- [ ] **Step 2: 写壳 workflow**

`.github/workflows/shell-drift.yml`：`on: [push, pull_request]` + `schedule: cron '0 2 * * *'`；`submodules: recursive`；跑 `node scripts/check-shell-drift.mjs`。public 仓零成本。

- [ ] **Step 3: 桥接 Release（保住已装机客户端的更新通道）**

**这是最容易被漏掉的一步**：`client/desktop/updater_release.go:33` 的 `githubAPIReleases` 指向 `repos/NoelOrin/GOSpeak/releases`，**已编译进 1.3.0-rc.1 的装机二进制**，无法远程修改。转移后该地址 301 到壳仓，而壳仓若不再发布桌面版，老客户端就**永远看不到新版**。

在壳仓发布一次桥接：

```bash
gh release create gospeak-desktop-v1.4.0-rc.1 \
  --repo GOSpeak-org/GOSpeak \
  --title "GOSpeak Desktop v1.4.0-rc.1 (moved)" \
  --notes "桌面端已迁移到 GOSpeak-Client。此条目仅用于让旧版客户端能发现新版本。" \
  --latest=false
```
Expected: `gh release view gospeak-desktop-v1.4.0-rc.1 --repo GOSpeak-org/GOSpeak` 成功。

**约束**：1.4.0 稳定版发布前，壳仓的 desktop Release 不能删；新资产的命名必须保持 `gospeak-desktop-v<ver>-<os>-<arch>.<ext>`（旧客户端 `pickDesktopAsset` 按此前缀匹配）。

- [ ] **Step 4: 提交**

```bash
cd /Users/noelorin/Projects/GOSpeak
git add scripts .github && git commit -m "ci: detect submodule drift and bridge legacy desktop updates" && git push origin HEAD:release
```

---

## Task 11: 仓名与镜像统一替换

**Files:**
- Modify: `server/server/internal/service/system_update_service.go`
- Modify: `client/desktop/updater_release.go`、`client/desktop/updater.go`、`client/web/src/components/modal/settingModal/tab_item/about.tsx`
- Modify: `client/docs/.vitepress`（在 Docs 仓）、`deploy/*`、`README.md`

**Interfaces:**
- Produces: 无 `noelorin` 残留引用。

- [ ] **Step 1: 用正确的正则找出全部旧引用**

**原计划的 `grep 'GOSpeak-org/GOSpeak[^-]'` 是错的**——那是转移**后**才存在的名字，当前树里零命中。

```bash
cd /Users/noelorin/Projects/GOSpeak
grep -rniE 'noelorin|gospeak-org' --include='*.md' --include='*.go' --include='*.ts' --include='*.tsx' --include='*.mjs' --include='*.yml' --include='*.yaml' --include='*.json' --include='*.sh' server client docs deploy 2>/dev/null | grep -v node_modules | head -30
```
Expected: 列出全部命中。已知必改：

| 文件 | 内容 |
|---|---|
| `client/desktop/updater_release.go:33` | `repos/NoelOrin/GOSpeak/releases` → `GOSpeak-Client` |
| `client/desktop/updater.go:39` | `releasePageURL` 指向 `GOSpeak-Client`（**原计划漏了这个文件**） |
| `client/web/src/.../about.tsx` | 「项目地址」 |
| `docs/.vitepress/theme/components/ClientDownload.vue:5` | `const REPO`（**原计划漏了**） |
| `docs/.vitepress/config.ts:112` | socialLinks |
| `server/server/internal/service/system_update_service.go` | server 仓地址与 tag 前缀 |
| `deploy/docker-compose*.yml` | `ghcr.io/noelorin/gospeak` → `ghcr.io/gospeak-org/gospeak-server` |
| `README.md`、`deploy/DEPLOY.md`、文档 | clone 与镜像地址 |

- [ ] **Step 2: 更新检测指向各自仓**

服务端 → `repos/GOSpeak-org/GOSpeak-Server`，tag 前缀 `gospeak-server-v`。
客户端 → `repos/GOSpeak-org/GOSpeak-Client`，tag 前缀保持 `gospeak-desktop-v` / `gospeak-mobile-v`（旧客户端匹配逻辑不变）。

```bash
cd server/server && go test ./internal/service/ -run TestSystemUpdate -count=1
```
Expected: 通过。

- [ ] **Step 3: 改一键安装脚本路径**

`deploy/install-binary.sh` 的 `GOSPEAK_REPO` 改为 `GOSpeak-org/GOSpeak-Server`；文档里的 raw URL 同步（壳仓已无 `deploy/`，旧 URL 会 404）。

- [ ] **Step 4: 验收**

```bash
grep -rni 'noelorin' server client docs deploy 2>/dev/null | grep -v node_modules | grep -v '.git/'
```
Expected: **无输出**。

- [ ] **Step 5: 提交（各仓）**

```bash
cd server && git add -A && git commit -m "chore: point update checks at GOSpeak-Server" && git push
cd ../client && git add -A && git commit -m "chore: point updates and links at GOSpeak-Client" && git push
cd ../docs && git add -A && git commit -m "docs: update repo references" && git push
```

---

## Task 12: 版本基线对齐

**Files:**
- Modify: `client/desktop/package.json`、`client/mobile/package.json`、`client/web/package.json`、`client/desktop/mygo.config.ts`
- Modify: `client/mobile/android/app/build.gradle`、`client/mobile/ios/.../project.pbxproj`
- Modify: `server/VERSION`（保持）

**Interfaces:**
- Produces: 客户端首个版本 ≥ 现网，更新检测不失联。

- [ ] **Step 1: 定客户端首个版本**

现网：desktop `1.3.0-rc.1`、mobile `1.1.0-rc.2`、server `1.3.0-rc.1`。客户端统一 **`1.4.0-rc.1`**。

**必须改全 5 处**（原计划只改 3 个 package.json，会留下双源漂移）：

| 位置 | 说明 |
|---|---|
| `desktop/package.json` | |
| `desktop/mygo.config.ts:21` | 打包出的 bundle 版本；不改会让 App **一直把自己当更新** |
| `mobile/package.json` | |
| `mobile/android/app/build.gradle:10-11` | `versionCode` / `versionName` |
| `mobile/ios/App/App.xcodeproj/project.pbxproj` | |
| `web/package.json` | `__APP_VERSION__` 来源 |

```bash
cd /Users/noelorin/Projects/GOSpeak/client
node scripts/mobile-version.mjs          # 是写入器，不是校验器
node scripts/check-version.mjs . "" desktop
node scripts/check-version.mjs . "" mobile
git diff --stat
```
Expected: `check-version` 通过；diff 显示 5 处版本变更。

**注意**：`mobile-version.mjs` **没有 `--check` 参数**（它会把它当 rootDir），且它是**写入器**；校验请用 `check-version.mjs`。

- [ ] **Step 2: 服务端版本保持**

`server/VERSION` 维持 `1.3.0-rc.1`，下次发版再 bump。

- [ ] **Step 3: 提交**

```bash
git add -A && git commit -m "chore: set client baseline version 1.4.0-rc.1" && git push
```

---

## Task 13: bump 壳指针、全量验证与回滚

**Files:** 无（验证任务）

- [ ] **Step 1: bump 壳指针（关键）**

`git submodule add` 把 gitlink 钉在 Task 1/2/3 的提交，Task 5–12 之后子仓已前进很多。不 bump 的话，clone 出来的仍是旧代码。

```bash
cd /Users/noelorin/Projects/GOSpeak
for d in server client docs; do
  git -C $d fetch origin release
  git -C $d checkout release && git -C $d reset --hard origin/release
done
git add server client docs
git commit -m "chore: bump submodules to post-split state"
git push origin HEAD:release
```
Expected: `git submodule status` 三行**无 `+`/`-` 前缀**。

- [ ] **Step 2: 全新 clone 复现**

```bash
rm -rf /tmp/verify-gospeak
git clone --recursive https://github.com/GOSpeak-org/GOSpeak.git /tmp/verify-gospeak
cd /tmp/verify-gospeak
ls server client docs
git submodule status
git -C server branch --show-current   # 应为 release，不是游离 HEAD
```
Expected: 三个目录都有内容；三行 gitlink；子仓在 `release` 上。

- [ ] **Step 3: 三仓门禁**

```bash
cd /tmp/verify-gospeak
(cd server && (cd server && go build ./...) && pnpm install && pnpm test:server)
(cd client && pnpm install && pnpm -r build && (cd desktop && go build ./...) && (cd web && pnpm test))
(cd docs && pnpm install && pnpm build)
```
Expected: 三条链全过。

- [ ] **Step 4: 相对路径回归**

```bash
cd /tmp/verify-gospeak/client
grep -n 'cp -R ../web/dist' desktop/mygo.config.ts
grep -n 'webDir' mobile/capacitor.config.ts
grep -n 'replace.*proxyrules' desktop/go.mod
test -d proxyrules && echo "proxyrules 在仓根 OK"
```
Expected: `../web/dist`、`"../web/dist-mobile"`、`=> ../proxyrules` 三处正确。

- [ ] **Step 5: 已装机客户端更新通道回归**

```bash
gh release list --repo GOSpeak-org/GOSpeak | grep desktop
gh api repos/GOSpeak-org/GOSpeak/releases --jq '.[0].tag_name'
```
Expected: 壳仓存在 `gospeak-desktop-v1.4.0-rc.1` 桥接 Release。**这是老客户端能看到新版的唯一通道**。

- [ ] **Step 6: 回滚预案**

按顺序执行，**不能跳步**：

```bash
cd /Users/noelorin/Projects/GOSpeak

# 1) 先 deinit 子模块（.gitmodules 还在时必须先做）
git submodule deinit -f --all
rm -rf .git/modules/server .git/modules/client .git/modules/docs
git config --remove-section submodule.server 2>/dev/null || true
git config --remove-section submodule.client 2>/dev/null || true
git config --remove-section submodule.docs   2>/dev/null || true

# 2) 删除子模块工作区
rm -rf server client docs

# 3) 回到拆分前的提交
git reset --hard pre-shell-split

# 4) 强制推送（回滚窗口内冻结他人推送）
git push --force-with-lease origin release

# 5) 恢复本地被忽略数据
rsync -a ~/gospeak-local-backup/ ./

# 6) 验证
test ! -e .gitmodules && echo "no .gitmodules OK"
git config --get-regexp '^submodule.' || echo "no submodule config OK"
git status --short
```
Expected: 三条断言全过；`git status` 干净。

**兜底**：`~/gospeak-pre-shell-split.bundle`（已 `bundle verify`）。

- [ ] **Step 7: 更新计划索引**

在 `GOSpeak-Server` 的 `docs/superpowers/plans/README.md`（拆完后该文件归服务端仓）把本计划标为已交付。

```bash
cd /Users/noelorin/Projects/GOSpeak/server
git add docs/superpowers/plans/README.md && git commit -m "docs: mark three-repo split as delivered" && git push
```
