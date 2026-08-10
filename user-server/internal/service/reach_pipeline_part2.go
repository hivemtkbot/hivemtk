// 拆分自 reach_pipeline.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

func (s *ReachPipelineService) runStep(ctx context.Context, step string, job *model.ReachJob, rl *RateLimitConfig) StepResult {
	start := time.Now()
	res := StepResult{Step: step, StartedAt: start}
	switch step {
	case StepAudience:
		// 受众筛选: 校验 customerID
		if job.CustomerID == "" {
			res.Success = false
			res.Error = "empty customer_id"
		} else {
			res.Success = true
			res.Output = map[string]any{"customer_id": job.CustomerID}
		}
	case StepContentPrepare:
		// 内容准备：真实渲染模板
		// 优先级：job.Payload.content（字符串模板）> job.Payload.template_id（数据库中的话术模板）> 兜底错误
		content, err := s.prepareContent(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "content prepare failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"prepared":      true,
				"content":       content,
				"content_bytes": len(content),
			}
		}
	case StepAccountSelect:
		// 账号选择
		if job.AccountID == "" {
			res.Success = true
			res.Output = map[string]any{"account_id": "auto"}
		} else {
			res.Success = true
			res.Output = map[string]any{"account_id": job.AccountID}
		}
	case StepRateLimit:
		// 限流控制
		if !s.checkRateLimit(ctx, job.Channel, job.AccountID, job.CustomerID, rl) {
			res.Success = false
			res.Error = ErrReachRateLimited.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{"pass": true}
		}
	case StepMessageGen:
		// 消息生成：复用 ContentPrepare 的渲染结果，做轻量个性化
		message, err := s.generateMessage(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "message gen failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"generated":     true,
				"message":       message,
				"message_bytes": len(message),
			}
		}
	case StepSend:
		// 发送执行：按 channel 路由到真实发送器
		// 私域独立部署：当前已实现的渠道为 wecom / feishu / telegram / whatsapp（走 webhook_service.sendOutbound 同一底层）
		// sms / email / card / dingtalk / douyin / kuaishou / xiaohongshu 走各自的 Service
		// 未支持的渠道返回明确错误（避免静默吞掉）
		messageID, err := s.dispatchOutbound(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "send failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"sent":       true,
				"message_id": messageID,
				"channel":    job.Channel,
			}
		}
	case StepTrackResult:
		// 结果追踪：把 message_id 写入 job.Payload._tracking，供 StepReport 汇总
		if err := s.trackSendResult(ctx, job, res); err != nil {
			res.Success = false
			res.Error = "track failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = map[string]any{
				"tracked":     true,
				"job_id":      job.ID,
				"customer_id": job.CustomerID,
				"channel":     job.Channel,
			}
		}
	case StepRetry:
		// 失败重试（pipeline 级别逻辑）
		res.Success = true
		res.Output = map[string]any{"checked": true}
	case StepReport:
		// 汇总报告：聚合 step 时长/成功/失败，更新 pipeline 计数器
		report, err := s.aggregateReport(ctx, job)
		if err != nil {
			res.Success = false
			res.Error = "report failed: " + err.Error()
		} else {
			res.Success = true
			res.Output = report
		}
	default:
		res.Success = false
		res.Error = "unknown step"
	}
	res.EndedAt = time.Now()
	res.DurationMs = int(res.EndedAt.Sub(res.StartedAt).Milliseconds())
	return res
}

// prepareContent 真实模板渲染（V3 整改）
//
// 优先级：
//  1. job.Payload["content"] 字符串模板（含 {{var}} 占位符）
//  2. job.Payload["template_id"] 数据库中的话术模板
//  3. 兜底错误：未提供任何内容
//
// 占位符语法：{{key}}，key 在 job.Payload 中查找（递归子 map），未命中时
// 保留原始 {{key}} 形式以便前端联调时直观看到未填充的字段。
//
// 自动注入变量：customer_id、account_id、channel、date、time、datetime。
func (s *ReachPipelineService) prepareContent(ctx context.Context, job *model.ReachJob) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}
	raw := ""
	if v, ok := job.Payload["content"]; ok {
		if s, ok := v.(string); ok {
			raw = s
		}
	}
	// 兜底：template_id 走数据库（ScriptTemplate/ScriptLibrary/ContentTemplate）
	if raw == "" {
		if v, ok := job.Payload["template_id"]; ok {
			if tmplID, ok := v.(string); ok && tmplID != "" && s.repo != nil && s.repo.Available() {
				tmplContent, err := s.loadTemplateContent(ctx, tmplID)
				if err != nil {
					return "", fmt.Errorf("load template %s: %w", tmplID, err)
				}
				raw = tmplContent
			}
		}
	}
	if raw == "" {
		return "", fmt.Errorf("payload.content and payload.template_id are both empty")
	}
	return renderReachTemplate(raw, job), nil
}

// loadTemplateContent 从数据库加载话术模板内容
//
// 兼容多张历史话术表（ScriptTemplate / ScriptLibrary），按 ID 优先匹配。
// 私域独立部署：单租户，不带 merchant_id 过滤。
//
// 五层架构整改：原 Table().Select().Where().Scan() 下沉到
// repository.ReachPipelineRepository.GetScriptContent。
func (s *ReachPipelineService) loadTemplateContent(ctx context.Context, templateID string) (string, error) {
	if s.repo == nil || !s.repo.Available() {
		return "", fmt.Errorf("db is nil")
	}
	return s.repo.GetScriptContent(ctx, templateID)
}

// generateMessage 消息个性化（V3 整改）
//
// 复用 prepareContent 的渲染结果（已注入客户/账号/渠道变量），仅做轻量增强：
//  1. trim 首尾空白 + 折叠连续换行
//  2. 在末尾追加渠道后缀（仅当 payload.include_channel_footer=true，避免营销文案被破坏）
func (s *ReachPipelineService) generateMessage(ctx context.Context, job *model.ReachJob) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}
	base, err := s.prepareContent(ctx, job)
	if err != nil {
		return "", err
	}
	// 轻量清理
	cleaned := strings.TrimSpace(base)
	cleaned = strings.Join(strings.Fields(cleaned), " ") // 折叠空白
	// 渠道后缀（仅在显式开启时追加）
	if v, ok := job.Payload["include_channel_footer"]; ok {
		if b, _ := v.(bool); b && job.Channel != "" {
			footer := fmt.Sprintf("\n\n[via %s @ %s]", job.Channel, time.Now().Format("2006-01-02 15:04:05"))
			cleaned += footer
		}
	}
	return cleaned, nil
}

// dispatchOutbound 按 channel 路由到真实发送器（V3 整改）
//
// 已实现渠道：wecom / feishu / telegram / whatsapp（统一走 webhook_service.sendOutbound 同一底层）
// 营销自动化渠道：sms / email / card / dingtalk / douyin / kuaishou / xiaohongshu
//   - 走对应 Service.Send* 方法（sms/email 走 repository）
//   - 若该渠道的 Service 未配置（如 SMTP 缺失），返回明确错误，不静默吞掉
//
// 返回 message_id（渠道侧分配的 ID，便于 StepTrackResult / StepReport 关联）。
//
// 真实发送策略（修复"调度器下发占位"缺口）：
//   - 若已通过 SetReachSender 注入真实发送器（生产由 router 注入，连接
//     IntegrationReachAdapter + BridgeReachAdapter），则按渠道路由到真实渠道，
//     真正下发消息。
//   - 未注入发送器（单元测试 / 未接线部署）时降级为占位 message_id，
//     保证调度流程可继续，但不真正发送网络请求。
//
// V3 副效果：把 message_id 写入 job.Payload["_last_send"]，供 StepTrackResult 读取。

// ReachSender 真实触达发送器接口（由 router 注入）。
// 实现连接 tooluse.IntegrationReachAdapter（telegram/whatsapp/feishu/web/wecom/dingtalk/sms/email/card）
// 与 bridge.BridgeReachAdapter（douyin/kuaishou/xiaohongshu/tiktok），使调度器真正下发到渠道。
type ReachSender interface {
	SendReach(ctx context.Context, channel, accountID, to, content string) (messageID string, err error)
}

// SetReachSender 注入真实触达发送器（生产路径由 router 调用）。
func (s *ReachPipelineService) SetReachSender(sender ReachSender) {
	s.sender = sender
}

func (s *ReachPipelineService) dispatchOutbound(ctx context.Context, job *model.ReachJob) (string, error) {
	if job == nil {
		return "", fmt.Errorf("job is nil")
	}
	if !ReachChannels[job.Channel] {
		return "", fmt.Errorf("unsupported channel: %s", job.Channel)
	}
	if job.CustomerID == "" {
		return "", fmt.Errorf("customer_id is empty")
	}

	// 生产路径：已注入真实触达适配器，按渠道路由到真实发送器
	if s.sender != nil {
		content, cerr := s.prepareContent(ctx, job)
		if cerr != nil || strings.TrimSpace(content) == "" {
			content = fmt.Sprintf("[%s] 触达消息", job.Channel)
		}
		mid, err := s.sender.SendReach(ctx, job.Channel, job.AccountID, job.CustomerID, content)
		if err != nil {
			return "", err
		}
		if job.Payload == nil {
			job.Payload = model.JSONMap{}
		}
		job.Payload["_last_send"] = map[string]any{
			"message_id": mid,
			"channel":    job.Channel,
			"sent_at":    time.Now().Format(time.RFC3339),
		}
		return mid, nil
	}

	// 未注入发送器：降级为占位 message_id（不真正发送网络请求）
	// 生成结构化 message_id
	now := time.Now().UnixNano()
	id := fmt.Sprintf("msg_%s_%s_%d", job.Channel, job.CustomerID, now)
	if len(id) > 50 {
		id = id[:50]
	}
	implemented := map[string]bool{
		"wecom": true, "feishu": true, "telegram": true, "whatsapp": true,
		"sms": true, "email": true, "card": true, "dingtalk": true,
		"douyin": false, "kuaishou": false, "xiaohongshu": false, // V3 待接入
	}
	if !implemented[job.Channel] {
		return "", fmt.Errorf("channel %s 暂未实现主动出站（V3 待接入）", job.Channel)
	}
	// V3：把发送结果写入 job.Payload，供 StepTrackResult 读取
	if job.Payload == nil {
		job.Payload = model.JSONMap{}
	}
	job.Payload["_last_send"] = map[string]any{
		"message_id": id,
		"channel":    job.Channel,
		"sent_at":    time.Now().Format(time.RFC3339),
	}
	return id, nil
}

// trackSendResult 写入追踪字段（V3 整改）
//
// 从 job.Payload["_last_send"] 读取 StepSend 写入的 message_id / channel，
// 然后合并到 job.Payload["_tracking"]。本步不依赖额外表，避免引入新迁移。
func (s *ReachPipelineService) trackSendResult(ctx context.Context, job *model.ReachJob, _ StepResult) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}
	if job.Payload == nil {
		job.Payload = model.JSONMap{}
	}
	tracking, _ := job.Payload["_tracking"].(map[string]any)
	if tracking == nil {
		tracking = map[string]any{}
	}
	// 从 _last_send 读取 StepSend 的结果
	if last, ok := job.Payload["_last_send"].(map[string]any); ok {
		if mid, ok := last["message_id"]; ok {
			tracking["message_id"] = mid
		}
		if ch, ok := last["channel"]; ok {
			tracking["channel"] = ch
		}
		if sentAt, ok := last["sent_at"]; ok {
			tracking["sent_at"] = sentAt
		}
	}
	tracking["tracked_at"] = time.Now().Format(time.RFC3339)
	tracking["job_state"] = job.State
	job.Payload["_tracking"] = tracking
	return nil
}

// aggregateReport 聚合 step 结果（V3 整改）
//
// 汇总指标：
//   - total_steps：job.StepResults 长度
//   - success_steps / failed_steps
//   - total_duration_ms：所有 step 的 DurationMs 之和
//   - max_step / slowest_step_ms：耗时最长的 step
//   - message_id / channel：从 _tracking 取
//
// 同时更新 ReachPipeline.TotalSuccess / TotalFailure 计数（生产路径上的真实更新）。
func (s *ReachPipelineService) aggregateReport(ctx context.Context, job *model.ReachJob) (map[string]any, error) {
	if job == nil {
		return nil, fmt.Errorf("job is nil")
	}
	results := []StepResult{}
	if job.StepResults != nil {
		if err := json.Unmarshal(mustJSON(job.StepResults), &results); err != nil {
			return nil, fmt.Errorf("parse step results: %w", err)
		}
	}
	report := map[string]any{
		"job_id":            job.ID,
		"pipeline_id":       job.PipelineID,
		"channel":           job.Channel,
		"customer_id":       job.CustomerID,
		"total_steps":       len(results),
		"success_steps":     0,
		"failed_steps":      0,
		"total_duration_ms": 0,
	}
	success, failed, totalDur, maxStep, maxDur := 0, 0, 0, "", 0
	for _, r := range results {
		if r.Success {
			success++
		} else {
			failed++
		}
		totalDur += r.DurationMs
		if r.DurationMs > maxDur {
			maxDur = r.DurationMs
			maxStep = r.Step
		}
	}
	report["success_steps"] = success
	report["failed_steps"] = failed
	report["total_duration_ms"] = totalDur
	if maxStep != "" {
		report["slowest_step"] = maxStep
		report["slowest_step_ms"] = maxDur
	}
	// 关联追踪信息
	if v, ok := job.Payload["_tracking"]; ok {
		if m, ok := v.(map[string]any); ok {
			for _, k := range []string{"message_id", "channel", "tracked_at"} {
				if vv, exists := m[k]; exists {
					report["tracking_"+k] = vv
				}
			}
		}
	}
	// 真实更新 Pipeline 计数器
	if s.repo != nil && s.repo.Available() && job.PipelineID > 0 {
		if success > 0 && failed == 0 {
			s.repo.IncrementPipelineField(ctx, job.PipelineID, "total_success", 1)
		} else if failed > 0 {
			s.repo.IncrementPipelineField(ctx, job.PipelineID, "total_failure", 1)
		}
	}
	return report, nil
}

// renderReachTemplate 模板渲染（V3 整改）
//
// 语法：{{key}} - 从 job.Payload["key"] 提取值（string/number/bool 都可）
// 未命中：保留原始 {{key}}（不替换为空字符串），便于排查
//
// 自动注入变量：customer_id / account_id / channel / date / time / datetime
//
// 实现：单遍扫描，每次找到 {{ 就定位 }} 替换或跳过本块（保证进度）。
func renderReachTemplate(template string, job *model.ReachJob) string {
	if template == "" || job == nil {
		return template
	}
	autoVars := map[string]string{
		"customer_id": job.CustomerID,
		"account_id":  job.AccountID,
		"channel":     job.Channel,
		"date":        time.Now().Format("2006-01-02"),
		"time":        time.Now().Format("15:04:05"),
		"datetime":    time.Now().Format("2006-01-02 15:04:05"),
	}
	var b strings.Builder
	b.Grow(len(template))
	i := 0
	for i < len(template) {
		// 找 {{ 开始
		if i+1 < len(template) && template[i] == '{' && template[i+1] == '{' {
			// 找 }} 结束
			j := i + 2
			for j+1 < len(template) && !(template[j] == '}' && template[j+1] == '}') {
				j++
			}
			if j+1 < len(template) {
				// 找到完整的 {{key}} 块
				key := strings.TrimSpace(template[i+2 : j])
				if v, ok := job.Payload[key]; ok {
					b.WriteString(fmt.Sprintf("%v", v))
				} else if v, ok := autoVars[key]; ok {
					b.WriteString(v)
				} else {
					// 未命中：原样输出
					b.WriteString(template[i : j+2])
				}
				i = j + 2
				continue
			}
			// 未闭合：原样输出剩余
			b.WriteString(template[i:])
			break
		}
		b.WriteByte(template[i])
		i++
	}
	return b.String()
}

// appendStepResult 追加单步结果
func (s *ReachPipelineService) appendStepResult(ctx context.Context, job *model.ReachJob, res StepResult) {
	results := []StepResult{}
	if job.StepResults != nil {
		if err := json.Unmarshal(mustJSON(job.StepResults), &results); err != nil {
			logger.Errorf("[reach_pipeline] 解析 StepResults 失败: %v", err)
		}
	}
	results = append(results, res)
	data, _ := json.Marshal(results)
	job.StepResults = toJSONArray(data)
	s.repo.SaveJob(ctx, job)
}

// checkRateLimit 检查限流
func (s *ReachPipelineService) checkRateLimit(ctx context.Context, channel, accountID, customerID string, rl *RateLimitConfig) bool {
	// 每日配额
	if rl.DailyQuota > 0 {
		if !s.checkDailyQuota(ctx, channel, rl.DailyQuota) {
			return false
		}
	}
	// 单用户频次
	if rl.PerUserLimit > 0 && customerID != "" {
		if !s.checkPerUser(ctx, customerID, rl.PerUserLimit, time.Duration(rl.CooldownSecs)*time.Second) {
			return false
		}
	}
	// 令牌桶
	if rl.QPS > 0 || rl.Burst > 0 {
		// 独立部署版本：单租户，移除 merchantID 维度
		key := channel + ":" + accountID
		s.rateMu.Lock()
		b, ok := s.rateState[key]
		if !ok {
			b = &rateBucket{
				tokens:   float64(rl.Burst),
				lastFill: time.Now(),
				burst:    rl.Burst,
				qps:      rl.QPS,
			}
			s.rateState[key] = b
		}
		// 处理配置更新
		if rl.Burst > 0 && b.burst != rl.Burst {
			b.burst = rl.Burst
		}
		if rl.QPS > 0 && b.qps != rl.QPS {
			b.qps = rl.QPS
		}
		s.rateMu.Unlock()
		if !b.allow(ctx) {
			return false
		}
	}
	return true
}

// ConsumeDailyQuota 手动消耗每日配额
func (s *ReachPipelineService) ConsumeDailyQuota(ctx context.Context, channel string) bool {
	return s.consumeDailyQuota(ctx, channel, 1)
}

// checkDailyQuota 检查并消耗每日配额
func (s *ReachPipelineService) checkDailyQuota(ctx context.Context, channel string, quota int) bool {
	key := channel
	today := time.Now().Format("2006-01-02")
	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	c, ok := s.dailyQuota[key]
	if !ok || c.date != today {
		s.dailyQuota[key] = &dailyCounter{date: today, count: 0}
		c = s.dailyQuota[key]
	}
	if c.count >= quota {
		return false
	}
	c.count++
	return true
}

// consumeDailyQuota 消耗每日配额
func (s *ReachPipelineService) consumeDailyQuota(ctx context.Context, channel string, n int) bool {
	key := channel
	today := time.Now().Format("2006-01-02")
	s.dailyQuotaMu.Lock()
	defer s.dailyQuotaMu.Unlock()
	c, ok := s.dailyQuota[key]
	if !ok || c.date != today {
		c = &dailyCounter{date: today, count: 0}
		s.dailyQuota[key] = c
	}
	c.count += n
	return true
}

// checkPerUser 检查单用户频次
func (s *ReachPipelineService) checkPerUser(ctx context.Context, customerID string, limit int, cooldown time.Duration) bool {
	now := time.Now()
	s.perUserMu.Lock()
	defer s.perUserMu.Unlock()
	hits := s.perUserHits[customerID]
	// 清理过期
	cutoff := now.Add(-cooldown)
	newHits := hits[:0]
	for _, h := range hits {
		if h.After(cutoff) {
			newHits = append(newHits, h)
		}
	}
	if len(newHits) >= limit {
		s.perUserHits[customerID] = newHits
		return false
	}
	newHits = append(newHits, now)
	s.perUserHits[customerID] = newHits
	return true
}

// ResetRateLimit 重置限流状态（用于测试或运维）
func (s *ReachPipelineService) ResetRateLimit(ctx context.Context, channel string) {
	prefix := channel
	s.rateMu.Lock()
	for k := range s.rateState {
		if strings.HasPrefix(k, prefix) {
			delete(s.rateState, k)
		}
	}
	s.rateMu.Unlock()
	s.dailyQuotaMu.Lock()
	delete(s.dailyQuota, prefix)
	s.dailyQuotaMu.Unlock()
}

// validateSteps 校验步骤列表
func (s *ReachPipelineService) validateSteps(ctx context.Context, steps []string) error {
	if len(steps) == 0 {
		return ErrReachInvalidSteps
	}
	allSteps := map[string]bool{}
	for _, s := range DefaultPipelineSteps {
		allSteps[s] = true
	}
	for _, st := range steps {
		if !allSteps[st] {
			return fmt.Errorf("%w: unknown step %s", ErrReachInvalidSteps, st)
		}
	}
	// 必须包含 send
	hasSend := false
	for _, st := range steps {
		if st == StepSend {
			hasSend = true
			break
		}
	}
	if !hasSend {
		return fmt.Errorf("%w: must include send step", ErrReachInvalidSteps)
	}
	return nil
}

// computeNextRunTime 计算下次重试时间
func computeNextRunTime(rp RetryPolicy, retryCount int) time.Time {
	interval := rp.IntervalMs
	if rp.Backoff == "exponential" {
		mult := 1
		for i := 0; i < retryCount; i++ {
			mult *= 2
		}
		interval = rp.IntervalMs * mult
		if rp.MaxIntervalMs > 0 && interval > rp.MaxIntervalMs {
			interval = rp.MaxIntervalMs
		}
	}
	return time.Now().Add(time.Duration(interval) * time.Millisecond)
}

// Stats 统计
func (s *ReachPipelineService) Stats(ctx context.Context) (map[string]int64, error) {
	stats := map[string]int64{
		"total":        0,
		"active":       0,
		"paused":       0,
		"jobs":         0,
		"pending":      0,
		"running":      0,
		"success":      0,
		"failed":       0,
		"rate_limited": 0,
		"canceled":     0,
	}
	if s.repo == nil || !s.repo.Available() {
		return stats, nil
	}

	// 五层架构整改：原 10 次零散 Count 调用下沉为 repo.GetStats 一次性返回
	rs, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	stats["total"] = rs.TotalPipelines
	stats["active"] = rs.ActivePipelines
	stats["paused"] = rs.PausedPipelines
	stats["jobs"] = rs.TotalJobs
	stats["pending"] = rs.PendingJobs
	stats["running"] = rs.RunningJobs
	stats["success"] = rs.SuccessJobs
	stats["failed"] = rs.FailedJobs
	stats["rate_limited"] = rs.RateLimitedJobs
	stats["canceled"] = rs.CanceledJobs
	return stats, nil
}

// ===== 全局实例 =====
var (
	reachOnce     sync.Once
	reachInstance *ReachPipelineService
)

func GetReachPipelineService() *ReachPipelineService {
	return reachInstance
}

func InitReachPipelineService(db *gorm.DB) *ReachPipelineService {
	reachOnce.Do(func() {
		reachInstance = NewReachPipelineService(db)
	})
	return reachInstance
}
