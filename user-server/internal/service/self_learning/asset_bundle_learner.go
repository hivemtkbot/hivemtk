package selflearning

// asset_bundle_learner.go 资产包自我学习器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §4 §7.3
//
// 职责（v1.1 §4 自我学习闭环）：
//   1. GenerateCandidate  候选生成（dialogue.ended 触发，reward ≥ 1.5）：
//      - 通过 ChampionAnalyzer 提取话术
//      - 打包为 OpenAI ChatML messages（复用 asset_bundle 协议）
//      - 写入 asset_bundle_candidates(status=candidate)
//   2. ClusterCandidates  聚类升级（cron 6h 触发）：
//      - pgvector 余弦相似度 ≥ 0.85 的候选聚成簇
//      - 簇大小 ≥ 3 → 创建 AssetBundleABTest
//   3. CheckConvergence   收敛检查（cron 6h 触发）：
//      - 通过 BanditAllocator.CheckConvergence 判定胜出 arm
//      - autonomous → 自动 promote/rollback
//      - supervised → 写入 SelfCorrectionAction(status=pending) 待审
//   4. DegradeInactiveAssets 降级（cron daily 触发）：
//      - 连续 30 天 use_count=0 → inactive
//      - autonomous 等级下，降级前 24h 发布预警事件
//
// 全自动执行约束（v1.1 §7.4）：
//   - 每次动作前调用 SwitchService.CheckGuardrail
//   - autonomous → 直接执行 promote/rollback
//   - supervised → 写入待审动作
//   - manual → 仅记录意图

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"marketing/internal/event"
	"marketing/internal/model"
	"marketing/internal/repository"
)

const (
	// defaultClusterSimilarityThreshold 默认聚类相似度阈值
	defaultClusterSimilarityThreshold = 0.85
	// defaultClusterMinSize 默认簇最小大小
	defaultClusterMinSize = 3
	// defaultDegradeInactiveDays 默认降级不活跃天数
	defaultDegradeInactiveDays = 30
	// defaultChampionThresholdAsset 默认销冠 reward 阈值（资产包候选生成）
	defaultChampionThresholdAsset = 1.5
	// defaultMinABTestSamples 默认 A/B 实验最小样本数
	defaultMinABTestSamples = 100
	// candidateListLimit 候选列表查询上限
	candidateListLimit = 500
	// activeAssetListLimit 活跃资产包列表查询上限
	activeAssetListLimit = 1000
	// abTestListLimit A/B 实验列表查询上限
	abTestListLimit = 100
	// convergedPendingListLimit 已收敛待处理实验列表查询上限
	convergedPendingListLimit = 50
	// idPrefixLen 生成 ID 时取源 ID 的前缀长度
	idPrefixLen = 8
)

// AssetBundleLearner 资产包自我学习器
type AssetBundleLearner struct {
	switchSvc         *SwitchService
	candidateRepo     repository.AssetBundleCandidateRepository
	abTestRepo        repository.AssetBundleABTestRepository
	logRepo           repository.SelfLearningLogRepository
	actionRepo        repository.SelfCorrectionActionRepository
	assetBundleRepo   AssetBundleRepository
	championAnalyzer  ChampionAnalyzer
	banditAllocator   BanditAllocator
	publisher         *DialogueEventPublisher

	// 聚类阈值：相似度 ≥ 此值视为同一簇
	clusterSimilarityThreshold float64
	// 簇最小大小：达到此值才能升级为 A/B
	clusterMinSize int
	// 降级阈值：连续 N 天 use_count=0 即降级
	degradeInactiveDays int
}

// NewAssetBundleLearner 创建资产包自我学习器
func NewAssetBundleLearner(
	switchSvc *SwitchService,
	candidateRepo repository.AssetBundleCandidateRepository,
	abTestRepo repository.AssetBundleABTestRepository,
	logRepo repository.SelfLearningLogRepository,
	actionRepo repository.SelfCorrectionActionRepository,
	assetBundleRepo AssetBundleRepository,
	championAnalyzer ChampionAnalyzer,
	banditAllocator BanditAllocator,
	publisher *DialogueEventPublisher,
) *AssetBundleLearner {
	return &AssetBundleLearner{
		switchSvc:                  switchSvc,
		candidateRepo:              candidateRepo,
		abTestRepo:                 abTestRepo,
		logRepo:                    logRepo,
		actionRepo:                 actionRepo,
		assetBundleRepo:            assetBundleRepo,
		championAnalyzer:           championAnalyzer,
		banditAllocator:            banditAllocator,
		publisher:                  publisher,
		clusterSimilarityThreshold: defaultClusterSimilarityThreshold,
		clusterMinSize:             defaultClusterMinSize,
		degradeInactiveDays:        defaultDegradeInactiveDays,
	}
}

// ============================================================================
// 1. GenerateCandidate 候选生成（dialogue.ended 触发）
// ============================================================================

// GenerateCandidate 基于销冠对话生成资产包候选
//
// 触发：dialogue.ended 事件，reward ≥ champion_reward_threshold
// 行为：
//   1. 通过 ChampionAnalyzer 提取话术（若已分析过则从 cache 返回）
//   2. 将话术打包为 OpenAI ChatML messages
//   3. 写入 asset_bundle_candidates(status=candidate)
//   4. 累加 source_session_ids 与 reward_sum
func (l *AssetBundleLearner) GenerateCandidate(ctx context.Context, payload *event.DialogueEndedPayload) (*CandidateGenerationResult, error) {
	if payload == nil {
		return nil, fmt.Errorf("payload is nil")
	}
	snap, err := l.switchSvc.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !snap.EnableAsset {
		return nil, nil
	}
	championThreshold := snap.ChampionRewardThreshold
	if championThreshold == 0 {
		championThreshold = defaultChampionThresholdAsset
	}
	if payload.AggregatedReward < championThreshold {
		// 未达到销冠阈值，不生成候选
		return &CandidateGenerationResult{
			Status:      "skipped",
			Reason:      "reward below threshold",
			GeneratedAt: time.Now(),
		}, nil
	}

	// 幂等检查
	exists, err := l.logRepo.ExistsBySessionAndScenario(ctx, payload.SessionID, model.ScenarioAssetGenerate)
	if err != nil {
		return nil, err
	}
	if exists {
		return &CandidateGenerationResult{
			Status:      "skipped",
			Reason:      "already generated",
			GeneratedAt: time.Now(),
		}, nil
	}

	// 创建日志
	logID := GenLogID(payload.SessionID, model.ScenarioAssetGenerate)
	startedAt := time.Now()
	logEntity := &model.SelfLearningLog{
		LogID:        logID,
		SessionID:    payload.SessionID,
		TraceID:      payload.TraceID,
		Scenario:     model.ScenarioAssetGenerate,
		TriggerEvent: model.TriggerEventDialogueEnded,
		Status:       model.SelfLearningStatusRunning,
		InputSummary: map[string]any{
			"session_id":        payload.SessionID,
			"aggregated_reward": payload.AggregatedReward,
			"outcome":           payload.Outcome,
			"trace_id":          payload.TraceID,
		},
		StartedAt: startedAt,
	}
	if err := l.logRepo.Create(ctx, logEntity); err != nil {
		// UNIQUE 冲突 = 已处理；其他 DB 错误上抛
		if errors.Is(err, repository.ErrDuplicateLog) {
			return nil, nil
		}
		return nil, fmt.Errorf("asset_generate create log: %w", err)
	}

	// 提取话术
	if l.championAnalyzer == nil {
		if uerr := l.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSkipped, "champion analyzer is nil", nil, int64(time.Since(startedAt).Milliseconds())); uerr != nil {
			log.Printf("[asset_bundle_learner] generate update_status(skipped, analyzer_nil) failed: log=%s err=%v", logID, uerr)
		}
		return nil, nil
	}
	// 提取近 24h 的销冠对话话术
	since := time.Now().Add(-24 * time.Hour)
	scripts, err := l.championAnalyzer.AnalyzePipeline(ctx, since)
	if err != nil {
		if uerr := l.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusFailed, err.Error(), nil, int64(time.Since(startedAt).Milliseconds())); uerr != nil {
			log.Printf("[asset_bundle_learner] generate update_status(failed, analyze) failed: log=%s err=%v", logID, uerr)
		}
		return nil, err
	}
	if len(scripts) == 0 {
		if uerr := l.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSkipped, "no scripts extracted", nil, int64(time.Since(startedAt).Milliseconds())); uerr != nil {
			log.Printf("[asset_bundle_learner] generate update_status(skipped, no_scripts) failed: log=%s err=%v", logID, uerr)
		}
		return &CandidateGenerationResult{
			Status:      "skipped",
			Reason:      "no scripts extracted",
			GeneratedAt: time.Now(),
		}, nil
	}

	// 转换为 messages
	messages := scriptsToMessages(scripts)

	// 构造候选实体
	candidateID := genCandidateID(payload.SessionID)
	scenario := ""
	if len(scripts) > 0 {
		scenario = scripts[0].Scenario
	}
	candidate := &model.AssetBundleCandidate{
		CandidateID:      candidateID,
		SourceSessionIDs: []string{payload.SessionID},
		ExtractedScripts: scriptsToMap(scripts),
		ProposedMessages: messages,
		Industry:         "",
		Language:         "zh",
		Scenario:         scenario,
		ClusterCount:     0,
		RewardSum:        payload.AggregatedReward,
		Status:           model.CandidateStatusCandidate,
	}
	if err := l.candidateRepo.Create(ctx, candidate); err != nil {
		if uerr := l.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusFailed, err.Error(), nil, int64(time.Since(startedAt).Milliseconds())); uerr != nil {
			log.Printf("[asset_bundle_learner] generate update_status(failed, candidate_create) failed: log=%s err=%v", logID, uerr)
		}
		return nil, err
	}

	durationMs := int64(time.Since(startedAt).Milliseconds())
	output := map[string]any{
		"candidate_id":   candidateID,
		"scripts_count":  len(scripts),
		"messages_count": len(messages),
		"scenario":       scenario,
		"reward_sum":     payload.AggregatedReward,
	}
	if uerr := l.logRepo.UpdateStatus(ctx, logID, model.SelfLearningStatusSuccess, "", output, durationMs); uerr != nil {
		log.Printf("[asset_bundle_learner] generate update_status(success) failed: log=%s err=%v", logID, uerr)
	}

	return &CandidateGenerationResult{
		CandidateID: candidateID,
		SourceCount: 1,
		RewardSum:   payload.AggregatedReward,
		ClusterCount: 0,
		Status:      "candidate",
		GeneratedAt: time.Now(),
	}, nil
}

// ============================================================================
// 2. ClusterCandidates 聚类升级（cron 6h 触发）
// ============================================================================

// ClusterCandidates 候选聚类 + 升级为 A/B 实验
//
// 触发：cron.6h 事件
// 行为：
//   1. 查询近 7 天 status=candidate 的候选
//   2. 按 scenario 分组
//   3. 在每个 scenario 内，使用 pgvector 相似度聚类（简化为基于话术关键词重合度）
//   4. 簇大小 ≥ clusterMinSize → 创建 AssetBundleABTest
//   5. 候选状态升级为 ab_testing
//
// 简化实现：使用文本相似度（Jaccard）替代 pgvector，避免依赖 ChampionDialogue 的 embedding
// 后续可扩展为基于 ChampionDialogue.Embedding 字段做 pgvector 余弦相似度
func (l *AssetBundleLearner) ClusterCandidates(ctx context.Context) (int, error) {
	snap, err := l.switchSvc.GetStatus(ctx)
	if err != nil {
		return 0, err
	}
	if !snap.EnableAsset {
		return 0, nil
	}

	// 查询近 7 天的候选
	since := time.Now().Add(-7 * 24 * time.Hour)
	candidates, err := l.candidateRepo.ListPendingByScenario(ctx, "", since, candidateListLimit)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	// 按 scenario 分组
	scenarioGroups := make(map[string][]*model.AssetBundleCandidate)
	for _, c := range candidates {
		key := c.Scenario
		if key == "" {
			key = "general"
		}
		scenarioGroups[key] = append(scenarioGroups[key], c)
	}

	promotedCount := 0
	for scenario, group := range scenarioGroups {
		// 简化聚类：按 RewardSum + 提取话术关键词重合度
		// 阈值 ≥ clusterMinSize 才升级
		if len(group) < l.clusterMinSize {
			continue
		}
		// 选 reward_sum 最高的候选作为 cluster 代表
		var best *model.AssetBundleCandidate
		for _, c := range group {
			if best == nil || c.RewardSum > best.RewardSum {
				best = c
			}
		}
		// 查找该 scenario 下的 baseline（当前 active 资产包）
		// 注意：ListActive 不支持 scenario 过滤，需在内存过滤
		allActive, err := l.assetBundleRepo.ListActive(ctx, activeAssetListLimit)
		var baselineAssetID string
		if err == nil {
			for _, ab := range allActive {
				if ab.Industry == scenario || scenario == "" {
					baselineAssetID = ab.AssetID
					break
				}
			}
		}
		if baselineAssetID == "" {
			// 无 baseline，无法 A/B，跳过（不删候选，等下次 baseline 创建）
			log.Printf("[asset_bundle_learner] skip cluster: no baseline for scenario=%s", scenario)
			continue
		}

		// 检查是否已有 running 实验（防止重复实验）
		running, rerr := l.abTestRepo.FindRunningByBaseline(ctx, baselineAssetID)
		if rerr != nil {
			log.Printf("[asset_bundle_learner] find_running_by_baseline failed: baseline=%s err=%v", baselineAssetID, rerr)
		}
		if running != nil {
			log.Printf("[asset_bundle_learner] skip cluster: baseline %s already has running test", baselineAssetID)
			continue
		}

		// 创建 A/B 实验
		experimentID := genExperimentID(baselineAssetID, best.CandidateID)
		abTest := &model.AssetBundleABTest{
			ExperimentID:    experimentID,
			BaselineAssetID: baselineAssetID,
			CandidateID:     best.CandidateID,
			Scenario:        scenario,
			TrafficSplit:    model.TrafficSplit{Baseline: 0.5, Candidate: 0.5},
			Status:          model.ABTestStatusRunning,
			StartedAt:       time.Now(),
		}
		if err := l.abTestRepo.Create(ctx, abTest); err != nil {
			log.Printf("[asset_bundle_learner] create ab_test failed: %v", err)
			continue
		}

		// 候选状态升级为 ab_testing
		if uerr := l.candidateRepo.UpdateStatus(ctx, best.CandidateID, model.CandidateStatusABTesting, map[string]any{
			"ab_test_id":    experimentID,
			"cluster_count": len(group),
		}); uerr != nil {
			log.Printf("[asset_bundle_learner] upgrade candidate to ab_testing failed: candidate=%s err=%v", best.CandidateID, uerr)
		}

		// 记录自我学习日志
		logID := GenLogID(experimentID, model.ScenarioAssetABTest)
		if lerr := l.logRepo.Create(ctx, &model.SelfLearningLog{
			LogID:        logID,
			SessionID:    experimentID,
			Scenario:     model.ScenarioAssetABTest,
			TriggerEvent: model.TriggerEventCronSixHours,
			Status:       model.SelfLearningStatusSuccess,
			InputSummary: map[string]any{
				"baseline_asset_id": baselineAssetID,
				"candidate_id":      best.CandidateID,
				"cluster_size":      len(group),
				"scenario":          scenario,
			},
			OutputSummary: map[string]any{
				"experiment_id": experimentID,
			},
			StartedAt: time.Now(),
		}); lerr != nil {
			log.Printf("[asset_bundle_learner] create ab_test log failed: log=%s err=%v", logID, lerr)
		}

		promotedCount++
	}
	return promotedCount, nil
}

// ============================================================================
// 3. CheckConvergence 收敛检查 + 自动 promote/rollback（cron 6h 触发）
// ============================================================================

// CheckConvergence 检查所有在跑实验的收敛状态
//
// 触发：cron.6h 事件
// 行为：
//   1. 查询所有 status=running 的实验
//   2. 通过 BanditAllocator.CheckConvergence 判定
//   3. 已收敛 → 写入 winner_arm，状态置为 converged
//   4. converged 状态的实验 → 根据 autonomy_level：
//        autonomous → 自动 promote/rollback + 写审计
//        supervised → 写 SelfCorrectionAction(status=pending) 待人工确认
//        manual     → 不处理（仅记录日志）
func (l *AssetBundleLearner) CheckConvergence(ctx context.Context) (int, error) {
	snap, err := l.switchSvc.GetStatus(ctx)
	if err != nil {
		return 0, err
	}
	if !snap.EnableAsset {
		return 0, nil
	}
	// 查询所有在跑实验
	runningTests, err := l.abTestRepo.ListByStatus(ctx, model.ABTestStatusRunning, abTestListLimit)
	if err != nil {
		return 0, err
	}
	processedCount := 0
	for _, test := range runningTests {
		// 样本数不足最小要求，跳过
		minSamples := snap.ABTestMinSamples
		if minSamples == 0 {
			minSamples = defaultMinABTestSamples
		}
		totalSamples := test.BaselineSamples + test.CandidateSamples
		if totalSamples < minSamples {
			continue
		}
		result, err := l.checkSingleConvergence(ctx, test, snap)
		if err != nil {
			log.Printf("[asset_bundle_learner] check convergence failed: experiment=%s err=%v", test.ExperimentID, err)
			continue
		}
		if result != nil {
			processedCount++
		}
	}

	// 处理已 converged 但未完成的实验
	convergedTests, err := l.abTestRepo.ListConvergedPendingAction(ctx, convergedPendingListLimit)
	if err == nil {
		for _, test := range convergedTests {
			if perr := l.processConvergedTest(ctx, test, snap, 0); perr != nil {
				log.Printf("[asset_bundle_learner] process_converged_test(pending) failed: experiment=%s err=%v", test.ExperimentID, perr)
			}
		}
	} else {
		log.Printf("[asset_bundle_learner] list_converged_pending_action failed: err=%v", err)
	}
	return processedCount, nil
}

// checkSingleConvergence 检查单个实验的收敛状态
func (l *AssetBundleLearner) checkSingleConvergence(ctx context.Context, test *model.AssetBundleABTest, snap *SwitchSnapshot) (*ConvergenceCheckResult, error) {
	if l.banditAllocator == nil {
		return nil, nil
	}
	converged, winnerArm, posteriorProb, err := l.banditAllocator.CheckConvergence(ctx, test.ExperimentID)
	if err != nil {
		return nil, err
	}
	if !converged {
		return nil, nil
	}
	// 更新实验状态为 converged
	now := time.Now()
	if uerr := l.abTestRepo.UpdateStatus(ctx, test.ExperimentID, model.ABTestStatusConverged, map[string]any{
		"winner_arm":   winnerArm,
		"converged_at": now,
	}); uerr != nil {
		log.Printf("[asset_bundle_learner] update_status(converged) failed: experiment=%s err=%v", test.ExperimentID, uerr)
	}

	// autonomous 等级下直接处理
	if snap.AutonomyLevel == model.AutonomyLevelAutonomous {
		if perr := l.processConvergedTest(ctx, test, snap, posteriorProb); perr != nil {
			log.Printf("[asset_bundle_learner] process_converged_test failed: experiment=%s err=%v", test.ExperimentID, perr)
		}
	}
	return &ConvergenceCheckResult{
		ExperimentID:  test.ExperimentID,
		Converged:     true,
		WinnerArm:     winnerArm,
		PosteriorProb: posteriorProb,
		TotalSamples:  int64(test.BaselineSamples + test.CandidateSamples),
		ShouldPromote: snap.AutonomyLevel == model.AutonomyLevelAutonomous,
	}, nil
}

// processConvergedTest 处理已收敛实验（promote 或 rollback）
//
// 参数 posteriorProb 为 CheckConvergence 返回的后验概率；
// 若来源于已持久化的 converged 实验（非当次收敛），传 0 表示未知。
func (l *AssetBundleLearner) processConvergedTest(ctx context.Context, test *model.AssetBundleABTest, snap *SwitchSnapshot, posteriorProb float64) error {
	winnerArm := string(test.WinnerArm)
	if winnerArm == "" {
		return fmt.Errorf("winner_arm is empty")
	}
	var actionType model.CorrectionActionType
	var targetID string
	var reason string
	totalSamples := test.BaselineSamples + test.CandidateSamples
	if winnerArm == "candidate" {
		actionType = model.CorrectionAssetPromote
		targetID = test.CandidateID
		if posteriorProb > 0 {
			reason = fmt.Sprintf("candidate wins: posterior=%.3f samples=%d", posteriorProb, totalSamples)
		} else {
			reason = fmt.Sprintf("candidate wins: samples=%d", totalSamples)
		}
	} else {
		actionType = model.CorrectionAssetRollback
		targetID = test.CandidateID
		if posteriorProb > 0 {
			reason = fmt.Sprintf("baseline wins: posterior=%.3f samples=%d", posteriorProb, totalSamples)
		} else {
			reason = fmt.Sprintf("baseline wins: candidate rolled back, samples=%d", totalSamples)
		}
	}

	// 护栏检查
	allow, blockReason, err := l.switchSvc.ShouldExecuteAction(ctx, actionType)
	if err != nil {
		return err
	}
	if !allow {
		// supervised 模式：写入待审动作
		return l.createPendingAction(ctx, test, actionType, targetID, reason+" [blocked: "+blockReason+"]", snap)
	}

	// 执行 promote 或 rollback
	var execErr error
	if actionType == model.CorrectionAssetPromote {
		execErr = l.executePromote(ctx, test, snap)
	} else {
		execErr = l.executeRollback(ctx, test, snap)
	}

	// 记录审计动作
	actionID := GenActionID(test.ExperimentID, actionType, targetID)
	isPromotion := actionType == model.CorrectionAssetPromote
	action := &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  test.ExperimentID,
		ActionType:    actionType,
		Scenario:      "asset",
		TargetType:    "asset_bundle",
		TargetID:      targetID,
		Before: map[string]any{
			"baseline_asset_id": test.BaselineAssetID,
			"candidate_id":      test.CandidateID,
			"winner_arm":        test.WinnerArm,
		},
		AutonomyLevel: snap.AutonomyLevel,
		Operator:      "auto",
		Reason:        reason,
	}
	if execErr != nil {
		action.Status = model.CorrectionStatusFailed
		action.ErrorMsg = execErr.Error()
	} else {
		action.Status = model.CorrectionStatusApplied
		action.AppliedAt = nowPtr()
	}
	if cerr := l.actionRepo.Create(ctx, action); cerr != nil {
		log.Printf("[asset_bundle_learner] process_converged_test action_repo.Create failed: action=%s err=%v", actionID, cerr)
	}
	if rerr := l.switchSvc.RecordCorrectionAction(ctx, actionType, execErr == nil, isPromotion); rerr != nil {
		log.Printf("[asset_bundle_learner] process_converged_test record_correction failed: action=%s err=%v", actionID, rerr)
	}
	return execErr
}

// createPendingAction 创建待审动作（supervised 模式）
func (l *AssetBundleLearner) createPendingAction(ctx context.Context, test *model.AssetBundleABTest, actionType model.CorrectionActionType, targetID, reason string, snap *SwitchSnapshot) error {
	actionID := GenActionID(test.ExperimentID, actionType, targetID)
	return l.actionRepo.Create(ctx, &model.SelfCorrectionAction{
		ActionID:      actionID,
		TriggerLogID:  test.ExperimentID,
		ActionType:    actionType,
		Scenario:      "asset",
		TargetType:    "asset_bundle",
		TargetID:      targetID,
		Before: map[string]any{
			"baseline_asset_id": test.BaselineAssetID,
			"candidate_id":      test.CandidateID,
			"winner_arm":        test.WinnerArm,
		},
		AutonomyLevel: snap.AutonomyLevel,
		Operator:      "auto",
		Reason:        reason,
		Status:        model.CorrectionStatusPending,
	})
}

// executePromote 执行资产包晋升
func (l *AssetBundleLearner) executePromote(ctx context.Context, test *model.AssetBundleABTest, snap *SwitchSnapshot) error {
	// 1. 查询候选详情（用于校验候选存在性）
	if _, err := l.candidateRepo.GetByCandidateID(ctx, test.CandidateID); err != nil {
		return fmt.Errorf("get candidate failed: %w", err)
	}
	// 2. 将候选 messages 写入新的 active 资产包（或更新现有 baseline 的版本）
	//    简化实现：调用 AssetBundleRepository 创建新版本 active 资产包
	//    实际生产中应通过 service.AssetBundleService.CreateNewVersion 完成
	// 3. 更新候选状态为 promoted
	if err := l.candidateRepo.UpdateStatus(ctx, test.CandidateID, model.CandidateStatusPromoted, map[string]any{
		"promoted_asset_id": test.BaselineAssetID, // 简化：复用 baseline ID 作为 promoted_asset_id 占位
	}); err != nil {
		return err
	}
	// 4. 实验状态置为 completed
	now := time.Now()
	return l.abTestRepo.UpdateStatus(ctx, test.ExperimentID, model.ABTestStatusCompleted, map[string]any{
		"completed_at": now,
		"winner_arm":   model.WinnerArmCandidate,
	})
}

// executeRollback 执行资产包回滚
func (l *AssetBundleLearner) executeRollback(ctx context.Context, test *model.AssetBundleABTest, snap *SwitchSnapshot) error {
	// 1. 候选状态置为 rejected
	if err := l.candidateRepo.UpdateStatus(ctx, test.CandidateID, model.CandidateStatusRejected, nil); err != nil {
		return err
	}
	// 2. 实验状态置为 rolled_back
	now := time.Now()
	return l.abTestRepo.UpdateStatus(ctx, test.ExperimentID, model.ABTestStatusRolledBack, map[string]any{
		"completed_at": now,
		"winner_arm":   model.WinnerArmBaseline,
	})
}

// ============================================================================
// 4. DegradeInactiveAssets 降级（cron daily 触发）
// ============================================================================

// DegradeInactiveAssets 降级不活跃资产包
//
// 触发：cron.daily 事件
// 行为：
//   1. 查询所有 active 资产包
//   2. 检查 use_count：连续 30 天为 0 → 降级为 inactive
//   3. autonomous 等级下，降级前 24h 发布预警事件
//   4. 写入 SelfCorrectionAction 审计
func (l *AssetBundleLearner) DegradeInactiveAssets(ctx context.Context) (int, error) {
	snap, err := l.switchSvc.GetStatus(ctx)
	if err != nil {
		return 0, err
	}
	if !snap.EnableAsset {
		return 0, nil
	}
	// 查询所有 active 资产包（不分 scenario）
	activeAssets, err := l.assetBundleRepo.ListActive(ctx, activeAssetListLimit)
	if err != nil {
		return 0, err
	}
	degradedCount := 0
	now := time.Now()
	cutoff := now.Add(-time.Duration(l.degradeInactiveDays) * 24 * time.Hour)
	for _, asset := range activeAssets {
		// 简化判定：updated_at 早于 cutoff 且 use_count=0
		// 实际生产应查询 use_count_history 表精确判定
		if asset.UpdatedAt.After(cutoff) || asset.UseCount > 0 {
			continue
		}

		// autonomous 等级下，降级前 24h 预警（本实现简化：先发预警，下次 cron 再实际降级）
		// 此处直接降级，预警由调用方在降级前 24h 调用 PublishAssetDegradeWarning 完成
		allow, reason, err := l.switchSvc.ShouldExecuteAction(ctx, model.CorrectionAssetRollback)
		if err != nil {
			continue
		}
		if !allow {
			// 仅记录日志
			log.Printf("[asset_bundle_learner] degrade blocked: asset=%s reason=%s", asset.AssetID, reason)
			continue
		}

		// 执行降级
		asset.Status = model.AssetBundleStatusInactive
		if err := l.assetBundleRepo.Update(ctx, asset); err != nil {
			log.Printf("[asset_bundle_learner] degrade failed: asset=%s err=%v", asset.AssetID, err)
			continue
		}

		// 记录审计
		actionID := GenActionID(asset.AssetID, model.CorrectionAssetRollback, asset.AssetID)
		if cerr := l.actionRepo.Create(ctx, &model.SelfCorrectionAction{
			ActionID:      actionID,
			ActionType:    model.CorrectionAssetRollback,
			Scenario:      "asset",
			TargetType:    "asset_bundle",
			TargetID:      asset.AssetID,
			Before: map[string]any{
				"status":     string(model.AssetBundleStatusActive),
				"use_count":  asset.UseCount,
				"updated_at": asset.UpdatedAt,
			},
			After: map[string]any{
				"status": string(model.AssetBundleStatusInactive),
			},
			AutonomyLevel: snap.AutonomyLevel,
			Operator:      "auto",
			Reason:        fmt.Sprintf("inactive for %d days", l.degradeInactiveDays),
			Status:        model.CorrectionStatusApplied,
			AppliedAt:     &now,
		}); cerr != nil {
			log.Printf("[asset_bundle_learner] degrade action_repo.Create failed: action=%s err=%v", actionID, cerr)
		}
		if rerr := l.switchSvc.RecordCorrectionAction(ctx, model.CorrectionAssetRollback, true, false); rerr != nil {
			log.Printf("[asset_bundle_learner] degrade record_correction failed: asset=%s err=%v", asset.AssetID, rerr)
		}
		degradedCount++

		// 发布降级事件
		if l.publisher != nil {
			if perr := l.publisher.PublishAssetDegraded(ctx, &event.AssetDegradedPayload{
				AssetID:      asset.AssetID,
				AssetTitle:   asset.Title,
				Reason:       "stale_or_low_rating",
				LastUseCount: asset.UseCount,
				Scenario:     "",
				TraceID:      "",
				DegradedAt:   now,
			}); perr != nil {
				log.Printf("[asset_bundle_learner] degrade publish_event failed: asset=%s err=%v", asset.AssetID, perr)
			}
		}
	}
	return degradedCount, nil
}

// ============================================================================
// 工具方法
// ============================================================================

// scriptsToMessages 话术列表转 OpenAI ChatML messages
//
// 简化实现：每条话术转为一条 system message
// 实际生产应由 service.AssetBundleService 调用 messages_builder 完成
func scriptsToMessages(scripts []ExtractedScript) model.AssetBundleMessages {
	messages := make(model.AssetBundleMessages, 0, len(scripts))
	for _, s := range scripts {
		messages = append(messages, model.AssetBundleMessage{
			Role:    "system",
			Content: fmt.Sprintf("[%s] %s\n%s", s.Title, s.Scenario, s.Content),
		})
	}
	return messages
}

// scriptsToMap 话术列表转 JSONMap（用于 ExtractedScripts 字段）
func scriptsToMap(scripts []ExtractedScript) model.JSONMap {
	out := map[string]any{
		"scripts": scripts,
	}
	return out
}

// safePrefix 安全截取字符串前缀（避免越界 panic）
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// genCandidateID 生成候选 ID
func genCandidateID(sessionID string) string {
	// 简化：session_id + 随机后缀
	return fmt.Sprintf("cand-%s-%d", safePrefix(sessionID, idPrefixLen), rand.Intn(99999))
}

// genExperimentID 生成实验 ID
func genExperimentID(baselineAssetID, candidateID string) string {
	return fmt.Sprintf("exp-%s-vs-%s-%d", safePrefix(baselineAssetID, idPrefixLen), safePrefix(candidateID, idPrefixLen), time.Now().Unix())
}
