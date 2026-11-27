import { describe, expect, it } from "vitest";
import {
	api,
	assertSuccess,
	getAdminToken,
	registerUser,
	unique,
} from "../helpers";

describe("user module", () => {
	it("reads and updates the current profile", async () => {
		const user = await registerUser("user");

		const profile = await api<{ name: string; display_name: string }>(
			"/api/v1/user/profile",
			{
				token: user.access_token,
			},
		);
		expect(profile.code).toBe(0);
		expect(profile.data?.name).toBe(user.username);

		const updated = await api<{ name: string; display_name: string }>(
			"/api/v1/user/update-profile",
			{
				token: user.access_token,
				body: { display_name: "integration-profile" },
			},
		);
		expect(updated.code).toBe(0);
		expect(updated.data?.display_name).toBe("integration-profile");
	});

	it("resolves users by name and id for authenticated callers", async () => {
		const user = await registerUser("user");

		const byName = await api<{ id: number; name: string }>(
			"/api/v1/user/info",
			{
				token: user.access_token,
				body: { identity: user.username },
			},
		);
		expect(byName.code).toBe(0);
		expect(byName.data?.name).toBe(user.username);

		// /user/get 挂 user:read 权限门：user:read/role:read 是平台后台只读
		// 权限，按设计仅 admin 持有（model.DefaultRolePermissions 注释），
		// 普通用户持有会让侧栏管理入口对所有人可见。
		const admin = await getAdminToken();
		const byId = await api<{ uuid: string; name: string }>("/api/v1/user/get", {
			token: admin,
			body: { user_uuid: user.user.uuid },
		});
		expect(byId.code).toBe(0);
		expect(byId.data?.uuid).toBe(user.user.uuid);
	});

	it("allows admins to list and update roles", async () => {
		const admin = await getAdminToken();
		const user = await registerUser("user");

		const list = await api<{
			list: Array<{ id: number; name: string; role: string }>;
			total: number;
		}>("/api/v1/user/list", {
			token: admin,
			body: { page: 1, page_size: 100, keyword: user.username },
		});
		expect(list.code).toBe(0);
		expect(list.data?.list.some((item) => item.name === user.username)).toBe(
			true,
		);

		const promote = await api("/api/v1/user/update-role", {
			token: admin,
			body: { user_uuid: user.user.uuid, role: "admin" },
		});
		assertSuccess(promote);

		// 写操作必须回读：只断言 code=0 抓不到「返回成功但没落库」。
		const promoted = await api<{ role: string }>("/api/v1/user/get", {
			token: admin,
			body: { user_uuid: user.user.uuid },
		});
		expect(assertSuccess(promoted).role).toBe("admin");

		const demote = await api("/api/v1/user/update-role", {
			token: admin,
			body: { user_uuid: user.user.uuid, role: "user" },
		});
		assertSuccess(demote);

		const demoted = await api<{ role: string }>("/api/v1/user/get", {
			token: admin,
			body: { user_uuid: user.user.uuid },
		});
		expect(assertSuccess(demoted).role).toBe("user");
	});

	it("lets admins delete a user", async () => {
		const admin = await getAdminToken();
		const user = await registerUser("user");

		const deleted = await api("/api/v1/user/delete", {
			token: admin,
			body: { user_uuid: user.user.uuid },
		});
		expect(deleted.code).toBe(0);

		const getDeleted = await api("/api/v1/user/get", {
			token: admin,
			body: { user_uuid: user.user.uuid },
		});
		expect(getDeleted.code).toBe(3001);
	});

	it("supports current-user group CRUD", async () => {
		const user = await registerUser("user");
		const groupName = unique("group");

		const created = await api<{ id: number; group_name: string }>(
			"/api/v1/user-group/create",
			{
				token: user.access_token,
				body: { group_name: groupName },
			},
		);
		const group = assertSuccess(created);
		expect(group.id).toBeGreaterThan(0);
		expect(group.group_name).toBe(groupName);

		const listed = await api<{
			groups: Array<{ id: number; group_name: string }>;
		}>("/api/v1/user-group/list", {
			token: user.access_token,
		});
		expect(
			assertSuccess(listed).groups.some((item) => item.id === group.id),
		).toBe(true);

		const renamedGroupName = `${groupName}_renamed`;
		const renamed = await api("/api/v1/user-group/update", {
			token: user.access_token,
			body: { id: group.id, group_name: renamedGroupName },
		});
		assertSuccess(renamed);

		// 改名后回读列表：确认落库的是新名字，而不是接口只吞了参数。
		const relisted = await api<{
			groups: Array<{ id: number; group_name: string }>;
		}>("/api/v1/user-group/list", {
			token: user.access_token,
		});
		const renamedGroup = assertSuccess(relisted).groups.find(
			(item) => item.id === group.id,
		);
		expect(renamedGroup?.group_name).toBe(renamedGroupName);

		const deleted = await api("/api/v1/user-group/delete", {
			token: user.access_token,
			body: { id: group.id },
		});
		assertSuccess(deleted);
	});
});
