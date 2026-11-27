# 仓迁移与建仓 Implementation Plan

> 【状态】2026-11-27 核验：该计划已完成（组织迁移与三个空子仓已建），仅供历史参考。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `NoelOrin/GOSpeak` 并入组织 `GOSpeak-org`，并在该组织下建好三个空子仓，为后续的三仓拆分准备干净的前置状态。

**Architecture:** 纯运维动作，**不改任何代码**。先把当前 feature 分支合并进 `release`（否则后续抽取会丢掉 24 个提交），再做仓库转移，最后建三个 public 空仓并锚定默认分支。全程可在任意一步回滚。

**Tech Stack:** Git 2.50 · GitHub CLI（gh 2.96）· rsync

**Spec:** `docs/superpowers/specs/2026-11-26-two-repo-split-design.md`
**后续计划:** `docs/superpowers/plans/2026-11-26-three-repo-split.md`（本计划完成后才可执行）

## Global Constraints

- 工作目录 `/Users/noelorin/Projects/GOSpeak`；分支 `feature/multiplatform-repo-restructure`。
- 目标组织 `GOSpeak-org`（free 套餐）；当前账号须为 `admin active`。
- **本计划不改任何源码**，只做备份、合并、转移、建仓。
- 四个仓全部 public。
- **回滚靠 `pre-shell-split` tag 与 bundle**，两者都在 `~`（不在 `/tmp`，macOS 会清理）。

### 实测基线（2026-11-26）

```
release 提交数            87
release..HEAD（feature）  24
现有 tag                  22（server 11 + desktop 11）
现有 release              9
现有 mode 160000          0（无 .gitmodules）
git status --ignored      76 条（含 app/server/db/app.db 425984 字节）
Pages                     build_type=workflow, source=release, https://noelorin.github.io/GOSpeak/
actions/secrets           0
```

## Review Focus

- **未跟踪数据被删且 bundle 救不回**：`git bundle --all` 只含 refs。Task 1 Step 1 用 rsync 备份 `app/server/db`、`uploads`、`.env` 到 `~`；Task 1 Step 1 钉住。
- **合并 feature 被跳过 → 后续抽取丢 24 个提交**：Task 2 Step 3 强制 `--ff-only` 并断言提交数从 87 增至约 111。
- **Pages 不随仓转移**：GitHub 官方明确不重定向 Pages，旧站点会 404。Task 3 Step 3 只记录现状，重建放后续计划的 Task 7。
- **旧仓地址永不复用**：`NoelOrin/GOSpeak` 被 GitHub retire 后不可重建；一旦重建，已装机 App 里烧死的 API 地址与所有老安装包链接一起 404。Task 3 Step 4 写进设计约束。
- **空子仓默认分支不确定**：`git clone` 可能拿到空工作区。Task 4 Step 2 在首次 push 后断言默认分支（首次 push 属后续计划，本计划只留检查清单）。

---

## Task 1: 备份与基线

**Files:** 无（准备任务）

**Interfaces:**
- Produces: `~/gospeak-local-backup/`、`~/gospeak-pre-shell-split.bundle`、`~/gospeak-baseline.txt`、`pre-shell-split` tag。全部供本计划回滚与后续计划对照。

- [ ] **Step 1: 备份被忽略的本地数据**

bundle 只含 refs，不含未跟踪/被忽略文件；而后续拆分要 `rm -rf`，会连带删掉这些。

```bash
cd /Users/noelorin/Projects/GOSpeak
mkdir -p ~/gospeak-local-backup
rsync -a --exclude node_modules \
  app/server/db app/server/uploads app/server/.env* deploy/.env* artifacts/ \
  ~/gospeak-local-backup/ 2>/dev/null || true
du -sh ~/gospeak-local-backup
ls ~/gospeak-local-backup/app/server/db 2>/dev/null | head -3
```
Expected: 目录非空；存在 `app.db`（约 425KB）。若 `app/server/db` 不存在，记录并继续。

- [ ] **Step 2: 提交文档并建回滚锚点**

```bash
git add -A
git commit -m "docs: add three-repo split design and plans"
git bundle create ~/gospeak-pre-shell-split.bundle --all
git bundle verify ~/gospeak-pre-shell-split.bundle
git tag -a pre-shell-split -m "anchor before org migration and three-repo split"
git push origin pre-shell-split
```
Expected: `git bundle verify` 输出含 `complete history`；`git tag -l | grep pre-shell-split` 命中；tag 已推送。

- [ ] **Step 3: 记录基线数字**

```bash
{
  echo "release_count=$(git rev-list --count release)"
  echo "feature_ahead=$(git rev-list --count release..HEAD)"
  echo "tags=$(git tag -l | wc -l)"
  echo "head=$(git rev-parse HEAD)"
  echo "branch=$(git branch --show-current)"
} | tee ~/gospeak-baseline.txt
```
Expected: `release_count=87`、`feature_ahead=24`、`tags=22`（若不同，把实际值记下供 Task 2 比对）。

---

## Task 2: 合并 feature 进 release

**Files:** 无（Git 分支操作）

**Interfaces:**
- Consumes: Task 1 的基线。
- Produces: `release` 包含全部 24 个提交，成为后续抽取的唯一源。

**为什么必须做**：三个子仓都从 `release` 抽取。不合并就会丢掉一组件一 Release 的全部改造（`updater_release.go` 88 行、`release.yml` 85 行、`version-bump.yml` 99 行、`build-mobile.yml` 79 行、`system_update_service.go` 15 行、`router.go` 6 行、`ClientDownload.vue` 67 行、`release-notes.mjs` 105 行、`.github/versions/*.yaml`）。

- [ ] **Step 1: 确认工作区干净**

```bash
cd /Users/noelorin/Projects/GOSpeak
git status --short
```
Expected: 输出为空。

- [ ] **Step 2: 确认可快进**

```bash
git fetch origin
git merge-base --is-ancestor origin/release HEAD && echo "FF possible" || echo "FF NOT possible"
git rev-list --count origin/release..HEAD
```
Expected: `FF possible`；领先数 `24`。若不能快进，停止并询问（可能有人在 `release` 推送了新提交）。

- [ ] **Step 3: 合并并推送**

```bash
git switch release
git merge --ff-only feature/multiplatform-repo-restructure
git push origin release
git rev-list --count release
```
Expected: 提交数 ≈ `87 + 24 = 111`；`git log --oneline -3` 顶部含 `docs: add three-repo split design and plans`。

- [ ] **Step 4: 验证关键改造已进 release**

```bash
git log --oneline -5 release
git show release:app/desktop/updater_release.go | grep -c 'GOSpeak'
git show release:.github/versions/desktop.yaml | head -3
```
Expected: `updater_release.go` 含仓库引用（非空）；`versions/desktop.yaml` 可读，版本为 `1.3.0-rc.1`。

---

## Task 3: 迁入 GOSpeak-org

**Files:** 无本地文件（远端转移 + remote 更新）

**Interfaces:**
- Consumes: Task 2 合并后的 `release`。
- Produces: `GOSpeak-org/GOSpeak`；本地 remote 指向组织路径；Pages 现状记录。

- [ ] **Step 1: 确认组织权限**

```bash
gh api orgs/GOSpeak-org --jq '.login'
gh api user/memberships/orgs/GOSpeak-org --jq '.role + " " + .state'
```
Expected: `GOSpeak-org`；`admin active`。否则停止并询问。

- [ ] **Step 2: 记录 Pages 现状**

转移**不会**重定向 Pages（GitHub 官方明文），旧站 `https://noelorin.github.io/GOSpeak/` 会失效。

```bash
gh api repos/NoelOrin/GOSpeak/pages 2>/dev/null | tee ~/gospeak-pages.json | head -8
```
Expected: 文件写入，含 `"html_url":"https://noelorin.github.io/GOSpeak/"`。重建放后续计划的 Task 7。

- [ ] **Step 3: 执行转移**

```bash
gh api -X POST repos/NoelOrin/GOSpeak/transfer -f new_owner=GOSpeak-org
sleep 5
gh repo view GOSpeak-org/GOSpeak --json name,visibility,defaultBranchRef
```
Expected: `Transfer initiated`；随后返回 `{"name":"GOSpeak","visibility":"PUBLIC","defaultBranchRef":{"name":"release"}}`。

- [ ] **Step 4: 更新本地 remote 并验证连带行为**

```bash
cd /Users/noelorin/Projects/GOSpeak
git remote set-url origin https://github.com/GOSpeak-org/GOSpeak.git
git fetch origin --tags
git tag -l | wc -l                                    # 应为 22
git rev-list --all --count                            # 与 Task 2 后一致
gh release list --repo GOSpeak-org/GOSpeak | head -3   # 应可见
gh api repos/GOSpeak-org/GOSpeak/actions/secrets --jq .total_count   # 应为 0
```
Expected: tag 22、提交数一致、Release 可见、secrets 为 0（实测原仓无 secret，无需重建）。

- [ ] **Step 5: 确认旧 URL 重定向并记录「永不重建」约束**

```bash
gh api repos/NoelOrin/GOSpeak --jq .full_name   # 应为 GOSpeak-org/GOSpeak
curl -sI https://github.com/NoelOrin/GOSpeak | head -1   # 应为 301
```
Expected: 返回新名；HTTP 301。

**必须写进设计文档的约束**：`NoelOrin/GOSpeak` 被 GitHub retire 后**不可重建**。一旦重建同名仓，已装机客户端里烧死的 `api.github.com/repos/NoelOrin/GOSpeak/releases` 与所有老安装包链接会一起 404。

- [ ] **Step 6: 提交约束到设计文档**

在设计 §6 补一行：

```markdown
**永不复用 `NoelOrin/GOSpeak` 这个路径**：已装机客户端的更新检测与所有老安装包链接都指向它，靠 GitHub 的仓库级 301 活着。
```

```bash
git add docs/superpowers/specs/2026-11-26-two-repo-split-design.md
git commit -m "docs: record irreversibility of the old repo path"
git push origin release
```

---

## Task 4: 建三个空子仓

**Files:** 无本地改动

**Interfaces:**
- Consumes: Task 3 的组织。
- Produces: `GOSpeak-org/GOSpeak-{Server,Client,Docs}` 三个空 public 仓。

- [ ] **Step 1: 建仓**

```bash
gh repo create GOSpeak-org/GOSpeak-Server --public --confirm
gh repo create GOSpeak-org/GOSpeak-Client --public --confirm
gh repo create GOSpeak-org/GOSpeak-Docs   --public --confirm
```
Expected: 三条均返回新仓 URL。

- [ ] **Step 2: 验证可写与可见性**

```bash
for r in Server Client Docs; do
  printf '%s: ' "$r"
  gh repo view "GOSpeak-org/GOSpeak-$r" --json name,visibility -q '.name + " " + .visibility'
  git ls-remote "https://github.com/GOSpeak-org/GOSpeak-$r.git" >/dev/null && echo "  anonymous-read OK"
done
```
Expected: 三个 `PUBLIC` + `anonymous-read OK`（公开仓无需凭据）。

- [ ] **Step 3: 记录默认分支待办**

空仓无法预设默认分支；首次 push 后必须断言。

```bash
cat > ~/gospeak-default-branch-checklist.txt <<'EOF'
# 每次子仓首次 push 后执行：
# gh api repos/GOSpeak-org/GOSpeak-<X> --jq .default_branch   → 应为 release
# 若不是：gh repo edit GOSpeak-org/GOSpeak-<X> --default-branch release
EOF
cat ~/gospeak-default-branch-checklist.txt
```
Expected: 清单文件写入。理由：实测空仓只推 `release` 时，`git clone` 会 `warning: remote HEAD refers to nonexistent ref` 并给出**空工作区**，而 `clone --recursive` 反而成功——后续计划验证不到。

---

## Task 5: 迁移完成验收与回滚

**Files:** 无（验收 + 预案）

**Interfaces:**
- Consumes: Task 1–4 的全部产物。
- Produces: 迁移完成结论；回滚可执行步骤。

- [ ] **Step 1: 逐项验收**

```bash
cd /Users/noelorin/Projects/GOSpeak
echo "--- remote ---";        git remote -v | head -2
echo "--- branch ---";        git branch --show-current
echo "--- release HEAD ---";  git ls-remote origin refs/heads/release | cut -c1-8
echo "--- local HEAD ---";    git rev-parse --short HEAD
echo "--- tags ---";          git tag -l | wc -l
echo "--- backup ---";        du -sh ~/gospeak-local-backup ~/gospeak-pre-shell-split.bundle
echo "--- sub repos ---"
for r in Server Client Docs; do gh repo view "GOSpeak-org/GOSpeak-$r" --json name -q .name; done
```
Expected: remote 指向 `GOSpeak-org/GOSpeak.git`；分支 `release`；远端与本地 HEAD 一致；tag 22；备份存在；三个子仓可见。

- [ ] **Step 2: 回滚预案（迁移阶段）**

若需撤销迁移：

```bash
# 1) 把仓转回个人账号（需再次具 admin 权限的确认）
gh api -X POST repos/GOSpeak-org/GOSpeak/transfer -f new_owner=NoelOrin

# 2) 本地 remote 改回
git remote set-url origin https://github.com/NoelOrin/GOSpeak.git

# 3) 若 Task 2 的合并也需要撤销（此时尚未拆分，回退一个提交即可）
git switch release
git reset --hard pre-shell-split   # 仅在确认要丢弃合并时

# 4) 删除三个空子仓（不可逆）
for r in Server Client Docs; do gh repo delete "GOSpeak-org/GOSpeak-$r" --yes; done
```
Expected: 仓回到个人账号；`release` 回到 87 提交（若执行了第 3 步）。

**注意**：删除子仓需要 token 带 `delete_repo` scope（`gh auth refresh -h github.com -s delete_repo`）。

- [ ] **Step 3: 更新计划索引**

在 `docs/superpowers/plans/README.md` 增加两行：本计划与后续的三仓拆分计划，标注「迁移已完成 / 拆分待执行」。

```bash
git add docs/superpowers/plans/README.md
git commit -m "docs: index migration and split plans"
git push origin release
```

---

## 交接给拆分计划

本计划完成后，以下状态应成立：

| 检查项 | 期望 |
|---|---|
| `GOSpeak-org/GOSpeak` | 存在，默认分支 `release`，111 提交，22 tag，9 release |
| `release` | 含 24 个 feature 提交（一组件一 Release 全套改造） |
| `GOSpeak-org/GOSpeak-Server` | 空 public 仓 |
| `GOSpeak-org/GOSpeak-Client` | 空 public 仓 |
| `GOSpeak-org/GOSpeak-Docs` | 空 public 仓 |
| `~/gospeak-local-backup/` | 含 `app/server/db/app.db` 等被忽略数据 |
| `~/gospeak-pre-shell-split.bundle` | 已 `bundle verify` |
| `pre-shell-split` tag | 已推送 |
| 本地 Pages 记录 | `~/gospeak-pages.json` |

然后执行 [三仓拆分计划](./2026-11-26-three-repo-split.md)。

---

