// GENERATED — 手改无效，跑 go run ./cmd/gen-protocol -out <dir>
// WS 事件 payload 类型：来源 app/server/internal/protocol/payloads.go
export interface AnnouncementPayload {
	uuid: string;
	scope_type: string;
	scope_id?: string;
	title: string;
	content: string;
	level: string;
	pinned: boolean;
	published_at?: number;
	expires_at?: number;
	created_by_uuid?: string;
	created_by_name: string;
	is_read: boolean;
	created_at: number;
}

export interface MemberInfo {
	user_uuid?: string;
	id: string;
	identity: string;
	name: string;
	display_name: string;
	avatar: string;
	is_muted: boolean;
	is_mic_muted: boolean;
	joined_at: number;
	stream?: string;
}

export interface MemberLeftPayload {
	room: string;
	domain_uuid?: string;
	identity: string;
	id: string;
	/** Enforcement 仅 member:kicked 后补发的 MemberLeft 事件携带。 */
	enforcement?: string;
}

export interface MemberMicStatePayload {
	room: string;
	domain_uuid?: string;
	is_mic_muted: boolean;
}

export interface MemberMutedStatePayload {
	identity: string;
	muted: boolean;
}

export interface MemberSpeakingPayload {
	room: string;
	domain_uuid?: string;
	speaking: boolean;
}

export interface MemberUpdatedPayload {
	room: string;
	domain_uuid?: string;
	identity: string;
	is_mic_muted: boolean;
}

export interface MessageDTO {
	uuid: string;
	room_uuid: string;
	author_id: string;
	author_uuid: string;
	author_name: string;
	author_avatar: string;
	content: string;
	reply_to?: string;
	mentions?: string[];
	edited_at?: string;
	deleted: boolean;
	created_at: string;
	client_nonce?: string;
	conversation_id?: string;
	target_identity?: string;
}

export interface MessageMutatePayload {
	room: string;
	domain_uuid?: string;
	message_uuid: string;
	content?: string;
	emoji?: string;
}

export interface MessageReactionPayload {
	action: string;
	message_uuid: string;
	user_id: string;
	emoji: string;
}

export interface MessageSendPayload {
	room: string;
	domain_uuid?: string;
	content: string;
	reply_to?: string;
	mentions?: string[];
	client_nonce?: string;
}

export interface MuteInfo {
	user_uuid: string;
	duration: number;
	permanent: boolean;
	reason: string;
	expires_at?: string;
	/** Enforcement 是媒体层强制结果：hard / degraded / soft，语义同 MemberLeftPayload.Enforcement。 */
	enforcement?: string;
}

export interface PrivateSendPayload {
	target_identity: string;
	content: string;
	client_nonce?: string;
}

export interface RoomActiveSpeakersPayload {
	room: string;
	domain_uuid?: string;
	identities: string[];
}

export interface RoomInfo {
	id: number;
	uuid: string;
	name: string;
	domain_uuid?: string;
	has_password: boolean;
	description: string;
	limit: number;
	type: string;
	audio_only: boolean;
	allow_audience: boolean;
	members: MemberInfo[];
	count: number;
	created_at: number;
	created_by?: string;
	group_uuid?: string;
	sort_order: number;
}

export interface RoomKickRequest {
	room: string;
	domain_uuid?: string;
	target_identity: string;
}

export interface RoomKickedPayload {
	room: string;
	domain_uuid?: string;
	target_identity: string;
	enforcement?: string;
}

export interface RoomLeftPayload {
	room: string;
	domain_uuid?: string;
}

export interface RoomListGroupPayload {
	uuid: string;
	name: string;
	sort_order: number;
}

export interface RoomListLayoutPayload {
	groups: RoomListGroupPayload[];
	ungrouped_position: number;
}

export interface RoomListRequest {
	domain_uuid?: string;
}

export interface RoomListResultPayload {
	rooms: RoomInfo[];
	count: number;
	layout: RoomListLayoutPayload;
}

export interface RoomRequest {
	room: string;
	/** DomainUUID 命名空间隔离：同一房间名在不同 Domain 下视为不同房间。 */
	domain_uuid?: string;
	password?: string;
	stream?: string;
}

export interface SFUProviderChangedPayload {
	provider: string;
}

export interface TypingPayload {
	room: string;
	domain_uuid?: string;
	identity: string;
	typing: boolean;
}
