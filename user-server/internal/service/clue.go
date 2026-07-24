package service

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/repository"
)

type ClueService struct {
	repo repository.ClueRepository
}

func NewClueService() *ClueService {
	return &ClueService{repo: repository.NewClueRepository()}
}

func (s *ClueService) Register(ctx context.Context, clue model.Clue) (*model.Clue, error) {
	if err := s.repo.Create(ctx, &clue); err != nil {
		return nil, err
	}
	return &clue, nil
}

func (s *ClueService) GetClue(ctx context.Context, id uint) (*model.Clue, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ClueService) GetClueList(ctx context.Context, page int, limit int) ([]*model.Clue, int64, error) {
	return s.repo.GetClueList(ctx, page, limit)
}

func (s *ClueService) DeleteClue(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
func (s *ClueService) GetRecentClueList(ctx context.Context) ([]*model.Clue, error) {
	return s.repo.GetRecentClueList(ctx)
}

func (s *ClueService) GetClueStatistics(ctx context.Context) ([]map[string]any, error) {
	return s.repo.GetClueStatistics(ctx)
}
func (s *ClueService) BatchSaveClue(ctx context.Context, clueList []*model.Clue) error {
	for _, clue := range clueList {
		if _, err := s.Register(ctx, *clue); err != nil {
			return err
		}
	}
	return nil
}

func (s *ClueService) GetClueAllList(ctx context.Context, clueType int64) ([]*model.Clue, int64, error) {
	return s.repo.GetClueAllList(ctx, clueType)
}

// BatchImportClues 批量导入线索，返回成功数量和跳过数量
func (s *ClueService) BatchImportClues(ctx context.Context, clues []*model.Clue) (successCount, skipCount int64, err error) {
	for _, clue := range clues {
		// 检查是否已存在
		exists, err := s.repo.ExistsByTypeAndAccount(ctx, clue.Type, clue.Account)
		if err != nil {
			return successCount, skipCount, err
		}
		if exists {
			skipCount++
			continue
		}

		// 创建新线索
		if err := s.repo.Create(ctx, clue); err != nil {
			return successCount, skipCount, err
		}
		successCount++
	}
	return successCount, skipCount, nil
}

// ClueType 线索类型
type ClueType struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// clueTypeMap 预定义的线索类型映射（与前端 utils/map.js 保持一致）
var clueTypeMap = map[int64]string{
	1: "QQ",
	2: "微信",
	3: "电话",
	4: "Telegram",
	5: "Whatsapp",
	6: "twitter",
}

// defaultClueTypes 默认线索类型列表
var defaultClueTypes = []ClueType{
	{Value: "1", Label: "QQ"},
	{Value: "2", Label: "微信"},
	{Value: "3", Label: "电话"},
	{Value: "4", Label: "Telegram"},
	{Value: "5", Label: "Whatsapp"},
	{Value: "6", Label: "twitter"},
}

// GetClueTypes 获取线索类型列表
// 始终返回预定义的完整类型列表，确保前端可正常选择类型
func (s *ClueService) GetClueTypes(ctx context.Context) ([]ClueType, error) {
	return defaultClueTypes, nil
}
