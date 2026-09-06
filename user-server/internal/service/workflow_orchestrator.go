package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

const (
	WorkflowNodeTypeTrigger   = "trigger"
	WorkflowNodeTypeAction    = "action"
	WorkflowNodeTypeCondition = "condition"
	WorkflowNodeTypeSubflow   = "subflow"
)

// 工作流编排服务的 sentinel error。
//
// 注意：与 workflow_orchestrator_deep_test.go 中 TestWO_GetLatestPublished_NoPublished
// 的断言保持兼容 —— 该用例使用 errors.Is(err, service.ErrNoPublishedVersion) 判定。
var (
	ErrNoPublishedVersion  = errors.New("no published version found")
	ErrExecutionNotRunning = errors.New("execution is not in running state")
)

// WorkflowOrchestratorService 工作流编排服务
type WorkflowOrchestratorService struct {
	versionRepo  *repository.WorkflowVersionRepository
	execRepo     *repository.WorkflowExecutionRepository
	nodeExecRepo *repository.WorkflowNodeExecutionRepository
	dispatcher   *WorkflowDispatcher
}

// NewWorkflowOrchestratorService 创建工作流编排服务
func NewWorkflowOrchestratorService(
	versionRepo *repository.WorkflowVersionRepository,
	execRepo *repository.WorkflowExecutionRepository,
	nodeExecRepo *repository.WorkflowNodeExecutionRepository,
) *WorkflowOrchestratorService {
	return &WorkflowOrchestratorService{
		versionRepo:  versionRepo,
		execRepo:     execRepo,
		nodeExecRepo: nodeExecRepo,
	}
}

// SetDispatcher 注入工作流执行调度器（避免循环依赖，启动期装配后调用）
func (s *WorkflowOrchestratorService) SetDispatcher(d *WorkflowDispatcher) {
	if s == nil {
		return
	}
	s.dispatcher = d
}

// CreateVersion 创建工作流版本
func (s *WorkflowOrchestratorService) CreateVersion(ctx context.Context, req *dto.WorkflowVersionCreateRequest) (*model.WorkflowVersion, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	if req == nil || req.WorkflowID == "" {
		return nil, utils.ErrInvalidInput
	}
	if req.Definition == nil {
		return nil, utils.ErrInvalidInput
	}

	count, err := s.versionRepo.CountByWorkflowID(ctx, req.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("count versions failed: %w", err)
	}

	version := &model.WorkflowVersion{
		WorkflowID:  req.WorkflowID,
		Version:     int(count) + 1,
		Name:        req.Name,
		Description: req.Description,
		Definition:  req.Definition,
		Status:      model.WorkflowStatusDraft,
		Changelog:   req.Changelog,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.versionRepo.Save(ctx, version); err != nil {
		return nil, fmt.Errorf("save version failed: %w", err)
	}

	return version, nil
}

// GetVersion 获取工作流版本详情
func (s *WorkflowOrchestratorService) GetVersion(ctx context.Context, id uint) (*model.WorkflowVersion, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	return s.versionRepo.GetByID(ctx, id)
}

// GetLatestPublished 获取最新已发布版本
//
// 当无已发布版本时返回 ErrNoPublishedVersion sentinel error（而非 gorm.ErrRecordNotFound），
// 便于上层/controller 用 errors.Is 精确判定。
func (s *WorkflowOrchestratorService) GetLatestPublished(ctx context.Context, workflowID string) (*model.WorkflowVersion, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	if workflowID == "" {
		return nil, utils.ErrInvalidInput
	}
	v, err := s.versionRepo.GetLatestPublished(ctx, workflowID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoPublishedVersion
		}
		return nil, err
	}
	return v, nil
}

// ListVersions 列出工作流版本
func (s *WorkflowOrchestratorService) ListVersions(ctx context.Context, workflowID string) ([]model.WorkflowVersion, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	return s.versionRepo.ListVersions(ctx, workflowID)
}

// ListAll 列出所有工作流版本（用于列表页，支持按 workflow_id 和 status 过滤）
func (s *WorkflowOrchestratorService) ListAll(ctx context.Context, workflowID, status string, page, pageSize int) ([]model.WorkflowVersion, int64, error) {
	if s == nil {
		return nil, 0, utils.ErrServiceNotInit
	}
	return s.versionRepo.ListAll(ctx, workflowID, status, page, pageSize)
}

// UpdateVersion 更新工作流版本
func (s *WorkflowOrchestratorService) UpdateVersion(ctx context.Context, id uint, req *dto.WorkflowVersionUpdateRequest) error {
	if s == nil {
		return utils.ErrServiceNotInit
	}
	if id == 0 || req == nil || req.Definition == nil {
		return utils.ErrInvalidInput
	}
	version, err := s.versionRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	version.Name = req.Name
	version.Description = req.Description
	version.Definition = req.Definition
	version.Changelog = req.Changelog
	version.UpdatedAt = time.Now()

	return s.versionRepo.Save(ctx, version)
}

// PublishVersion 发布工作流版本
func (s *WorkflowOrchestratorService) PublishVersion(ctx context.Context, id uint) error {
	if s == nil {
		return utils.ErrServiceNotInit
	}
	if id == 0 {
		return utils.ErrInvalidInput
	}
	return s.versionRepo.UpdateStatus(ctx, id, model.WorkflowStatusPublished)
}

// ArchiveVersion 归档工作流版本
func (s *WorkflowOrchestratorService) ArchiveVersion(ctx context.Context, id uint) error {
	if s == nil {
		return utils.ErrServiceNotInit
	}
	if id == 0 {
		return utils.ErrInvalidInput
	}
	return s.versionRepo.UpdateStatus(ctx, id, model.WorkflowStatusArchived)
}

// DeleteVersion 删除工作流版本
func (s *WorkflowOrchestratorService) DeleteVersion(ctx context.Context, id uint) error {
	if s == nil {
		return utils.ErrServiceNotInit
	}
	if id == 0 {
		return utils.ErrInvalidInput
	}
	return s.versionRepo.DeleteByID(ctx, id)
}

// Execute 执行工作流
//
// 创建执行实例后，若已注入 WorkflowDispatcher 则通过 go d.Run(ctx, exec.ID, traceID)
// 异步驱动全部节点执行；dispatcher 为 nil 时仅创建记录（保持旧行为，便于单测）。
func (s *WorkflowOrchestratorService) Execute(ctx context.Context, req *dto.WorkflowExecuteRequest) (*model.WorkflowExecution, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	if req == nil || req.WorkflowID == "" {
		return nil, utils.ErrInvalidInput
	}
	if req.TriggerPayload == nil {
		req.TriggerPayload = model.JSONMap{}
	}

	version, err := s.versionRepo.GetLatestPublished(ctx, req.WorkflowID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoPublishedVersion
		}
		return nil, fmt.Errorf("get latest published version failed: %w", err)
	}

	exec := &model.WorkflowExecution{
		WorkflowID:     req.WorkflowID,
		Version:        version.Version,
		TriggerPayload: req.TriggerPayload,
		Status:         model.WorkflowExecRunning,
		CurrentNodeID:  "",
		Context:        model.JSONMap{},
		StartedAt:      time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.execRepo.Create(ctx, exec); err != nil {
		return nil, fmt.Errorf("create execution failed: %w", err)
	}

	if s.dispatcher != nil {
		traceID := tracing.TraceIDFromContext(ctx)
		if traceID == "" {
			traceID = fmt.Sprintf("wf-%d-%d", exec.ID, time.Now().UnixNano())
		}
		s.dispatcher.Run(ctx, exec.ID, traceID)
	}

	return exec, nil
}

// GetExecution 获取执行详情
func (s *WorkflowOrchestratorService) GetExecution(ctx context.Context, execID uint) (*model.WorkflowExecution, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	if execID == 0 {
		return nil, utils.ErrInvalidInput
	}
	exec, err := s.execRepo.GetByID(ctx, execID)
	if err != nil {
		return nil, err
	}

	nodeExecs, err := s.nodeExecRepo.ListByExecutionID(ctx, execID)
	if err != nil {
		return nil, err
	}

	_ = nodeExecs
	return exec, nil
}

// GetNodeExecutions 获取节点执行明细
func (s *WorkflowOrchestratorService) GetNodeExecutions(ctx context.Context, execID uint) ([]model.WorkflowNodeExecution, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	if execID == 0 {
		return nil, utils.ErrInvalidInput
	}
	return s.nodeExecRepo.ListByExecutionID(ctx, execID)
}

// ListExecutions 列出执行实例
func (s *WorkflowOrchestratorService) ListExecutions(ctx context.Context, workflowID, status string, page, pageSize int) ([]model.WorkflowExecution, int64, error) {
	if s == nil {
		return nil, 0, utils.ErrServiceNotInit
	}
	return s.execRepo.List(ctx, workflowID, status, page, pageSize)
}

// StopExecution 停止执行
//
// 仅当执行处于 running 状态时方可停止，否则返回 ErrExecutionNotRunning sentinel error。
// 终止后 dispatcher 的 runExecution 循环会在下一轮检测到 status 变化自动退出。
func (s *WorkflowOrchestratorService) StopExecution(ctx context.Context, execID uint) error {
	if s == nil {
		return utils.ErrServiceNotInit
	}
	if execID == 0 {
		return utils.ErrInvalidInput
	}
	exec, err := s.execRepo.GetByID(ctx, execID)
	if err != nil {
		return err
	}

	if exec.Status != model.WorkflowExecRunning {
		return ErrExecutionNotRunning
	}

	now := time.Now()
	exec.Status = model.WorkflowExecTerminated
	exec.FinishedAt = &now
	exec.Error = "manual stopped"
	exec.UpdatedAt = now

	return s.execRepo.Update(ctx, exec)
}

// FindStuckExecutions 查找卡死的执行
func (s *WorkflowOrchestratorService) FindStuckExecutions(ctx context.Context, threshold time.Time, limit int) ([]model.WorkflowExecution, error) {
	if s == nil {
		return nil, utils.ErrServiceNotInit
	}
	return s.execRepo.FindStuck(ctx, threshold, limit)
}
