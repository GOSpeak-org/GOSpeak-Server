# 项目缺口全景

**更新**: 2026-11-24（基于当前工作区代码核对；分数为定性估计，非测试报告）

## 成熟度评估

| 层 | 完成度 | 说明 |
|----|--------|------|
| 后端核心 | ~90% | 认证/用户/房间/信令/SFU（LiveKit）/文本聊天/邀请码与入域审核完备 |
| 后端测试 | ~60% | `test/` 多套集成套件（auth/oauth/signal/user/role/room/mute/domain/message/storage/sfu/email/system/announcement/conversation/invite/smoke 等，精确清单见 `test/`）+ Go 单测；前端测试已接入 CI（见工程闭环） |
| 前端核心 | ~90% | React 19 全量迁移完成，房间/语音/文字/邀请落地/管理页可用；搜索/回复/线程/Markdown 已实现 |
| 前端测试 | ~55% | Vitest 覆盖 stores/socket/api/components；Playwright 视觉基线与房间语音 E2E；`test-web` / `test-packages` 已进入 CI Gate |
| 桌面端 | ~90% | MyGo 原生宿主（托盘/自启/覆盖层/自定义标题栏/回环代理/ Supervisor 自愈）+ 自动更新（`client/desktop/updater.go`、`client/packages/desktop/src/capabilities/updater.ts`） |
| 文档 | ~85% | 架构/部署/API/SFU/桌面端齐备；模块级 AGENTS 已补齐，规格/评审档案需按代码状态标注时效性 |

---

## 功能缺失

### 文本聊天 — 🟢 MVP 已交付

后端: Message model/repo/service/handler/route 完整实现，信令层 `message:*` 事件齐全，
broadcast-first + JetStream async persist 架构，双槽 Hub (text/voice)。
前端: chatStore + MessageList (virtual scroll) + MessageInput + TextRoomPanel 完整可用，
垂直分栏 layout 支持文字/语音房间同时在线。
已补齐: 消息搜索、线程回复（`textRoomUiStore` 的 `searchMessages` / `threadParent`）、Markdown 渲染（`MarkdownText.tsx`）。
剩余: 附件上传（对象存储 local/S3 已就绪，消息层 attachment 元数据与预览 UI 未实现）。

### bot 模块 — ⚪ 已移除

`client/packages/bot/` 已从仓库移除（未实现），相关计划一并清理。

### 访客体系 — ⚪ 已下线

guest 全链路（service/guard/表/前端入口）已删除；增长与治理改为邀请码注册 + 域级入域审核，见规格 `docs/superpowers/specs/2026-11-10-invite-registration-approval-design.md` 与后端 `invite_service.go` / `invite_join_service.go` / `join_request_service.go`、前端 `api/invite.ts` / `api/inviteJoin.ts` 与 `pages/invite/d/$code.tsx`。注意规格中的 `/link` 邀请码管理页当前树中未找到，核心邀请与审核功能已落地但管理入口须以代码为准。

---

## 当前可拓展方向（摘要）

> 详细清单与优先级见与本项目探索记录配套的说明；这里只落代码可核对的缺口。

| 方向 | 代码依据 | 状态 |
|------|----------|------|
| 聊天附件上传 | `client/packages/desktop` 与 `internal/storage` 已具备上传能力，`Message` DTO 无 attachment 字段 | 未实现 |
| WS 封禁实时策略 | `internal/ws/upgrader.go` 已接 `ConnectPolicy` 与 `PolicyGuard`，与 HTTP `BanCheck` 共用 `policy.Engine` | 已实现（历史 P0 关闭） |
| NATS 状态持久化 | `config.NATStoreDirPath()` + `bus.Init.StoreDir` + compose `gospeak-nats` 卷；JWT 密钥/黑名单跨重启保留 | 已实现 |
| `ListParticipants` 错误语义 | `handler/signal_handler.go` 通过 `pkg.HandleError` 返回非 0 code，测试 `TestListParticipants_SFUError` 覆盖 | 已修复（历史 B05 回归） |
| 前端 server state 收敛 | 架构审查 G1/G2 仍成立：`domainStore` 手写缓存 + react-query 重复键；首屏 `/my-domains` 多次请求 | 部分处理 |
| 桌面覆盖层场景深化 | 覆盖层为 MyGo `ui` 原生 GPU 绘制（`client/desktop/overlay_view.go`），非 WebView、非 GPUI crate；能力已较完整，场景化能力待定 | 待探索 |

### 前端占位/简陋文件

React 19 迁移时已全部重写，下列旧占位文件均已不存在：

| 旧文件 | 现状 |
|------|------|
| `hooks/useTitle.ts` | 已删除；标题由 `components/titleBar/routeTitle.ts` 统一推导 |
| `layouts/ErrorComponent.tsx` | 已由 `components/common/errorBoundary` 三级边界取代 |
| `home/homePage.tsx`、`(app)/index/index.tsx` | 已由 `pages/(app)/index.tsx` 等文件路由取代 |

---

## 测试覆盖缺口

### 前端测试

- Vitest：覆盖 stores、socket、api、components 与工具；`test-web` / `test-packages` 已进 CI Gate（见 `.github/workflows/ci.yml`）。
- Playwright：视觉基线（`client/web/visual-baseline`）与房间语音 E2E（`room-voice-e2e` skill）。

### 后端集成测试

已有套件：auth / oauth / signal / user / role / room / mute / domain / message / storage / sfu / email / system / conversation / smoke / invite / join 等（精确清单以 `test/` 目录为准）。

**待补测试**（以代码现状为准）：

| 模块 | 缺口 | 优先级 |
|------|------|--------|
| Signal | `ListParticipants` 上游失败返回非 0 code 已由 `TestListParticipants_SFUError` 覆盖（历史 B05 回归防护已补） | 已完成 |
| Invite | 过期码不建 users、需审核域只建 pending、并发消耗、跨域隔离 | 高 |
| Mute | `/mute/*` 端点集成覆盖 | 已完成（`test/mute` 已覆盖基本路径） |
| Storage / SFU Config / OAuth Admin / Permission RBAC | 集成覆盖较薄 | 低 |

---

## 文档缺口

| 文档 | 状态 |
|------|------|
| 文档站（`docs`，VitePress） | ✅ guide / sfu / deployment / architecture 四组齐备，含桌面客户端页 |
| 桌面端说明（`client/desktop/README.md`） | ✅ 架构、开发、打包、覆盖层与启动耗时；覆盖层已注明 MyGo `ui` 原生绘制 |
| 测试指南 | ⚠️ 仅 `docs/e2e-test-spec.md`（语音 E2E）与 `agent_test_logs/AGENTS.md`（日志规范），无通用测试手册 |
| 贡献者指南 | ❌ 缺失 |
| 架构决策记录 (ADR) | ❌ 缺失 |
| 用户使用指南 | ✅ 文档站 `guide/quickstart` / `guide/tour` / `guide/desktop` |

### AGENTS.md 已同步

1. ✅ 路由表、模型与事件单一事实源指针已建立（见根 `AGENTS.md` §2）
2. ✅ `hooks/livekit/` 引用已替换为 `client/packages/sfu-client`
3. ✅ `useSubcribeTrack.ts` / `useTitle.ts` 已随 React 迁移移除，标题逻辑迁至 `components/titleBar/`
4. ✅ 桌面端能力边界与自定义标题栏规范已补入「Web / 桌面能力边界」小节
