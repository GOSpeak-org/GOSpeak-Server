package service

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
	"strings"
)

type RoomBrief struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Count       int    `json:"count"`
	HasPassword bool   `json:"has_password"`
	SortOrder   int    `json:"sort_order"`
}

// RoomGroupInput 是房间分组的 service 层 DTO；HTTP 边界只依赖本类型，
// repository.GroupInput 由 service 内部转换，不向 handler 泄漏。
type RoomGroupInput struct {
	UUID      string   `json:"uuid"`
	Name      string   `json:"name"`
	SortOrder int      `json:"sort_order"`
	RoomUUIDs []string `json:"room_uuids"`
}

type GroupLayout struct {
	UUID      string      `json:"uuid"`
	Name      string      `json:"name"`
	SortOrder int         `json:"sort_order"`
	Rooms     []RoomBrief `json:"rooms"`
}

type LayoutView struct {
	Groups            []GroupLayout `json:"groups"`
	Ungrouped         []RoomBrief   `json:"ungrouped"`
	UngroupedPosition int           `json:"ungrouped_position"`
	CanManage         bool          `json:"can_manage"`
}

type RoomLayoutService struct {
	repo *repository.RoomLayoutRepository
}

func NewRoomLayoutService(repo *repository.RoomLayoutRepository) *RoomLayoutService {
	return &RoomLayoutService{repo: repo}
}

func toBrief(r model.Room, order int) RoomBrief {
	return RoomBrief{
		UUID: r.UUID, Name: r.Name, Type: r.Type,
		HasPassword: r.Password != "", SortOrder: order,
	}
}

func (s *RoomLayoutService) GetLayout(domainUUID string, canManage bool) (*LayoutView, *pkg.AppError) {
	groups, err := s.repo.ListGroups(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	rooms, err := s.repo.DomainRooms(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	layout, err := s.repo.GetLayout(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	byGroup := make(map[string][]RoomBrief)
	var ungrouped []RoomBrief
	for _, r := range rooms {
		if r.GroupUUID == "" {
			ungrouped = append(ungrouped, toBrief(r, r.SortOrder))
		} else {
			byGroup[r.GroupUUID] = append(byGroup[r.GroupUUID], toBrief(r, r.SortOrder))
		}
	}
	view := &LayoutView{
		Groups:    make([]GroupLayout, 0, len(groups)),
		Ungrouped: ungrouped, UngroupedPosition: layout.UngroupedPosition,
		CanManage: canManage,
	}
	for _, g := range groups {
		items := byGroup[g.UUID]
		if items == nil {
			items = []RoomBrief{}
		}
		view.Groups = append(view.Groups, GroupLayout{
			UUID: g.UUID, Name: g.Name, SortOrder: g.SortOrder, Rooms: items,
		})
	}
	return view, nil
}

func (s *RoomLayoutService) SaveLayout(domainUUID string, groups []RoomGroupInput, ungroupedUUIDs []string, ungroupedPosition int) (*LayoutView, *pkg.AppError) {
	rooms, err := s.repo.DomainRooms(domainUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	valid := make(map[string]bool, len(rooms))
	for _, r := range rooms {
		valid[r.UUID] = true
	}
	seen := make(map[string]bool)
	names := make(map[string]bool)
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" || names[name] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "分组名不能为空且不能重复")
		}
		names[name] = true
		for _, uuid := range g.RoomUUIDs {
			if !valid[uuid] || seen[uuid] {
				return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "房间不存在或重复分配")
			}
			seen[uuid] = true
		}
	}
	for _, uuid := range ungroupedUUIDs {
		if !valid[uuid] || seen[uuid] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "房间不存在或重复分配")
		}
		seen[uuid] = true
	}
	if ungroupedPosition < 0 {
		ungroupedPosition = 0
	}
	if ungroupedPosition > len(groups) {
		ungroupedPosition = len(groups)
	}
	repoGroups := make([]repository.GroupInput, 0, len(groups))
	for _, g := range groups {
		repoGroups = append(repoGroups, repository.GroupInput{
			UUID: g.UUID, Name: g.Name, SortOrder: g.SortOrder, RoomUUIDs: g.RoomUUIDs,
		})
	}
	if err := s.repo.SaveLayout(domainUUID, repoGroups, ungroupedUUIDs, ungroupedPosition); err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	return s.GetLayout(domainUUID, true)
}
