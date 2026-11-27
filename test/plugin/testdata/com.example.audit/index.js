import { createPlugin } from "@gospeak/plugin-sdk"

const plugin = await createPlugin({
	pluginId: "com.example.audit",
	name: "内容审核插件",
	version: "1.0.0",
	hookTypes: ["message.created"],
	onEvent(event, payload) {
		if (event !== "message.created") return
		const content = payload?.content ?? ""
		if (!content.includes("spam")) return
		plugin.api
			.muteUser({
				userUuid: payload.user_uuid,
				roomUuid: payload.room_uuid,
				reason: "automated audit: spam detected"
			})
			.then(() => plugin.kv.save("last_mute", { at: Date.now() }, "audit"))
			.catch((err) => plugin.reportError("muteUser failed", { message: String(err) }))
	}
})

plugin.on("room.joined", (payload) => {
	plugin.reportError("audit: room joined", { roomUuid: payload?.room_uuid }).catch(() => {})
})

process.on("SIGTERM", () => {
	plugin.close().finally(() => process.exit(0))
})
