#!/usr/bin/env node
// 校验服务端 proxyrules 副本与客户端仓源实现一致 —— 漂移门禁。
//
// 背景：proxyrules 是纯 Go 的反代 / SSRF 校验规则，源实现位于 GOSpeak-Client 仓的
// proxyrules/（桌面与移动端 gomobile 绑定共用）。三仓拆分后服务端无法用相对 replace
// 引用客户端仓，因此在 server/internal/plugin/proxyrules/ 保留了一份原样副本。
// 副本没有门禁就会静默漂移，所以本脚本把"两处必须逐字节一致"变成 CI 必过项。
//
// 用法：
//   node scripts/check-proxyrules-sync.mjs          按 sha256 比对（不一致退出 1）
//   node scripts/check-proxyrules-sync.mjs --fix    用远端内容覆盖本地副本
//
// 环境变量：
//   PROXYRULES_REF       客户端仓 ref，默认 release
//   PROXYRULES_REPO      客户端仓，默认 GOSpeak-org/GOSpeak-Client
//   PROXYRULES_LOCAL_DIR 本地副本目录，默认 <repoRoot>/server/internal/plugin/proxyrules
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export const DEFAULT_REPO = "GOSpeak-org/GOSpeak-Client";
export const DEFAULT_REF = "release";

export const FILES = ["proxyrules.go", "proxyrules_test.go"];

export function sha256(buffer) {
	return createHash("sha256").update(buffer).digest("hex");
}

async function fetchRemoteFile(repo, ref, name) {
	const url =
		"https://raw.githubusercontent.com/" + repo + "/" + ref + "/proxyrules/" + name;
	// raw.githubusercontent 偶发瞬断，重试三次再判失败，避免门禁假红。
	let lastError;
	for (let attempt = 1; attempt <= 3; attempt += 1) {
		try {
			const res = await fetch(url);
			if (!res.ok) {
				throw new Error("拉取 " + url + " 失败：HTTP " + res.status);
			}
			return Buffer.from(await res.arrayBuffer());
		} catch (err) {
			lastError = err;
			if (attempt < 3) {
				await new Promise((resolve) => setTimeout(resolve, 500 * attempt));
			}
		}
	}
	throw lastError;
}

function resolveRoot() {
	const here = dirname(fileURLToPath(import.meta.url));
	return join(here, "..");
}

export async function run(options = {}) {
	const repo = options.repo || process.env.PROXYRULES_REPO || DEFAULT_REPO;
	const ref = options.ref || process.env.PROXYRULES_REF || DEFAULT_REF;
	const localDir =
		options.localDir ||
		process.env.PROXYRULES_LOCAL_DIR ||
		join(resolveRoot(), "server/internal/plugin/proxyrules");
	const fix = Boolean(options.fix);
	const fetchFile = options.fetchFile || ((name) => fetchRemoteFile(repo, ref, name));
	const mismatches = [];
	for (const name of FILES) {
		const localPath = join(localDir, name);
		const local = await readFile(localPath);
		const remote = await fetchFile(name);
		const localHash = sha256(local);
		const remoteHash = sha256(remote);
		if (localHash === remoteHash) continue;
		if (!fix) {
			mismatches.push({ name, localPath, localHash, remoteHash });
			continue;
		}
		await writeFile(localPath, remote);
		console.log("[fix] " + name + " 已用 " + repo + "@" + ref + " 覆盖");
	}
	return { repo, ref, mismatches };
}

async function main() {
	const fix = process.argv.includes("--fix");
	let result;
	try {
		result = await run({ fix });
	} catch (err) {
		console.error("[error] proxyrules 比对失败：" + err.message);
		console.error("若是网络问题，请确认 runner 可访问 raw.githubusercontent.com。");
		process.exit(2);
	}
	if (result.mismatches.length === 0) {
		console.log("[ok] proxyrules 副本与 " + result.repo + "@" + result.ref + " 一致。");
		return;
	}
	console.error("[fail] proxyrules 副本与 " + result.repo + "@" + result.ref + " 不一致：");
	for (const item of result.mismatches) {
		console.error("  - " + item.name);
		console.error("      本地 sha256 " + item.localHash);
		console.error("      远端 sha256 " + item.remoteHash);
	}
	console.error("修复：运行 node scripts/check-proxyrules-sync.mjs --fix（规则改动请先改客户端仓）。");
	process.exit(1);
}

if (import.meta.url === pathToFileURL(process.argv[1] || "").href) {
	await main();
}
