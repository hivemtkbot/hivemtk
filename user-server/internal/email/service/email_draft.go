package email

import (
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"strings"

	"context"

	"github.com/google/uuid"
)

// EmailDraftService 草稿服务
type EmailDraftService struct {
	repo repository.EmailDraftRepository
}

// NewEmailDraftService 创建草稿服务实例
func NewEmailDraftService() *EmailDraftService {
	return &EmailDraftService{repo: repository.NewEmailDraftRepository()}
}

// CreateEmailDraft 创建草稿
func (s *EmailDraftService) CreateEmailDraft(ctx context.Context, draft model.EmailDraft) (*model.EmailDraft, error) {
	if err := s.repo.Create(ctx, &draft); err != nil {
		return nil, err
	}
	return &draft, nil
}

// GetEmailDraftByID 根据ID获取草稿
func (s *EmailDraftService) GetEmailDraftByID(ctx context.Context, id uuid.UUID) (*model.EmailDraft, error) {
	return s.repo.GetByID(ctx, id)
}

// GetEmailDraftList 获取草稿列表
func (s *EmailDraftService) GetEmailDraftList(ctx context.Context) ([]*model.EmailDraft, error) {
	return s.repo.List(ctx)
}

// UpdateEmailDraft 更新草稿
func (s *EmailDraftService) UpdateEmailDraft(ctx context.Context, draft model.EmailDraft) error {
	return s.repo.Update(ctx, &draft)
}

// DeleteEmailDraft 删除草稿
func (s *EmailDraftService) DeleteEmailDraft(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// ---- DTO 外观方法：供 controller 调用，避免 controller 直接依赖 model ----

// CreateEmailDraftDTO 通过请求 DTO 创建草稿
func (s *EmailDraftService) CreateEmailDraftDTO(ctx context.Context, req dto.CreateEmailDraftRequest) (*dto.EmailDraftResponse, error) {
	created, err := s.CreateEmailDraft(ctx, model.EmailDraft{
		Subject:     req.Subject,
		Content:     req.Content,
		Attachments: strings.Join(req.Attachments, ","),
	})
	if err != nil {
		return nil, err
	}
	return toEmailDraftResponse(created), nil
}

// GetEmailDraftListDTO 获取草稿列表（返回 DTO）
func (s *EmailDraftService) GetEmailDraftListDTO(ctx context.Context) (*dto.GetEmailDraftListResponse, error) {
	drafts, err := s.GetEmailDraftList(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]*dto.EmailDraftResponse, 0, len(drafts))
	for _, d := range drafts {
		list = append(list, toEmailDraftResponse(d))
	}
	return &dto.GetEmailDraftListResponse{List: list, Total: int64(len(list))}, nil
}

// GetEmailDraftByIDDTO 根据 ID 获取草稿（返回 DTO）
func (s *EmailDraftService) GetEmailDraftByIDDTO(ctx context.Context, id uuid.UUID) (*dto.EmailDraftResponse, error) {
	d, err := s.GetEmailDraftByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toEmailDraftResponse(d), nil
}

// UpdateEmailDraftDTO 通过请求 DTO 更新草稿
func (s *EmailDraftService) UpdateEmailDraftDTO(ctx context.Context, req dto.UpdateEmailDraftRequest) error {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return err
	}
	return s.UpdateEmailDraft(ctx, model.EmailDraft{
		ID:          id,
		Subject:     req.Subject,
		Content:     req.Content,
		Attachments: strings.Join(req.Attachments, ","),
	})
}

func toEmailDraftResponse(d *model.EmailDraft) *dto.EmailDraftResponse {
	if d == nil {
		return nil
	}
	return &dto.EmailDraftResponse{
		ID:          d.ID.String(),
		Subject:     d.Subject,
		Content:     d.Content,
		Attachments: splitCSV(d.Attachments),
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

// splitCSV 将逗号分隔的附件字符串还原为切片（与请求端 Join 对称）
func splitCSV(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}
