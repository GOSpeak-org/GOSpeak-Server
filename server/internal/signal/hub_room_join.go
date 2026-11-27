package signal

import (
	"GOSpeak/internal/bus"
	"GOSpeak/internal/logger"
	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/permcode"
	"GOSpeak/internal/protocol"
	"GOSpeak/internal/sfu"
	"GOSpeak/internal/ws"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// canCreateRoom 按 permcode.PermRoomCreate 判定建房权限，与 HTTP 通道一致。
// 生产由 gin.go 注入两个 checker；未注入时（嵌入式/测试）回退到成员关系，
// 并留一条 warn，避免漏装配静默变回无权限校验。
func (h *Hub) canCreateRoom(c ws.ClientMessenger, domainUUID string) bool {
	claims := c.Claims()
	userUUID := ""
	role := ""
	if claims != nil {
		userUUID = claims.UserUUID
		if userUUID == "" {
			userUUID = claims.Username
		}
		role = claims.Role
	}
	if domainUUID != "" {
		if h.domainPermChecker == nil {
			logger.WithComponent("Signal").Warn("[Signal] domain permission checker not injected; room:create falls back to membership only")
			return h.domainMemberAllowed(c, domainUUID)
		}
		return userUUID != "" && h.domainPermChecker(domainUUID, userUUID, permcode.PermRoomCreate)
	}
	if h.permChecker == nil {
		// 生产由 gin.go 注入 permSvc；未注入仅见于嵌入式/测试路径，放行以兼容旧装配。
		logger.WithComponent("Signal").Warn("[Signal] global permission checker not injected; platform room:create allows unguarded")
		return true
	}
	return permissionGranted(claims, role, permcode.PermRoomCreate, h.permChecker)
}

func (h *Hub) OnRoomCreate(c ws.ClientMessenger, data string) {
	var req RoomRequest
	if err := parseJSON(data, &req); err != nil || req.Room == "" {
		c.Send(map[string]interface{}{"event": EventRoomCreated, "data": map[string]interface{}{
			"error": "room name is required",
		}})
		return
	}

	if !h.domainMemberAllowed(c, req.DomainUUID) {
		c.Send(map[string]interface{}{"event": EventRoomCreated, "data": map[string]interface{}{
			"error": "not a member of this domain",
		}})
		return
	}
	// 与 HTTP POST /room/create 同口径：域房间按域角色权限判定，平台房间按
	// 全局角色权限判定。仅校验域成员不足以建房，否则任意成员可经 WS 绕过 PermRoomCreate。
	if !h.canCreateRoom(c, req.DomainUUID) {
		c.Send(map[string]interface{}{"event": EventRoomCreated, "data": map[string]interface{}{
			"error": "insufficient room permission",
		}})
		return
	}
	h.setClientDomain(c, req.DomainUUID)

	h.mu.Lock()
	if _, exists := h.rooms[roomKey(req.DomainUUID, req.Room)]; exists {
		h.mu.Unlock()
		c.Send(map[string]interface{}{"event": EventRoomCreated, "data": map[string]interface{}{
			"error": "room already exists",
		}})
		return
	}

	hashedPassword, hashErr := pkg.HashPassword(req.Password)
	if hashErr != nil {
		h.mu.Unlock()
		c.Send(map[string]interface{}{"event": EventRoomCreated, "data": map[string]interface{}{"error": "invalid password"}})
		return
	}
	h.rooms[roomKey(req.DomainUUID, req.Room)] = &Room{
		Name:       req.Room,
		Password:   hashedPassword,
		Members:    make(map[string]*MemberInfo),
		ByIdentity: make(map[string]*MemberInfo),
		MicMuted:   make(map[string]bool),
		Speaking:   make(map[string]bool),
		CreatedAt:  time.Now(),
		CreatedBy:  clientIdentity(c),
	}
	roomInfo := h.roomInfoLocked(roomKey(req.DomainUUID, req.Room))
	h.mu.Unlock()

	// 临时房间元数据同步到共享 KV，远端实例可校验密码/类型/人数上限。
	h.syncRoomMeta(roomKey(req.DomainUUID, req.Room), bus.RoomMeta{
		Name:        req.Room,
		DomainUUID:  req.DomainUUID,
		Password:    hashedPassword,
		Type:        model.RoomTypeVoice,
		CreatedAtMS: time.Now().UnixMilli(),
		OwnerNodeID: h.instanceID,
	})
	// 通知远端实例重新计算房间列表，临时房间不再只靠主动拉取刷新。
	h.notifyRoomStateChanged(roomKey(req.DomainUUID, req.Room))

	logger.WithComponent("Signal").Infof("[Signal] room created: %s by %s", req.Room, c.ID())

	c.Send(map[string]interface{}{"event": EventRoomCreated, "data": roomInfo})
	h.broadcastRoomUpdatedLocal(roomKey(req.DomainUUID, req.Room))
}

// ─── 加入房间 ───
//
// 加入流程分两阶段：
//  1. OnRoomJoin     — 信令面。仅校验权限 + fanout room join，不写 h.rooms 成员。
//  2. OnRoomJoinSFU  — 媒体面。SFU 连接确认后写成员 + 广播 MemberJoined/RoomUpdated。

// ─── 加入房间 ───
//
// 加入流程分两阶段：
//  1. OnRoomJoin     — 信令面。仅校验权限 + fanout room join，不写 h.rooms 成员。
//  2. OnRoomJoinSFU  — 媒体面。SFU 连接确认后写成员 + 广播 MemberJoined/RoomUpdated。
func (h *Hub) OnRoomJoin(c ws.ClientMessenger, data string) (string, error) {
	var req RoomRequest
	if err := parseJSON(data, &req); err != nil || req.Room == "" {
		return marshalAck(map[string]interface{}{
			"error": "room name is required",
		})
	}

	if !h.domainMemberAllowed(c, req.DomainUUID) {
		return marshalAck(map[string]interface{}{
			"error": "not a member of this domain",
		})
	}

	identity, err := resolveIdentity(c)
	if err != nil {
		return marshalAck(map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 被踢冷却：同一房间短时间内禁止重连。
	if h.isRejoinBlocked(roomKey(req.DomainUUID, req.Room), identity) {
		return marshalAck(map[string]interface{}{
			"error": "kicked, try again later",
			"room":  req.Room,
		})
	}

	// 密码校验（DB 为准）
	if ok, pwdErr := h.CheckRoomPassword(req.DomainUUID, req.Room, req.Password); !ok {
		if pwdErr != nil {
			return marshalAck(map[string]interface{}{
				"error": "room requires password",
			})
		}
		return marshalAck(map[string]interface{}{
			"error": "wrong room password",
		})
	}

	// 禁言检查（fail-closed）
	if h.muteStore != nil {
		muted, mute, muteErr := h.muteStore.IsMutedByIdentity(identity)
		if muteErr != nil {
			logger.WithComponent("Signal").Infof("[signal] IsMutedByIdentity error: identity=%q err=%v", identity, muteErr)
			return marshalAck(map[string]interface{}{
				"error": "mute check failed",
			})
		}
		if muted {
			remaining := int64(0)
			if !mute.Permanent && mute.ExpiresAt != nil {
				remaining = int64(time.Until(*mute.ExpiresAt).Seconds())
				if remaining < 0 {
					remaining = 0
				}
			}
			return marshalAck(map[string]interface{}{
				"error":     "user is muted",
				"muted":     true,
				"permanent": mute.Permanent,
				"remaining": remaining,
			})
		}
	}

	// 服务端校验房间人数上限（DB/KV 读失败时 fail-closed）
	if full, limit, count, err := h.CheckRoomLimit(req.DomainUUID, req.Room); err != nil {
		logger.WithComponent("Signal").Infof("[signal] room limit check error: room=%q err=%v", req.Room, err)
		return marshalAck(map[string]interface{}{"error": "room limit check failed"})
	} else if full {
		return marshalAck(map[string]interface{}{
			"error": "room is full",
			"limit": limit,
			"count": count,
		})
	}

	// 提前拦截重复身份：已通过 OnRoomJoinSFU 写入 h.rooms 的在线用户，新 socket 在 OnRoomJoin 阶段即阻断。
	// 并发场景由 OnRoomJoinSFU 的 duplicate check 兜底，此处为提前拦截以节省媒体连接开销。
	h.mu.RLock()
	dup := h.duplicateIdentityLocked(roomKey(req.DomainUUID, req.Room), identity, c.ID())
	h.mu.RUnlock()
	if dup {
		return marshalAck(map[string]interface{}{
			"error":    "duplicate connection not allowed",
			"room":     req.Room,
			"identity": identity,
		})
	}

	// Dual-slot tracking: resolve room type, manage text/voice slot per conn.
	key := roomKey(req.DomainUUID, req.Room)
	roomType := model.RoomTypeVoice
	if h.roomStore != nil {
		if dbRoom, dbErr := h.roomStore.GetByDomainAndName(req.DomainUUID, req.Room); dbErr == nil && dbRoom != nil {
			roomType = model.NormalizeRoomType(dbRoom.Type)
		}
	}
	if roomType == model.RoomTypeVoice {
		if meta, metaErr := h.getRoomMeta(roomKey(req.DomainUUID, req.Room)); metaErr == nil {
			roomType = model.NormalizeRoomType(meta.Type)
		}
	}

	// 解禁后重连自愈：离线期间 unmute 无法定位 stream（registry/KV 已清），
	// join 时以 DB 禁言状态为权威重建媒体状态。
	// 被禁言用户在上面的 fail-closed 检查即被拒绝，不会走到这里。
	// 仅语音房 + 黑名单型（degraded）provider 执行；LiveKit 等 hard track 级
	// mute 离线时随会话自然消失，不需要自愈清理。
	if roomType == model.RoomTypeVoice && h.sfuProvider != nil && h.sfuProvider.Capabilities().MuteLevel == sfu.EnforcementDegraded {
		unmuteErr := h.muteParticipantTimedOrPlain(key, identity, false, 0)
		if unmuteErr != nil {
			h.logMuteResidueError("clear mute residue failed", key, identity, unmuteErr)
		}

		// unmute 与管理员新禁言之间存在竞态窗口：unmute 刚清掉黑名单后，
		// 立即复查 DB；若已禁言，按当前禁言记录重新媒体 mute，避免清掉新黑名单。
		if h.muteStore != nil {
			muted, mute, recheckErr := h.muteStore.IsMutedByIdentity(identity)
			if recheckErr != nil {
				logger.Warnf("[signal] mute residue recheck failed, keep unmute result room=%s identity=%s err=%v", key, identity, recheckErr)
			} else if muted {
				remaining := int64(0)
				if mute != nil && !mute.Permanent {
					if mute.ExpiresAt != nil {
						remaining = int64(time.Until(*mute.ExpiresAt).Seconds())
						if remaining < 0 {
							remaining = 0
						}
					}
					if remaining <= 0 {
						// 已到期/剩余不足 1 秒：不 re-mute（避免把临时禁言误写成 ttl<=0），
						// 直接按禁言拒绝并跳过媒体重写。
						return marshalAck(map[string]interface{}{
							"error":     "user is muted",
							"muted":     true,
							"permanent": false,
							"remaining": 0,
						})
					}
				}
				if sfu.LevelEnabled(h.sfuProvider.Capabilities().MuteLevel) {
					ttlSeconds := sfu.PermanentMuteTTLSeconds
					if mute != nil && !mute.Permanent {
						ttlSeconds = int(remaining)
					}
					muteErr := h.muteParticipantTimedOrPlain(key, identity, true, ttlSeconds)
					if muteErr != nil {
						h.logMuteResidueError("re-mute new mute failed", key, identity, muteErr)
					}
				}
				// 管理员禁言路径已广播 member:muted，且用户未成功 join，
				// 这里只返回 muted ack，避免重复广播。
				return marshalAck(map[string]interface{}{
					"error":     "user is muted",
					"muted":     true,
					"permanent": mute != nil && mute.Permanent,
					"remaining": remaining,
				})
			}
		}
	}

	h.mu.Lock()
	slots := h.connSlots[c.ID()]
	if slots == nil {
		slots = &connRoomSlots{}
		h.connSlots[c.ID()] = slots
	}
	if roomType == model.RoomTypeText {
		// Switching text rooms: leave old slot
		if slots.TextRoom != "" && slots.TextRoom != key {
			h.fanout.Leave(slots.TextRoom, c.ID())
		}
		slots.TextRoom = key
		h.fanout.Join(key, c.ID())
	} else {
		// Voice slot does not switch until room:join:sfu succeeds.
		// Fanout join is safe here: receiving room events in the new room
		// before SFU confirm is harmless and avoids stranding the socket if
		// the join is rejected.
		// 首次加入/同房重试提前 join fanout；切换场景保持旧房 fanout，
		// 避免 phase2 失败时 socket 被孤立在新房且旧槽位未回滚。
		if slots.VoiceRoom == "" || slots.VoiceRoom == key {
			h.fanout.Join(key, c.ID())
		}
	}
	h.mu.Unlock()

	// 断连宽限期内同一 identity 重新走 room:join（语音房重连只补信令面注册、媒体仍在）
	// 也视为已重新加入：取消待执行的 SFU 清理，避免媒体 participant 被移除后用户掉线。
	h.cancelPendingCleanup(key, identity)

	if roomType == model.RoomTypeText {
		h.setClientDomain(c, req.DomainUUID)
	}
	logger.WithComponent("Signal").Infof("[Signal] %s (%s) signaling ready for room: %s (type=%s)", c.ID(), identity, req.Room, roomType)

	return marshalAck(map[string]interface{}{
		"room":     req.Room,
		"identity": identity,
	})
}

// logMuteResidueError 分级记录自愈清理错误：预期性失败（不支持或目标不存在）降级为 warn，
// 其余视为真实异常按 error 记录，避免离线重连场景高频刷 error 日志。
func (h *Hub) logMuteResidueError(action, room, identity string, err error) {
	if err == nil {
		return
	}
	if isExpectedMuteResidueError(err) {
		logger.Warnf("[signal] %s (expected) room=%s identity=%s err=%v", action, room, identity, err)
		return
	}
	logger.Errorf("[signal] %s room=%s identity=%s err=%v", action, room, identity, err)
}

// isExpectedMuteResidueError 识别 provider 在清理“不存在的参与者/流”时的预期失败：
// 不支持操作、NOT_FOUND 业务码，以及 provider 文案中的 participant/stream not found。
func isExpectedMuteResidueError(err error) bool {
	if errors.Is(err, pkg.ErrSFUNotSupported) || errors.Is(err, pkg.ErrNotFound) {
		return true
	}
	var appErr *pkg.AppError
	if errors.As(err, &appErr) && appErr.Code == pkg.NOT_FOUND {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "participant not found") || strings.Contains(msg, "stream not found")
}

// OnRoomJoinSFU 在 SFU 媒体连接确认后写成员并广播加入。

// OnRoomJoinSFU 在 SFU 媒体连接确认后写成员并广播加入。
func (h *Hub) OnRoomJoinSFU(c ws.ClientMessenger, data string) (string, error) {
	var req RoomRequest
	if err := parseJSON(data, &req); err != nil || req.Room == "" {
		return marshalAck(map[string]interface{}{"error": "room name is required"})
	}

	if !h.domainMemberAllowed(c, req.DomainUUID) {
		return marshalAck(map[string]interface{}{
			"error": "not a member of this domain",
		})
	}

	identity, err := resolveIdentity(c)
	if err != nil {
		return marshalAck(map[string]interface{}{"error": err.Error()})
	}

	// 被踢冷却：媒体面加入同样拦截。
	if h.isRejoinBlocked(roomKey(req.DomainUUID, req.Room), identity) {
		h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
		return marshalAck(map[string]interface{}{
			"error": "kicked, try again later",
			"room":  req.Room,
		})
	}

	// 房间在 DB / 共享 KV / 本地均不存在时，首次加入会把房间物化到 h.rooms。
	// 这条「凭空建临时房」路径必须持有建房权限，与 OnRoomCreate 同口径；
	// 加入已存在房间仍只需成员身份 + 密码，不在此追加建房权限判定。
	creatingRoom := !h.roomExists(req.DomainUUID, req.Room)
	if creatingRoom && !h.canCreateRoom(c, req.DomainUUID) {
		if h.fanout != nil {
			h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
		}
		return marshalAck(map[string]interface{}{"error": "insufficient room permission"})
	}
	// 密码在锁外先算好，避免持 h.mu 期间执行 bcrypt。
	var tempRoomPassword string
	if creatingRoom {
		hashedPassword, hashErr := pkg.HashPassword(req.Password)
		if hashErr != nil {
			if h.fanout != nil {
				h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
			}
			return marshalAck(map[string]interface{}{"error": "invalid password"})
		}
		tempRoomPassword = hashedPassword
	} else if meta, metaErr := h.getRoomMeta(roomKey(req.DomainUUID, req.Room)); metaErr == nil {
		// 房间仅存在于共享 KV（跨实例/重启）时，用 meta 中的密码物化本地房间，
		// 否则本地 Room.Password 为空会让 CheckRoomPassword 误判为无密码房而放行。
		tempRoomPassword = meta.Password
	}

	// Text room guard: text rooms have no media, reject SFU join.
	roomType := ""
	if h.roomStore != nil {
		if dbRoom, dbErr := h.roomStore.GetByDomainAndName(req.DomainUUID, req.Room); dbErr == nil && dbRoom != nil {
			roomType = model.NormalizeRoomType(dbRoom.Type)
		}
	}
	if roomType == "" {
		if meta, metaErr := h.getRoomMeta(roomKey(req.DomainUUID, req.Room)); metaErr == nil {
			roomType = model.NormalizeRoomType(meta.Type)
		}
	}
	if roomType == model.RoomTypeText {
		return marshalAck(map[string]interface{}{"error": "text room has no media"})
	}

	// phase2 复检 mute / limit / password，避免 phase1 通过后状态变化
	if ok, pwdErr := h.CheckRoomPassword(req.DomainUUID, req.Room, req.Password); !ok {
		h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
		if pwdErr != nil {
			return marshalAck(map[string]interface{}{"error": "room requires password"})
		}
		return marshalAck(map[string]interface{}{"error": "wrong room password"})
	}
	if h.muteStore != nil {
		muted, _, muteErr := h.muteStore.IsMutedByIdentity(identity)
		if muteErr != nil {
			h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
			return marshalAck(map[string]interface{}{"error": "mute check failed"})
		}
		if muted {
			h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
			return marshalAck(map[string]interface{}{"error": "user is muted", "muted": true})
		}
	}
	full, limit, count, limitErr := h.CheckRoomLimit(req.DomainUUID, req.Room)
	if limitErr != nil {
		h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
		logger.WithComponent("Signal").Infof("[signal] sfu join room limit check error: room=%q err=%v", req.Room, limitErr)
		return marshalAck(map[string]interface{}{"error": "room limit check failed"})
	}
	if full {
		h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
		return marshalAck(map[string]interface{}{"error": "room is full", "limit": limit, "count": count})
	}

	// 服务端覆写 stream：客户端提报值不可信，使用服务端基于 room+identity 计算的预期值
	if h.streamResolver != nil {
		req.Stream = h.streamResolver.StreamName(roomKey(req.DomainUUID, req.Room), identity)
	}

	member := &MemberInfo{
		ID:       c.ID(),
		Identity: identity,
		JoinedAt: time.Now().UnixMilli(),
		Stream:   req.Stream,
	}

	newKey := roomKey(req.DomainUUID, req.Room)
	type switchCleanup struct {
		key         string
		identity    string
		stream      string
		wasSpeaking bool
		roomDeleted bool
	}
	var oldCleanups []*switchCleanup

	// 锁外执行 KV 注册，避免后端抖动时持全局写锁阻塞整机信令。
	// 锁内只构造注册快照与旧房间清理信息。
	now := time.Now().UnixMilli()
	regLocal := make([]bus.MemberRecord, 0, 2)
	h.mu.Lock()
	// 重复身份检查：同一房间不允许同一 identity 用不同 socket 重复加入
	if h.duplicateIdentityLocked(newKey, identity, c.ID()) {
		h.mu.Unlock()
		h.fanout.Leave(newKey, c.ID())
		return marshalAck(map[string]interface{}{
			"error":    "duplicate connection not allowed",
			"room":     req.Room,
			"identity": identity,
		})
	}
	if room, ok := h.rooms[newKey]; ok {
		for sid, m := range room.Members {
			if m == nil {
				continue
			}
			regLocal = append(regLocal, bus.MemberRecord{
				Room:        newKey,
				Identity:    m.Identity,
				SocketHint:  sid,
				InstanceID:  h.instanceID,
				Stream:      m.Stream,
				MicMuted:    room.MicMuted[m.Identity],
				Speaking:    room.Speaking[m.Identity],
				UpdatedAtMS: now,
				ExpiresAtMS: now + membershipLeaseDuration.Milliseconds(),
			})
		}
	}
	regLocal = append(regLocal, bus.MemberRecord{
		Room:        newKey,
		Identity:    identity,
		SocketHint:  c.ID(),
		InstanceID:  h.instanceID,
		Stream:      req.Stream,
		UpdatedAtMS: now,
		ExpiresAtMS: now + membershipLeaseDuration.Milliseconds(),
	})
	// 收集旧房间清理信息（暂不删除，注册失败时保持原状）
	for oldKey, oldRoom := range h.rooms {
		if oldKey == newKey {
			continue
		}
		oldMember, ok := oldRoom.Members[c.ID()]
		if !ok {
			continue
		}
		oldCleanups = append(oldCleanups, &switchCleanup{
			key:         oldKey,
			identity:    oldMember.Identity,
			stream:      oldMember.Stream,
			wasSpeaking: oldRoom.Speaking[oldMember.Identity],
		})
	}
	h.mu.Unlock()
	// 本地预提交（锁内）后再写 KV：即使进程在 KV 写后崩溃，远端也能通过本地
	// 提交后的广播看到成员，避免“KV 有成员但本地未提交”的幽灵窗口。
	// 旧房间清理推迟到 KV 成功之后，KV 失败时新房间回滚、旧房间保持原状。
	var previousVoiceRoom string
	h.mu.Lock()
	slots := h.connSlots[c.ID()]
	if slots != nil {
		previousVoiceRoom = slots.VoiceRoom
	} else {
		slots = &connRoomSlots{}
		h.connSlots[c.ID()] = slots
	}
	slots.VoiceRoom = newKey
	if h.fanout != nil {
		h.fanout.Join(newKey, c.ID())
	}

	room, exists := h.rooms[newKey]
	if !exists {
		room = &Room{
			Name:       req.Room,
			Password:   tempRoomPassword,
			Members:    make(map[string]*MemberInfo),
			ByIdentity: make(map[string]*MemberInfo),
			MicMuted:   make(map[string]bool),
			Speaking:   make(map[string]bool),
			CreatedAt:  time.Now(),
			CreatedBy:  identity,
		}
		h.rooms[newKey] = room
	}
	roomSetMember(room, c.ID(), identity, member)
	memberList := h.memberSnapshotLocked(newKey)
	h.mu.Unlock()

	if regErr := h.registerRoomMembers(newKey, regLocal, int(limit), identity); regErr != nil {
		h.fanout.Leave(newKey, c.ID())
		h.mu.Lock()
		h.rollbackLocalJoinLocked(newKey, c.ID(), identity, req.Stream, previousVoiceRoom)
		h.mu.Unlock()
		switch {
		case errors.Is(regErr, errDuplicateRemoteIdentity):
			return marshalAck(map[string]interface{}{
				"error":    "duplicate connection not allowed",
				"room":     req.Room,
				"identity": identity,
			})
		case errors.Is(regErr, errRoomLimitExceeded):
			return marshalAck(map[string]interface{}{"error": "room is full", "limit": limit, "count": count})
		default:
			logger.WithComponent("Signal").Infof("[Signal] sfu join registration failed room=%s identity=%s: %v", req.Room, identity, regErr)
			return marshalAck(map[string]interface{}{"error": "room join failed"})
		}
	}

	// KV 注册期间可能有其他 socket 完成同身份加入，锁内再次校验并回滚。
	h.mu.Lock()
	if h.duplicateIdentityLocked(newKey, identity, c.ID()) {
		h.rollbackLocalJoinLocked(newKey, c.ID(), identity, req.Stream, previousVoiceRoom)
		h.mu.Unlock()
		// 回滚已写入 KV 的本实例记录，避免残留成员。
		h.syncRoomToStore(newKey)
		h.fanout.Leave(newKey, c.ID())
		return marshalAck(map[string]interface{}{
			"error":    "duplicate connection not allowed",
			"room":     req.Room,
			"identity": identity,
		})
	}

	// KV 成功后再清理旧房间成员，失败路径不会触碰旧房状态。
	for _, old := range oldCleanups {
		oldRoom, ok := h.rooms[old.key]
		if !ok {
			continue
		}
		if _, ok := oldRoom.Members[c.ID()]; ok {
			delete(oldRoom.Speaking, old.identity)
			delete(oldRoom.MicMuted, old.identity)
		}
		roomDelMember(oldRoom, c.ID())
		old.roomDeleted = h.deleteRoomIfEmptyLocked(old.key)
		if h.fanout != nil {
			h.fanout.Leave(old.key, c.ID())
		}
	}
	h.mu.Unlock()

	for _, old := range oldCleanups {
		h.syncRoomToStore(old.key)
		if old.stream != "" {
		}
		oldDomain, oldRoomName := splitRoomKey(old.key)
		h.publishRoom(old.key, EventMemberLeft, protocol.MemberLeftPayload{
			Room:       oldRoomName,
			DomainUUID: oldDomain,
			Identity:   old.identity,
			ID:         c.ID(),
		})
		h.removeParticipantSafe(old.key, old.identity)
		if old.wasSpeaking && !old.roomDeleted {
			h.broadcastActiveSpeakers(oldDomain, oldRoomName)
		}
		if old.roomDeleted {
			h.deleteRoomSafe(old.key)
			h.broadcastRoomList(oldDomain)
		} else {
			h.broadcastRoomUpdatedLocal(old.key)
		}
	}

	h.ensureRoomOwnership(newKey, req, room)
	memberList = h.enrichMembers(memberList)
	if req.Stream != "" {
	}
	h.setClientDomain(c, req.DomainUUID)

	logger.WithComponent("Signal").Infof("[Signal] sfu confirmed: %s in %s", c.ID(), req.Room)

	h.mu.RLock()
	_, memberExists := h.rooms[roomKey(req.DomainUUID, req.Room)]
	if memberExists {
		_, memberExists = h.rooms[roomKey(req.DomainUUID, req.Room)].Members[c.ID()]
	}
	h.mu.RUnlock()
	if !memberExists {
		logger.WithComponent("Signal").Infof("[Signal] sfu join rejected: %s not in room %s", c.ID(), req.Room)
		// 回退 phase1 的 fanout.Join，避免 socket 残留在 WS room
		h.fanout.Leave(roomKey(req.DomainUUID, req.Room), c.ID())
		return marshalAck(map[string]interface{}{
			"error": "not in room",
		})
	}

	// 断连宽限期内重新加入同一房间：取消待执行的 SFU 清理，媒体 participant 得以保留。
	h.cancelPendingCleanup(newKey, identity)

	h.publishRoom(roomKey(req.DomainUUID, req.Room), EventMemberJoined, map[string]interface{}{
		"room":        req.Room,
		"domain_uuid": req.DomainUUID,
		"identity":    identity,
		"id":          c.ID(),
		"stream":      req.Stream,
	})

	h.broadcastRoomUpdatedLocal(roomKey(req.DomainUUID, req.Room))

	// 加入回放：新成员此刻已在 fanout，向房间广播当前 active speakers，
	// 让无 SFU 原生检测的 provider 下，加入者立即看到正在发言的人。
	h.broadcastActiveSpeakers(req.DomainUUID, req.Room)

	return marshalAck(map[string]interface{}{
		"ok":       true,
		"room":     req.Room,
		"identity": identity,
		"members":  memberList,
	})
}

func marshalAck(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"error":"serialization failed"}`, nil
	}
	return string(data), nil
}

// ─── 离开房间 ───

// rollbackLocalJoinLocked 回滚 OnRoomJoinSFU 的本地预提交状态；调用方须持有 h.mu。
func (h *Hub) rollbackLocalJoinLocked(newKey, clientID, identity, stream, prevVoiceRoom string) {
	if room, ok := h.rooms[newKey]; ok {
		roomDelMember(room, clientID)
		delete(room.Speaking, identity)
		delete(room.MicMuted, identity)
		if len(room.Members) == 0 {
			delete(h.rooms, newKey)
		}
	}
	if slots := h.connSlots[clientID]; slots != nil {
		slots.VoiceRoom = prevVoiceRoom
		if prevVoiceRoom == "" && slots.TextRoom == "" {
			delete(h.connSlots, clientID)
		}
	}
}
