# Plans 索引 — 状态总览

> 更新于 2026-11-27 核验：历史计划一律保留并在文档顶部标注核验状态，不再删除，历史见 git log。
> 状态以各文档顶部标注及本文为准；部分计划尚未补写 Status 块。

## 已完成

| 计划 | 状态 | 说明 |
|---|---|---|
| [仓迁移与建仓](./2026-11-26-org-migration.md) | **迁移已完成** | `NoelOrin/GOSpeak` 已转入 `GOSpeak-org/GOSpeak`，三个空 public 子仓已建；受保护分支下的文档改动走 PR #71 |
| [屏幕共享按需订阅](./2026-11-24-screen-share-on-demand-subscribe.md) | **已交付（2026-11-27 核验）** | 6 Task 全部合入 client `release`；sfu-client 55 项 + web 702 项测试全绿；checkbox 与状态块已回填 |
| [插件平台 01：基础协议与 JS SDK](./2026-11-24-plugin-01-protocol-sdk.md) | **已交付** | proto + JS SDK + 示例插件模板；已从 feature-plugin-platform 迁移 |
| [插件平台 02：运行时与进程管理](./2026-11-24-plugin-02-runtime-manager.md) | **已交付** | 数据表/审计 + runtime 插件 + Manager；依赖 01；已从 feature-plugin-platform 迁移 |
| [插件平台 03：管理面与插件能力](./2026-11-24-plugin-03-capabilities.md) | **已交付** | 管理 API/页面 + 插件 API + Hook + 页面挂载；依赖 01/02；已从 feature-plugin-platform 迁移 |
| [插件平台 04：集成验证](./2026-11-24-plugin-04-integration.md) | **已交付** | gospeak_plugin_ 指标 + 离线 E2E 夹具 + api_endpoint 契约修复；依赖 01/02/03；已从 feature-plugin-platform 迁移 |
| [插件平台 05：文档与文档站](./2026-11-24-plugin-05-docs.md) | **已交付** | AGENTS/部署文档指针 + VitePress 插件指南与 API 参考；依赖 01–04；已从 feature-plugin-platform 迁移 |

## 进行中 / 待跟进

| 计划 | 状态 | 剩余 |
|---|---|---|
| [app/mobile 移动客户端](./2026-11-16-app-mobile.md) | 主体已交付 | 真机后台音频 PoC（暂缓）、服务端推送（暂缓）、移动端测试包已改独立手动 workflow |
| [域文件夹与个人布局持久化](./2026-11-21-domain-folder-layout.md) | 部分交付 | Task 1 后端 `user_domain_layout` 模型与测试已落地；计划勾选未回填，与房间分组方案有重叠，需确认剩余范围 |
| [域内房间分组与排序](./2026-11-21-domain-room-grouping-sort.md) | 主体已交付 | 计划状态块未回填；后端布局摘要、前端侧栏分组拖拽与排序已落地 |
| [Release 产物分离与独立发版](./2026-11-26-per-component-release.md) | **已交付（前置子集）** | 一组件一 Release、独立发版、移动端 CI 优化均已落地；作为 Submodule 拆分的 Task 12 前置 |
| [音频输出策略抽象与 Web Audio 优先 fallback](./2026-11-25-audio-output-strategy.md) | 待执行 | 6 个 Task；Web Audio 优先，media element fallback |

> 关于页 GitHub 项目地址入口已交付并移出索引：`Browser.OpenExternal` 桥接与前端「项目地址」入口均已落地。