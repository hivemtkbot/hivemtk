package email

import (
	"context"
	"errors"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
)

type EmailSmtpService struct {
	repo repository.EmailSmtpRepository
}

func NewEmailSmtpService() *EmailSmtpService {
	return &EmailSmtpService{repo: repository.NewEmailSmtpRepository()}
}

func (s *EmailSmtpService) CreateEmailSmtp(ctx context.Context, emailSmtp model.EmailSmtp) (*model.EmailSmtp, error) {
	if err := s.repo.Create(ctx, &emailSmtp); err != nil {
		return nil, err
	}
	return &emailSmtp, nil
}

func (s *EmailSmtpService) GetEmailSmtp(ctx context.Context, id string) (*model.EmailSmtp, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *EmailSmtpService) GetEmailSmtpList(ctx context.Context) ([]*model.EmailSmtp, error) {
	return s.repo.GetEmailSmtpList(ctx)
}

func (s *EmailSmtpService) UpdateEmailSmtp(ctx context.Context, emailSmtp model.EmailSmtp) error {
	return s.repo.Update(ctx, &emailSmtp)
}

func (s *EmailSmtpService) DeleteEmailSmtp(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *EmailSmtpService) GetRandEmailSmtp(ctx context.Context) (*model.EmailSmtp, error) {
	// 读取所有列表
	emailSmtpList, err := s.repo.GetEmailSmtpList(ctx)
	if err != nil {
		return nil, err
	}
	// 循环判断今日发送个数 超过limit 查找下一个
	emailListService := NewEmailListService()
	for _, emailSmtp := range emailSmtpList {
		// 从email list 统计 今日发送格式
		todayCount, err := emailListService.GetTodayCountByFrom(ctx, emailSmtp.Name)
		if err != nil {
			return nil, err
		}
		if todayCount < emailSmtp.Limit {
			return emailSmtp, nil
		}
	}
	// 没有找到 随机返回一个
	return nil, errors.New("没有找到可用的smtp")
}

// ---- DTO 外观方法：供 controller 调用，避免 controller 直接依赖 model ----

// CreateEmailSmtpDTO 通过请求 DTO 创建 SMTP 配置
func (s *EmailSmtpService) CreateEmailSmtpDTO(ctx context.Context, req dto.CreateEmailSmtpRequest) (*dto.EmailSmtpResponse, error) {
	created, err := s.CreateEmailSmtp(ctx, model.EmailSmtp{
		Name:     req.Name,
		Server:   req.Server,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}
	return toEmailSmtpResponse(created), nil
}

// GetEmailSmtpListDTO 获取 SMTP 配置列表（返回 DTO）
func (s *EmailSmtpService) GetEmailSmtpListDTO(ctx context.Context) (*dto.GetEmailSmtpListResponse, error) {
	list, err := s.GetEmailSmtpList(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*dto.EmailSmtpResponse, 0, len(list))
	for _, item := range list {
		result = append(result, toEmailSmtpResponse(item))
	}
	return &dto.GetEmailSmtpListResponse{List: result, Total: int64(len(result))}, nil
}

// GetEmailSmtpDTO 根据 ID 获取 SMTP 配置（返回 DTO）
func (s *EmailSmtpService) GetEmailSmtpDTO(ctx context.Context, id string) (*dto.EmailSmtpResponse, error) {
	item, err := s.GetEmailSmtp(ctx, id)
	if err != nil {
		return nil, err
	}
	return toEmailSmtpResponse(item), nil
}

// UpdateEmailSmtpDTO 通过请求 DTO 更新 SMTP 配置
func (s *EmailSmtpService) UpdateEmailSmtpDTO(ctx context.Context, req dto.UpdateEmailSmtpRequest) error {
	return s.UpdateEmailSmtp(ctx, model.EmailSmtp{
		ID:       req.ID,
		Name:     req.Name,
		Server:   req.Server,
		Port:     req.Port,
		Username: req.Username,
		Password: req.Password,
		Limit:    req.Limit,
	})
}

func toEmailSmtpResponse(s *model.EmailSmtp) *dto.EmailSmtpResponse {
	if s == nil {
		return nil
	}
	return &dto.EmailSmtpResponse{
		ID:       s.ID,
		Name:     s.Name,
		Server:   s.Server,
		Port:     s.Port,
		Username: s.Username,
		Limit:    s.Limit,
	}
}
