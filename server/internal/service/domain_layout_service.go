package service

import (
	"GOSpeak/internal/model"
	"GOSpeak/internal/pkg"
	"GOSpeak/internal/repository"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxLayoutGroups = 20

type LayoutGroup struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

type LayoutItem struct {
	DomainUUID string `json:"domain_uuid"`
	GroupUUID  string `json:"group_uuid"`
	SortOrder  int    `json:"sort_order"`
}

// MarshalJSON 把未分组条目的 group_uuid 输出为 null，与可空列语义及前端类型保持一致。
func (i LayoutItem) MarshalJSON() ([]byte, error) {
	if i.GroupUUID != "" {
		type alias LayoutItem
		return json.Marshal(alias(i))
	}
	return json.Marshal(struct {
		DomainUUID string      `json:"domain_uuid"`
		GroupUUID  interface{} `json:"group_uuid"`
		SortOrder  int         `json:"sort_order"`
	}{DomainUUID: i.DomainUUID, SortOrder: i.SortOrder})
}

type Layout struct {
	Groups []LayoutGroup `json:"groups"`
	Items  []LayoutItem  `json:"items"`
}

type DomainLayoutService struct {
	layoutRepo *repository.UserDomainLayoutRepository
}

func NewDomainLayoutService(layoutRepo *repository.UserDomainLayoutRepository) *DomainLayoutService {
	return &DomainLayoutService{layoutRepo: layoutRepo}
}

var layoutColors = map[string]bool{
	"lime": true, "sky": true, "rose": true,
	"amber": true, "violet": true, "slate": true,
}

func normalizeLayoutGroups(groups []LayoutGroup) []LayoutGroup {
	out := make([]LayoutGroup, len(groups))
	copy(out, groups)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].UUID < out[j].UUID
	})
	for i := range out {
		out[i].SortOrder = i
	}
	return out
}

func layoutContainerOrder(groups []LayoutGroup, container string) int {
	for i, group := range groups {
		if group.UUID == container {
			return i
		}
	}
	return len(groups)
}

// normalizeLayoutItems 按容器（分组或未分组）排序并把 sort_order 归一化为 0..n-1，
// 输出顺序为各分组依次展开、最后接未分组。引用已删除分组的条目自动落入未分组。
func normalizeLayoutItems(groups []LayoutGroup, items []LayoutItem) []LayoutItem {
	buckets := make(map[string][]LayoutItem, len(groups)+1)
	for _, item := range items {
		container := item.GroupUUID
		if layoutContainerOrder(groups, container) == len(groups) {
			container = ""
		}
		buckets[container] = append(buckets[container], item)
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return layoutContainerOrder(groups, keys[i]) < layoutContainerOrder(groups, keys[j])
	})

	result := make([]LayoutItem, 0, len(items))
	for _, key := range keys {
		bucket := buckets[key]
		sort.SliceStable(bucket, func(i, j int) bool {
			if bucket[i].SortOrder != bucket[j].SortOrder {
				return bucket[i].SortOrder < bucket[j].SortOrder
			}
			return bucket[i].DomainUUID < bucket[j].DomainUUID
		})
		for i := range bucket {
			bucket[i].GroupUUID = key
			bucket[i].SortOrder = i
			result = append(result, bucket[i])
		}
	}
	return result
}

func (s *DomainLayoutService) GetLayout(userUUID string) (*Layout, *pkg.AppError) {
	storedGroups, err := s.layoutRepo.ListGroups(userUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	rawGroups := make([]LayoutGroup, 0, len(storedGroups))
	for _, group := range storedGroups {
		rawGroups = append(rawGroups, LayoutGroup{
			UUID: group.UUID, Name: group.Name, Color: group.Color, SortOrder: group.SortOrder,
		})
	}
	groups := normalizeLayoutGroups(rawGroups)
	groupSet := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupSet[group.UUID] = true
	}

	storedItems, err := s.layoutRepo.ListItems(userUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	joined, err := s.layoutRepo.JoinedDomainUUIDs(userUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	joinedSet := make(map[string]bool, len(joined))
	for _, domainUUID := range joined {
		joinedSet[domainUUID] = true
	}

	items := make([]LayoutItem, 0, len(joined))
	seen := make(map[string]bool, len(joined))
	for _, item := range storedItems {
		if !joinedSet[item.DomainUUID] || seen[item.DomainUUID] {
			continue
		}
		seen[item.DomainUUID] = true
		groupUUID := ""
		if item.GroupUUID.Valid && groupSet[item.GroupUUID.String] {
			groupUUID = item.GroupUUID.String
		}
		items = append(items, LayoutItem{
			DomainUUID: item.DomainUUID, GroupUUID: groupUUID, SortOrder: item.SortOrder,
		})
	}

	ungroupedCount := 0
	for _, item := range items {
		if item.GroupUUID == "" {
			ungroupedCount++
		}
	}
	for _, domainUUID := range joined {
		if seen[domainUUID] {
			continue
		}
		seen[domainUUID] = true
		items = append(items, LayoutItem{DomainUUID: domainUUID, SortOrder: ungroupedCount})
		ungroupedCount++
	}

	return &Layout{Groups: groups, Items: normalizeLayoutItems(groups, items)}, nil
}

func (s *DomainLayoutService) SaveLayout(userUUID string, layout *Layout) (*Layout, *pkg.AppError) {
	if layout == nil {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "layout is required")
	}
	if len(layout.Groups) > maxLayoutGroups {
		return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "too many groups")
	}

	groups := append([]LayoutGroup(nil), layout.Groups...)
	seenUUID := make(map[string]bool, len(groups))
	seenName := make(map[string]bool, len(groups))
	for i := range groups {
		group := &groups[i]
		group.UUID = strings.TrimSpace(group.UUID)
		if group.UUID == "" {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "group uuid is required")
		}
		if seenUUID[group.UUID] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "duplicate group uuid")
		}
		seenUUID[group.UUID] = true

		name := strings.TrimSpace(group.Name)
		if name == "" || utf8.RuneCountInString(name) > 16 {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "group name must be 1-16 characters")
		}
		if seenName[name] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "duplicate group name")
		}
		seenName[name] = true
		group.Name = name

		if !layoutColors[group.Color] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "invalid group color")
		}
	}
	groups = normalizeLayoutGroups(groups)
	groupSet := make(map[string]bool, len(groups))
	for _, group := range groups {
		groupSet[group.UUID] = true
	}

	existingGroups, err := s.layoutRepo.ListGroups(userUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	uuids := make([]string, 0, len(groups))
	for _, group := range groups {
		uuids = append(uuids, group.UUID)
	}
	ownedGroups, err := s.layoutRepo.GroupsByUUIDs(uuids)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	for _, group := range ownedGroups {
		if group.UserUUID != userUUID {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "group does not belong to user")
		}
	}
	existingSet := make(map[string]bool, len(existingGroups))
	for _, group := range existingGroups {
		existingSet[group.UUID] = true
	}

	joined, err := s.layoutRepo.JoinedDomainUUIDs(userUUID)
	if err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	joinedSet := make(map[string]bool, len(joined))
	for _, domainUUID := range joined {
		joinedSet[domainUUID] = true
	}

	items := make([]LayoutItem, 0, len(layout.Items))
	seenDomain := make(map[string]bool, len(layout.Items))
	for _, item := range layout.Items {
		if !joinedSet[item.DomainUUID] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "domain is not joined")
		}
		if seenDomain[item.DomainUUID] {
			return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "duplicate domain in layout")
		}
		seenDomain[item.DomainUUID] = true
		if item.GroupUUID != "" && !groupSet[item.GroupUUID] {
			if !existingSet[item.GroupUUID] {
				return nil, pkg.NewAppError(pkg.INVALID_PARAMS, "group does not belong to user")
			}
			item.GroupUUID = ""
		}
		items = append(items, item)
	}
	items = normalizeLayoutItems(groups, items)

	modelGroups := make([]model.UserDomainGroup, 0, len(groups))
	for _, group := range groups {
		modelGroups = append(modelGroups, model.UserDomainGroup{
			UUID: group.UUID, Name: group.Name, Color: group.Color, SortOrder: group.SortOrder,
		})
	}
	modelItems := make([]model.UserDomainLayoutItem, 0, len(items))
	for _, item := range items {
		group := sql.NullString{}
		if item.GroupUUID != "" {
			group = sql.NullString{String: item.GroupUUID, Valid: true}
		}
		modelItems = append(modelItems, model.UserDomainLayoutItem{
			DomainUUID: item.DomainUUID, GroupUUID: group, SortOrder: item.SortOrder,
		})
	}
	if err := s.layoutRepo.Replace(userUUID, modelGroups, modelItems); err != nil {
		return nil, pkg.NewAppError(pkg.INTERNAL_ERROR, err.Error())
	}
	return s.GetLayout(userUUID)
}
