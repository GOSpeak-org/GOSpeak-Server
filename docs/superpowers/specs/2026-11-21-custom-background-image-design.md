# 自定义背景图：全局共享背景 + 玻璃强度/不透明度滑杆

> 【状态】2026-11-27 核验：该设计已落地（背景图与玻璃强度 / 不透明度滑杆已交付），仅供历史参考。

日期：2026-11-21
分支：dev-1121

## 背景与目标

外观设置（`app/web/src/components/modal/settingModal/tab_item/appearance.tsx`）新增背景图配置。目标：上传一张图片作为**个人全局背景**，铺在应用最底层，通过两条滑杆自由控制「玻璃强度」（模糊半径）与「不透明度」（面板透明度）。

决策演进：

- 最初三档预设 → 升级为两条滑杆 + 三档快选；
- 最初深色/浅色各存一张图 → **改为全局共用一张图**（2026-11-21 用户要求，两种模式不再分离）；
- 持久化：图片 Blob 存 IndexedDB，偏好存 localStorage，纯前端零后端改动。

## 非目标

- 不跨设备同步；
- 不做内置图库、URL 输入、域级背景；
- 不做多标签页实时同步；
- 不改移动端设置页；
- 不做饱和度独立滑杆（由强度派生）；
- 不做 card / muted 分别独立的透明度滑杆。

## 设计

### A. 存储层（单图）

- IndexedDB `gospeak-web` / `backgrounds`，固定 key `shared`；读取时若 `shared` 缺失，回退到旧 key `light` / `dark`（迁移老用户已有图片），写入/删除时清理旧 key。
- localStorage key：`background-glass`，值 JSON `{ "strength": number, "opacity": number }`（0–100 整数）。旧 key `background-glass-light/dark` 与 `background-tier-light/dark` 仅在首次读不到新值时迁移，不再写入。
- 背景图对深色与浅色模式**共用同一张**；切换主题不重新读图、不换图。

### B. 图片压缩（`utils/backgroundImage.ts`）

- canvas 等比压缩，最长边 ≤ 2560px；`toBlob("image/webp", 0.95)`，不支持 webp 时回退 JPEG；
- 非 `image/*` 直接拒绝（toast）；压缩失败 toast 并保持不变。

### C. Store（`stores/backgroundStore.ts`）

- zustand「命令式门面 + hook」双入口；
- state：`glass: { strength, opacity }`、`hasImage`、`activeUrl`、`available`；
- actions：`init()`、`setImage(file)`、`removeImage()`、`setGlass(patch)`、`applyPreset(tier)`；
- 导出 `backgroundPresets`、`activePreset(glass)`、`glassBlurPx`、`glassSaturate`；
- objectURL 生命周期：set/remove 时 revoke 旧 URL；不再订阅 themeStore（图案不随模式切换）。

### D. DOM 应用

`applyBackground()`：

- `<html>` toggle `bg-custom`，写内联变量 `--bg-blur`、`--bg-card-alpha`、`--bg-muted-alpha`、`--bg-saturate`；
- `body.style.backgroundImage = url(objectURL)`（cover / center / fixed）；
- 无图时清空所有变量与 class，完全回到现状。

映射：

| 滑杆 | 取值 | 派生 |
|------|------|------|
| 强度 strength | 0–100 | `blur = round(strength × 24 / 100 × 2) / 2` px；`saturate = 1 + strength/100 × 0.36` |
| 不透明度 opacity | 0–100 | `--card alpha = opacity%`；`--muted alpha = round(opacity × 0.75)%` |

预设：轻透 25/85、均衡 58/68（默认）、沉浸 83/40。

### E. 主题 CSS（`packages/components/src/theme.css`）

- 保留 `--card-base` / `--muted-base` / `--background-base` 副本；
- `html.bg-custom` 一条规则用 `color-mix` 派生半透明 `--card` / `--muted`；
- `surface-glass` / `manage-shell` 继续用 `var(--bg-blur)` / `var(--bg-saturate)`。

### F. 外壳表面补齐（修复「直接透明」）

根因：部分外壳容器无背景色，原先靠 body 实色兜底；启用背景图后直接透出未模糊的图片。修复：给这些容器补 `bg-card/40 surface-glass`（无背景图时与现状一致，有背景图时半透明 + 模糊）：

- `header.tsx` 顶部栏根 div；
- `layout.tsx` 左栏外层（图标栏 + 动态列表 + 用户栏）与域工作区主容器；
- 管理页 `manage-shell` 已有专门半透明规则，保持不变。

### G. 设置 UI（`BackgroundSection.tsx`）

- Section「背景图」：说明文字改为「上传一张图片作为全局背景，深色与浅色模式共用」；
- 有图：16:9 预览 + 更换/删除；无图：虚线上传框；
- 三档快选 + 两条 `Slider`（玻璃强度、不透明度），自定义时显示「自定义」；
- 无图或环境不支持时控件置灰。

### H. 错误处理

- IndexedDB 失败：toast + 控件置灰；Blob 损坏：静默回退；桌面 WebView 原生支持 IndexedDB。

### I. 测试

- `backgroundDb.test.ts`：单图往返 + 旧 key 迁移；
- `backgroundStore.test.ts`：init / setImage / removeImage / setGlass / applyPreset、URL revoke、偏好迁移、DOM 变量；
- `BackgroundSection.test.tsx`：上传、置灰、删除、预设、滑杆、共享说明文案；
- e2e：注入棋盘格背景巡检发现域 / 管理 / 聊天页，确认无「清晰透出」的外壳区域。
