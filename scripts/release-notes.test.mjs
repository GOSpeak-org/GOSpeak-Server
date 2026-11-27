import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

import {
	containerImage,
	desktopArtifacts,
	releaseTagForVersion,
	renderDesktopTable,
	renderReleaseBody,
	renderServerTable,
	serverArtifacts,
} from "./release-notes.mjs";
import { checksumName } from "./release-naming.mjs";

const SCRIPT = fileURLToPath(new URL("./release-notes.mjs", import.meta.url));
const REPO = "GOSpeak-org/GOSpeak-Server";

test("desktop artifacts follow the naming convention for every row", () => {
	for (const row of desktopArtifacts("1.0.0")) {
		assert.match(
			row.primaryAsset,
			/^gospeak-desktop-v1\.0\.0-(darwin|windows|linux)-(amd64|arm64)\.(dmg|deb|exe|tar\.gz)$/,
		);
		if (row.alternativeAsset) {
			assert.match(row.alternativeAsset, /^gospeak-desktop-v1\.0\.0-/);
		}
	}
});

test("unified release tag is the server tag", () => {
	assert.equal(
		releaseTagForVersion("1.1.0-rc.1"),
		"gospeak-server-v1.1.0-rc.1",
	);
});

test("server artifacts follow the naming convention and cover every matrix row", () => {
	const rows = serverArtifacts("1.0.0");
	assert.equal(rows.length, 10);
	const targets = rows.map((row) => row.target).join(",");
	for (const target of [
		"linux-amd64",
		"linux-arm64",
		"linux-armv7",
		"darwin-arm64",
		"darwin-amd64",
		"freebsd-amd64",
		"freebsd-arm64",
		"openbsd-amd64",
		"windows-amd64",
		"windows-arm64",
	]) {
		assert.ok(targets.includes(target), `missing matrix target ${target}`);
	}
	for (const row of rows) {
		assert.match(
			row.asset,
			/^gospeak-server-v1\.0\.0-(linux|darwin|freebsd|openbsd|windows)-(amd64|arm64|armv7)(\.exe)?$/,
		);
	}
});

test("server table ends with a docker row before the checksum row", () => {
	const table = renderServerTable("1.1.0-rc.1", REPO);
	const lines = table.split("\n");
	const dockerIndex = lines.findIndex((line) => line.includes("**Docker**"));
	const sumsIndex = lines.findIndex((line) => line.includes("**校验和**"));

	assert.ok(dockerIndex > 0, "docker row missing");
	assert.ok(sumsIndex > dockerIndex, "checksum row must come after docker");
	assert.equal(
		sumsIndex,
		lines.length - 1,
		"checksum row must be the last row",
	);

	const image = containerImage("1.1.0-rc.1", REPO);
	assert.equal(image, "ghcr.io/gospeak-org/gospeak-server:1.1.0-rc.1");
	assert.ok(
		lines[dockerIndex].includes(image),
		"docker row must name the image",
	);
	assert.ok(
		lines[dockerIndex].includes(`docker pull ${image}`),
		"docker row must show the pull command",
	);
	// 镜像不是文件产物，不能指向 release 的下载直链。
	assert.doesNotMatch(lines[dockerIndex], /releases\/download/);
});

test("server release body puts the download table above its notes", () => {
	const body = renderReleaseBody({
		component: "server",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-server-v1.0.0",
		serverNotes: "服务端变更内容",
	});

	const downloads = body.indexOf("## 下载 GOSpeak Server");
	const serverTable = body.indexOf("gospeak-server-v1.0.0-linux-amd64");
	const deploy = body.indexOf("## 部署 GOSpeak 服务端");
	const divider = body.indexOf("\n---\n");
	const notes = body.indexOf("## release notes");

	assert.ok(downloads >= 0, "downloads section missing");
	assert.ok(serverTable > downloads, "server table must follow the heading");
	assert.ok(deploy > serverTable, "deploy section must follow the table");
	assert.ok(divider > deploy, "divider must follow deploy section");
	assert.ok(notes > divider, "notes must be after the divider");
	assert.ok(body.trimEnd().endsWith("服务端变更内容"));
});

test("desktop release body has no server or deploy sections", () => {
	const body = renderReleaseBody({
		component: "desktop",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-desktop-v1.0.0",
		desktopNotes: "桌面端变更内容",
	});
	assert.match(body, /## 下载 GOSpeak Desktop/);
	assert.doesNotMatch(body, /## 部署 GOSpeak 服务端/);
	assert.doesNotMatch(body, /docker pull/);
	assert.doesNotMatch(body, /gospeak-server-v/);
	assert.ok(body.trimEnd().endsWith("桌面端变更内容"));
});

test("each component release links only its own artifacts", () => {
	// desktop release：链接全部指向 gospeak-desktop-v* tag
	const desktopBody = renderReleaseBody({
		component: "desktop",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-desktop-v1.0.0",
	});
	for (const row of desktopArtifacts("1.0.0")) {
		const url = `https://github.com/${REPO}/releases/download/gospeak-desktop-v1.0.0/${row.primaryAsset}`;
		assert.ok(
			desktopBody.includes(url),
			`missing desktop url for ${row.primaryAsset}`,
		);
	}
	assert.ok(
		desktopBody.includes(
			`/releases/download/gospeak-desktop-v1.0.0/${checksumName("desktop", "1.0.0")}`,
		),
	);
	assert.doesNotMatch(desktopBody, /releases\/download\/gospeak-server-v/);

	// server release：链接全部指向 gospeak-server-v* tag
	const serverBody = renderReleaseBody({
		component: "server",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-server-v1.0.0",
	});
	for (const row of serverArtifacts("1.0.0")) {
		const url = `https://github.com/${REPO}/releases/download/gospeak-server-v1.0.0/${row.asset}`;
		assert.ok(serverBody.includes(url), `missing server url for ${row.asset}`);
	}
	assert.ok(
		serverBody.includes(
			`/releases/download/gospeak-server-v1.0.0/${checksumName("server", "1.0.0")}`,
		),
	);
	assert.doesNotMatch(serverBody, /releases\/download\/gospeak-desktop-v/);
});

test("each table only advertises its own artifacts", () => {
	const desktopTable = renderDesktopTable("1.0.0", REPO);
	assert.doesNotMatch(desktopTable, /gospeak-server-v[\d.]+-/);
	assert.match(desktopTable, /gospeak-desktop-v1\.0\.0-darwin-arm64\.dmg/);

	const serverTable = renderServerTable("1.0.0", REPO);
	assert.doesNotMatch(serverTable, /gospeak-desktop-/);
	assert.match(serverTable, /gospeak-server-v1\.0\.0-linux-amd64/);
});

test("server release body never mentions desktop artifacts", () => {
	const body = renderReleaseBody({
		component: "server",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-server-v1.0.0",
		serverNotes: "",
	});

	// 服务端表格与 Docker 行在；不再有桌面端表格或回退链接。
	assert.match(body, /gospeak-server-v1\.0\.0-linux-amd64/);
	assert.match(body, /docker pull ghcr\.io\/gospeak-org\/gospeak-server:1\.0\.0/);
	assert.doesNotMatch(body, /gospeak-desktop-/);
	assert.doesNotMatch(body, /\.dmg/);
});

test("desktop notes are omitted when the desktop did not ship", () => {
	const body = renderReleaseBody({
		component: "server",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-server-v1.0.0",
		serverNotes: "服务端变更内容",
	});

	assert.doesNotMatch(body, /## 桌面端 release notes/);
	assert.doesNotMatch(body, /桌面端变更内容/);
});

test("empty server notes degrade to a readable placeholder", () => {
	const body = renderReleaseBody({
		component: "server",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-server-v1.0.0",
		serverNotes: "   ",
	});
	assert.match(body, /本次发布无额外说明/);
	assert.doesNotMatch(body, /## 桌面端 release notes/);
});

test("CLI renders a release body and requires --repo", () => {
	const ok = spawnSync(
		process.execPath,
		[SCRIPT, "release", "--version", "1.0.0", "--repo", REPO],
		{ encoding: "utf8" },
	);
	assert.equal(ok.status, 0, ok.stderr);
	assert.match(ok.stdout, /## 下载 GOSpeak Server/);
	assert.match(ok.stdout, /docker pull ghcr\.io\/gospeak-org\/gospeak-server:1\.0\.0/);
	assert.match(ok.stdout, /## release notes/);

	const missing = spawnSync(
		process.execPath,
		[SCRIPT, "release", "--version", "1.0.0"],
		{
			encoding: "utf8",
		},
	);
	assert.equal(missing.status, 1);
	assert.match(missing.stderr, /--repo is required/);

	const wrongMode = spawnSync(
		process.execPath,
		[SCRIPT, "desktop", "--version", "1.0.0", "--repo", REPO],
		{ encoding: "utf8" },
	);
	assert.equal(wrongMode.status, 1);
	assert.match(wrongMode.stderr, /mode must be "release"/);
});

test("server release body excludes desktop artifacts", () => {
	const body = renderReleaseBody({
		component: "server",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-server-v1.0.0",
		serverNotes: "server notes",
	});
	assert.ok(body.includes("gospeak-server-v1.0.0"), "contains server tag");
	assert.ok(!body.includes(".dmg"), "no dmg");
	assert.ok(!body.includes(".deb"), "no deb");
	assert.ok(!body.includes("install.sh"), "no install.sh");
});

test("desktop release body excludes server binaries", () => {
	const body = renderReleaseBody({
		component: "desktop",
		version: "1.0.0",
		repo: REPO,
		tag: "gospeak-desktop-v1.0.0",
		desktopNotes: "desktop notes",
	});
	assert.ok(body.includes("gospeak-desktop-v1.0.0"), "contains desktop tag");
	assert.ok(!body.includes("SHA256SUMS-gospeak-server"), "no server checksum");
	assert.ok(!body.includes("docker pull"), "no docker line");
});
