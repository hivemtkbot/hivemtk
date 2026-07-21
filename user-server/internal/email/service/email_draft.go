package email

import (
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
	"strings"

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
func (s *EmailDraftService) CreateEmailDraft(draft model.EmailDraft) (*model.EmailDraft, error) {
	if err := s.repo.Create(&draft); err != nil {
		return nil, err
	}
	return &draft, nil
}

// GetEmailDraftByID 根据ID获取草稿
func (s *EmailDraftService) GetEmailDraftByID(id uuid.UUID) (*model.EmailDraft, error) {
	return s.repo.GetByID(id)
}

// GetEmailDraftList 获取草稿列表
func (s *EmailDraftService) GetEmailDraftList() ([]*model.EmailDraft, error) {
	return s.repo.List()
}

// UpdateEmailDraft 更新草稿
func (s *EmailDraftService) UpdateEmailDraft(draft model.EmailDraft) error {
	return s.repo.Update(&draft)
}

// DeleteEmailDraft 删除草稿
func (s *EmailDraftService) DeleteEmailDraft(id uuid.UUID) error {
	return s.repo.Delete(id)
}

// ---- DTO 外观方法：供 controller 调用，避免 controller 直接依赖 model ----

// CreateEmailDraftDTO 通过请求 DTO 创建草稿
func (s *EmailDraftService) CreateEmailDraftDTO(req dto.CreateEmailDraftRequest) (*dto.EmailDraftResponse, error) {
	created, err := s.CreateEmailDraft(model.EmailDraft{
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
func (s *EmailDraftService) GetEmailDraftListDTO() (*dto.GetEmailDraftListResponse, error) {
	drafts, err := s.GetEmailDraftList()
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
func (s *EmailDraftService) GetEmailDraftByIDDTO(id uuid.UUID) (*dto.EmailDraftResponse, error) {
	d, err := s.GetEmailDraftByID(id)
	if err != nil {
		return nil, err
	}
	return toEmailDraftResponse(d), nil
}

// UpdateEmailDraftDTO 通过请求 DTO 更新草稿
func (s *EmailDraftService) UpdateEmailDraftDTO(req dto.UpdateEmailDraftRequest) error {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return err
	}
	return s.UpdateEmailDraft(model.EmailDraft{
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
