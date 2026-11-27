import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { beforeAll, describe, expect, it } from "vitest";
import {
	api,
	assertSuccess,
	getAdminToken,
	waitFor,
	getBaseURL,
} from "../helpers/api";

const PLUGIN_ID = "com.example.e2e";
const serverInfo = JSON.parse(
	readFileSync(join(import.meta.dirname, "../.runtime/server.json"), "utf8"),
) as {
	plugin_root: string;
	plugin_source: string;
	plugin_page_url: string;
};
const pluginRoot = serverInfo.plugin_root;

interface PluginItem {
	plugin_id: string;
	version: string;
	enabled: boolean;
	route_path: string;
}

interface RuntimeVersion {
	kind: string;
	version: string;
	active: boolean;
}

async function pluginApi<T>(
	token: string,
	path: string,
	body: unknown = {},
	method = "POST",
) {
	const result = await api<T>(`/api/v1/plugin${path}`, {
		token,
		method,
		...(method === "GET" ? {} : { body }),
	});
	if (result.status >= 200 && result.status < 300 && result.code !== 0) {
		throw new Error(
			`plugin api ${path} failed: ${result.status}/${result.code} ${result.msg} ${result.raw}`,
		);
	}
	return result;
}

describe("plugin end-to-end", () => {
	let token: string;

	beforeAll(async () => {
		token = await getAdminToken();
	});

	it("selects the offline node runtime via management API", async () => {
		const list = assertSuccess(
			await pluginApi<{ items: RuntimeVersion[] }>(
				token,
				"/runtime/list",
				{},
				"GET",
			),
		);
		expect(list.items.some((v) => v.kind === "node" && v.active)).toBe(true);
		const use = await pluginApi<{ ok: boolean }>(token, "/runtime/use", {
			kind: "node",
			version: "22.0.0",
		});
		expect(use.code).toBe(0);
		expect(assertSuccess(use).ok).toBe(true);
		const python = await pluginApi<{ unsupported: boolean }>(
			token,
			"/runtime/install",
			{
				kind: "python",
				version: "3.12.0",
			},
		);
		expect(assertSuccess(python).unsupported).toBe(true);
	});

	it("installs a local git plugin offline and registers it", async () => {
		const source = serverInfo.plugin_source;
		expect(
			source,
			"fixture git repo path must be injected by globalSetup",
		).toBeTruthy();
		const installed = await pluginApi(token, "/install", {
			source: "git-fixture",
			url: source,
		});
		if (installed.code !== 0) {
			throw new Error(
				`install failed: ${installed.status}/${installed.code} ${installed.msg} ${installed.raw}`,
			);
		}
		const list = assertSuccess(
			await pluginApi<{ items: PluginItem[] }>(token, "/list", {}, "GET"),
		);
		expect(list.items.map((i) => i.plugin_id)).toContain(PLUGIN_ID);
		const statePath = join(
			pluginRoot,
			"runtime",
			"state",
			PLUGIN_ID,
			"calls.json",
		);
		const callsRaw = await waitFor(
			async () => existsSync(statePath) && readFileSync(statePath, "utf8"),
			(v) => typeof v === "string" && v.includes("GetUser"),
			{ label: "plugin API calls" },
		);
		if (typeof callsRaw !== "string") throw new Error("calls.json is empty");
		const calls = JSON.parse(callsRaw) as Array<{
			label: string;
			ok: boolean;
			error?: string;
			result?: unknown;
		}>;
		for (const label of [
			"GetUser",
			"ListRooms",
			"GetConfig",
			"KVSave",
			"KVLoad",
			"CreateRoom",
			"ReportError",
		]) {
			const call = calls.find((c) => c.label === label);
			expect(call, `plugin API ${label} missing: ${callsRaw}`).toBeDefined();
			expect(call?.error, `plugin API ${label} failed`).toBeUndefined();
			expect(call?.ok, `plugin API ${label} not ok`).toBe(true);
			expect(call?.result, `plugin API ${label} returned no data`).toBeTruthy();
		}
		const config = assertSuccess(
			await pluginApi(token, "/config", {
				plugin_id: PLUGIN_ID,
				config_json: { enabled: true },
			}),
		) as { config_json: string };
		expect(JSON.parse(config.config_json)).toEqual({ enabled: true });
		const pages = await api<unknown>(`/api/v1/plugin/pages/${PLUGIN_ID}`, {
			token,
			method: "GET",
		});
		expect(pages.status).toBe(200);
	});

	it("exposes plugin metrics on /metrics", async () => {
		const base = getBaseURL();
		const raw = await fetch(`${base}/metrics`).then((r) => r.text());
		for (const name of [
			"gospeak_plugin_installed",
			"gospeak_plugin_active",
			"gospeak_plugin_restarts",
			"gospeak_plugin_api_calls",
			"gospeak_plugin_api_errors",
		]) {
			expect(raw).toContain(name);
		}
	});

	it("reloads, upgrades, disables and uninstalls the plugin with audit trail", async () => {
		expect(
			(await pluginApi(token, "/reload", { plugin_id: PLUGIN_ID })).code,
		).toBe(0);
		expect(
			(await pluginApi(token, "/upgrade", { plugin_id: PLUGIN_ID })).code,
		).toBe(0);
		expect(
			(await pluginApi(token, "/disable", { plugin_id: PLUGIN_ID })).code,
		).toBe(0);
		expect(
			(await pluginApi(token, "/uninstall", { plugin_id: PLUGIN_ID })).code,
		).toBe(0);
		const list = assertSuccess(
			await pluginApi<{ items: PluginItem[] }>(token, "/list", {}, "GET"),
		);
		expect(list.items.map((i) => i.plugin_id)).not.toContain(PLUGIN_ID);
		const audit = await api<{
			list: Array<Record<string, unknown>>;
			total: number;
		}>("/api/v1/audit/list", {
			token,
			body: {
				actor_uuid: "f916c978-d0a5-5e1c-b4f9-8bef8909bc1d",
				page: 1,
				page_size: 50,
			},
		});
		if (audit.code === 0 && audit.data) {
			expect(audit.data.list.some((row) => row.actor_kind === "plugin")).toBe(
				true,
			);
		}
	});
});
