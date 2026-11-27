import { describe, expect, it } from "vitest";
import {
	api,
	assertSuccess,
	connectWS,
	createDomain,
	createRoom,
	defaultInviteCode,
	joinViaInvite,
	registerUser,
	unique,
	waitFor,
} from "../helpers";

interface MessageData {
	uuid: string;
	room_uuid: string;
	author_id: string;
	content: string;
	edited_at?: string | null;
}

const react = (token: string, body: Record<string, string>) =>
	api("/api/v1/room/messages/react", { token, body });

const listMessages = async (
	token: string,
	roomUuid: string,
): Promise<MessageData[]> => {
	const result = await api<{ items: MessageData[] }>(
		"/api/v1/room/messages/list",
		{
			token,
			body: { room_uuid: roomUuid, limit: 50 },
		},
	);
	return result.data?.items ?? [];
};

describe("message module", () => {
	it("sends and mutates messages over WS, reads and reacts over REST", async () => {
		const author = await registerUser("message");
		const reader = await registerUser("message");
		const domain = await createDomain(
			author.access_token,
			unique("message_domain"),
		);
		await joinViaInvite(
			reader.access_token,
			await defaultInviteCode(author.access_token, domain.uuid),
		);
		const room = await createRoom(
			author.access_token,
			domain.uuid,
			unique("text_room"),
			"text",
		);

		const authorWs = await connectWS(author.access_token);
		const readerWs = await connectWS(reader.access_token);
		try {
			// 进房（room:join 需要 identity 与 JWT username 一致）
			const joined = await authorWs.emitAck("room:join", {
				room: room.name,
				identity: author.username,
				domain_uuid: domain.uuid,
			});
			expect(joined.error).toBeUndefined();
			const readerJoined = await readerWs.emitAck("room:join", {
				room: room.name,
				identity: reader.username,
				domain_uuid: domain.uuid,
			});
			// reader 进房失败会让后续用例在「房间没人」的假象下继续跑，必须显式断言。
			expect(readerJoined.error).toBeUndefined();

			// WS 发消息
			const sent = await authorWs.emitAck("message:send", {
				room: room.name,
				domain_uuid: domain.uuid,
				content: "hello integration",
			});
			expect(sent.error).toBeUndefined();

			// 回读用有界轮询：WS 写入经 jobs 异步落库（service.Send → chat.persist），
			// 立即读可能拿到旧快照；超时仍失败才算用例失败。
			const message = await waitFor(
				async () =>
					(await listMessages(reader.access_token, room.uuid)).find(
						(item) => item.content === "hello integration",
					),
				(found) => found !== undefined,
				{ label: "message:send persisted" },
			);
			expect(message.author_id).toBe(author.username);

			// WS 编辑
			const edited = await authorWs.emitAck("message:edit", {
				room: room.name,
				domain_uuid: domain.uuid,
				message_uuid: message.uuid,
				content: "edited integration",
			});
			expect(edited.error).toBeUndefined();

			const editedMessage = await waitFor(
				async () =>
					(await listMessages(reader.access_token, room.uuid)).find(
						(item) => item.uuid === message.uuid,
					),
				(item) => item?.content === "edited integration",
				{ label: "message:edit persisted" },
			);
			expect(editedMessage?.content).toBe("edited integration");

			// REST 表情（保留的 REST 死侧外端点）
			const reacted = await react(reader.access_token, {
				room_uuid: room.uuid,
				message_uuid: message.uuid,
				emoji: "+1",
			});
			expect(reacted.code).toBe(0);

			// WS 撤销表情
			const unreacted = await readerWs.emitAck("message:unreact", {
				room: room.name,
				domain_uuid: domain.uuid,
				message_uuid: message.uuid,
				emoji: "+1",
			});
			expect(unreacted.error).toBeUndefined();

			// 无 delete_others 权限的成员删除他人消息被拒：域成员角色只有
			// room:create/read + message:send/read，缺 message:delete_others
			// （model.DefaultDomainRolePermissions），service 返回 FORBIDDEN(1013)。
			const forbidden = await readerWs.emitAck("message:delete", {
				room: room.name,
				domain_uuid: domain.uuid,
				message_uuid: message.uuid,
			});
			expect(forbidden.error).toBeTruthy();
			expect(forbidden.code).toBe(1013);

			// 作者删除自己的消息
			const deleted = await authorWs.emitAck("message:delete", {
				room: room.name,
				domain_uuid: domain.uuid,
				message_uuid: message.uuid,
			});
			expect(deleted.error).toBeUndefined();

			await waitFor(
				async () =>
					(await listMessages(reader.access_token, room.uuid)).some(
						(item) => item.uuid === message.uuid,
					),
				(present) => !present,
				{ label: "message:delete persisted" },
			);
		} finally {
			authorWs.close();
			readerWs.close();
		}
	});

	it("searches room messages", async () => {
		const author = await registerUser("message");
		const domain = await createDomain(
			author.access_token,
			unique("message_domain"),
		);
		const room = await createRoom(
			author.access_token,
			domain.uuid,
			unique("text_room"),
			"text",
		);

		const ws = await connectWS(author.access_token);
		try {
			await ws.emitAck("room:join", {
				room: room.name,
				identity: author.username,
				domain_uuid: domain.uuid,
			});
			const marker = unique("needle");
			const sent = await ws.emitAck("message:send", {
				room: room.name,
				domain_uuid: domain.uuid,
				content: `searchable ${marker}`,
			});
			expect(sent.error).toBeUndefined();

			// 搜索同样读库，等异步落库完成再断言。
			const result = await waitFor(
				async () =>
					api<{ items: MessageData[] }>("/api/v1/room/messages/search", {
						token: author.access_token,
						body: { room_uuid: room.uuid, query: marker },
					}),
				(res) =>
					(res.data?.items ?? []).some((item) => item.content.includes(marker)),
				{ label: "message searchable after message:send" },
			);
			expect(
				assertSuccess(result).items.some((item) =>
					item.content.includes(marker),
				),
			).toBe(true);
		} finally {
			// 不关连接会让 server 端连接数持续增长，后续用例可能撞到连接/资源上限。
			ws.close();
		}
	});
});
