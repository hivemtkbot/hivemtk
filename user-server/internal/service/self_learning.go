package service

// self_learning_service.go 对话驱动自我学习三位一体机制 L3 业务服务（门面）
//
// 五层架构归属: L3 业务层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.4
//
// 职责：
//   1. 包装 L4 self_learning 组件（SwitchService / RAGSelfSupervisor /
//      AssetBundleSelfSupervisor / Orchestrator / SelfCorrectionDispatcher）
//   2. 提供列表/查询能力（logs / candidates / ab-tests / corrections）
//      —— L4 组件聚焦"自动化执行"，列表查询属于业务编排，放在 L3
//   3. 人工审核动作（supervised 模式下 approve / reject）
//
// 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/repository"
	selflearning "marketing/internal/service/self_learning"
)

// SelfLearningService 自我学习三位一体机制 L3 门面服务
//
// 不重复实现 L4 已有逻辑，仅做编排 + 列表查询 + DTO 转换
type SelfLearningService struct {
	components *SelfLearningComponents

	// 列表查询所需仓储（由 InitSelfLearningService 注入）
	logRepo        repository.SelfLearningLogRepository
	candidateRepo  repository.AssetBundleCandidateRepository
	abTestRepo     repository.AssetBundleABTestRepository
	actionRepo     repository.SelfCorrectionActionRepository
}

// NewSelfLearningService 创建 L3 门面服务
//
// components: 由 InitSelfLearningComponents 返回的组件集合（非空）
// db: 已初始化的 *gorm.DB（用于构造查询仓储）
func NewSelfLearningService(components *SelfLearningComponents, db *gorm.DB) *SelfLearningService {
	if components == nil {
		return nil
	}
	svc := &SelfLearningService{
		components: components,
	}
	if db != nil {
		svc.logRepo = repository.NewSelfLearningLogRepository(db)
		svc.candidateRepo = repository.NewAssetBundleCandidateRepository(db)
		svc.abTestRepo = repository.NewAssetBundleABTestRepository(db)
		svc.actionRepo = repository.NewSelfCorrectionActionRepository(db)
	}
	return svc
}

// ============================================================================
// 1. 开关 API（用户开启即全自动执行 - v1.1 §7.4）
// ============================================================================

// GetSwitchStatus 获取开关状态
func (s *SelfLearningService) GetSwitchStatus(ctx context.Context) (*dto.SwitchStatusResponse, error) {
	if s == nil || s.components == nil || s.components.SwitchSvc == nil {
		return nil, fmt.Errorf("self_learning: switch service not initialized")
	}
	snap, err := s.components.SwitchSvc.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return snap.ToDTO(), nil
}

// UpdateSwitch 更新开关配置（用户开启/关闭全自动机制）
func (s *SelfLearningService) UpdateSwitch(ctx context.Context, req *dto.SwitchConfigRequest, operatorID uint) (*dto.SwitchStatusResponse, error) {
	if s == nil || s.components == nil || s.components.SwitchSvc == nil {
		return nil, fmt.Errorf("self_learning: switch service not initialized")
	}
	if err := s.validateSwitchConfig(req); err != nil {
		return nil, err
	}
	snap, err := s.components.SwitchSvc.UpdateSwitch(ctx, req, operatorID)
	if err != nil {
		return nil, err
	}
	return snap.ToDTO(), nil
}

// ============================================================================
// 2. 看板 API
// ============================================================================

// GetDashboard 获取自我学习总看板（今日执行统计 + 失败率 + 熔断状态）
func (s *SelfLearningService) GetDashboard(ctx context.Context) (*dto.SelfLearningDashboardResponse, error) {
	if s == nil || s.components == nil {
		return nil, fmt.Errorf("self_learning: components not initialized")
	}
	resp := &dto.SelfLearningDashboardResponse{
		TodayCount:        make(map[model.SelfLearningStatus]int64),
		UpdatedAt:         time.Now(),
	}

	// 1. 开关状态
	if s.components.SwitchSvc != nil {
		snap, err := s.components.SwitchSvc.GetStatus(ctx)
		if err == nil {
			resp.Switch = snap.ToDTO()
		}
	}

	// 2. 今日统计
	if s.logRepo != nil {
		counts, err := s.logRepo.CountToday(ctx)
		if err == nil {
			resp.TodayCount = counts
			for status, n := range counts {
				resp.TodayTotal += n
				switch status {
				case model.SelfLearningStatusSuccess:
					resp.TodaySuccess += n
				case model.SelfLearningStatusFailed:
					resp.TodayFailed += n
				}
			}
			if resp.TodayTotal > 0 {
				resp.SuccessRate = float64(resp.TodaySuccess) / float64(resp.TodayTotal)
				resp.FailedRate = float64(resp.TodayFailed) / float64(resp.TodayTotal)
			}
		}
		// 3. 最近失败日志（10 条）
		failedLogs, _ := s.logRepo.ListByStatus(ctx, model.SelfLearningStatusFailed, 10)
		for _, lg := range failedLogs {
			resp.RecentFailedLogs = append(resp.RecentFailedLogs, toLogResponse(lg))
		}
	}

	// 4. 最近待审矫正动作（10 条）
	if s.actionRepo != nil {
		pending, _ := s.actionRepo.ListPending(ctx, 10)
		for _, a := range pending {
			resp.RecentChampionOps = append(resp.RecentChampionOps, toCorrectionItem(a))
		}
	}

	return resp, nil
}

// GetRAGSupervisionDashboard 获取 RAG 监督看板（5 维指标）
func (s *SelfLearningService) GetRAGSupervisionDashboard(ctx context.Context, rangeStr string) (*dto.SupervisionDashboardResponse, error) {
	if s == nil || s.components == nil || s.components.RAGSupervisor == nil {
		return nil, fmt.Errorf("self_learning: rag supervisor not initialized")
	}
	return s.components.RAGSupervisor.GetDashboard(ctx, rangeStr)
}

// GetAssetSupervisionDashboard 获取资产包监督看板（5 维专属指标 + A/B 汇总）
func (s *SelfLearningService) GetAssetSupervisionDashboard(ctx context.Context, rangeStr string) (*dto.AssetSupervisionDashboardResponse, error) {
	if s == nil || s.components == nil || s.components.AssetSupervisor == nil {
		return nil, fmt.Errorf("self_learning: asset supervisor not initialized")
	}
	return s.components.AssetSupervisor.GetAssetDashboard(ctx, rangeStr)
}

// GetOrchestratorStats 获取 Orchestrator 运行时统计
func (s *SelfLearningService) GetOrchestratorStats() selflearning.OrchestratorStats {
	if s == nil || s.components == nil || s.components.Orchestrator == nil {
		return selflearning.OrchestratorStats{}
	}
	return s.components.Orchestrator.GetStats()
}

// ============================================================================
// 3. 日志查询 API
// ============================================================================

// ListLogs 查询自我学习日志列表
func (s *SelfLearningService) ListLogs(ctx context.Context, req *dto.SelfLearningLogListRequest) (*dto.SelfLearningLogListResponse, error) {
	if s == nil || s.logRepo == nil {
		return &dto.SelfLearningLogListResponse{List: []*dto.SelfLearningLogResponse{}}, nil
	}
	if err := s.validateListRequest(req); err != nil {
		return nil, err
	}
	// 按 status / scenario / since 查询；当前 repo 接口仅支持 limit，分页字段保留在响应中
	limit := req.Size

	var logs []*model.SelfLearningLog
	var err error
	switch {
	case req.Status != "":
		logs, err = s.logRepo.ListByStatus(ctx, req.Status, limit)
	case req.Scenario != "":
		since := req.Since
		if since.IsZero() {
			since = time.Now().Add(-7 * 24 * time.Hour)
		}
		logs, err = s.logRepo.ListByScenario(ctx, req.Scenario, since, limit)
	default:
		// 默认查最近 7 天，limit 取 size
		since := time.Now().Add(-7 * 24 * time.Hour)
		logs, err = s.logRepo.ListByScenario(ctx, "", since, limit)
	}
	if err != nil {
		return nil, err
	}

	out := &dto.SelfLearningLogListResponse{
		List:  make([]*dto.SelfLearningLogResponse, 0, len(logs)),
		Total: int64(len(logs)),
		Page:  req.Page,
		Size:  req.Size,
	}
	for _, lg := range logs {
		out.List = append(out.List, toLogResponse(lg))
	}
	return out, nil
}

// ============================================================================
// 4. 候选管理 API
// ============================================================================

// ListCandidates 查询资产包候选列表
func (s *SelfLearningService) ListCandidates(ctx context.Context, req *dto.CandidateListRequest) (*dto.CandidateListResponse, error) {
	if s == nil || s.candidateRepo == nil {
		return &dto.CandidateListResponse{List: []*dto.AssetBundleCandidateResponse{}}, nil
	}
	if err := s.validateListRequest(req); err != nil {
		return nil, err
	}

	// 按状态查询
	var candidates []*model.AssetBundleCandidate
	if req.Status != "" {
		cs, err := s.candidateRepo.ListByStatus(ctx, model.AssetBundleCandidateStatus(req.Status), req.Size)
		if err != nil {
			return nil, err
		}
		candidates = cs
	} else {
		// 默认查 candidate 状态
		cs, err := s.candidateRepo.ListByStatus(ctx, model.CandidateStatusCandidate, req.Size)
		if err != nil {
			return nil, err
		}
		candidates = cs
	}

	out := &dto.CandidateListResponse{
		List:  make([]*dto.AssetBundleCandidateResponse, 0, len(candidates)),
		Total: int64(len(candidates)),
		Page:  req.Page,
		Size:  req.Size,
	}
	for _, c := range candidates {
		out.List = append(out.List, toCandidateResponse(c))
	}
	return out, nil
}

// ============================================================================
// 5. A/B 实验 API
// ============================================================================

// ListABTests 查询 A/B 实验列表
func (s *SelfLearningService) ListABTests(ctx context.Context, req *dto.ABTestListRequest) (*dto.ABTestListResponse, error) {
	if s == nil || s.abTestRepo == nil {
		return &dto.ABTestListResponse{List: []*dto.ABTestResponse{}}, nil
	}
	if err := s.validateListRequest(req); err != nil {
		return nil, err
	}

	var tests []*model.AssetBundleABTest
	if req.Status != "" {
		ts, err := s.abTestRepo.ListByStatus(ctx, model.AssetBundleABTestStatus(req.Status), req.Size)
		if err != nil {
			return nil, err
		}
		tests = ts
	} else {
		// 默认查 running
		ts, err := s.abTestRepo.ListByStatus(ctx, model.ABTestStatusRunning, req.Size)
		if err != nil {
			return nil, err
		}
		tests = ts
	}

	out := &dto.ABTestListResponse{
		List:  make([]*dto.ABTestResponse, 0, len(tests)),
		Total: int64(len(tests)),
		Page:  req.Page,
		Size:  req.Size,
	}
	for _, t := range tests {
		out.List = append(out.List, toABTestResponse(t))
	}
	return out, nil
}

// PromoteABTest 人工晋升 A/B 实验结果（supervised 模式下人工确认）
func (s *SelfLearningService) PromoteABTest(ctx context.Context, req *dto.ABTestPromoteRequest) error {
	if s == nil || s.components == nil || s.components.Dispatcher == nil {
		return fmt.Errorf("self_learning: dispatcher not initialized")
	}
	if err := s.validateABTestPromote(req); err != nil {
		return err
	}
	// 调用 L4 dispatcher 的人工审核接口
	if req.WinnerArm == "candidate" {
		// candidate 胜出 → approve（执行 promote）
		return s.components.Dispatcher.ApproveAction(ctx, req.ExperimentID, req.OperatorID, req.Note)
	}
	// baseline 胜出 → reject（执行 rollback）
	return s.components.Dispatcher.RejectAction(ctx, req.ExperimentID, req.OperatorID, req.Note)
}

// ============================================================================
// 6. 矫正动作审计 API
// ============================================================================

// ListCorrections 查询矫正动作列表
func (s *SelfLearningService) ListCorrections(ctx context.Context, req *dto.CorrectionListRequest) (*dto.CorrectionAuditResponse, error) {
	if s == nil || s.actionRepo == nil {
		return &dto.CorrectionAuditResponse{List: []*dto.CorrectionActionItem{}}, nil
	}
	if err := s.validateListRequest(req); err != nil {
		return nil, err
	}
	filter := repository.CorrectionActionFilter{
		ActionType: model.CorrectionActionType(req.ActionType),
		TargetType: req.TargetType,
		Status:     model.CorrectionActionStatus(req.Status),
		Since:      req.Since,
		Until:      req.Until,
		Page:       req.Page,
		Size:       req.Size,
	}
	actions, total, err := s.actionRepo.ListByFilter(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := &dto.CorrectionAuditResponse{
		List:  make([]*dto.CorrectionActionItem, 0, len(actions)),
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
	}
	for _, a := range actions {
		out.List = append(out.List, toCorrectionItem(a))
	}
	return out, nil
}

// ApproveCorrection 人工批准待审矫正动作（supervised 模式）
func (s *SelfLearningService) ApproveCorrection(ctx context.Context, req *dto.CorrectionRollbackRequest) error {
	if s == nil || s.components == nil || s.components.Dispatcher == nil {
		return fmt.Errorf("self_learning: dispatcher not initialized")
	}
	if err := s.validateCorrectionRollback(req); err != nil {
		return err
	}
	return s.components.Dispatcher.ApproveAction(ctx, req.ActionID, req.OperatorID, req.Reason)
}

// RejectCorrection 人工拒绝待审矫正动作
func (s *SelfLearningService) RejectCorrection(ctx context.Context, req *dto.CorrectionRollbackRequest) error {
	if s == nil || s.components == nil || s.components.Dispatcher == nil {
		return fmt.Errorf("self_learning: dispatcher not initialized")
	}
	if err := s.validateCorrectionRollback(req); err != nil {
		return err
	}
	return s.components.Dispatcher.RejectAction(ctx, req.ActionID, req.OperatorID, req.Reason)
}

// ============================================================================
// DTO 转换辅助
// ============================================================================

func toLogResponse(lg *model.SelfLearningLog) *dto.SelfLearningLogResponse {
	if lg == nil {
		return nil
	}
	return &dto.SelfLearningLogResponse{
		LogID:         lg.LogID,
		SessionID:     lg.SessionID,
		TraceID:       lg.TraceID,
		Scenario:      lg.Scenario,
		TriggerEvent:  lg.TriggerEvent,
		Status:        lg.Status,
		InputSummary:  lg.InputSummary,
		OutputSummary: lg.OutputSummary,
		ErrorMsg:      lg.ErrorMsg,
		DurationMs:    lg.DurationMs,
		StartedAt:     lg.StartedAt,
		FinishedAt:    lg.FinishedAt,
		CreatedAt:     lg.CreatedAt,
	}
}

func toCandidateResponse(c *model.AssetBundleCandidate) *dto.AssetBundleCandidateResponse {
	if c == nil {
		return nil
	}
	// pq.StringArray → []string
	srcIDs := make([]string, 0, len(c.SourceSessionIDs))
	srcIDs = append(srcIDs, c.SourceSessionIDs...)
	// JSONMap → map[string]any
	var scripts map[string]any
	if c.ExtractedScripts != nil {
		scripts = c.ExtractedScripts
	}
	return &dto.AssetBundleCandidateResponse{
		CandidateID:      c.CandidateID,
		SourceSessionIDs: srcIDs,
		ExtractedScripts: scripts,
		ProposedMessages: c.ProposedMessages,
		Industry:         c.Industry,
		Language:         c.Language,
		Scenario:         c.Scenario,
		ClusterCount:     c.ClusterCount,
		RewardSum:        c.RewardSum,
		Status:           c.Status,
		ABTestID:         c.ABTestID,
		PromotedAssetID:  c.PromotedAssetID,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}

func toABTestResponse(t *model.AssetBundleABTest) *dto.ABTestResponse {
	if t == nil {
		return nil
	}
	return &dto.ABTestResponse{
		ExperimentID:     t.ExperimentID,
		BaselineAssetID:  t.BaselineAssetID,
		CandidateID:      t.CandidateID,
		Scenario:         t.Scenario,
		TrafficSplit:     t.TrafficSplit,
		Status:           t.Status,
		WinnerArm:        t.WinnerArm,
		BaselineSamples:  t.BaselineSamples,
		CandidateSamples: t.CandidateSamples,
		BaselineReward:   t.BaselineReward,
		CandidateReward:  t.CandidateReward,
		StartedAt:        t.StartedAt,
		ConvergedAt:      t.ConvergedAt,
		CompletedAt:      t.CompletedAt,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}

func toCorrectionItem(a *model.SelfCorrectionAction) *dto.CorrectionActionItem {
	if a == nil {
		return nil
	}
	return &dto.CorrectionActionItem{
		ActionID:      a.ActionID,
		ActionType:    string(a.ActionType),
		Scenario:      a.Scenario,
		TriggerLogID:  a.TriggerLogID,
		TargetType:    a.TargetType,
		TargetID:      a.TargetID,
		Before:        a.Before,
		After:         a.After,
		AutonomyLevel: a.AutonomyLevel,
		Operator:      a.Operator,
		Reason:        a.Reason,
		Status:        string(a.Status),
		AppliedAt:     a.AppliedAt,
		RolledBackAt:  a.RolledBackAt,
		CreatedAt:     a.CreatedAt,
	}
}

// ============================================================================
// 校验辅助（五层架构规范 §七：DTO 层禁止含业务逻辑，校验下沉至 service 层）
// ============================================================================

// validateSwitchConfig 校验自我学习开关配置请求
//
// 委托至 L4 层 selflearning.ValidateSwitchConfig，避免校验逻辑重复
func (s *SelfLearningService) validateSwitchConfig(req *dto.SwitchConfigRequest) error {
	return selflearning.ValidateSwitchConfig(req)
}

// validateListRequest 通用列表分页校验
//
// 等价于原各列表 DTO 的 Validate() 方法体
// 适用：SelfLearningLogListRequest / CandidateListRequest / ABTestListRequest / CorrectionListRequest
//
// 注意：调用方需在调用前进行类型断言，将 req 的 *Page/*Size 字段直接传入。
// 本函数会修改 page/size 指向的值（默认 Page=1, Size=50, 上限 500）。
func (s *SelfLearningService) validateListRequest(req interface{}) error {
	if req == nil {
		return dto.ErrSelfLearningRequestNil
	}
	// 通过类型 switch 处理 4 种列表 DTO，共享相同的分页规范化逻辑
	// （DTO 层禁止方法，因此规范化逻辑集中在 service 层）
	switch r := req.(type) {
	case *dto.SelfLearningLogListRequest:
		if r.Size <= 0 || r.Size > 500 {
			r.Size = 50
		}
		if r.Page <= 0 {
			r.Page = 1
		}
	case *dto.CandidateListRequest:
		if r.Size <= 0 || r.Size > 500 {
			r.Size = 50
		}
		if r.Page <= 0 {
			r.Page = 1
		}
	case *dto.ABTestListRequest:
		if r.Size <= 0 || r.Size > 500 {
			r.Size = 50
		}
		if r.Page <= 0 {
			r.Page = 1
		}
	case *dto.CorrectionListRequest:
		if r.Size <= 0 || r.Size > 500 {
			r.Size = 50
		}
		if r.Page <= 0 {
			r.Page = 1
		}
	default:
		return fmt.Errorf("self_learning: unsupported list request type %T", req)
	}
	return nil
}

// validateABTestPromote 校验 A/B 实验晋升请求
//
// 等价于原 dto.ABTestPromoteRequest.Validate() 方法体
func (s *SelfLearningService) validateABTestPromote(req *dto.ABTestPromoteRequest) error {
	if req == nil {
		return dto.ErrSelfLearningRequestNil
	}
	if req.ExperimentID == "" {
		return dto.ErrExperimentIDEmpty
	}
	if req.WinnerArm != "baseline" && req.WinnerArm != "candidate" {
		return dto.ErrInvalidWinnerArm
	}
	return nil
}

// validateCorrectionRollback 校验矫正动作回滚请求
//
// 等价于原 dto.CorrectionRollbackRequest.Validate() 方法体
func (s *SelfLearningService) validateCorrectionRollback(req *dto.CorrectionRollbackRequest) error {
	if req == nil {
		return dto.ErrSelfLearningRequestNil
	}
	if req.ActionID == "" {
		return dto.ErrActionIDEmpty
	}
	return nil
}
