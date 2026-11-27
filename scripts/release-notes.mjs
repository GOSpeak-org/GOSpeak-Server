#!/usr/bin/env node
// GitHub Release 正文生成器 —— 一版一个 Release（对齐 SyncTV 的发布页形态）。
//
// 版本对应的**唯一** Release 挂在 `gospeak-server-v<version>` tag 上，服务端
// 二进制与桌面端安装包都作为附件进入同一个 Release：
//
//   1. 正文顶部是「桌面端下载表格」：用户点一下就拿到 dmg / deb / exe
//   2. 中间是桌面端安装说明与部署指引
//   3. 分隔线之后依次是桌面端、服务端的 release notes
//
// 桌面端 tag（`gospeak-desktop-v<version>`）仍然存在，但只承担构建触发与版本
// 门禁，不再单独创建 Release —— 否则 Releases 页会为同一版本出现两条同名条目。
//
// 所有产物名由 scripts/release-naming.mjs 派生，本文件不手拼文件名。
//
// 用法：
//   node scripts/release-notes.mjs release --version 1.0.0 --repo o/r \
//        --component <server|desktop|mobile> --tag gospeak-<c>-v<ver> \
//        [--notes-file component-notes.md]
//
// 输出写到 stdout。

import { readFileSync } from "node:fs";
import { pathToFileURL } from "node:url";

import {
	artifactForTarget,
	checksumName,
	releaseTag,
} from "./release-naming.mjs";

const BADGE = (label, color, logo) =>
	`![Download ${label}](https://img.shields.io/badge/Download-${label}-${color}?style=for-the-badge&logo=${logo}&logoColor=white)`;

const PLATFORM_BADGE = {
	dmg: BADGE("DMG", "000000", "apple"),
	deb: BADGE("DEB", "FCC624", "linux"),
	exe: BADGE("EXE", "0078D4", "windows11"),
};

function downloadUrl(repo, tag, asset) {
	return `https://github.com/${repo}/releases/download/${tag}/${asset}`;
}

/** 服务端 Release tag（一组件一 Release 后，各组件用自己的 tag）。 */
export function releaseTagForVersion(version, component = "server") {
	return releaseTag(component, version);
}

/**
 * 桌面端可下载产物清单。os/arch/ext 与 release.yml 的构建矩阵一一对应，
 * 改矩阵时必须同步这里（单测会校验文件名仍符合命名规范）。
 */
export function desktopArtifacts(version) {
	return [
		{
			platform: "macOS",
			device: "Apple Silicon",
			bestFor: "M 系列 Mac",
			primary: { ext: "dmg", target: "darwin-arm64" },
			alternative: null,
		},
		{
			platform: "macOS",
			device: "Intel",
			bestFor: "Intel 芯片 Mac",
			primary: { ext: "dmg", target: "darwin-amd64" },
			alternative: null,
		},
		{
			platform: "Windows",
			device: "x64",
			bestFor: "64 位 Windows 10 / 11",
			primary: { ext: "exe", target: "windows-amd64" },
			alternative: null,
		},
		{
			platform: "Linux",
			device: "x64",
			bestFor: "Intel / AMD 的 Debian、Ubuntu",
			primary: { ext: "deb", target: "linux-amd64" },
			alternative: { ext: "tar.gz", target: "linux-amd64" },
		},
		{
			platform: "Linux",
			device: "arm64",
			bestFor: "ARM64 服务器与开发板",
			primary: { ext: "deb", target: "linux-arm64" },
			alternative: { ext: "tar.gz", target: "linux-arm64" },
		},
	].map((row) => ({
		...row,
		primaryAsset: artifactForTarget(
			"desktop",
			version,
			row.primary.target,
			row.primary.ext,
		),
		alternativeAsset: row.alternative
			? artifactForTarget(
					"desktop",
					version,
					row.alternative.target,
					row.alternative.ext,
				)
			: null,
	}));
}

function renderCell(ext, repo, tag, asset) {
	const badge =
		PLATFORM_BADGE[ext] ?? BADGE(ext.toUpperCase(), "24292F", "github");
	return `[${badge}](${downloadUrl(repo, tag, asset)})<br>\`${asset}\``;
}

/** 通用下载链接（无徽章，用于 Docker 这类非文件产物）。 */
function renderTextCell(label, href) {
	return href ? `[${label}](${href})` : label;
}

function renderChecksumCell(repo, tag, sums) {
	return `[${BADGE("SHA-256", "24292F", "github")}](${downloadUrl(repo, tag, sums)})<br>\`${sums}\``;
}

/**
 * 桌面端下载表格（Markdown）。
 *
 * `tag` 是桌面端自己的 Release tag（gospeak-desktop-v<ver>）；安装包与校验和
 * 都在该 Release 的 Assets 里。未传时回退到按版本派生的桌面端 tag。
 */
export function renderDesktopTable(version, repo, tag) {
	const releaseAnchor = tag ?? releaseTagForVersion(version, "desktop");
	const rows = desktopArtifacts(version).map((row) => {
		const recommended = renderCell(
			row.primary.ext,
			repo,
			releaseAnchor,
			row.primaryAsset,
		);
		const alternative = row.alternativeAsset
			? `[Portable ${row.alternative.ext.toUpperCase()}](${downloadUrl(repo, releaseAnchor, row.alternativeAsset)})`
			: "-";
		return `| **${row.platform}** | ${row.device} | ${recommended} | ${alternative} | ${row.bestFor} |`;
	});

	const sums = checksumName("desktop", version);
	rows.push(
		`| **校验和** | 全部文件 | ${renderChecksumCell(repo, releaseAnchor, sums)} | - | 完整性校验 |`,
	);

	return [
		"| 平台 | 架构 | 推荐下载 | 备选 | 适用 |",
		"|:---|:---|:---:|:---|:---|",
		...rows,
	].join("\n");
}

/**
 * 服务端可下载产物清单。target 与 release.yml 的构建矩阵一一对应，
 * 改矩阵时必须同步这里（单测会校验文件名仍符合命名规范）。
 */
export function serverArtifacts(version) {
	return [
		{
			os: "Linux",
			arch: "amd64",
			target: "linux-amd64",
			ext: "",
			bestFor: "服务器 / NAS / 虚拟机",
		},
		{
			os: "Linux",
			arch: "arm64",
			target: "linux-arm64",
			ext: "",
			bestFor: "ARM64 服务器与开发板",
		},
		{
			os: "Linux",
			arch: "armv7",
			target: "linux-armv7",
			ext: "",
			bestFor: "树莓派等 32 位 ARM",
		},
		{
			os: "macOS",
			arch: "Apple Silicon",
			target: "darwin-arm64",
			ext: "",
			bestFor: "本机试用 / 开发",
		},
		{
			os: "macOS",
			arch: "Intel",
			target: "darwin-amd64",
			ext: "",
			bestFor: "Intel 芯片 Mac",
		},
		{
			os: "FreeBSD",
			arch: "amd64",
			target: "freebsd-amd64",
			ext: "",
			bestFor: "FreeBSD 服务器",
		},
		{
			os: "FreeBSD",
			arch: "arm64",
			target: "freebsd-arm64",
			ext: "",
			bestFor: "ARM64 FreeBSD",
		},
		{
			os: "OpenBSD",
			arch: "amd64",
			target: "openbsd-amd64",
			ext: "",
			bestFor: "OpenBSD 服务器",
		},
		{
			os: "Windows",
			arch: "x64",
			target: "windows-amd64",
			ext: "exe",
			bestFor: "64 位 Windows",
		},
		{
			os: "Windows",
			arch: "arm64",
			target: "windows-arm64",
			ext: "exe",
			bestFor: "ARM64 Windows",
		},
	].map((row) => ({
		...row,
		asset: artifactForTarget("server", version, row.target, row.ext),
	}));
}

/** 容器镜像地址：ghcr.io/<owner>/<repo>:<version>。 */
export function containerImage(version, repo) {
	return `ghcr.io/${repo.toLowerCase()}:${version}`;
}

/** 容器镜像包页面（docker pull 之外的图形入口）。 */
export function containerPackageUrl(repo) {
	return `https://github.com/${repo}/pkgs/container/gospeak`;
}

/**
 * 服务端下载表格：每个构建矩阵一行，末尾固定多一行 Docker（镜像不随
 * matrix 变化，放在最后一眼就能看到推荐部署方式）。
 */
export function renderServerTable(version, repo, tag) {
	const releaseAnchor = tag ?? releaseTagForVersion(version);
	const rows = serverArtifacts(version).map(
		(row) =>
			`| **${row.os}** | ${row.arch} | ${renderCell(row.ext || "bin", repo, releaseAnchor, row.asset)} | ${row.bestFor} |`,
	);

	const image = containerImage(version, repo);
	rows.push(
		`| **Docker** | 多架构 | ${renderTextCell(`\`${image}\``, containerPackageUrl(repo))}<br>\`docker pull ${image}\` | 推荐部署方式，免装依赖 |`,
	);

	const sums = checksumName("server", version);
	rows.push(
		`| **校验和** | 全部文件 | ${renderChecksumCell(repo, releaseAnchor, sums)} | 完整性校验 |`,
	);

	return [
		"| 平台 | 架构 | 下载 | 适用 |",
		"|:---|:---|:---:|:---|",
		...rows,
	].join("\n");
}

function renderInstallNotes() {
	return [
		"### 安装说明",
		"",
		"- macOS 分 Apple Silicon 与 Intel 两个版本，下错架构打不开。",
		"- macOS 安装包未签名，首次打开如被拦截，去「系统设置 → 隐私与安全性」点「仍要打开」。",
		"- Windows 提供 EXE 安装包，双击安装。",
		"- Linux 优先用 `.deb`（双击即装），`.tar.gz` 为免安装便携包。",
		"- `SHA256SUMS` 文件可用于校验下载完整性。",
	].join("\n");
}

function renderDeploySection(repo, tag, version) {
	return [
		"## 部署 GOSpeak 服务端",
		"",
		`- **Docker Compose**（推荐）：[在服务器 / NAS / 虚拟机上部署](https://github.com/${repo}/blob/${tag}/deploy/DEPLOY.md)`,
		`- **容器镜像**：\`docker pull ${containerImage(version, repo)}\`，多架构自动匹配`,
		"- **二进制**：见上方服务端下载表格，下载后 `chmod +x` 直接运行 `./gospeak-server-* server -e prod`",
		"",
		`[浏览 ${tag} 的部署文档](https://github.com/${repo}/tree/${tag}/deploy)`,
	].join("\n");
}

/**
 * 按组件渲染 Release 正文（一组件一 Release）：
 *   server  → 服务端下载表格（含 Docker 行）+ 部署说明 + notes
 *   desktop → 桌面安装包表格 + 安装说明 + notes
 *   mobile  → APK 说明
 *
 * 每个正文只描述自己组件的产物，不再混装其他组件。
 */
export function renderReleaseBody({
	component,
	version,
	repo,
	tag,
	desktopNotes,
	serverNotes,
}) {
	// tag 由调用方（workflow）传入实际发布 tag；未传时按 component 派生。
	const bodyTag = tag || releaseTag(component, version);
	const parts = [];

	if (component === "desktop") {
		parts.push("## 下载 GOSpeak Desktop");
		parts.push("");
		parts.push(renderDesktopTable(version, repo, bodyTag));
		parts.push("");
		parts.push(renderInstallNotes());
		if ((desktopNotes ?? "").trim()) {
			parts.push("");
			parts.push("## release notes");
			parts.push("");
			parts.push(desktopNotes.trim());
		} else {
			parts.push("");
			parts.push("* 本次发布无额外说明。*");
		}
		return parts.join("\n");
	}

	if (component === "mobile") {
		parts.push("## 下载 GOSpeak Mobile");
		parts.push("");
		parts.push(
			`- Android APK：\`gospeak-mobile-${version}-debug.apk\`（当前为 debug 构建，正式签名产物待补）`,
		);
		parts.push("- iOS：构建暂未启用。");
		return parts.join("\n");
	}

	// 默认 server
	parts.push("## 下载 GOSpeak Server");
	parts.push("");
	parts.push(renderServerTable(version, repo, bodyTag));
	parts.push("");
	parts.push(renderDeploySection(repo, bodyTag, version));
	parts.push("");
	parts.push("---");
	parts.push("");
	parts.push("## release notes");
	parts.push("");
	parts.push((serverNotes ?? "").trim() || "* 本次发布无额外说明。");
	parts.push("");

	return parts.join("\n");
}

function parseArgs(argv) {
	const [mode, ...rest] = argv;
	const options = { mode };
	for (let i = 0; i < rest.length; i += 1) {
		const arg = rest[i];
		if (!arg.startsWith("--")) continue;
		const key = arg.slice(2);
		const value = rest[i + 1];
		if (value === undefined || value.startsWith("--")) {
			throw new Error(`option --${key} requires a value`);
		}
		options[key] = value;
		i += 1;
	}
	return options;
}

function readNotes(path) {
	if (!path) return "";
	try {
		return readFileSync(path, "utf8");
	} catch {
		return "";
	}
}

function main() {
	const options = parseArgs(process.argv.slice(2));
	const { mode, version, repo } = options;
	if (mode !== "release") {
		throw new Error(`mode must be "release", got ${JSON.stringify(mode)}`);
	}
	if (!version) throw new Error("--version is required");
	if (!repo) throw new Error("--repo is required");

	const component = options.component || "server";
	if (!["server", "desktop", "mobile"].includes(component)) {
		throw new Error(`unknown --component ${JSON.stringify(component)}`);
	}
	const body = renderReleaseBody({
		component,
		version,
		repo,
		tag: options.tag,
		desktopNotes: readNotes(options["notes-file"]),
		serverNotes: readNotes(options["notes-file"]),
	});

	process.stdout.write(body);
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
	try {
		main();
	} catch (err) {
		process.stderr.write(
			`::error::${err instanceof Error ? err.message : err}\n`,
		);
		process.exitCode = 1;
	}
}
