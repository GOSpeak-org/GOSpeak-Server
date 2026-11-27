import assert from "node:assert/strict";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { FILES, run, sha256 } from "./check-proxyrules-sync.mjs";

const GOV = "package proxyrules\n\nfunc A() {}\n";
const TEST = "package proxyrules\n\n// tests\n";

async function makeFixture(localGo, localTest = TEST) {
	const dir = await mkdtemp(join(tmpdir(), "proxyrules-sync-"));
	await writeFile(join(dir, "proxyrules.go"), localGo, "utf8");
	await writeFile(join(dir, "proxyrules_test.go"), localTest, "utf8");
	return dir;
}

function remoteOf(goText, testText = TEST) {
	return (name) =>
		Promise.resolve(
			Buffer.from(name === "proxyrules.go" ? goText : testText, "utf8"),
		);
}

test("sha256 对相同内容稳定、对不同内容不同", () => {
	assert.equal(sha256(Buffer.from("abc")), sha256(Buffer.from("abc")));
	assert.notEqual(sha256(Buffer.from("abc")), sha256(Buffer.from("abd")));
});

test("FILES 覆盖实现与单测两份", () => {
	assert.deepEqual(FILES, ["proxyrules.go", "proxyrules_test.go"]);
});

test("内容一致时无 mismatch", async () => {
	const dir = await makeFixture(GOV);
	const result = await run({ localDir: dir, fetchFile: remoteOf(GOV) });
	assert.equal(result.mismatches.length, 0);
});

test("内容不一致时报出两份哈希且不改文件", async () => {
	const dir = await makeFixture(GOV);
	const drifted = GOV.replace("func A() {}", "func A() { /* drift */ }");
	const result = await run({ localDir: dir, fetchFile: remoteOf(drifted) });
	assert.equal(result.mismatches.length, 1);
	const [item] = result.mismatches;
	assert.equal(item.name, "proxyrules.go");
	assert.notEqual(item.localHash, item.remoteHash);
	assert.equal(await readFile(join(dir, "proxyrules.go"), "utf8"), GOV);
});

test("--fix 用远端内容覆盖并保留其他文件", async () => {
	const dir = await makeFixture(GOV);
	const drifted = GOV.replace("func A() {}", "func A() { /* drift */ }");
	const result = await run({
		localDir: dir,
		fix: true,
		fetchFile: remoteOf(drifted, TEST + "// remote\n"),
	});
	assert.equal(result.mismatches.length, 0);
	const fixed = await readFile(join(dir, "proxyrules.go"), "utf8");
	assert.equal(fixed, drifted);
	assert.equal(sha256(Buffer.from(fixed)), sha256(Buffer.from(drifted)));
	assert.equal(await readFile(join(dir, "proxyrules_test.go"), "utf8"), TEST + "// remote\n");
});

test("两份文件同时漂移会全部报出", async () => {
	const dir = await makeFixture(GOV);
	const result = await run({
		localDir: dir,
		fetchFile: remoteOf(GOV + "// x\n", TEST + "// y\n"),
	});
	assert.equal(result.mismatches.length, 2);
	assert.deepEqual(
		result.mismatches.map((m) => m.name),
		["proxyrules.go", "proxyrules_test.go"],
	);
});
