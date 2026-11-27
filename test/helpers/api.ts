import { randomBytes } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export interface AuthUser {
	id: number;
	uuid: string;
	name: string;
	display_name: string;
	role: string;
}

export interface AuthData {
	access_token: string;
	refresh_token: string;
	user: AuthUser;
	need_change_password?: boolean;
}

export interface ApiResult<T = unknown> {
	status: number;
	code: number | null;
	msg: string | null;
	data: T | null;
	raw: string;
	path: string;
}

export interface ApiOptions {
	token?: string;
	method?: string;
	body?: unknown;
	headers?: Record<string, string>;
	formData?: FormData;
	redirect?: "follow" | "manual" | "error";
}

const helperDir = dirname(fileURLToPath(import.meta.url));
const serverInfoPath = resolve(helperDir, "../.runtime/server.json");

function readServerUrl(): string {
	const fromEnv = process.env.GOSPEAK_TEST_URL;
	if (fromEnv) {
		return fromEnv.replace(/\/$/, "");
	}
	if (existsSync(serverInfoPath)) {
		try {
			const parsed = JSON.parse(readFileSync(serverInfoPath, "utf8")) as {
				url?: string;
			};
			if (parsed.url) {
				return parsed.url.replace(/\/$/, "");
			}
		} catch {
			// fall through to a clear error
		}
	}
	throw new Error("integration server is not running; run pnpm test:server");
}

export function getBaseURL(): string {
	return readServerUrl();
}

export function unique(prefix = "item"): string {
	return `${prefix}_${Date.now().toString(36)}_${randomBytes(4).toString("hex")}`;
}

/**
 * 有界轮询直到 predicate 成立，用于「写后读」。
 * WS 写入经 jobs 异步落库（chat.persist / chat.mutate），立即回读可能读到旧快照。
 * 首条 chat job 在冷启动 server 上实测约 4s（之后为毫秒级），
 * 因此默认给 10s 预算（< vitest testTimeout 30s）。
 * 超时仍不成立才判失败，错误里带最后一次探针值与标签。
 */
export async function waitFor<T>(
	probe: () => Promise<T>,
	predicate: (value: T) => boolean,
	options: { label?: string; timeoutMs?: number; intervalMs?: number } = {},
): Promise<T> {
	const timeoutMs = options.timeoutMs ?? 10_000;
	const intervalMs = options.intervalMs ?? 50;
	const deadline = Date.now() + timeoutMs;
	let value = await probe();
	while (!predicate(value)) {
		if (Date.now() >= deadline) {
			const label = options.label ? ` (${options.label})` : "";
			throw new Error(
				`waitFor timed out after ${timeoutMs}ms${label}; last=${JSON.stringify(value)}`,
			);
		}
		await new Promise((resolvePromise) =>
			setTimeout(resolvePromise, intervalMs),
		);
		value = await probe();
	}
	return value;
}

function randomClientIP(): string {
	const part = () => 2 + Math.floor(Math.random() * 253);
	return `10.${part()}.${part()}.${part()}`;
}

export async function api<T = unknown>(
	path: string,
	options: ApiOptions = {},
): Promise<ApiResult<T>> {
	const headers = new Headers(options.headers);
	headers.set("X-Forwarded-For", randomClientIP());
	if (options.token) {
		headers.set("Authorization", `Bearer ${options.token}`);
	}

	let body: BodyInit | undefined;
	if (options.formData) {
		body = options.formData;
	} else if (options.body !== undefined) {
		headers.set("Content-Type", "application/json");
		body = JSON.stringify(options.body);
	}

	const response = await fetch(`${readServerUrl()}${path}`, {
		method: options.method ?? "POST",
		headers,
		body,
		redirect: options.redirect ?? "follow",
	});
	const raw = await response.text();
	let parsed: { code?: number; msg?: string; data?: T | null } | null = null;
	try {
		parsed = JSON.parse(raw) as {
			code?: number;
			msg?: string;
			data?: T | null;
		};
	} catch {
		parsed = null;
	}
	return {
		status: response.status,
		code: parsed?.code ?? null,
		msg: parsed?.msg ?? null,
		data: parsed?.data ?? null,
		raw,
		path,
	};
}

/**
 * 断言统一响应外壳的成功四段：status(2xx) / code(0) / msg("success") / data。
 * data 由调用方按接口契约继续断言（写接口的 data 合法为 null），这里同时返回它。
 * 失败信息带 path + status + code + msg + 原始 body，便于定位。
 */
export function assertSuccess<T = unknown>(result: ApiResult<T>): T {
	const context = `path=${result.path} status=${result.status} code=${result.code} msg=${result.msg} body=${result.raw}`;
	if (result.status < 200 || result.status >= 300) {
		throw new Error(`expected 2xx status, got ${context}`);
	}
	if (result.code !== 0) {
		throw new Error(`expected success code=0, got ${context}`);
	}
	if (result.msg !== "success") {
		throw new Error(`expected success msg, got ${context}`);
	}
	return result.data as T;
}

export async function login(
	username: string,
	password: string,
): Promise<AuthData> {
	const result = await api<AuthData>("/api/v1/auth/login", {
		body: { username, password },
	});
	return assertSuccess(result);
}

export async function registerUser(
	prefix = "user",
): Promise<AuthData & { username: string; password: string }> {
	const username = unique(prefix);
	const password = `Passw0rd!${randomBytes(6).toString("hex")}`;
	const result = await api<AuthData>("/api/v1/auth/register", {
		body: { username, password },
	});
	const data = assertSuccess(result);
	return { ...data, username, password };
}

/**
 * 种子管理员凭据：与 server 启动种子一致，可用环境变量覆盖。
 * 凭据只在这里定义一处，测试文件不要重复硬编码。
 */
const ADMIN_USERNAME = process.env.GOSPEAK_TEST_ADMIN_USERNAME ?? "admin";
const SEED_ADMIN_PASSWORD =
	process.env.GOSPEAK_TEST_ADMIN_PASSWORD ?? "admin123";
// 服务端强制管理员改掉默认口令；测试脚手架统一改成这个固定口令。
// 跨 vitest worker 共用同一个临时 server，因此必须是固定值而非每 worker 随机值。
const INTEGRATION_ADMIN_PASSWORD = "IntegrationPassw0rd!2026";

/**
 * 用种子管理员账号登录，供需要断言凭据本身可用的用例复用。
 *
 * 仍使用默认口令的管理员会被 RequirePasswordChanged 拦下（1013）。
 * 这里按候选口令依次尝试：第一个成功且 need_change_password 的登录会先
 * 调用 first_change_password 完成改密，后续 worker 直接用改密后的口令登录。
 * 不用模块级 token/password 缓存，避免跨文件模块隔离导致状态丢失；
 * 请求级随机 X-Forwarded-For 也使失败尝试不会撞登录限流。
 */
export async function loginAdmin(): Promise<AuthData> {
	const candidates = [
		...new Set([SEED_ADMIN_PASSWORD, INTEGRATION_ADMIN_PASSWORD]),
	];
	let lastError: unknown;
	for (const password of candidates) {
		try {
			const data = await login(ADMIN_USERNAME, password);
			if (!data.need_change_password) {
				return data;
			}
			const changed = await api<AuthData>(
				"/api/v1/auth/first_change_password",
				{
					token: data.access_token,
					body: { new_password: INTEGRATION_ADMIN_PASSWORD },
				},
			);
			return assertSuccess(changed);
		} catch (error) {
			lastError = error;
		}
	}
	throw lastError;
}

/**
 * 取管理员 token：每次调用都重新登录，不做模块级缓存。
 * 缓存会让任一用例登出/改密后污染后续用例（用例之间必须互相独立）。
 */
export async function getAdminToken(): Promise<string> {
	const data = await loginAdmin();
	return data.access_token;
}

export async function createDomain(
	token: string,
	name: string,
): Promise<{ uuid: string; name: string }> {
	const result = await api<{ uuid: string; name: string }>(
		"/api/v1/domain/create",
		{
			token,
			body: { name, description: "integration domain", is_public: false },
		},
	);
	return assertSuccess(result);
}

/** 为域新建一个邀请码（域创建时会自动生成默认码，这里用于需要显式控制的场景）。 */
export async function createDomainInvite(
	token: string,
	domainUUID: string,
): Promise<{ id: number; code: string; max_uses: number; used_count: number }> {
	const result = await api<{
		id: number;
		code: string;
		max_uses: number;
		used_count: number;
	}>("/api/v1/domain/invites/create", {
		token,
		body: { domain_uuid: domainUUID },
	});
	return assertSuccess(result);
}

/** 列出域内邀请码，用于取默认邀请码。 */
export async function listDomainInvites(
	token: string,
	domainUUID: string,
): Promise<Array<{ id: number; code: string; disabled_at?: string | null }>> {
	const result = await api<{
		invites: Array<{ id: number; code: string; disabled_at?: string | null }>;
	}>("/api/v1/domain/invites/list", {
		token,
		body: { domain_uuid: domainUUID },
	});
	return assertSuccess(result).invites ?? [];
}

/** 已登录用户凭邀请码加入域（返回 status: joined | pending）。 */
export async function joinViaInvite(
	token: string,
	code: string,
): Promise<{ status: string; domain: { uuid: string } }> {
	const result = await api<{ status: string; domain: { uuid: string } }>(
		"/api/v1/invite/join",
		{
			token,
			body: { code },
		},
	);
	return assertSuccess(result);
}

export async function listDomainRoles(
	token: string,
	domainUUID: string,
): Promise<{
	roles: Array<{ name: string; is_system: boolean; permissions: string[] }>;
	assignable: string[];
}> {
	const result = await api(`/api/v1/domain/roles/list`, {
		token,
		body: { domain_uuid: domainUUID },
	});
	return assertSuccess(result);
}

export async function createDomainRole(
	token: string,
	domainUUID: string,
	name: string,
	permissions: string[],
): Promise<unknown> {
	const result = await api(`/api/v1/domain/roles/create`, {
		token,
		body: { domain_uuid: domainUUID, name, permissions },
	});
	return assertSuccess(result);
}

export async function updateDomainMemberRole(
	token: string,
	domainUUID: string,
	userUUID: string,
	roleName: string,
): Promise<unknown> {
	const result = await api(`/api/v1/domain/members/update-role`, {
		token,
		body: { domain_uuid: domainUUID, user_uuid: userUUID, role_name: roleName },
	});
	return assertSuccess(result);
}

/** 取域的默认邀请码（域创建时自动生成）。 */
export async function defaultInviteCode(
	token: string,
	domainUUID: string,
): Promise<string> {
	const invites = await listDomainInvites(token, domainUUID);
	const usable = invites.find((item) => !item.disabled_at);
	if (!usable)
		throw new Error(`domain ${domainUUID} has no usable invite code`);
	return usable.code;
}

export async function createRoom(
	token: string,
	domainUUID: string,
	name: string,
	type = "voice",
): Promise<{
	id: number;
	uuid: string;
	name: string;
	type: string;
	domain_uuid: string;
}> {
	const result = await api<{
		id: number;
		uuid: string;
		name: string;
		type: string;
		domain_uuid: string;
	}>("/api/v1/room/create", {
		token,
		body: {
			name,
			domain_uuid: domainUUID,
			type,
			description: "integration room",
			limit: 10,
			audio_only: true,
			allow_audience: true,
		},
	});
	return assertSuccess(result);
}
