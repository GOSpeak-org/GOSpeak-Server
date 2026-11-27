import { describe, expect, it } from "vitest";
import { api, assertSuccess, registerUser, waitFor } from "../helpers";

interface ConversationSummary {
	conversation_id: string;
	other_identity: string;
	unread_count: number;
}

interface ConversationMessages {
	messages: Array<{ content: string; author_id: string }>;
}

describe("conversation module", () => {
	it("sends, lists, reads and marks private messages read", async () => {
		const sender = await registerUser("conv");
		const receiver = await registerUser("conv");

		const sent = await api<{
			conversation_id: string;
			content: string;
			target_identity: string;
		}>("/api/v1/conversation/send", {
			token: sender.access_token,
			body: { target_identity: receiver.username, content: "private hello" },
		});
		const sentData = assertSuccess(sent);
		const conversationId = sentData.conversation_id;
		expect(conversationId).toBeTruthy();
		expect(sentData.target_identity).toBe(receiver.username);

		const listConversations = async (): Promise<ConversationSummary[]> =>
			assertSuccess(
				await api<ConversationSummary[]>("/api/v1/conversation/list", {
					token: receiver.access_token,
					body: {},
				}),
			);

		// 私聊落库同样经 jobs 异步队列，立即回读可能拿到 unread_count=0 的旧快照。
		const conversation = await waitFor(
			async () =>
				(await listConversations()).find(
					(item) => item.conversation_id === conversationId,
				),
			(item) => item?.unread_count === 1,
			{ label: "conversation unread_count after send" },
		);
		expect(conversation?.other_identity).toBe(sender.username);
		expect(conversation?.unread_count).toBe(1);

		const messages = await waitFor(
			async () =>
				assertSuccess(
					await api<ConversationMessages>("/api/v1/conversation/messages", {
						token: receiver.access_token,
						body: { conversation_id: conversationId },
					}),
				),
			(data) => data.messages.some((item) => item.content === "private hello"),
			{ label: "private message persisted" },
		);
		expect(
			messages.messages.some((item) => item.content === "private hello"),
		).toBe(true);

		const markedRead = await api("/api/v1/conversation/mark-read", {
			token: receiver.access_token,
			body: { conversation_id: conversationId },
		});
		assertSuccess(markedRead);

		const afterRead = await waitFor(
			async () =>
				(await listConversations()).find(
					(item) => item.conversation_id === conversationId,
				),
			(item) => item?.unread_count === 0,
			{ label: "conversation marked read" },
		);
		expect(afterRead?.unread_count).toBe(0);
	});
});
