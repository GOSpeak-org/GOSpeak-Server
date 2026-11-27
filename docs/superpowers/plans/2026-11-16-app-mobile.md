# app/mobile 移动客户端实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Status**（2026-11-16 更新）：主体已交付。Task 1-4、6-12 已完成并合入；Task 5（真机后台 WebRTC 音频 PoC）按用户指示暂缓，Push 服务端链路暂缓。计划保留跟进，剩余项见文末「剩余工作」。
>
> 交付核验：能力包 web/mobile 两个 target 测试全绿；`check-mobile-bridge` 桥接门禁 7/7；`pnpm typecheck` 与 `pnpm lint` 全仓通过；web / desktop / mobile 三目标构建成功。


**Goal:** 交付 `app/mobile`（Capacitor 原生壳）与 `packages/mobile-capabilities`（移动能力 SDK），复用现有 `app/web` React 源码产出可日常使用的 iOS/Android 语音客户端。

**Architecture:** `app/web` 保持唯一 React 源码，新增 `GOSPEAK_TARGET=mobile` 构建目标产出移动 bundle；Capacitor 壳把该 bundle 装进原生工程。平台差异全部收敛到 `packages/mobile-capabilities`（模式对齐 `packages/desktop`），每个能力同时提供原生实现与 Web 降级，业务代码只经 `inMobile()` + 能力门面访问。

**Tech Stack:** Capacitor 8.6.2 · React 19 · TypeScript · Vite（rolldown）· Vitest · Swift/AVAudioSession（iOS）· Kotlin/Android Service（Android）

**Spec:** `docs/superpowers/specs/2026-11-16-app-mobile-selection-design.md`

## Global Constraints

- 目标常量：`GOSPEAK_TARGET` 取 `web` | `desktop` | `mobile`，默认 `web`。Vite 注入 `__GOSPEAK_TARGET__` / `__GOSPEAK_DESKTOP__` / `__GOSPEAK_MOBILE__`。
- 业务代码判定平台**一律用 `inMobile()` / `inDesktop()`**；禁止页面直接 import Capacitor 插件。
- 模块级裁剪写 `if (__GOSPEAK_MOBILE__)`（编译期常量，可折叠），UI 条件渲染写 `inMobile()`。
- 每个能力必须同时实现 native 与 Web 降级；降级**无副作用、可调用、不抛异常**。
- 不改后端协议、不新增 WS 事件、不新增 SFU provider（仍 LiveKit 唯一）。
- 不做第二套 UI 源码，不引入新状态管理。
- 代码不加注释（复杂逻辑除外），代码内不用 emoji，Biome 统一 tab 缩进 / LF。
- 提交遵循 Conventional Commits。
- Capacitor 版本统一 `^8.6.2`；`@capacitor/push-notifications` `^8.1.3`；`@capacitor/app` `^8.1.2`。
- 改 `app/web` 后仍需跑 `pnpm --filter @gospeak/web build:desktop` 验证桌面不回退。

---

## 文件结构

**新建**

| 文件 | 职责 |
|---|---|
| `packages/mobile-capabilities/package.json` | 包声明，依赖 `@capacitor/core` |
| `packages/mobile-capabilities/tsconfig.json` | 与 desktop 包同构的编译配置 |
| `packages/mobile-capabilities/vitest.config.ts` | 注入三值目标常量 |
| `packages/mobile-capabilities/src/globals.d.ts` | `__GOSPEAK_TARGET__` / `__GOSPEAK_MOBILE__` 声明 |
| `packages/mobile-capabilities/src/conditional.ts` | `BUILD_TARGET` / `MOBILE_BUILD` / `inMobile()` / `assertMobile()` |
| `packages/mobile-capabilities/src/bridge.ts` | `pluginMethod()`、`getPlugin()`、`listenPlugin()` |
| `packages/mobile-capabilities/src/capabilities/audio-session.ts` | 后台音频会话 activate/deactivate |
| `packages/mobile-capabilities/src/capabilities/audio-route.ts` | 音频路由枚举与切换 |
| `packages/mobile-capabilities/src/capabilities/media-control.ts` | 锁屏/通知栏控制 |
| `packages/mobile-capabilities/src/capabilities/push.ts` | 推送注册与 token 上报 |
| `packages/mobile-capabilities/src/capabilities/deeplink.ts` | `gospeak://` 深链 |
| `packages/mobile-capabilities/src/capabilities/lifecycle.ts` | 前后台切换与恢复钩子 |
| `packages/mobile-capabilities/src/index.ts` | 具名导出 + `createMobile()` |
| `packages/mobile-capabilities/AGENTS.md` | 能力模式与契约说明 |
| `app/mobile/package.json` | `@gospeak/mobile`，Capacitor 依赖与脚本 |
| `app/mobile/capacitor.config.ts` | appId / webDir / dev server / scheme |
| `app/mobile/ios/` `app/mobile/android/` | `cap add` 生成的原生工程（提交） |
| `app/mobile/ios/App/App/AudioSessionPlugin.swift` | iOS 音频会话与路由插件 |
| `app/mobile/android/app/src/main/java/dev/gospeak/mobile/AudioSessionPlugin.kt` | Android 音频会话与路由插件 |
| `app/mobile/AGENTS.md` | 壳结构、构建、原生工程维护约定 |
| `scripts/check-mobile-bridge.mjs` | 桥接契约校验 |
| `scripts/check-mobile-bridge.test.mjs` | 门禁脚本单测 |

**修改**

| 文件 | 改动 |
|---|---|
| `app/web/vite.config.ts:31-46` | 目标解析加 `mobile`，注入 `__GOSPEAK_MOBILE__`，mobile 输出 `dist-mobile` |
| `app/web/package.json` | 加 `build:mobile` / `dev:mobile` |
| `app/web/types/gospeak-build.d.ts:9-10` | 目标类型加 `"mobile"`，声明 `__GOSPEAK_MOBILE__` |
| `packages/desktop/src/globals.d.ts:11` | 目标类型加 `"mobile"`（保持两处副本一致） |
| `packages/desktop/src/conditional.ts:19` | `BuildTarget` 加 `"mobile"` |
| `turbo.json` | 加 `build:mobile` / `cap:sync` 任务 |
| `biome.json:14-20` | includes 加 `packages/mobile-capabilities/**` 与 `app/mobile/*.{ts,json,jsonc}` |
| `.github/workflows/ci.yml:91` | biome ci 目录加 `./app/mobile` |
| `lefthook.yml:22` | biome ci 目录加 `./app/mobile`；pre-push 加移动桥接门禁 |
| `AGENTS.md` | 目录骨架与单一事实源索引补移动端 |

---

## Task 1: `packages/mobile-capabilities` 骨架与条件编译

**Files:**
- Create: `packages/mobile-capabilities/package.json`
- Create: `packages/mobile-capabilities/tsconfig.json`
- Create: `packages/mobile-capabilities/vitest.config.ts`
- Create: `packages/mobile-capabilities/src/globals.d.ts`
- Create: `packages/mobile-capabilities/src/conditional.ts`
- Test: `packages/mobile-capabilities/src/conditional.test.ts`

**Interfaces:**
- Produces: `BuildTarget = "web" | "desktop" | "mobile"`、`BUILD_TARGET`、`MOBILE_BUILD`、`WEB_BUILD`、`inMobile(): boolean`、`assertMobile(feature: string): void`、`ifMobile<T>(mobile, fallback)`、`importMobile<M>(loader): Promise<M | null>`

- [ ] **Step 1: 建包声明**

创建 `packages/mobile-capabilities/package.json`：

```json
{
	"name": "@gospeak/mobile-capabilities",
	"version": "0.1.0",
	"private": true,
	"type": "module",
	"main": "./src/index.ts",
	"module": "./src/index.ts",
	"types": "./src/index.ts",
	"exports": {
		".": {
			"types": "./src/index.ts",
			"import": "./src/index.ts"
		}
	},
	"scripts": {
		"build": "tsc",
		"typecheck": "tsc --noEmit",
		"lint": "biome check",
		"dev": "tsc --watch",
		"test": "vitest run --passWithNoTests",
		"check": "biome check",
		"format": "biome format --write"
	},
	"dependencies": {
		"@capacitor/core": "^8.6.2"
	},
	"devDependencies": {
		"typescript": "~7.0.2",
		"vitest": "^5.1.0"
	}
}
```

- [ ] **Step 2: 建 tsconfig 与 vitest 配置**

创建 `packages/mobile-capabilities/tsconfig.json`：

```json
{
	"compilerOptions": {
		"target": "ES2022",
		"module": "ESNext",
		"moduleResolution": "bundler",
		"lib": ["ESNext", "DOM"],
		"strict": true,
		"skipLibCheck": true,
		"noEmit": true,
		"paths": {
			"@/*": ["./src/*"]
		}
	},
	"include": ["src"]
}
```

创建 `packages/mobile-capabilities/vitest.config.ts`：

```ts
import { defineConfig } from "vitest/config";

export default defineConfig(() => {
	const target =
		process.env.GOSPEAK_TARGET === "mobile"
			? "mobile"
			: process.env.GOSPEAK_TARGET === "desktop"
				? "desktop"
				: "web";
	return {
		define: {
			__GOSPEAK_TARGET__: JSON.stringify(target),
			__GOSPEAK_DESKTOP__: String(target === "desktop"),
			__GOSPEAK_MOBILE__: String(target === "mobile"),
		},
		test: {
			environment: "node",
		},
	};
});
```

- [ ] **Step 3: 写失败测试**

创建 `packages/mobile-capabilities/src/conditional.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { BUILD_TARGET, MOBILE_BUILD, inMobile } from "./conditional";

describe("build target constants", () => {
	it("defaults to a known target outside a bundler", () => {
		expect(["web", "desktop", "mobile"]).toContain(BUILD_TARGET);
	});

	it("MOBILE_BUILD mirrors the mobile target", () => {
		expect(MOBILE_BUILD).toBe(BUILD_TARGET === "mobile");
	});
});

describe("inMobile", () => {
	it("returns a boolean regardless of runtime", () => {
		expect(typeof inMobile()).toBe("boolean");
	});

	it("does not throw when Capacitor is absent", () => {
		expect(() => inMobile()).not.toThrow();
	});
});
```

- [ ] **Step 4: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test`
Expected: FAIL — `Failed to resolve import "./conditional"`

- [ ] **Step 5: 实现条件编译**

创建 `packages/mobile-capabilities/src/globals.d.ts`：

```ts
/**
 * Build-target ambient tokens injected by the bundler via `define`.
 *
 * Keep in sync with `app/web/types/gospeak-build.d.ts` and
 * `packages/desktop/src/globals.d.ts`.
 */
declare const __GOSPEAK_TARGET__: "web" | "desktop" | "mobile";
declare const __GOSPEAK_DESKTOP__: boolean;
declare const __GOSPEAK_MOBILE__: boolean;
```

创建 `packages/mobile-capabilities/src/conditional.ts`：

```ts
/**
 * 构建期条件编译工具集（与 @gospeak/desktop 的 conditional.ts 同构）。
 *
 * 1. 编译期常量 __GOSPEAK_MOBILE__ / __GOSPEAK_TARGET__ 由打包器 define 注入，
 *    在调用处写 if 即可常量折叠，死分支连同其中的 import() 一并剔除。
 * 2. 运行时检测 isCapacitor() 兜底「web 构建被原生壳打开」的 dev 场景。
 *
 * inMobile() 是两者的合成，用于 UI 条件渲染；模块级裁剪用 token。
 */

export type BuildTarget = "web" | "desktop" | "mobile";

export const BUILD_TARGET: BuildTarget =
	typeof __GOSPEAK_TARGET__ === "undefined" ? "web" : __GOSPEAK_TARGET__;

export const MOBILE_BUILD: boolean = BUILD_TARGET === "mobile";

export const DESKTOP_BUILD: boolean = BUILD_TARGET === "desktop";

export const WEB_BUILD: boolean = !MOBILE_BUILD && !DESKTOP_BUILD;

interface CapacitorGlobal {
	Capacitor?: { isNativePlatform?: () => boolean };
}

export function isCapacitor(): boolean {
	const holder = globalThis as CapacitorGlobal;
	const native = holder.Capacitor;
	if (!native || typeof native.isNativePlatform !== "function") return false;
	try {
		return native.isNativePlatform() === true;
	} catch {
		return false;
	}
}

export function inMobile(): boolean {
	if (MOBILE_BUILD) return true;
	return isCapacitor();
}

export function assertMobile(feature: string): void {
	if (!inMobile()) {
		throw new Error(
			`[gospeak/mobile] "${feature}" 仅在移动端可用（当前既非 mobile 构建，也没有 Capacitor 运行时）。` +
				"请用 __GOSPEAK_MOBILE__ / inMobile() 守卫调用点，或改用移动构建（GOSPEAK_TARGET=mobile）。",
		);
	}
}

export function ifMobile<T>(mobile: () => T, fallback: () => T): T {
	return __GOSPEAK_MOBILE__ ? mobile() : fallback();
}

export async function importMobile<M>(
	loader: () => Promise<M>,
): Promise<M | null> {
	if (!__GOSPEAK_MOBILE__) return null;
	return loader();
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test`
Expected: PASS（4 tests）

- [ ] **Step 7: 安装依赖并跑类型检查**

Run: `pnpm install && pnpm --filter @gospeak/mobile-capabilities typecheck`
Expected: 无类型错误

- [ ] **Step 8: 提交**

```bash
git add packages/mobile-capabilities
git commit -m "feat(mobile): scaffold mobile capabilities package with conditional compilation"
```

---

## Task 2: 桥接层与插件调用封装

**Files:**
- Create: `packages/mobile-capabilities/src/bridge.ts`
- Test: `packages/mobile-capabilities/src/bridge.test.ts`

**Interfaces:**
- Consumes: Task 1 的 `inMobile()`
- Produces: `pluginMethod(plugin: string, method: string): string`、`getPlugin<T>(name: string): Promise<T>`、`listenPlugin<T>(name: string, event: string, handler: (payload: T) => void): Promise<() => void>`

- [ ] **Step 1: 写失败测试**

创建 `packages/mobile-capabilities/src/bridge.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { pluginMethod } from "./bridge";

describe("pluginMethod", () => {
	it("joins plugin and method with a dot", () => {
		expect(pluginMethod("AudioSession", "activate")).toBe(
			"AudioSession.activate",
		);
	});

	it("keeps the exact casing of both halves", () => {
		expect(pluginMethod("MediaControl", "setMetadata")).toBe(
			"MediaControl.setMetadata",
		);
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- bridge`
Expected: FAIL — `Failed to resolve import "./bridge"`

- [ ] **Step 3: 实现桥接层**

创建 `packages/mobile-capabilities/src/bridge.ts`：

```ts
/**
 * Capacitor 插件访问层。
 *
 * 与 @gospeak/desktop 的 bridge.ts 对应：Capacitor 插件以
 * registerPlugin("<Name>") 取得代理，方法名必须与原生侧
 * @CapacitorPlugin(name = ...) + @PluginMethod 逐字一致。
 * 方法名拼错时运行期只会得到 "not implemented"，多数能力会静默吞掉，
 * 因此由 scripts/check-mobile-bridge.mjs 在 CI 拦截。
 *
 * @capacitor/core 只在原生分支动态加载，web / desktop 构建里整段被折叠剔除。
 */

export function pluginMethod(plugin: string, method: string): string {
	return `${plugin}.${method}`;
}

interface PluginProxy {
	addListener?(
		event: string,
		handler: (payload: unknown) => void,
	): Promise<{ remove: () => Promise<void> }> | { remove: () => Promise<void> };
}

export async function getPlugin<T extends object>(name: string): Promise<T> {
	if (!__GOSPEAK_MOBILE__) {
		throw new Error(
			`[gospeak/mobile] 插件 "${name}" 需要移动端运行时（当前不是 mobile 构建）。`,
		);
	}
	const { registerPlugin } = await import("@capacitor/core");
	return registerPlugin<T>(name);
}

export async function listenPlugin<T>(
	name: string,
	event: string,
	handler: (payload: T) => void,
): Promise<() => void> {
	if (!__GOSPEAK_MOBILE__) return () => {};
	const plugin = await getPlugin<PluginProxy>(name);
	if (typeof plugin.addListener !== "function") return () => {};
	const handle = await plugin.addListener(event, (payload) =>
		handler(payload as T),
	);
	return () => {
		void handle.remove();
	};
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- bridge`
Expected: PASS（2 tests）

- [ ] **Step 5: 提交**

```bash
git add packages/mobile-capabilities/src/bridge.ts packages/mobile-capabilities/src/bridge.test.ts
git commit -m "feat(mobile): add capacitor plugin bridge layer"
```

---

## Task 3: `app/web` 增加 `mobile` 构建目标

**Files:**
- Modify: `app/web/vite.config.ts:31-46`
- Modify: `app/web/package.json`
- Modify: `app/web/types/gospeak-build.d.ts:9-10`
- Modify: `packages/desktop/src/globals.d.ts:11-12`
- Modify: `packages/desktop/src/conditional.ts:19`

**Interfaces:**
- Produces: `GOSPEAK_TARGET=mobile` 构建产出 `app/web/dist-mobile`；注入 `__GOSPEAK_MOBILE__`

- [ ] **Step 1: 扩展 vite 目标解析**

把 `app/web/vite.config.ts` 第 31-34 行替换为：

```ts
  // 条件编译目标：GOSPEAK_TARGET=desktop 构建桌面端，=mobile 构建移动端
  // （分别由 MyGo / Capacitor 壳装载），默认 web。见 @gospeak/desktop 的 conditional.ts。
  const rawTarget = process.env.GOSPEAK_TARGET;
  const gospeakTarget: "web" | "desktop" | "mobile" =
    rawTarget === "desktop" ? "desktop" : rawTarget === "mobile" ? "mobile" : "web";
```

- [ ] **Step 2: 注入 mobile 常量**

把 `define` 块（第 38-46 行）替换为：

```ts
    define: {
      __GOSPEAK_TARGET__: JSON.stringify(gospeakTarget),
      __GOSPEAK_DESKTOP__: String(gospeakTarget === "desktop"),
      __GOSPEAK_MOBILE__: String(gospeakTarget === "mobile"),
      // 导出日志的版本头用；与 package.json 的 version 同步。
      __APP_VERSION__: JSON.stringify(
        JSON.parse(readFileSync(new URL("./package.json", import.meta.url), "utf8"))
          .version,
      ),
    },
```

- [ ] **Step 3: 移动构建输出独立目录**

在 `build` 配置块（第 156 行 `build: {`）内、`target` 之前插入：

```ts
      outDir: gospeakTarget === "mobile" ? "dist-mobile" : "dist",
```

- [ ] **Step 4: 让 dev 监听新包源码**

把 `server.watch.ignored` 数组（第 106-110 行）改为：

```ts
        ignored: [
          "!**/packages/sfu-client/src/**",
          "!**/packages/components/src/**",
          "!**/packages/desktop/src/**",
          "!**/packages/mobile-capabilities/src/**",
        ],
```

并在 `optimizeDeps.exclude` 数组（第 120-126 行）追加一项：

```ts
        "@gospeak/mobile-capabilities",
```

- [ ] **Step 5: 加构建脚本**

在 `app/web/package.json` 的 `scripts` 中，`"dev:desktop"` 之后加：

```json
    "build:mobile": "GOSPEAK_TARGET=mobile vite build && tsc",
    "dev:mobile": "GOSPEAK_TARGET=mobile vite",
```

- [ ] **Step 6: 同步三处目标类型声明**

`app/web/types/gospeak-build.d.ts` 第 9-10 行改为：

```ts
declare const __GOSPEAK_TARGET__: "web" | "desktop" | "mobile";
declare const __GOSPEAK_DESKTOP__: boolean;
declare const __GOSPEAK_MOBILE__: boolean;
```

`packages/desktop/src/globals.d.ts` 第 11-12 行改为：

```ts
declare const __GOSPEAK_TARGET__: "web" | "desktop" | "mobile";
declare const __GOSPEAK_DESKTOP__: boolean;
declare const __GOSPEAK_MOBILE__: boolean;
```

`packages/desktop/src/conditional.ts` 第 19 行改为：

```ts
export type BuildTarget = "web" | "desktop" | "mobile";
```

- [ ] **Step 7: 验证 web 与 desktop 构建不回退**

Run:
```bash
pnpm --filter @gospeak/web typecheck
pnpm --filter @gospeak/web build:desktop
pnpm --filter @gospeak/web build
```
Expected: 三条命令全部成功

- [ ] **Step 8: 验证 mobile 构建**

Run: `pnpm --filter @gospeak/web build:mobile`
Expected: 成功，产物在 `app/web/dist-mobile`，含 `index.html`

- [ ] **Step 9: 提交**

```bash
git add app/web/vite.config.ts app/web/package.json app/web/types/gospeak-build.d.ts packages/desktop/src/globals.d.ts packages/desktop/src/conditional.ts
git commit -m "feat(web): add mobile build target"
```

---

## Task 4: `app/mobile` Capacitor 壳与原生工程

**Files:**
- Create: `app/mobile/package.json`
- Create: `app/mobile/capacitor.config.ts`
- Create: `app/mobile/ios/`、`app/mobile/android/`（由 `cap add` 生成）
- Modify: `turbo.json`

**Interfaces:**
- Consumes: Task 3 的 `app/web/dist-mobile`
- Produces: 可 `cap sync` 的原生工程

- [ ] **Step 1: 建壳包**

创建 `app/mobile/package.json`：

```json
{
	"name": "@gospeak/mobile",
	"version": "1.0.3",
	"private": true,
	"description": "GOSpeak mobile client host (Capacitor).",
	"type": "module",
	"scripts": {
		"build:web": "pnpm --filter @gospeak/web build:mobile",
		"sync": "pnpm run build:web && cap sync",
		"sync:ios": "pnpm run build:web && cap sync ios",
		"sync:android": "pnpm run build:web && cap sync android",
		"open:ios": "cap open ios",
		"open:android": "cap open android",
		"run:ios": "pnpm run sync:ios && cap run ios",
		"run:android": "pnpm run sync:android && cap run android"
	},
	"dependencies": {
		"@capacitor/android": "^8.6.2",
		"@capacitor/app": "^8.1.2",
		"@capacitor/core": "^8.6.2",
		"@capacitor/ios": "^8.6.2",
		"@capacitor/push-notifications": "^8.1.3",
		"@gospeak/mobile-capabilities": "workspace:*"
	},
	"devDependencies": {
		"@capacitor/cli": "^8.6.2"
	}
}
```

- [ ] **Step 2: 写 Capacitor 配置**

创建 `app/mobile/capacitor.config.ts`：

```ts
import type { CapacitorConfig } from "@capacitor/cli";

/**
 * 移动端宿主配置。
 *
 * - webDir 指向 app/web 的 mobile 目标产物（见 app/web/vite.config.ts 的 outDir）。
 * - dev：设 GOSPEAK_MOBILE_DEV_URL 后窗口直接加载局域网 Vite（真机热重载），
 *   与桌面端 GOSPEAK_DESKTOP_DEV 的思路一致。
 * - release：加载内嵌 bundle。
 */
const devUrl = process.env.GOSPEAK_MOBILE_DEV_URL;

const config: CapacitorConfig = {
	appId: "dev.gospeak.mobile",
	appName: "GOSpeak",
	webDir: "../web/dist-mobile",
	server: devUrl ? { url: devUrl, cleartext: true } : undefined,
	ios: {
		contentInset: "always",
		scheme: "GOSpeak",
	},
	android: {
		allowMixedContent: false,
	},
	plugins: {
		PushNotifications: {
			presentationOptions: ["badge", "sound", "alert"],
		},
	},
};

export default config;
```

- [ ] **Step 3: 安装依赖**

Run: `pnpm install`
Expected: 成功，`pnpm-lock.yaml` 更新

- [ ] **Step 4: 生成原生工程**

Run:
```bash
cd app/mobile && pnpm exec cap add ios && pnpm exec cap add android
```
Expected: 生成 `app/mobile/ios/App/App.xcodeproj` 与 `app/mobile/android/app/build.gradle`

- [ ] **Step 5: 配置 iOS 后台音频与权限**

编辑 `app/mobile/ios/App/App/Info.plist`，在顶层 `<dict>` 内加入：

```xml
	<key>UIBackgroundModes</key>
	<array>
		<string>audio</string>
	</array>
	<key>NSMicrophoneUsageDescription</key>
	<string>GOSpeak 需要使用麦克风进行游戏语音通话。</string>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>gospeak</string>
			</array>
		</dict>
	</array>
```

- [ ] **Step 6: 配置 Android 权限与深链**

编辑 `app/mobile/android/app/src/main/AndroidManifest.xml`，在 `<manifest>` 内加：

```xml
	<uses-permission android:name="android.permission.RECORD_AUDIO" />
	<uses-permission android:name="android.permission.MODIFY_AUDIO_SETTINGS" />
	<uses-permission android:name="android.permission.INTERNET" />
	<uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
	<uses-permission android:name="android.permission.FOREGROUND_SERVICE_MICROPHONE" />
	<uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
```

在 `<activity>` 内加深链 intent-filter：

```xml
			<intent-filter>
				<action android:name="android.intent.action.VIEW" />
				<category android:name="android.intent.category.DEFAULT" />
				<category android:name="android.intent.category.BROWSABLE" />
				<data android:scheme="gospeak" />
			</intent-filter>
```

- [ ] **Step 7: 加 turbo 任务**

在 `turbo.json` 的 `tasks` 中，`build` 之后加入：

```json
		"build:mobile": {
			"dependsOn": ["^build"],
			"outputs": ["dist-mobile/**"]
		},
		"cap:sync": {
			"dependsOn": ["^build"],
			"cache": false
		},
```

- [ ] **Step 8: 验证同步链路**

Run: `pnpm --filter @gospeak/mobile sync`
Expected: 构建 web mobile 产物并 `cap sync` 成功；`app/mobile/ios/App/App/public` 出现 `index.html`

- [ ] **Step 9: 提交**

```bash
git add app/mobile turbo.json pnpm-lock.yaml
git commit -m "feat(mobile): add capacitor shell with ios and android projects"
```

---

## Task 5: 后台 WebRTC 音频 PoC 门禁（手动，阻塞后续）

**Files:**
- Create: `agent_test_logs/移动端后台音频PoC-2026-11-16.md`

**Interfaces:**
- Produces: 通过/不通过的结论，决定 Task 6-11 是否继续

- [ ] **Step 1: 准备真机与后端**

确认手机与开发机同网段，后端已起（`pnpm dev:server`，:8998），LiveKit 可用。

- [ ] **Step 2: 以 dev 模式装入真机**

Run:
```bash
GOSPEAK_MOBILE_DEV_URL=http://<开发机局域网IP>:3000 pnpm --filter @gospeak/mobile run:ios
```
（Android 用 `run:android`）

- [ ] **Step 3: 验证前台通话**

登录 → 进入一个语音房间 → 开麦说话 → 确认另一端能听到。

- [ ] **Step 4: 验证锁屏后音频持续**

锁屏 5 分钟，期间另一端持续说话；解锁后确认：
1. 仍能听到对方（下行未断）
2. 自己说话对方仍能听到（上行未断）

- [ ] **Step 5: 验证后台切换**

切到桌面/其他 App 30 秒后返回，重复 Step 4 的两项检查。

- [ ] **Step 6: 记录结论**

创建 `agent_test_logs/移动端后台音频PoC-2026-11-16.md`，按 `agent_test_logs/AGENTS.md` 模板记录：设备型号、系统版本、iOS/Android 分别结论、失败时的表现（音频静音 / 会话断开 / 进程被杀）。

- [ ] **Step 7: 决策门禁**

- **两端都通过** → 继续 Task 6。
- **任一平台不通过** → **停止**，把结论回报给用户，按 spec §4.3 重新决策（下沉原生媒体层或改路线）。

- [ ] **Step 8: 提交记录**

```bash
git add agent_test_logs/移动端后台音频PoC-2026-11-16.md
git commit -m "test(mobile): record background webrtc audio poc results"
```

---

## Task 6: AudioSession 能力（后台音频）

**Files:**
- Create: `packages/mobile-capabilities/src/capabilities/audio-session.ts`
- Create: `app/mobile/ios/App/App/AudioSessionPlugin.swift`
- Create: `app/mobile/ios/App/App/AudioSessionPlugin.m`
- Create: `app/mobile/android/app/src/main/java/dev/gospeak/mobile/AudioSessionPlugin.kt`
- Test: `packages/mobile-capabilities/src/capabilities/audio-session.test.ts`

**Interfaces:**
- Consumes: `pluginMethod`、`getPlugin`（Task 2）
- Produces: `createAudioSession(): AudioSession`，`AudioSession = { activate(): Promise<void>; deactivate(): Promise<void>; isActive(): boolean }`

- [ ] **Step 1: 写失败测试**

创建 `packages/mobile-capabilities/src/capabilities/audio-session.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { createAudioSession } from "./audio-session";

describe("createAudioSession in web fallback", () => {
	it("resolves activate without a native runtime", async () => {
		const session = createAudioSession();
		await expect(session.activate()).resolves.toBeUndefined();
	});

	it("resolves deactivate without a native runtime", async () => {
		const session = createAudioSession();
		await expect(session.deactivate()).resolves.toBeUndefined();
	});

	it("reports inactive by default", () => {
		expect(createAudioSession().isActive()).toBe(false);
	});

	it("tracks active state after activate in fallback", async () => {
		const session = createAudioSession();
		await session.activate();
		expect(session.isActive()).toBe(true);
	});

	it("never throws when deactivated twice", async () => {
		const session = createAudioSession();
		await session.activate();
		await session.deactivate();
		await expect(session.deactivate()).resolves.toBeUndefined();
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- audio-session`
Expected: FAIL — `Failed to resolve import "./audio-session"`

- [ ] **Step 3: 实现能力**

创建 `packages/mobile-capabilities/src/capabilities/audio-session.ts`：

```ts
import { getPlugin, pluginMethod } from "../bridge";

/**
 * 后台音频会话。
 *
 * 语音会话开始时 activate，把系统音频类别切到「录音 + 播放」（iOS
 * playAndRecord / Android MODE_IN_COMMUNICATION），否则锁屏或切后台后
 * 采集会被系统掐断。会话结束时 deactivate，把音频交还系统。
 */

interface AudioSessionPlugin {
	activate(): Promise<void>;
	deactivate(): Promise<void>;
}

export interface AudioSession {
	activate(): Promise<void>;
	deactivate(): Promise<void>;
	isActive(): boolean;
}

export function createAudioSession(): AudioSession {
	let active = false;

	const call = async (method: "activate" | "deactivate") => {
		if (!__GOSPEAK_MOBILE__) return;
		try {
			const plugin = await getPlugin<AudioSessionPlugin>("AudioSession");
			await plugin[method]();
		} catch (error) {
			console.warn(
				`[gospeak/mobile] ${pluginMethod("AudioSession", method)} failed`,
				error,
			);
		}
	};

	return {
		async activate() {
			await call("activate");
			active = true;
		},
		async deactivate() {
			await call("deactivate");
			active = false;
		},
		isActive() {
			return active;
		},
	};
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- audio-session`
Expected: PASS（5 tests）

- [ ] **Step 5: 写 iOS 原生插件**

创建 `app/mobile/ios/App/App/AudioSessionPlugin.swift`：

```swift
import AVFoundation
import Capacitor
import Foundation

@objc(AudioSessionPlugin)
public class AudioSessionPlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "AudioSessionPlugin"
    public let jsName = "AudioSession"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "activate", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "deactivate", returnType: CAPPluginReturnPromise),
    ]

    @objc func activate(_ call: CAPPluginCall) {
        do {
            let session = AVAudioSession.sharedInstance()
            try session.setCategory(
                .playAndRecord,
                mode: .voiceChat,
                options: [.allowBluetooth, .allowBluetoothA2DP, .defaultToSpeaker]
            )
            try session.setActive(true, options: [])
            call.resolve()
        } catch {
            call.reject("failed to activate audio session: \(error.localizedDescription)")
        }
    }

    @objc func deactivate(_ call: CAPPluginCall) {
        do {
            try AVAudioSession.sharedInstance().setActive(
                false,
                options: [.notifyOthersOnDeactivation]
            )
            call.resolve()
        } catch {
            call.reject("failed to deactivate audio session: \(error.localizedDescription)")
        }
    }
}
```

创建 `app/mobile/ios/App/App/AudioSessionPlugin.m`：

```objc
#import <Capacitor/Capacitor.h>

CAP_PLUGIN(AudioSessionPlugin, "AudioSession",
           CAP_PLUGIN_METHOD(activate, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(deactivate, CAPPluginReturnPromise);
)
```

- [ ] **Step 6: 写 Android 原生插件**

创建 `app/mobile/android/app/src/main/java/dev/gospeak/mobile/AudioSessionPlugin.kt`：

```kotlin
package dev.gospeak.mobile

import android.content.Context
import android.media.AudioManager
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin

@CapacitorPlugin(name = "AudioSession")
class AudioSessionPlugin : Plugin() {

    private var previousMode: Int? = null

    @PluginMethod
    fun activate(call: PluginCall) {
        try {
            val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            if (previousMode == null) previousMode = audio.mode
            audio.mode = AudioManager.MODE_IN_COMMUNICATION
            audio.isSpeakerphoneOn = false
            call.resolve()
        } catch (error: Exception) {
            call.reject("failed to activate audio session: ${error.message}")
        }
    }

    @PluginMethod
    fun deactivate(call: PluginCall) {
        try {
            val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            previousMode?.let { audio.mode = it }
            previousMode = null
            call.resolve()
        } catch (error: Exception) {
            call.reject("failed to deactivate audio session: ${error.message}")
        }
    }
}
```

- [ ] **Step 7: 注册 Android 插件**

编辑 `app/mobile/android/app/src/main/java/dev/gospeak/mobile/MainActivity.kt`，在 `onCreate` 内、`super.onCreate` 之前注册：

```kotlin
    override fun onCreate(savedInstanceState: android.os.Bundle?) {
        registerPlugin(AudioSessionPlugin::class.java)
        super.onCreate(savedInstanceState)
    }
```

- [ ] **Step 8: 验证原生编译与同步**

Run:
```bash
pnpm --filter @gospeak/mobile sync
cd app/mobile/android && ./gradlew assembleDebug
```
Expected: Gradle 编译成功（iOS 用 `pnpm --filter @gospeak/mobile open:ios` 后在 Xcode 编译）

- [ ] **Step 9: 提交**

```bash
git add packages/mobile-capabilities/src/capabilities/audio-session.ts packages/mobile-capabilities/src/capabilities/audio-session.test.ts app/mobile/ios app/mobile/android
git commit -m "feat(mobile): add audio session capability for background voice"
```

---

## Task 7: 音频路由能力

**Files:**
- Create: `packages/mobile-capabilities/src/capabilities/audio-route.ts`
- Modify: `app/mobile/ios/App/App/AudioSessionPlugin.swift`
- Modify: `app/mobile/ios/App/App/AudioSessionPlugin.m`
- Modify: `app/mobile/android/app/src/main/java/dev/gospeak/mobile/AudioSessionPlugin.kt`
- Test: `packages/mobile-capabilities/src/capabilities/audio-route.test.ts`

**Interfaces:**
- Produces: `createAudioRoute(): AudioRoute`，`AudioRoute = { list(): Promise<AudioRouteOption[]>; current(): AudioRouteKind; set(kind: AudioRouteKind): Promise<void> }`，`AudioRouteKind = "speaker" | "earpiece" | "bluetooth" | "headset"`

- [ ] **Step 1: 写失败测试**

创建 `packages/mobile-capabilities/src/capabilities/audio-route.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { createAudioRoute, isAudioRouteKind } from "./audio-route";

describe("isAudioRouteKind", () => {
	it("accepts known routes", () => {
		expect(isAudioRouteKind("speaker")).toBe(true);
		expect(isAudioRouteKind("bluetooth")).toBe(true);
	});

	it("rejects unknown values", () => {
		expect(isAudioRouteKind("airplay")).toBe(false);
		expect(isAudioRouteKind(undefined)).toBe(false);
	});
});

describe("createAudioRoute in web fallback", () => {
	it("defaults to speaker", () => {
		expect(createAudioRoute().current()).toBe("speaker");
	});

	it("returns the default option list", async () => {
		const options = await createAudioRoute().list();
		expect(options.map((o) => o.kind)).toEqual(["speaker", "earpiece"]);
	});

	it("ignores set for an unsupported route", async () => {
		const route = createAudioRoute();
		await route.set("bluetooth");
		expect(route.current()).toBe("speaker");
	});

	it("never throws on set", async () => {
		await expect(createAudioRoute().set("earpiece")).resolves.toBeUndefined();
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- audio-route`
Expected: FAIL — `Failed to resolve import "./audio-route"`

- [ ] **Step 3: 实现能力**

创建 `packages/mobile-capabilities/src/capabilities/audio-route.ts`：

```ts
import { getPlugin } from "../bridge";

/**
 * 音频路由（扬声器 / 听筒 / 蓝牙 / 有线耳机）。
 *
 * 语音房间默认外放，但插上蓝牙耳机或贴耳通话时需要切换；无原生能力时
 * 一律降级为「扬声器」，调用方可无脑调用。
 */

export type AudioRouteKind = "speaker" | "earpiece" | "bluetooth" | "headset";

const ROUTE_KINDS: readonly AudioRouteKind[] = [
	"speaker",
	"earpiece",
	"bluetooth",
	"headset",
];

export interface AudioRouteOption {
	kind: AudioRouteKind;
	label: string;
}

interface AudioRoutePlugin {
	list(): Promise<{ routes: { kind: string; label: string }[] }>;
	current(): Promise<{ kind: string }>;
	set(options: { kind: string }): Promise<void>;
}

export interface AudioRoute {
	list(): Promise<AudioRouteOption[]>;
	current(): AudioRouteKind;
	set(kind: AudioRouteKind): Promise<void>;
}

export function isAudioRouteKind(value: unknown): value is AudioRouteKind {
	return (
		typeof value === "string" &&
		(ROUTE_KINDS as readonly string[]).includes(value)
	);
}

const FALLBACK_ROUTES: AudioRouteOption[] = [
	{ kind: "speaker", label: "扬声器" },
	{ kind: "earpiece", label: "听筒" },
];

export function createAudioRoute(): AudioRoute {
	let current: AudioRouteKind = "speaker";

	return {
		async list() {
			if (!__GOSPEAK_MOBILE__) return FALLBACK_ROUTES;
			try {
				const plugin = await getPlugin<AudioRoutePlugin>("AudioSession");
				const result = await plugin.list();
				const routes = result.routes
					.filter((r) => isAudioRouteKind(r.kind))
					.map((r) => ({ kind: r.kind as AudioRouteKind, label: r.label }));
				return routes.length > 0 ? routes : FALLBACK_ROUTES;
			} catch (error) {
				console.warn("[gospeak/mobile] AudioSession.list failed", error);
				return FALLBACK_ROUTES;
			}
		},
		current() {
			return current;
		},
		async set(kind) {
			if (!isAudioRouteKind(kind)) return;
			if (!__GOSPEAK_MOBILE__) return;
			try {
				const plugin = await getPlugin<AudioRoutePlugin>("AudioSession");
				await plugin.set({ kind });
				current = kind;
			} catch (error) {
				console.warn("[gospeak/mobile] AudioSession.set failed", error);
			}
		},
	};
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- audio-route`
Expected: PASS（6 tests）

- [ ] **Step 5: 扩展 iOS 插件**

在 `AudioSessionPlugin.swift` 的 `pluginMethods` 数组追加三项：

```swift
        CAPPluginMethod(name: "list", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "current", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "set", returnType: CAPPluginReturnPromise),
```

并在类内追加方法：

```swift
    @objc func list(_ call: CAPPluginCall) {
        var routes: [[String: String]] = [
            ["kind": "speaker", "label": "扬声器"],
            ["kind": "earpiece", "label": "听筒"],
        ]
        let available = AVAudioSession.sharedInstance().availableInputs ?? []
        for input in available {
            switch input.portType {
            case .bluetoothHFP, .bluetoothLE:
                routes.append(["kind": "bluetooth", "label": "蓝牙耳机"])
            case .headsetMic, .headphones:
                routes.append(["kind": "headset", "label": "有线耳机"])
            default:
                break
            }
        }
        call.resolve(["routes": routes])
    }

    @objc func current(_ call: CAPPluginCall) {
        let outputs = AVAudioSession.sharedInstance().currentRoute.outputs
        let isBluetooth = outputs.contains {
            $0.portType == .bluetoothHFP || $0.portType == .bluetoothLE
        }
        let isHeadset = outputs.contains {
            $0.portType == .headphones || $0.portType == .headsetMic
        }
        let kind = isBluetooth ? "bluetooth" : (isHeadset ? "headset" : "speaker")
        call.resolve(["kind": kind])
    }

    @objc func set(_ call: CAPPluginCall) {
        let kind = call.getString("kind") ?? "speaker"
        do {
            let session = AVAudioSession.sharedInstance()
            if kind == "speaker" {
                try session.overrideOutputAudioPort(.speaker)
            } else if kind == "earpiece" {
                try session.overrideOutputAudioPort(.none)
            }
            call.resolve()
        } catch {
            call.reject("failed to set audio route: \(error.localizedDescription)")
        }
    }
```

在 `AudioSessionPlugin.m` 的 `CAP_PLUGIN` 块内追加：

```objc
           CAP_PLUGIN_METHOD(list, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(current, CAPPluginReturnPromise);
           CAP_PLUGIN_METHOD(set, CAPPluginReturnPromise);
```

- [ ] **Step 6: 扩展 Android 插件**

在 `AudioSessionPlugin.kt` 类内追加：

```kotlin
    @PluginMethod
    fun list(call: PluginCall) {
        val routes = JSArray()
        routes.put(JSObject().put("kind", "speaker").put("label", "扬声器"))
        routes.put(JSObject().put("kind", "earpiece").put("label", "听筒"))
        val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
        if (audio.isBluetoothScoAvailableOffCall) {
            routes.put(JSObject().put("kind", "bluetooth").put("label", "蓝牙耳机"))
        }
        if (audio.isWiredHeadsetOn) {
            routes.put(JSObject().put("kind", "headset").put("label", "有线耳机"))
        }
        call.resolve(JSObject().put("routes", routes))
    }

    @PluginMethod
    fun current(call: PluginCall) {
        val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
        val kind = when {
            audio.isBluetoothScoOn -> "bluetooth"
            audio.isWiredHeadsetOn -> "headset"
            audio.isSpeakerphoneOn -> "speaker"
            else -> "earpiece"
        }
        call.resolve(JSObject().put("kind", kind))
    }

    @PluginMethod
    fun set(call: PluginCall) {
        val kind = call.getString("kind") ?: "speaker"
        val audio = context.getSystemService(Context.AUDIO_SERVICE) as AudioManager
        try {
            when (kind) {
                "speaker" -> {
                    audio.stopBluetoothSco()
                    audio.isBluetoothScoOn = false
                    audio.isSpeakerphoneOn = true
                }
                "earpiece" -> {
                    audio.stopBluetoothSco()
                    audio.isBluetoothScoOn = false
                    audio.isSpeakerphoneOn = false
                }
                "bluetooth" -> {
                    audio.startBluetoothSco()
                    audio.isBluetoothScoOn = true
                    audio.isSpeakerphoneOn = false
                }
                else -> Unit
            }
            call.resolve()
        } catch (error: Exception) {
            call.reject("failed to set audio route: ${error.message}")
        }
    }
```

并在文件顶部 import 区加：

```kotlin
import com.getcapacitor.JSArray
import com.getcapacitor.JSObject
```

- [ ] **Step 7: 验证编译**

Run:
```bash
pnpm --filter @gospeak/mobile sync
cd app/mobile/android && ./gradlew assembleDebug
```
Expected: 编译成功

- [ ] **Step 8: 提交**

```bash
git add packages/mobile-capabilities/src/capabilities/audio-route.ts packages/mobile-capabilities/src/capabilities/audio-route.test.ts app/mobile/ios app/mobile/android
git commit -m "feat(mobile): add audio route capability"
```

---

## Task 8: 锁屏与通知栏控制

**Files:**
- Create: `packages/mobile-capabilities/src/capabilities/media-control.ts`
- Create: `app/mobile/android/app/src/main/java/dev/gospeak/mobile/MediaControlPlugin.kt`
- Test: `packages/mobile-capabilities/src/capabilities/media-control.test.ts`

**Interfaces:**
- Produces: `createMediaControl(): MediaControl`，`MediaControl = { setMetadata(input: MediaMetadata): Promise<void>; clear(): Promise<void>; onAction(handler: (action: MediaAction) => void): () => void }`，`MediaAction = "mute" | "unmute" | "hangup"`

- [ ] **Step 1: 写失败测试**

创建 `packages/mobile-capabilities/src/capabilities/media-control.test.ts`：

```ts
import { describe, expect, it, vi } from "vitest";
import { createMediaControl, isMediaAction } from "./media-control";

describe("isMediaAction", () => {
	it("accepts known actions", () => {
		expect(isMediaAction("mute")).toBe(true);
		expect(isMediaAction("hangup")).toBe(true);
	});

	it("rejects unknown actions", () => {
		expect(isMediaAction("skip")).toBe(false);
		expect(isMediaAction(null)).toBe(false);
	});
});

describe("createMediaControl in web fallback", () => {
	it("resolves setMetadata without native runtime", async () => {
		await expect(
			createMediaControl().setMetadata({ title: "房间", subtitle: "3 人" }),
		).resolves.toBeUndefined();
	});

	it("resolves clear", async () => {
		await expect(createMediaControl().clear()).resolves.toBeUndefined();
	});

	it("returns an unsubscribe function", () => {
		const off = createMediaControl().onAction(() => {});
		expect(typeof off).toBe("function");
		expect(() => off()).not.toThrow();
	});

	it("does not invoke handlers in fallback", () => {
		const handler = vi.fn();
		createMediaControl().onAction(handler);
		expect(handler).not.toHaveBeenCalled();
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- media-control`
Expected: FAIL — `Failed to resolve import "./media-control"`

- [ ] **Step 3: 实现能力**

创建 `packages/mobile-capabilities/src/capabilities/media-control.ts`：

```ts
import { getPlugin, listenPlugin, pluginMethod } from "../bridge";

/**
 * 锁屏 / 通知栏媒体控制。
 *
 * Android WebView 不支持 Media Session Web API，必须走原生插件；iOS 侧同样由
 * 插件接管，保证两端行为一致。锁屏上的 静音 / 挂断 通过事件回传 web 层，
 * 由语音会话模块执行真正的动作。
 */

export type MediaAction = "mute" | "unmute" | "hangup";

const MEDIA_ACTIONS: readonly MediaAction[] = ["mute", "unmute", "hangup"];

export interface MediaMetadata {
	title: string;
	subtitle?: string;
	/** 当前是否静音，用于切换锁屏按钮文案。 */
	muted?: boolean;
}

interface MediaControlPlugin {
	setMetadata(options: MediaMetadata): Promise<void>;
	clear(): Promise<void>;
}

export interface MediaControl {
	setMetadata(input: MediaMetadata): Promise<void>;
	clear(): Promise<void>;
	onAction(handler: (action: MediaAction) => void): () => void;
}

export function isMediaAction(value: unknown): value is MediaAction {
	return (
		typeof value === "string" &&
		(MEDIA_ACTIONS as readonly string[]).includes(value)
	);
}

export function createMediaControl(): MediaControl {
	const handlers = new Set<(action: MediaAction) => void>();
	let listening = false;

	const ensureListener = async () => {
		if (listening || !__GOSPEAK_MOBILE__) return;
		listening = true;
		try {
			await listenPlugin<{ action?: string }>(
				"MediaControl",
				"action",
				(payload) => {
					if (!isMediaAction(payload?.action)) return;
					for (const handler of handlers) handler(payload.action);
				},
			);
		} catch (error) {
			listening = false;
			console.warn(
				`[gospeak/mobile] ${pluginMethod("MediaControl", "action")} listener failed`,
				error,
			);
		}
	};

	return {
		async setMetadata(input) {
			if (!__GOSPEAK_MOBILE__) return;
			try {
				const plugin = await getPlugin<MediaControlPlugin>("MediaControl");
				await plugin.setMetadata(input);
			} catch (error) {
				console.warn("[gospeak/mobile] MediaControl.setMetadata failed", error);
			}
		},
		async clear() {
			if (!__GOSPEAK_MOBILE__) return;
			try {
				const plugin = await getPlugin<MediaControlPlugin>("MediaControl");
				await plugin.clear();
			} catch (error) {
				console.warn("[gospeak/mobile] MediaControl.clear failed", error);
			}
		},
		onAction(handler) {
			handlers.add(handler);
			void ensureListener();
			return () => {
				handlers.delete(handler);
			};
		},
	};
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- media-control`
Expected: PASS（6 tests）

- [ ] **Step 5: 写 Android 原生插件**

创建 `app/mobile/android/app/src/main/java/dev/gospeak/mobile/MediaControlPlugin.kt`：

```kotlin
package dev.gospeak.mobile

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import androidx.core.app.NotificationCompat
import com.getcapacitor.JSObject
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin

@CapacitorPlugin(name = "MediaControl")
class MediaControlPlugin : Plugin() {

    private val channelId = "gospeak_voice"

    @PluginMethod
    fun setMetadata(call: PluginCall) {
        val title = call.getString("title") ?: "GOSpeak"
        val subtitle = call.getString("subtitle") ?: ""
        ensureChannel()
        val notification = NotificationCompat.Builder(context, channelId)
            .setContentTitle(title)
            .setContentText(subtitle)
            .setSmallIcon(android.R.drawable.ic_btn_speak_now)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
        notificationManager().notify(1001, notification)
        call.resolve()
    }

    @PluginMethod
    fun clear(call: PluginCall) {
        notificationManager().cancel(1001)
        call.resolve()
    }

    @PluginMethod
    fun emitAction(call: PluginCall) {
        val action = call.getString("action") ?: ""
        notifyListeners("action", JSObject().put("action", action))
        call.resolve()
    }

    private fun notificationManager(): NotificationManager =
        context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = notificationManager()
        if (manager.getNotificationChannel(channelId) != null) return
        manager.createNotificationChannel(
            NotificationChannel(channelId, "语音通话", NotificationManager.IMPORTANCE_LOW)
        )
    }
}
```

- [ ] **Step 6: 注册 Android 插件**

在 `MainActivity.kt` 的 `onCreate` 中，与 Task 6 的注册并列：

```kotlin
        registerPlugin(MediaControlPlugin::class.java)
```

- [ ] **Step 7: 验证编译**

Run:
```bash
pnpm --filter @gospeak/mobile sync
cd app/mobile/android && ./gradlew assembleDebug
```
Expected: 编译成功

- [ ] **Step 8: 提交**

```bash
git add packages/mobile-capabilities/src/capabilities/media-control.ts packages/mobile-capabilities/src/capabilities/media-control.test.ts app/mobile/android
git commit -m "feat(mobile): add lock screen media control capability"
```

---

## Task 9: 推送通知能力

**Files:**
- Create: `packages/mobile-capabilities/src/capabilities/push.ts`
- Test: `packages/mobile-capabilities/src/capabilities/push.test.ts`

**Interfaces:**
- Produces: `createPush(): Push`，`Push = { register(): Promise<string | null>; onToken(handler: (token: string) => void): () => void; onNotificationOpened(handler: (data: Record<string, unknown>) => void): () => void }`

- [ ] **Step 1: 写失败测试**

创建 `packages/mobile-capabilities/src/capabilities/push.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { createPush, normalizePushData } from "./push";

describe("normalizePushData", () => {
	it("keeps string values", () => {
		expect(normalizePushData({ path: "/invite/d/abc" })).toEqual({
			path: "/invite/d/abc",
		});
	});

	it("returns an empty object for non-objects", () => {
		expect(normalizePushData(null)).toEqual({});
		expect(normalizePushData("x")).toEqual({});
	});
});

describe("createPush in web fallback", () => {
	it("resolves register to null without a native runtime", async () => {
		await expect(createPush().register()).resolves.toBeNull();
	});

	it("returns unsubscribe functions", () => {
		const push = createPush();
		expect(typeof push.onToken(() => {})).toBe("function");
		expect(typeof push.onNotificationOpened(() => {})).toBe("function");
	});

	it("does not throw when unsubscribing twice", () => {
		const push = createPush();
		const off = push.onToken(() => {});
		off();
		expect(() => off()).not.toThrow();
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- push`
Expected: FAIL — `Failed to resolve import "./push"`

- [ ] **Step 3: 实现能力**

创建 `packages/mobile-capabilities/src/capabilities/push.ts`：

```ts
import { getPlugin, listenPlugin } from "../bridge";

/**
 * 推送通知（APNs / FCM）。
 *
 * 只负责「拿到 device token 并交给上层上报」与「点击通知时把 payload 交回」；
 * 后端绑定关系、去重、权限策略都留在 web 层与后端，能力层不做业务。
 */

interface PushPlugin {
	requestPermissions(): Promise<{ receive: string }>;
	register(): Promise<void>;
}

export interface Push {
	register(): Promise<string | null>;
	onToken(handler: (token: string) => void): () => void;
	onNotificationOpened(
		handler: (data: Record<string, unknown>) => void,
	): () => void;
}

export function normalizePushData(value: unknown): Record<string, unknown> {
	if (typeof value !== "object" || value === null) return {};
	return { ...(value as Record<string, unknown>) };
}

export function createPush(): Push {
	const tokenHandlers = new Set<(token: string) => void>();
	const openHandlers = new Set<(data: Record<string, unknown>) => void>();
	let started = false;
	let cachedToken: string | null = null;

	const start = async () => {
		if (started || !__GOSPEAK_MOBILE__) return;
		started = true;
		try {
			const plugin = await getPlugin<PushPlugin>("PushNotifications");
			await plugin.requestPermissions();
			await listenPlugin<{ value?: string }>(
				"PushNotifications",
				"registration",
				(payload) => {
					const token = typeof payload?.value === "string" ? payload.value : null;
					if (!token) return;
					cachedToken = token;
					for (const handler of tokenHandlers) handler(token);
				},
			);
			await listenPlugin<{ notification?: { data?: unknown } }>(
				"PushNotifications",
				"pushNotificationActionPerformed",
				(payload) => {
					const data = normalizePushData(payload?.notification?.data);
					for (const handler of openHandlers) handler(data);
				},
			);
			await plugin.register();
		} catch (error) {
			started = false;
			console.warn("[gospeak/mobile] push registration failed", error);
		}
	};

	return {
		async register() {
			await start();
			return cachedToken;
		},
		onToken(handler) {
			tokenHandlers.add(handler);
			if (cachedToken) handler(cachedToken);
			void start();
			return () => {
				tokenHandlers.delete(handler);
			};
		},
		onNotificationOpened(handler) {
			openHandlers.add(handler);
			void start();
			return () => {
				openHandlers.delete(handler);
			};
		},
	};
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- push`
Expected: PASS（5 tests）

- [ ] **Step 5: 提交**

```bash
git add packages/mobile-capabilities/src/capabilities/push.ts packages/mobile-capabilities/src/capabilities/push.test.ts
git commit -m "feat(mobile): add push notification capability"
```

---

## Task 10: 深链与生命周期能力

**Files:**
- Create: `packages/mobile-capabilities/src/capabilities/deeplink.ts`
- Create: `packages/mobile-capabilities/src/capabilities/lifecycle.ts`
- Test: `packages/mobile-capabilities/src/capabilities/deeplink.test.ts`
- Test: `packages/mobile-capabilities/src/capabilities/lifecycle.test.ts`

**Interfaces:**
- Produces: `normalizeDeepLink(url: string): string | null`、`createDeepLink(): DeepLink`、`createLifecycle(): Lifecycle`，`Lifecycle = { onStateChange(handler: (state: AppState) => void): () => void }`，`AppState = "active" | "background"`

- [ ] **Step 1: 写深链失败测试**

创建 `packages/mobile-capabilities/src/capabilities/deeplink.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { createDeepLink, normalizeDeepLink } from "./deeplink";

describe("normalizeDeepLink", () => {
	it("converts the invite scheme to an in-app path", () => {
		expect(normalizeDeepLink("gospeak://invite/d/ABC123")).toBe(
			"/invite/d/ABC123",
		);
	});

	it("keeps an existing in-app path", () => {
		expect(normalizeDeepLink("/invite/d/ABC123")).toBe("/invite/d/ABC123");
	});

	it("rejects foreign schemes", () => {
		expect(normalizeDeepLink("https://evil.example/invite/d/x")).toBeNull();
	});

	it("rejects non-string input", () => {
		expect(normalizeDeepLink(undefined as unknown as string)).toBeNull();
	});
});

describe("createDeepLink in web fallback", () => {
	it("returns an unsubscribe function", () => {
		const off = createDeepLink().onLink(() => {});
		expect(typeof off).toBe("function");
	});

	it("resolves the initial link to null", async () => {
		await expect(createDeepLink().initial()).resolves.toBeNull();
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- deeplink`
Expected: FAIL — `Failed to resolve import "./deeplink"`

- [ ] **Step 3: 实现深链能力**

创建 `packages/mobile-capabilities/src/capabilities/deeplink.ts`：

```ts
import { getPlugin, listenPlugin } from "../bridge";

/**
 * 深链 gospeak://invite/d/<CODE>。
 *
 * 与桌面端 @gospeak/desktop 的 deeplink 能力语义一致：宿主把外部链接归一化成
 * 应用内相对路径，页面只处理以 "/" 开头的路径，避免任意 URL 把页面带走。
 */

const SCHEME_PREFIX = "gospeak://";

export function normalizeDeepLink(url: string): string | null {
	if (typeof url !== "string" || url.length === 0) return null;
	if (url.startsWith("/")) return url;
	if (!url.startsWith(SCHEME_PREFIX)) return null;
	const path = url.slice(SCHEME_PREFIX.length);
	return path.length > 0 ? `/${path.replace(/^\/+/, "")}` : null;
}

interface AppPlugin {
	getLaunchUrl(): Promise<{ url?: string } | null>;
}

export interface DeepLink {
	initial(): Promise<string | null>;
	onLink(handler: (path: string) => void): () => void;
}

export function createDeepLink(): DeepLink {
	const handlers = new Set<(path: string) => void>();
	let started = false;
	let initialUrl: string | null = null;

	const start = async () => {
		if (started || !__GOSPEAK_MOBILE__) return;
		started = true;
		try {
			await listenPlugin<{ url?: string }>("App", "appUrlOpen", (payload) => {
				const path = normalizeDeepLink(payload?.url ?? "");
				if (!path) return;
				for (const handler of handlers) handler(path);
			});
		} catch (error) {
			started = false;
			console.warn("[gospeak/mobile] deeplink listener failed", error);
		}
	};

	return {
		async initial() {
			if (!__GOSPEAK_MOBILE__) return null;
			if (initialUrl) return initialUrl;
			try {
				const plugin = await getPlugin<AppPlugin>("App");
				const result = await plugin.getLaunchUrl();
				initialUrl = normalizeDeepLink(result?.url ?? "");
				return initialUrl;
			} catch (error) {
				console.warn("[gospeak/mobile] getLaunchUrl failed", error);
				return null;
			}
		},
		onLink(handler) {
			handlers.add(handler);
			void start();
			return () => {
				handlers.delete(handler);
			};
		},
	};
}
```

- [ ] **Step 4: 写生命周期失败测试**

创建 `packages/mobile-capabilities/src/capabilities/lifecycle.test.ts`：

```ts
import { describe, expect, it, vi } from "vitest";
import { createLifecycle, isAppState } from "./lifecycle";

describe("isAppState", () => {
	it("accepts active and background", () => {
		expect(isAppState("active")).toBe(true);
		expect(isAppState("background")).toBe(true);
	});

	it("rejects unknown states", () => {
		expect(isAppState("suspended")).toBe(false);
		expect(isAppState(1)).toBe(false);
	});
});

describe("createLifecycle in web fallback", () => {
	it("returns an unsubscribe function", () => {
		const off = createLifecycle().onStateChange(() => {});
		expect(typeof off).toBe("function");
	});

	it("does not invoke handlers in fallback", () => {
		const handler = vi.fn();
		createLifecycle().onStateChange(handler);
		expect(handler).not.toHaveBeenCalled();
	});

	it("allows double unsubscribe", () => {
		const off = createLifecycle().onStateChange(() => {});
		off();
		expect(() => off()).not.toThrow();
	});
});
```

- [ ] **Step 5: 实现生命周期能力**

创建 `packages/mobile-capabilities/src/capabilities/lifecycle.ts`：

```ts
import { listenPlugin } from "../bridge";

/**
 * 应用前后台切换。
 *
 * 语音场景的关键钩子：回到前台时上层要做 socket 重连与媒体健康检查，
 * 切后台时要保证音频会话仍然活跃（否则锁屏会静音）。
 */

export type AppState = "active" | "background";

const APP_STATES: readonly AppState[] = ["active", "background"];

export function isAppState(value: unknown): value is AppState {
	return (
		typeof value === "string" && (APP_STATES as readonly string[]).includes(value)
	);
}

export interface Lifecycle {
	onStateChange(handler: (state: AppState) => void): () => void;
}

export function createLifecycle(): Lifecycle {
	const handlers = new Set<(state: AppState) => void>();
	let started = false;

	const start = async () => {
		if (started || !__GOSPEAK_MOBILE__) return;
		started = true;
		try {
			await listenPlugin<{ isActive?: boolean }>(
				"App",
				"appStateChange",
				(payload) => {
					const state: AppState = payload?.isActive ? "active" : "background";
					for (const handler of handlers) handler(state);
				},
			);
		} catch (error) {
			started = false;
			console.warn("[gospeak/mobile] appStateChange listener failed", error);
		}
	};

	return {
		onStateChange(handler) {
			handlers.add(handler);
			void start();
			return () => {
				handlers.delete(handler);
			};
		},
	};
}
```

- [ ] **Step 6: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test`
Expected: PASS（全部用例）

- [ ] **Step 7: 提交**

```bash
git add packages/mobile-capabilities/src/capabilities/deeplink.ts packages/mobile-capabilities/src/capabilities/deeplink.test.ts packages/mobile-capabilities/src/capabilities/lifecycle.ts packages/mobile-capabilities/src/capabilities/lifecycle.test.ts
git commit -m "feat(mobile): add deeplink and lifecycle capabilities"
```

---

## Task 11: 能力聚合与 `@gospeak/web` 接入

**Files:**
- Create: `packages/mobile-capabilities/src/index.ts`
- Modify: `app/web/package.json`（加依赖）
- Modify: `app/web/src/hooks/useDeepLinkNavigation.ts`
- Test: `packages/mobile-capabilities/src/index.test.ts`

**Interfaces:**
- Consumes: Task 6-10 的全部 `create*` 工厂
- Produces: `createMobile()` 聚合对象；`useDeepLinkNavigation` 同时支持桌面与移动深链

- [ ] **Step 1: 写失败测试**

创建 `packages/mobile-capabilities/src/index.test.ts`：

```ts
import { describe, expect, it } from "vitest";
import { createMobile } from "./index";

describe("createMobile", () => {
	it("exposes every capability", () => {
		const mobile = createMobile();
		expect(Object.keys(mobile).sort()).toEqual([
			"audioRoute",
			"audioSession",
			"deepLink",
			"lifecycle",
			"mediaControl",
			"push",
			"runtime",
		]);
	});

	it("reports the runtime kind", () => {
		expect(["web", "desktop", "mobile"]).toContain(createMobile().runtime);
	});
});
```

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm --filter @gospeak/mobile-capabilities test -- index`
Expected: FAIL — `Failed to resolve import "./index"`

- [ ] **Step 3: 实现聚合入口**

创建 `packages/mobile-capabilities/src/index.ts`：

```ts
/**
 * @gospeak/mobile-capabilities
 *
 * 移动端能力 SDK：@gospeak/web 与原生宿主（app/mobile，Capacitor）之间的桥接。
 * 每个能力都有原生实现与 Web 安全降级，同一份组件代码在浏览器与原生壳里都能跑。
 *
 * 业务代码判定平台用 inMobile()；需要整体裁剪的模块用 __GOSPEAK_MOBILE__。
 */

export {
	BUILD_TARGET,
	DESKTOP_BUILD,
	MOBILE_BUILD,
	WEB_BUILD,
	assertMobile,
	ifMobile,
	importMobile,
	inMobile,
	isCapacitor,
} from "./conditional";
export type { BuildTarget } from "./conditional";

export { getPlugin, listenPlugin, pluginMethod } from "./bridge";

export { createAudioSession } from "./capabilities/audio-session";
export type { AudioSession } from "./capabilities/audio-session";

export { createAudioRoute, isAudioRouteKind } from "./capabilities/audio-route";
export type {
	AudioRoute,
	AudioRouteKind,
	AudioRouteOption,
} from "./capabilities/audio-route";

export { createMediaControl, isMediaAction } from "./capabilities/media-control";
export type {
	MediaAction,
	MediaControl,
	MediaMetadata,
} from "./capabilities/media-control";

export { createPush, normalizePushData } from "./capabilities/push";
export type { Push } from "./capabilities/push";

export { createDeepLink, normalizeDeepLink } from "./capabilities/deeplink";
export type { DeepLink } from "./capabilities/deeplink";

export { createLifecycle, isAppState } from "./capabilities/lifecycle";
export type { AppState, Lifecycle } from "./capabilities/lifecycle";

import { BUILD_TARGET } from "./conditional";
import { createAudioSession } from "./capabilities/audio-session";
import { createAudioRoute } from "./capabilities/audio-route";
import { createMediaControl } from "./capabilities/media-control";
import { createPush } from "./capabilities/push";
import { createDeepLink } from "./capabilities/deeplink";
import { createLifecycle } from "./capabilities/lifecycle";

export function createMobile() {
	return {
		runtime: BUILD_TARGET,
		audioSession: createAudioSession(),
		audioRoute: createAudioRoute(),
		mediaControl: createMediaControl(),
		push: createPush(),
		deepLink: createDeepLink(),
		lifecycle: createLifecycle(),
	};
}

export type Mobile = ReturnType<typeof createMobile>;
```

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm --filter @gospeak/mobile-capabilities test`
Expected: PASS

- [ ] **Step 5: 给 web 加依赖**

在 `app/web/package.json` 的 `dependencies` 中，`@gospeak/desktop` 之后加：

```json
    "@gospeak/mobile-capabilities": "workspace:*",
```

Run: `pnpm install`
Expected: 成功

- [ ] **Step 6: 让深链钩子同时支持移动端**

把 `app/web/src/hooks/useDeepLinkNavigation.ts` 全文替换为：

```ts
import { getInitialDeepLink, onDeepLink } from "@gospeak/desktop";
import { createDeepLink } from "@gospeak/mobile-capabilities";
import { useRouter } from "@tanstack/react-router";
import { useEffect } from "react";

/**
 * 把宿主收到的深链（gospeak://invite/d/CODE，宿主已归一化为 /invite/d/CODE）
 * 变成应用内跳转。桌面走 MyGo 桥接，移动端走 Capacitor App 插件。
 *
 * 冷启动时链接早于页面订阅，所以先订阅再取 Initial —— 顺序反了会丢掉启动链接。
 * 只处理应用内相对路径，避免宿主传来的任意 URL 把页面带走。
 */
export function useDeepLinkNavigation() {
	const router = useRouter();

	useEffect(() => {
		const go = (target: unknown) => {
			if (typeof target !== "string" || !target.startsWith("/")) return;
			const current = `${window.location.pathname}${window.location.search}`;
			if (current === target) return;
			void router.navigate({ to: target });
		};

		const unsubscribers: (() => void)[] = [];

		if (__GOSPEAK_DESKTOP__) {
			unsubscribers.push(onDeepLink(go));
			void getInitialDeepLink()
				.then((initial) => {
					if (initial) go(initial);
				})
				.catch(() => {});
		}

		if (__GOSPEAK_MOBILE__) {
			const deepLink = createDeepLink();
			unsubscribers.push(deepLink.onLink(go));
			void deepLink
				.initial()
				.then((initial) => {
					if (initial) go(initial);
				})
				.catch(() => {});
		}

		return () => {
			for (const off of unsubscribers) off();
		};
	}, [router]);
}
```

- [ ] **Step 7: 验证三种构建**

Run:
```bash
pnpm --filter @gospeak/web typecheck
pnpm --filter @gospeak/web build
pnpm --filter @gospeak/web build:desktop
pnpm --filter @gospeak/web build:mobile
```
Expected: 四条全部成功

- [ ] **Step 8: 提交**

```bash
git add packages/mobile-capabilities/src/index.ts packages/mobile-capabilities/src/index.test.ts app/web/package.json app/web/src/hooks/useDeepLinkNavigation.ts pnpm-lock.yaml
git commit -m "feat(mobile): aggregate capabilities and wire deeplink into web client"
```

---

## Task 12: 桥接契约门禁与 CI / 文档

**Files:**
- Create: `scripts/check-mobile-bridge.mjs`
- Create: `scripts/check-mobile-bridge.test.mjs`
- Modify: `.github/workflows/ci.yml:91`
- Modify: `lefthook.yml:22`
- Modify: `biome.json:14-20`
- Create: `packages/mobile-capabilities/AGENTS.md`
- Create: `app/mobile/AGENTS.md`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 2 的 `pluginMethod()` 调用点、Task 6-8 的原生插件方法名

- [ ] **Step 1: 写门禁脚本单测**

创建 `scripts/check-mobile-bridge.test.mjs`：

```js
import assert from "node:assert/strict";
import { test } from "node:test";
import {
	checkMethods,
	extractKotlinMethods,
	extractSwiftMethods,
	extractTsMethods,
} from "./check-mobile-bridge.mjs";

test("extractTsMethods collects pluginMethod calls", () => {
	const source = 'pluginMethod("AudioSession", "activate");';
	assert.deepEqual([...extractTsMethods([{ source }])], ["AudioSession.activate"]);
});

test("extractSwiftMethods collects jsName + CAPPluginMethod pairs", () => {
	const source = [
		'public let jsName = "AudioSession"',
		'CAPPluginMethod(name: "activate", returnType: CAPPluginReturnPromise),',
	].join("\n");
	assert.deepEqual([...extractSwiftMethods([{ source }])], [
		"AudioSession.activate",
	]);
});

test("extractKotlinMethods collects plugin name + @PluginMethod fun", () => {
	const source = [
		'@CapacitorPlugin(name = "MediaControl")',
		"@PluginMethod",
		"fun setMetadata(call: PluginCall) {}",
	].join("\n");
	assert.deepEqual([...extractKotlinMethods([{ source }])], [
		"MediaControl.setMetadata",
	]);
});

test("checkMethods reports methods missing on the native side", () => {
	const result = checkMethods({
		ts: new Set(["AudioSession.activate", "AudioSession.deactivate"]),
		native: new Set(["AudioSession.activate"]),
	});
	assert.deepEqual(result.missing, ["AudioSession.deactivate"]);
	assert.equal(result.ok, false);
});

test("checkMethods passes when native is a superset", () => {
	const result = checkMethods({
		ts: new Set(["AudioSession.activate"]),
		native: new Set(["AudioSession.activate", "AudioSession.list"]),
	});
	assert.equal(result.ok, true);
});
```

- [ ] **Step 2: 运行单测确认失败**

Run: `node --test scripts/check-mobile-bridge.test.mjs`
Expected: FAIL — 找不到模块 `./check-mobile-bridge.mjs`

- [ ] **Step 3: 实现门禁脚本**

创建 `scripts/check-mobile-bridge.mjs`：

```js
#!/usr/bin/env node
// 校验移动桥接契约：TS 侧 pluginMethod("Plugin", "method") 必须与原生侧
// 插件名 + 方法名逐字一致（iOS jsName/CAPPluginMethod、Android @CapacitorPlugin/@PluginMethod）。
// 拼错时运行期只会得到 "not implemented"，多数能力会静默吞掉，因此在 CI 拦截。
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const TS_DIR = "packages/mobile-capabilities/src/capabilities";
const IOS_DIR = "app/mobile/ios/App/App";
const ANDROID_DIR = "app/mobile/android/app/src/main/java/dev/gospeak/mobile";

export function extractTsMethods(files) {
	const methods = new Set();
	for (const file of files) {
		for (const m of file.source.matchAll(
			/pluginMethod\(\s*"([A-Z]\w*)"\s*,\s*"([A-Za-z]\w*)"\s*\)/g,
		)) {
			methods.add(`${m[1]}.${m[2]}`);
		}
	}
	return methods;
}

export function extractSwiftMethods(files) {
	const methods = new Set();
	for (const file of files) {
		const nameMatch = file.source.match(/jsName\s*=\s*"([A-Z]\w*)"/);
		if (!nameMatch) continue;
		const plugin = nameMatch[1];
		for (const m of file.source.matchAll(
			/CAPPluginMethod\(\s*name:\s*"([A-Za-z]\w*)"/g,
		)) {
			methods.add(`${plugin}.${m[1]}`);
		}
	}
	return methods;
}

export function extractKotlinMethods(files) {
	const methods = new Set();
	for (const file of files) {
		const nameMatch = file.source.match(
			/@CapacitorPlugin\(\s*name\s*=\s*"([A-Z]\w*)"/,
		);
		if (!nameMatch) continue;
		const plugin = nameMatch[1];
		for (const m of file.source.matchAll(
			/@PluginMethod\s*(?:\r?\n\s*)*fun\s+([A-Za-z]\w*)\s*\(/g,
		)) {
			methods.add(`${plugin}.${m[1]}`);
		}
	}
	return methods;
}

export function checkMethods({ ts, native }) {
	const missing = [...ts].filter((m) => !native.has(m)).sort();
	const unused = [...native].filter((m) => !ts.has(m)).sort();
	return { missing, unused, ok: missing.length === 0 };
}

function readDir(dir, extensions) {
	try {
		return readdirSync(dir)
			.filter((f) => extensions.some((ext) => f.endsWith(ext)))
			.map((f) => ({
				path: join(dir, f),
				source: readFileSync(join(dir, f), "utf8"),
			}));
	} catch {
		return [];
	}
}

export function checkRepo(root = ".") {
	const ts = extractTsMethods(readDir(join(root, TS_DIR), [".ts"]));
	const native = new Set([
		...extractSwiftMethods(readDir(join(root, IOS_DIR), [".swift"])),
		...extractKotlinMethods(readDir(join(root, ANDROID_DIR), [".kt"])),
	]);
	return { ts, native, ...checkMethods({ ts, native }) };
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
	const { missing, unused } = checkRepo(".");
	for (const m of unused) {
		console.warn(`::warning::原生实现了但 TS 未调用: ${m}`);
	}
	for (const m of missing) {
		console.error(`::error::TS 调用了原生未实现的方法: ${m}`);
	}
	if (missing.length > 0) process.exit(1);
	console.log("mobile bridge methods are in sync");
}
```

- [ ] **Step 4: 运行单测与全仓校验**

Run:
```bash
node --test scripts/check-mobile-bridge.test.mjs
node scripts/check-mobile-bridge.mjs
```
Expected: 5 tests PASS；全仓校验输出 `mobile bridge methods are in sync`

- [ ] **Step 5: 接入 CI 与 lefthook**

`.github/workflows/ci.yml` 第 91 行的 biome 目录加 `./app/mobile`：

```yaml
        run: pnpm exec biome ci --files-ignore-unknown=true ./app/web ./packages ./scripts ./test ./app/desktop ./app/mobile
```

在同文件「Desktop bridge contract」步骤之后加：

```yaml
      - name: Mobile bridge contract (plugin methods ↔ native)
        if: >-
          ${{ github.event_name == 'workflow_dispatch' || github.event_name == 'workflow_call' ||
              needs.changes.outputs.js == 'true' || needs.changes.outputs.ci == 'true' }}
        run: |
          node --test scripts/check-mobile-bridge.test.mjs
          node scripts/check-mobile-bridge.mjs
```

`lefthook.yml` 第 22 行同样加 `./app/mobile`，并在 pre-push 追加：

```yaml
    "mobile bridge contract":
      run: node scripts/check-mobile-bridge.mjs
```

- [ ] **Step 6: 更新 biome 扫描范围**

`biome.json` 的 `files.includes` 数组，在 `"app/desktop/*.{ts,json,jsonc}"` 之后加两项：

```json
			"packages/mobile-capabilities/**/*.{ts,tsx,json,jsonc,css}",
			"app/mobile/*.{ts,json,jsonc}",
```

- [ ] **Step 7: 写包文档**

创建 `packages/mobile-capabilities/AGENTS.md`，内容为：

```markdown
# packages/mobile-capabilities — 移动能力 SDK

@ `@gospeak/mobile-capabilities`：@gospeak/web 与原生宿主（app/mobile，Capacitor 8）之间的 TS 桥接。**每个能力都有原生实现 + Web 安全降级**，同一份组件代码在浏览器与原生壳里都能跑。模式对齐 packages/desktop。

## 结构

| 文件 | 职责 |
|------|------|
| conditional.ts | BUILD_TARGET / MOBILE_BUILD / WEB_BUILD / inMobile / assertMobile / ifMobile / importMobile |
| bridge.ts | pluginMethod / getPlugin / listenPlugin，动态加载 @capacitor/core |
| capabilities/* | audio-session / audio-route / media-control / push / deeplink / lifecycle |
| index.ts | 具名导出 + createMobile() 聚合对象 |

## 条件编译

GOSPEAK_TARGET=mobile 时 Vite 注入 __GOSPEAK_MOBILE__=true。

- 业务代码判定平台**一律用 inMobile()**；模块级裁剪写 if (__GOSPEAK_MOBILE__)。
- @capacitor/core 只在 __GOSPEAK_MOBILE__ 分支内动态 import，web/desktop 构建整段被剔除。

## 桥接契约

- TS 侧 pluginMethod("Plugin", "method") 必须与原生 @CapacitorPlugin(name=) / CAPPluginMethod(name:) / @PluginMethod fun 逐字一致。
- scripts/check-mobile-bridge.mjs 在 CI 与 pre-push 校验，写错会在运行期得到 "not implemented"。

## 约定

- 新增能力：capabilities/<name>.ts 导出 create<Name>()，内部同时实现 native 与 fallback，并在 index.ts 导出（加入 createMobile()）。
- 降级必须**无副作用且可调用**，不要抛异常让调用方崩；失败要有明确返回值或 console.warn。
```

- [ ] **Step 8: 写壳文档**

创建 `app/mobile/AGENTS.md`，内容为：

```markdown
# app/mobile — 移动宿主（Capacitor）

Capacitor 8 原生壳。**不自带前端**：加载 app/web 的 mobile 目标产物（GOSPEAK_TARGET=mobile → app/web/dist-mobile）。

## 构建链

1. pnpm --filter @gospeak/web build:mobile 产出 app/web/dist-mobile。
2. cap sync 把产物拷进 ios/、android/ 并安装原生插件。
3. dev：设 GOSPEAK_MOBILE_DEV_URL=http://<局域网IP>:3000 走真机热重载；release：加载内嵌 bundle。

```bash
pnpm --filter @gospeak/mobile sync          # 构建 + 同步两端
pnpm --filter @gospeak/mobile run:ios       # 同步后跑 iOS
pnpm --filter @gospeak/mobile run:android   # 同步后跑 Android
```

## 原生工程

- ios/App/App/Info.plist：UIBackgroundModes: audio、NSMicrophoneUsageDescription、gospeak URL scheme。**删掉任一项都会导致后台静音或深链失效**。
- android/.../AndroidManifest.xml：RECORD_AUDIO、FOREGROUND_SERVICE_MICROPHONE、POST_NOTIFICATIONS 与 gospeak intent-filter。
- 自定义插件：AudioSessionPlugin（会话 + 路由）、MediaControlPlugin（锁屏通知），须在 MainActivity.onCreate 里 registerPlugin。

## 约束

- 原生插件方法名与 @gospeak/mobile-capabilities 的 pluginMethod() 必须逐字一致，scripts/check-mobile-bridge.mjs 会拦。
- 改 app/web 后要重跑 build:mobile + cap sync，否则真机看到的是旧包。
- 后台音频成立与否由真机 PoC 结论决定，见 agent_test_logs/ 记录。
```

- [ ] **Step 9: 更新根 AGENTS.md**

在根 `AGENTS.md` 的目录骨架代码块中，`└── docs/` 之前加一行：

```
├── mobile/          Capacitor 原生壳（加载 web 的 mobile 目标产物）
```

在 `packages/` 骨架中加一行：

```
├── mobile-capabilities/ 移动能力 SDK（Capacitor 插件 + Web 降级）
```

在 §7 模块级指北表格末尾加一行：

```markdown
| `app/mobile/AGENTS.md` / `packages/mobile-capabilities/AGENTS.md` | 移动壳、移动能力 SDK |
```

- [ ] **Step 10: 全量验证**

Run:
```bash
pnpm install
pnpm typecheck
pnpm lint
node --test scripts/check-mobile-bridge.test.mjs
node scripts/check-mobile-bridge.mjs
node scripts/check-desktop-commands.mjs
pnpm --filter @gospeak/mobile-capabilities test
pnpm --filter @gospeak/web build:mobile
```
Expected: 全部通过，无新增 biome / 类型错误

- [ ] **Step 11: 提交**

```bash
git add scripts/check-mobile-bridge.mjs scripts/check-mobile-bridge.test.mjs .github/workflows/ci.yml lefthook.yml biome.json packages/mobile-capabilities/AGENTS.md app/mobile/AGENTS.md AGENTS.md
git commit -m "chore(mobile): add bridge contract guard and mobile docs"
```

---

## Self-Review

**Spec 覆盖核对**

| Spec 章节 | 对应任务 |
|---|---|
| §1 选型结论 | 全计划前提（无代码任务，已记录于 spec） |
| §2 架构与复用边界 | Task 1（能力包）、Task 4（壳包）、Task 11（web 接入） |
| §3 构建目标 | Task 3（vite/脚本/类型）、Task 4 Step 7（turbo） |
| §4.1 ①后台音频 | Task 6 |
| §4.1 ②锁屏控制 | Task 8 |
| §4.1 ③推送 | Task 9 |
| §4.1 ④深链 | Task 10 + Task 11 Step 6 |
| §4.1 ⑤音频路由 | Task 7 |
| §4.1 ⑥会话连续性 | Task 10（lifecycle）+ 复用 sfu-client 既有重连策略 |
| §4.2 能力层收敛 | Task 1、Task 2、Task 11 |
| §4.3 PoC 风险 | Task 5（阻塞门禁） |
| §6 测试/CI/门禁 | Task 12 |
| §7 文档 | Task 12 Step 7-9 |
| §8 明确不做 | Global Constraints |

**已知取舍**

- Task 10 的「会话连续性」只提供生命周期钩子；真正的前台重连由既有 @gospeak/sfu-client 重连策略 + socketStore 承担，本计划不重复实现，避免与既有策略冲突。
- MediaControlPlugin.emitAction 是给原生通知按钮回调用的桥；UI 侧按钮绑定留到后续迭代，MVP 先保证锁屏通知与事件通道存在。
- iOS 的 media-control 未单建插件（MVP 以 Android 通知栏 + iOS 系统媒体控件为主），留到后续迭代。

---

## 剩余工作（计划保留跟进的原因）

| 项 | 状态 | 说明 |
|---|---|---|
| Task 5 真机后台 WebRTC 音频 PoC | 暂缓（用户指示） | 混合方案是否成立的前提。执行方式见 Task 5；通过后按 spec 继续，不通过需回用户处重新决策（下沉原生媒体层或改路线）。 |
| 服务端推送（APNs / FCM） | 暂缓（用户指示） | 后端当前没有 token 表与上报接口，客户端 `push` 能力仅保留 token 接收。需另出后端设计后接入。 |
| 本地推送 | 已补齐 | 取代不了服务端推送：仅在 App 运行/刚切后台时有效，覆盖「被踢」「语音断连」。 |
| CI 原生编译 job | 已调整 | `build-mobile`（macOS runner 编 iOS + Android）**已移除 `continue-on-error`**：失败必须是红的（它曾把 `version:sync` 的 ENOENT 压成 success，长期掩盖真实故障）。仍**不**纳入 `gate` 的 needs——macOS 排队 7-17 分钟会抖动合并门禁。即「失败显眼，但不阻塞合并」。 |
| iOS 锁屏自定义按钮 | 未做 | 沿用系统媒体控件；Android 已有 静音 / 挂断 按钮。 |
