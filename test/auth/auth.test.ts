import { describe, expect, it } from "vitest";
import { api, loginAdmin, registerUser, unique } from "../helpers";

describe("auth module", () => {
	it("logs in with the seeded admin account", async () => {
		// 凭据只在 helpers/api.ts 定义一处（可用 GOSPEAK_TEST_ADMIN_* 覆盖）。
		const data = await loginAdmin();
		expect(data.access_token).toBeTruthy();
		expect(data.user.role).toBe("admin");
	});

	it("registers a new user and rejects a duplicate username", async () => {
		const user = await registerUser("auth");
		expect(user.access_token).toBeTruthy();
		expect(user.user.name).toBe(user.username);

		const duplicate = await api("/api/v1/auth/register", {
			body: { username: user.username, password: user.password },
		});
		expect(duplicate.status).toBe(400);
		expect(duplicate.code).toBe(1012);
	});

	it("rejects a wrong password", async () => {
		const user = await registerUser("auth");
		const result = await api("/api/v1/auth/login", {
			body: { username: user.username, password: "wrong-password" },
		});
		expect(result.status).toBe(401);
		expect(result.code).toBe(1010);
	});

	it("refreshes the access token with a refresh token", async () => {
		const user = await registerUser("auth");
		const result = await api<{ access_token: string }>(
			"/api/v1/auth/refresh_token",
			{
				body: { refresh_token: user.refresh_token },
			},
		);
		expect(result.code).toBe(0);
		expect(result.data?.access_token).toBeTruthy();
	});

	it("requires a token on protected routes", async () => {
		const result = await api("/api/v1/user/profile");
		expect(result.status).toBe(401);
		expect(result.code).toBe(1001);
	});

	it("revokes the access token after logout", async () => {
		const user = await registerUser("auth");
		const logout = await api("/api/v1/auth/logout", {
			token: user.access_token,
			body: { refresh_token: user.refresh_token },
		});
		expect(logout.code).toBe(0);

		const afterLogout = await api("/api/v1/user/profile", {
			token: user.access_token,
		});
		expect(afterLogout.code).toBe(1014);
	});

	it("does not allow registering with the reserved bot prefix", async () => {
		const result = await api("/api/v1/auth/register", {
			body: { username: unique("bot_"), password: "secret123" },
		});
		expect(result.code).toBe(1012);
	});
});
