package email

import (
	"errors"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/mail"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"

	"context"
	"github.com/google/uuid"
)

// EmailListService 列表服务
type EmailListService struct {
	repo             repository.EmailListRepository
	clueRepo         repository.ClueRepository
	systemConfigRepo repository.SystemConfigRepository
}

// NewEmailListService 创建列表服务实例
func NewEmailListService() *EmailListService {
	return &EmailListService{
		repo:             repository.NewEmailListRepository(),
		clueRepo:         repository.NewClueRepository(),
		systemConfigRepo: repository.NewSystemConfigRepository(),
	}
}

// CreateEmailList 创建列表
//
// 直接注入 clue / systemConfig repository（避免 service→tooluse→email/service 循环依赖）。
func (s *EmailListService) CreateEmailList(ctx context.Context, subject string, content string, attachments string) (total int64, err error) {
	// 从线索库读取所有线索
	cluesList, clueTotal, err := s.clueRepo.GetClueAllList(ctx, 1)
	if err != nil {
		return 0, err
	}
	if clueTotal == 0 {
		return 0, errors.New("线索库没有线索")
	}

	// 新建任务
	jobsService := NewEmailJobsService()
	jobs := model.EmailJobs{
		Subject:      subject,
		EmailTotal:   clueTotal,
		SendTotal:    0,
		SuccessTotal: 0,
		FailTotal:    0,
		ReadTotal:    0,
	}
	job, err := jobsService.CreateEmailJobs(ctx, jobs)
	if err != nil {
		return 0, err
	}

	// 从系统配置库读取系统配置
	systemConfig, err := s.systemConfigRepo.GetConfig(ctx)
	if err != nil {
		return 0, err
	}

	emailList := make([]*model.EmailList, 0)
	for _, clue := range cluesList {

		// 获取接收人
		toAccount := clue.Account
		// 接收人为空
		if toAccount == "" {
			continue
		}

		// 判断是否qq邮箱 没有拼接@qq.com
		if !strings.Contains(toAccount, "@qq.com") {
			toAccount = toAccount + "@qq.com"
		}

		parsemap := mail.TemplateParseMap{
			Name:    clue.Name,
			City:    clue.City,
			Address: clue.Address,
			Account: clue.Account,
		}

		// 替换 subject content 上的自定义变量
		// 复制  subject  content 避免应用传递

		traceID := uuid.New()

		subjectCopy := subject
		contentCopy := content
		subjectCopy = mail.Parse(subjectCopy, parsemap)
		contentCopy = mail.Parse(contentCopy, parsemap)

		contentCopy = mail.BuildTrace(contentCopy, traceID, systemConfig.WebsiteURL)

		emailInfo := model.EmailList{
			Subject:     subjectCopy,
			Content:     contentCopy,
			Attachments: attachments,
			IsSend:      0,
			SendTime:    time.Time{},
			IsRead:      0,
			ReadTime:    time.Time{},
			To:          toAccount,
			JobsID:      job.ID,
			TraceID:     traceID,
			CreatedAt:   time.Now(),
		}

		emailList = append(emailList, &emailInfo)
	}

	total, err = s.repo.BatchCreate(ctx, emailList)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// GetEmailListByID 根据ID获取列表
func (s *EmailListService) GetEmailListByID(ctx context.Context, id uuid.UUID) (*model.EmailList, error) {
	return s.repo.GetByID(ctx, id)
}

// GetEmailListList 获取列表列表
func (s *EmailListService) GetEmailListList(ctx context.Context, page int, pageSize int) ([]*model.EmailList, int64, error) {
	return s.repo.List(ctx, page, pageSize)
}

// UpdateEmailList 更新列表
func (s *EmailListService) UpdateEmailList(ctx context.Context, list model.EmailList) error {
	return s.repo.Update(ctx, &list)
}

// DeleteEmailList 删除列表
func (s *EmailListService) DeleteEmailList(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// GetUnsentEmailList 获取未发送的邮件列表
func (s *EmailListService) GetUnsentEmailList(ctx context.Context, limit int) ([]*model.EmailList, error) {
	return s.repo.GetUnsentEmailList(ctx, limit)
}

// GetTodayCountByFrom 获取今日发送个数
func (s *EmailListService) GetTodayCountByFrom(ctx context.Context, from string) (int64, error) {
	return s.repo.GetTodayCountByFrom(ctx, from)
}

// UpdateEmailListReadInfo 更新邮件状态
func (s *EmailListService) UpdateEmailListReadInfo(ctx context.Context, traceID uuid.UUID) error {
	// 基于trace id 获取 邮件信息
	emailList, err := s.repo.GetByTraceID(ctx, traceID)
	if err != nil {
		return err
	}
	// is_read > 0 表示已经阅读
	if emailList.IsRead > 0 {
		return nil
	}

	// 更新 邮件 阅读信息
	emailList.IsRead = 1
	emailList.ReadTime = time.Now()
	// 更新 邮件 状态
	res := s.repo.Update(ctx, emailList)

	// 更新jobs 统计
	jobsService := NewEmailJobsService()
	jobsService.IncreaseReadTotal(ctx, emailList.JobsID)

	if res != nil {
		return res
	}
	return nil
}

// ---- DTO 外观方法：供 controller 调用，避免 controller 直接依赖 model ----

// GetEmailListListDTO 获取邮件列表（返回 DTO）
func (s *EmailListService) GetEmailListListDTO(ctx context.Context, page, pageSize int) (*dto.GetEmailListResponse, error) {
	lists, total, err := s.GetEmailListList(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]*dto.EmailListResponse, 0, len(lists))
	for _, l := range lists {
		list = append(list, toEmailListResponse(l))
	}
	return &dto.GetEmailListResponse{List: list, Total: total}, nil
}

// GetEmailListByIDDTO 根据 ID 获取邮件（返回 DTO）
func (s *EmailListService) GetEmailListByIDDTO(ctx context.Context, id uuid.UUID) (*dto.EmailListResponse, error) {
	l, err := s.GetEmailListByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toEmailListResponse(l), nil
}

// UpdateEmailListDTO 通过请求 DTO 更新邮件
func (s *EmailListService) UpdateEmailListDTO(ctx context.Context, req dto.UpdateEmailListRequest) error {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return err
	}
	return s.UpdateEmailList(ctx, model.EmailList{
		ID:          id,
		Subject:     req.Subject,
		Content:     req.Content,
		Attachments: strings.Join(req.Attachments, ","),
	})
}

func toEmailListResponse(l *model.EmailList) *dto.EmailListResponse {
	if l == nil {
		return nil
	}
	return &dto.EmailListResponse{
		ID:          l.ID.String(),
		Subject:     l.Subject,
		Content:     l.Content,
		Attachments: splitCSV(l.Attachments),
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
		From:        l.From,
		To:          l.To,
		IsSend:      int64(l.IsSend),
		SendTime:    l.SendTime,
		IsRead:      int64(l.IsRead),
		ReadTime:    l.ReadTime,
		JobsID:      l.JobsID.String(),
		IsSuccess:   int64(l.IsSuccess),
	}
}
