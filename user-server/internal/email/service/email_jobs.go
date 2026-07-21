package email

import (
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"

	"github.com/google/uuid"
)

// EmailJobsService 任务服务
type EmailJobsService struct {
	repo repository.EmailJobsRepository
}

// NewEmailJobsService 创建任务服务实例
func NewEmailJobsService() *EmailJobsService {
	return &EmailJobsService{repo: repository.NewEmailJobsRepository()}
}

// CreateEmailJobs 创建任务
func (s *EmailJobsService) CreateEmailJobs(jobs model.EmailJobs) (*model.EmailJobs, error) {
	if err := s.repo.Create(&jobs); err != nil {
		return nil, err
	}
	return &jobs, nil
}

// GetEmailJobsByID 根据ID获取任务
func (s *EmailJobsService) GetEmailJobsByID(id uuid.UUID) (*model.EmailJobs, error) {
	return s.repo.GetByID(id)
}

// GetEmailJobsList 获取任务列表
func (s *EmailJobsService) GetEmailJobsList(page int, pageSize int) ([]*model.EmailJobs, int64, error) {
	return s.repo.List(page, pageSize)
}

// UpdateEmailJobs 更新任务
func (s *EmailJobsService) UpdateEmailJobs(jobs model.EmailJobs) error {
	return s.repo.Update(&jobs)
}

// DeleteEmailJobs 删除任务
func (s *EmailJobsService) DeleteEmailJobs(id uuid.UUID) error {
	return s.repo.Delete(id)
}

// IncreaseSendTotal 增加发送总数
func (s *EmailJobsService) IncreaseSendTotal(jobs_id uuid.UUID) error {
	jobs, err := s.repo.GetByID(jobs_id)
	if err != nil {
		logger.Error(fmt.Errorf("Failed to get email job: %v", err), "获取邮件任务失败")
		return err
	}
	// Add nil check for jobs
	if jobs == nil {
		logger.Error(fmt.Errorf("Email job %s not found", jobs_id), "查找邮件任务失败")
		return fmt.Errorf("email job not found")
	}
	jobs.SendTotal++
	return s.repo.Update(jobs)
}

// IncreaseSuccessTotal 增加成功总数
func (s *EmailJobsService) IncreaseSuccessTotal(jobs_id uuid.UUID) error {
	jobs, err := s.repo.GetByID(jobs_id)
	if err != nil {
		return err
	}
	// Add nil check for jobs
	if jobs == nil {
		logger.Error(fmt.Errorf("Email job %s not found", jobs_id), "查找邮件任务失败")
		return fmt.Errorf("email job not found")
	}
	jobs.SuccessTotal++
	return s.repo.Update(jobs)
}

// IncreaseFailTotal 增加失败总数
func (s *EmailJobsService) IncreaseFailTotal(jobs_id uuid.UUID) error {
	jobs, err := s.repo.GetByID(jobs_id)
	if err != nil {
		return err
	}
	// Add nil check for jobs
	if jobs == nil {
		logger.Error(fmt.Errorf("Email job %s not found", jobs_id), "查找邮件任务失败")
		return fmt.Errorf("email job not found")
	}
	jobs.FailTotal++
	return s.repo.Update(jobs)
}

// IncreaseReadTotal 增加阅读总数
func (s *EmailJobsService) IncreaseReadTotal(jobs_id uuid.UUID) error {
	jobs, err := s.repo.GetByID(jobs_id)
	if err != nil {
		return err
	}
	// Add nil check for jobs
	if jobs == nil {
		logger.Error(fmt.Errorf("Email job %s not found", jobs_id), "查找邮件任务失败")
		return fmt.Errorf("email job not found")
	}
	jobs.ReadTotal++
	return s.repo.Update(jobs)
}

// ---- DTO 外观方法：供 controller 调用，避免 controller 直接依赖 model ----

// CreateEmailJobsDTO 通过请求 DTO 创建任务
func (s *EmailJobsService) CreateEmailJobsDTO(req dto.CreateEmailJobsRequest) (*dto.EmailJobsResponse, error) {
	created, err := s.CreateEmailJobs(model.EmailJobs{
		Subject:      req.Subject,
		EmailTotal:   req.EmailTotal,
		SendTotal:    req.SendTotal,
		ReadTotal:    req.ReadTotal,
		SuccessTotal: req.SuccessTotal,
		FailTotal:    req.FailTotal,
	})
	if err != nil {
		return nil, err
	}
	return toEmailJobsResponse(created), nil
}

// GetEmailJobsListDTO 获取任务列表（返回 DTO）
func (s *EmailJobsService) GetEmailJobsListDTO(page, pageSize int) (*dto.GetEmailJobsListResponse, error) {
	jobs, total, err := s.GetEmailJobsList(page, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]*dto.EmailJobsResponse, 0, len(jobs))
	for _, j := range jobs {
		list = append(list, toEmailJobsResponse(j))
	}
	return &dto.GetEmailJobsListResponse{List: list, Total: total}, nil
}

// GetEmailJobsByIDDTO 根据 ID 获取任务（返回 DTO）
func (s *EmailJobsService) GetEmailJobsByIDDTO(id uuid.UUID) (*dto.EmailJobsResponse, error) {
	j, err := s.GetEmailJobsByID(id)
	if err != nil {
		return nil, err
	}
	return toEmailJobsResponse(j), nil
}

// UpdateEmailJobsDTO 通过请求 DTO 更新任务
func (s *EmailJobsService) UpdateEmailJobsDTO(req dto.UpdateEmailJobsRequest) error {
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return err
	}
	return s.UpdateEmailJobs(model.EmailJobs{
		ID:           id,
		Subject:      req.Subject,
		EmailTotal:   req.EmailTotal,
		ReadTotal:    req.ReadTotal,
		SendTotal:    req.SendTotal,
		SuccessTotal: req.SuccessTotal,
		FailTotal:    req.FailTotal,
	})
}

func toEmailJobsResponse(j *model.EmailJobs) *dto.EmailJobsResponse {
	if j == nil {
		return nil
	}
	return &dto.EmailJobsResponse{
		ID:           j.ID.String(),
		Subject:      j.Subject,
		EmailTotal:   j.EmailTotal,
		SendTotal:    j.SendTotal,
		ReadTotal:    j.ReadTotal,
		SuccessTotal: j.SuccessTotal,
		FailTotal:    j.FailTotal,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}
