import { describe, expect, it } from "vitest";
import { api, getAdminToken, registerUser } from "../helpers";

describe("mute module", () => {
	it("blocks regular users from managing mutes", async () => {
		const user = await registerUser("mute");
		const result = await api("/api/v1/mute/create", {
			token: user.access_token,
			body: { user_uuid: user.user.uuid, permanent: true },
		});
		expect(result.code).toBe(1013);
	});

	it("creates, lists, checks and cancels a permanent mute", async () => {
		const admin = await getAdminToken();
		const target = await registerUser("mute");

		const created = await api<{ user_uuid: string; permanent: boolean }>(
			"/api/v1/mute/create",
			{
				token: admin,
				body: {
					user_uuid: target.user.uuid,
					permanent: true,
					reason: "integration test",
				},
			},
		);
		expect(created.code).toBe(0);
		expect(created.data?.user_uuid).toBe(target.user.uuid);
		expect(created.data?.permanent).toBe(true);

		const status = await api<{ user_uuid: string; permanent: boolean }>(
			"/api/v1/mute/status",
			{
				token: admin,
				body: { user_uuid: target.user.uuid },
			},
		);
		expect(status.code).toBe(0);
		expect(status.data?.user_uuid).toBe(target.user.uuid);

		const list = await api<Array<{ user_uuid: string }>>("/api/v1/mute/list", {
			token: admin,
		});
		expect(list.data?.some((item) => item.user_uuid === target.user.uuid)).toBe(
			true,
		);

		const cancelled = await api("/api/v1/mute/cancel", {
			token: admin,
			body: { user_uuid: target.user.uuid },
		});
		expect(cancelled.code).toBe(0);

		const afterCancel = await api<null>("/api/v1/mute/status", {
			token: admin,
			body: { user_uuid: target.user.uuid },
		});
		expect(afterCancel.code).toBe(0);
		expect(afterCancel.data).toBeNull();
	});
});
