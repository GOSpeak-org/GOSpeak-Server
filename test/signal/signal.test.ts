import { describe, expect, it } from "vitest";
import { api, assertSuccess, registerUser, unique } from "../helpers";

describe("signal module", () => {
	it("issues a join token for an authenticated user", async () => {
		const user = await registerUser("signal");
		const room = unique("voice_room");
		const result = await api<{
			token: string;
			provider: string;
			sfuRoom: string;
			identity: string;
		}>("/api/v1/signal/token", {
			token: user.access_token,
			body: { room, domain_uuid: "", identity: "ignored" },
		});
		const joinToken = assertSuccess(result);
		expect(joinToken.token).toBeTruthy();
		// identity 以 JWT 为准，客户端传的 "ignored" 必须被忽略。
		expect(joinToken.identity).toBe(user.username);
		// provider 只有 livekit 一种（sfu 工厂约束）；平台房间（domain_uuid 空）
		// 的 SFU 房间键就是逻辑房间名（pkg.RoomKey）。
		expect(joinToken.provider).toBe("livekit");
		expect(joinToken.sfuRoom).toBe(room);
	});

	it("generates a livekit join token", async () => {
		const user = await registerUser("signal");
		const room = unique("voice_room");
		const result = await api<{
			token: string;
			provider: string;
			sfuRoom: string;
		}>("/api/v1/signal/token", {
			token: user.access_token,
			body: { room, domain_uuid: "", identity: "ignored" },
		});
		expect(result.code).toBe(0);
		expect(result.data?.token).toBeTruthy();
		expect(result.data?.provider).toBe("livekit");
		expect(result.data?.sfuRoom).toBe(room);
	});
});
