import { describe, expect, it } from "vitest";
import {
	api,
	assertSuccess,
	createDomain,
	createRoom,
	defaultInviteCode,
	joinViaInvite,
	registerUser,
	unique,
} from "../helpers";

interface LayoutGroup {
	uuid: string;
	name: string;
	sort_order: number;
	rooms: Array<{ uuid: string; name: string }>;
}

interface LayoutView {
	groups: LayoutGroup[];
	ungrouped: Array<{ uuid: string; name: string }>;
	ungrouped_position: number;
	can_manage: boolean;
}

async function getLayout(token: string, domainUUID: string) {
	const result = await api<LayoutView>("/api/v1/domain/room-layout/get", {
		token,
		body: { domain_uuid: domainUUID },
	});
	return assertSuccess(result);
}

describe("room layout module", () => {
	it("returns rooms as ungrouped for a fresh domain", async () => {
		const owner = await registerUser("roomlayout");
		const domain = await createDomain(owner.access_token, unique("layout"));
		const lobby = await createRoom(
			owner.access_token,
			domain.uuid,
			unique("lobby"),
		);
		await createRoom(owner.access_token, domain.uuid, unique("text"), "text");

		const layout = await getLayout(owner.access_token, domain.uuid);
		expect(layout.groups).toEqual([]);
		expect(layout.can_manage).toBe(true);
		expect(layout.ungrouped.map((room) => room.uuid)).toContain(lobby.uuid);
	});

	it("creates a group and keeps sorted rooms across save and read", async () => {
		const owner = await registerUser("roomlayout");
		const domain = await createDomain(owner.access_token, unique("layout"));
		const lobby = await createRoom(
			owner.access_token,
			domain.uuid,
			unique("lobby"),
		);
		const squad = await createRoom(
			owner.access_token,
			domain.uuid,
			unique("squad"),
		);
		const chat = await createRoom(
			owner.access_token,
			domain.uuid,
			unique("chat"),
			"text",
		);

		const saved = assertSuccess(
			await api<LayoutView>("/api/v1/domain/room-layout/save", {
				token: owner.access_token,
				body: {
					domain_uuid: domain.uuid,
					groups: [
						{ name: "新分组", sort_order: 0, room_uuids: [lobby.uuid] },
						{ name: "上分队", sort_order: 1, room_uuids: [squad.uuid] },
					],
					ungrouped_position: 2,
					ungrouped_room_uuids: [chat.uuid],
				},
			}),
		);
		expect(saved.groups.map((group) => group.name)).toEqual([
			"新分组",
			"上分队",
		]);
		expect(saved.groups[0].uuid).not.toBe("");
		expect(saved.groups[0].rooms.map((room) => room.uuid)).toEqual([
			lobby.uuid,
		]);

		const reread = await getLayout(owner.access_token, domain.uuid);
		expect(reread.groups.map((group) => group.name)).toEqual([
			"新分组",
			"上分队",
		]);
		expect(reread.groups[1].rooms.map((room) => room.uuid)).toEqual([
			squad.uuid,
		]);
		expect(reread.ungrouped.map((room) => room.uuid)).toEqual([chat.uuid]);
		expect(reread.ungrouped_position).toBe(2);
	});

	it("updates an existing group instead of duplicating it", async () => {
		const owner = await registerUser("roomlayout");
		const domain = await createDomain(owner.access_token, unique("layout"));
		const lobby = await createRoom(
			owner.access_token,
			domain.uuid,
			unique("lobby"),
		);

		const first = assertSuccess(
			await api<LayoutView>("/api/v1/domain/room-layout/save", {
				token: owner.access_token,
				body: {
					domain_uuid: domain.uuid,
					groups: [{ name: "主分组", sort_order: 0, room_uuids: [lobby.uuid] }],
					ungrouped_position: 1,
					ungrouped_room_uuids: [],
				},
			}),
		);
		const groupUUID = first.groups[0].uuid;

		const second = assertSuccess(
			await api<LayoutView>("/api/v1/domain/room-layout/save", {
				token: owner.access_token,
				body: {
					domain_uuid: domain.uuid,
					groups: [
						{
							uuid: groupUUID,
							name: "改名后",
							sort_order: 0,
							room_uuids: [lobby.uuid],
						},
					],
					ungrouped_position: 1,
					ungrouped_room_uuids: [],
				},
			}),
		);
		expect(second.groups).toHaveLength(1);
		expect(second.groups[0].uuid).toBe(groupUUID);
		expect(second.groups[0].name).toBe("改名后");
	});

	it("rejects duplicate group names and rooms in two groups", async () => {
		const owner = await registerUser("roomlayout");
		const domain = await createDomain(owner.access_token, unique("layout"));
		const lobby = await createRoom(
			owner.access_token,
			domain.uuid,
			unique("lobby"),
		);

		const duplicateName = await api("/api/v1/domain/room-layout/save", {
			token: owner.access_token,
			body: {
				domain_uuid: domain.uuid,
				groups: [
					{ name: "同名", sort_order: 0, room_uuids: [] },
					{ name: "同名", sort_order: 1, room_uuids: [] },
				],
				ungrouped_position: 2,
				ungrouped_room_uuids: [],
			},
		});
		expect(duplicateName.code, JSON.stringify(duplicateName)).not.toBe(0);

		const duplicateRoom = await api("/api/v1/domain/room-layout/save", {
			token: owner.access_token,
			body: {
				domain_uuid: domain.uuid,
				groups: [{ name: "A", sort_order: 0, room_uuids: [lobby.uuid] }],
				ungrouped_position: 1,
				ungrouped_room_uuids: [lobby.uuid],
			},
		});
		expect(duplicateRoom.code, JSON.stringify(duplicateRoom)).not.toBe(0);
	});

	it("forbids members without the manage permission", async () => {
		const owner = await registerUser("roomlayout");
		const member = await registerUser("roomlayout");
		const domain = await createDomain(owner.access_token, unique("layout"));
		await createRoom(owner.access_token, domain.uuid, unique("lobby"));
		const code = await defaultInviteCode(owner.access_token, domain.uuid);
		await joinViaInvite(member.access_token, code);

		const result = await api("/api/v1/domain/room-layout/save", {
			token: member.access_token,
			body: {
				domain_uuid: domain.uuid,
				groups: [{ name: "越权", sort_order: 0, room_uuids: [] }],
				ungrouped_position: 1,
				ungrouped_room_uuids: [],
			},
		});
		expect(result.code, JSON.stringify(result)).not.toBe(0);
	});
});
