package service

import (
	"errors"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"

	"github.com/google/uuid"
)

type CommunityService struct {
	repo repository.CommunityRepository
}

func NewCommunityService() *CommunityService {
	// 从全局数据库连接创建repo
	repo := repository.NewCommunityRepository(db.GetDB())
	return &CommunityService{repo: repo}
}

func (s *CommunityService) GetGroups(page, pageSize int, search string) ([]*dto.CommunityGroupResponse, int64, error) {
	groups, total, err := s.repo.GetGroups(page, pageSize, search)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*dto.CommunityGroupResponse, 0, len(groups))
	for _, group := range groups {
		resp := &dto.CommunityGroupResponse{
			ID:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			MemberCount: group.MemberCount,
			CreatedAt:   group.CreatedAt,
			UpdatedAt:   group.UpdatedAt,
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}

func (s *CommunityService) GetGroupByID(id string) (*dto.CommunityGroupResponse, error) {
	group, err := s.repo.GetGroupByID(id)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, errors.New("社群不存在")
	}

	resp := &dto.CommunityGroupResponse{
		ID:          group.ID,
		Name:        group.Name,
		Description: group.Description,
		MemberCount: group.MemberCount,
		CreatedAt:   group.CreatedAt,
		UpdatedAt:   group.UpdatedAt,
	}
	return resp, nil
}

func (s *CommunityService) CreateGroup(req *dto.CreateCommunityGroupRequest) (*dto.CommunityGroupResponse, error) {
	// 生成唯一ID
	id := uuid.New().String()

	// 创建模型
	group := &model.CommunityGroup{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// 保存到数据库
	savedGroup, err := s.repo.CreateGroup(group)
	if err != nil {
		return nil, err
	}

	response := &dto.CommunityGroupResponse{
		ID:          savedGroup.ID,
		Name:        savedGroup.Name,
		Description: savedGroup.Description,
		MemberCount: savedGroup.MemberCount,
		CreatedAt:   savedGroup.CreatedAt,
		UpdatedAt:   savedGroup.UpdatedAt,
	}

	return response, nil
}

func (s *CommunityService) UpdateGroup(id string, req *dto.UpdateCommunityGroupRequest) error {
	updates := make(map[string]any)
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	updates["updated_at"] = time.Now()

	return s.repo.UpdateGroup(id, updates)
}

func (s *CommunityService) DeleteGroup(id string) error {
	return s.repo.DeleteGroup(id)
}

func (s *CommunityService) GetMembers(groupID string, page, pageSize int, search string) ([]*dto.CommunityMemberResponse, int64, error) {
	members, total, err := s.repo.GetMembers(groupID, page, pageSize, search)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*dto.CommunityMemberResponse, 0, len(members))
	for _, member := range members {
		resp := &dto.CommunityMemberResponse{
			ID:       member.ID,
			GroupID:  member.GroupID,
			Name:     member.Name,
			Username: member.Username,
			Role:     member.Role,
			Status:   member.Status,
			JoinDate: member.JoinDate,
			LastSeen: member.LastSeen,
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}

func (s *CommunityService) GetMemberByID(id string) (*dto.CommunityMemberResponse, error) {
	member, err := s.repo.GetMemberByID(id)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, errors.New("社群成员不存在")
	}

	resp := &dto.CommunityMemberResponse{
		ID:       member.ID,
		GroupID:  member.GroupID,
		Name:     member.Name,
		Username: member.Username,
		Role:     member.Role,
		Status:   member.Status,
		JoinDate: member.JoinDate,
		LastSeen: member.LastSeen,
	}
	return resp, nil
}

func (s *CommunityService) AddMember(req *dto.AddCommunityMemberRequest) (*dto.CommunityMemberResponse, error) {
	// 生成唯一ID
	id := uuid.New().String()

	// 创建模型
	member := &model.CommunityMember{
		ID:        id,
		GroupID:   req.GroupID,
		Name:      req.Name,
		Username:  req.Username,
		Role:      req.Role,
		Status:    "active",
		JoinDate:  time.Now(),
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 保存到数据库
	savedMember, err := s.repo.AddMember(member)
	if err != nil {
		return nil, err
	}

	response := &dto.CommunityMemberResponse{
		ID:       savedMember.ID,
		GroupID:  savedMember.GroupID,
		Name:     savedMember.Name,
		Username: savedMember.Username,
		Role:     savedMember.Role,
		Status:   savedMember.Status,
		JoinDate: savedMember.JoinDate,
		LastSeen: savedMember.LastSeen,
	}

	return response, nil
}

func (s *CommunityService) UpdateMember(id string, req *dto.UpdateCommunityMemberRequest) error {
	updates := make(map[string]any)
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Role != "" {
		updates["role"] = req.Role
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	updates["updated_at"] = time.Now()

	return s.repo.UpdateMember(id, updates)
}

func (s *CommunityService) RemoveMember(id string) error {
	return s.repo.RemoveMember(id)
}

func (s *CommunityService) GetMessages(groupID string, page, pageSize int) ([]*dto.CommunityMessageResponse, int64, error) {
	messages, total, err := s.repo.GetMessages(groupID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	responses := make([]*dto.CommunityMessageResponse, 0, len(messages))
	for _, message := range messages {
		resp := &dto.CommunityMessageResponse{
			ID:          message.ID,
			GroupID:     message.GroupID,
			UserID:      message.UserID,
			UserName:    message.UserName,
			Content:     message.Content,
			Timestamp:   message.Timestamp,
			MessageType: message.MessageType,
		}
		responses = append(responses, resp)
	}

	return responses, total, nil
}

func (s *CommunityService) GetStatistics() (*dto.CommunityStatisticsResponse, error) {
	stats, err := s.repo.GetStatistics()
	if err != nil {
		return nil, err
	}

	return &dto.CommunityStatisticsResponse{
		TotalGroups:     int((*stats)["total_groups"].(int64)),
		TotalMembers:    int((*stats)["total_members"].(int64)),
		TotalMessages:   int((*stats)["total_messages"].(int64)),
		ActiveGroups:    int((*stats)["active_groups"].(int64)),
		NewMembersToday: int((*stats)["new_members_today"].(int64)),
	}, nil
}
