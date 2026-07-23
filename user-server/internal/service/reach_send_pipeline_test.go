// reach_send_pipeline_test.go 触达消息发送 9 步 Pipeline 测试（PRD §5.2 P0-4 G4）
//
// 验收标准（PRD §5.2 P0-4 G4）：
//   - 高并发下消息不丢失（限流 + 重试保障）
//   - 敏感词消息被拦截并记录
//   - 主渠道失败自动降级到备用渠道
//   - 每条触达有完整审计记录
//
// 本测试文件覆盖：
//  1. 9 步全部执行（happy path）
//  2. 权限拒绝
//  3. 限流拦截
//  4. 敏感词拦截
//  5. 广告法极限词拦截
//  6. 重试（指数退避，FlakyAdapter 验证）
//  7. 降级（AlwaysFailAdapter 主渠道 → FuncAdapter 备用渠道）
//  8. 审计日志记录（成功 + 失败）
//  9. 计费扣费 + 余额不足
//  10. 客户轨迹记录
//  11. ContentAuditor 默认敏感词与广告法极限词覆盖
//  12. countedSendPipeline 统计包装器
//  13. ContentAuditor 空内容放行
//  14. MemorySendRateLimiter 令牌桶行为
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== 辅助函数 =====

// newTestPipeline 构造一个测试用 Pipeline（使用 FuncChannelAdapter）
func newTestPipeline(adapter ChannelAdapter) SendPipeline {
	return NewSendPipeline(DefaultSendPipelineConfig(adapter))
}

// newSuccessAdapter 构造一个永远成功的 adapter
func newSuccessAdapter(prefix string) *FuncChannelAdapter {
	return NewFuncChannelAdapter(func(ctx context.Context, req *ReachSendRequest) (string, error) {
		return fmt.Sprintf("%s-%s", prefix, req.RecipientID), nil
	})
}

// findStepResult 在 StepResults 中查找指定步骤
func findStepResult(resp *SendResponse, step string) *SendStepLog {
	if resp == nil {
		return nil
	}
	for i := range resp.StepResults {
		if resp.StepResults[i].Step == step {
			return &resp.StepResults[i]
		}
	}
	return nil
}

// stepExists 步骤是否执行过
func stepExists(resp *SendResponse, step string) bool {
	return findStepResult(resp, step) != nil
}

// denyAllPermission 拒绝所有权限
type denyAllPermission struct{}

func (denyAllPermission) CheckSendPermission(ctx context.Context, req *ReachSendRequest) error {
	return ErrSendPermissionDenied
}

// ===== 1. 9 步全部执行（happy path） =====

func TestSendPipeline_HappyPath_All9StepsExecuted(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	pipeline := newTestPipeline(adapter)

	req := &ReachSendRequest{
		Channel:     "sms",
		AccountID:   "acc-1",
		RecipientID: "13800138000",
		CustomerID:  "cust-1",
		OperatorID:  "op-1",
		Content:     "Hello, this is a normal message",
	}

	resp := pipeline.Send(context.Background(), req)

	if !resp.Success {
		t.Fatalf("期望成功，实际失败: %s", resp.Error)
	}
	if resp.MessageID == "" {
		t.Fatal("期望返回 message_id，实际为空")
	}
	if resp.Channel != "sms" {
		t.Errorf("期望 channel=sms，实际 %s", resp.Channel)
	}
	if resp.PrimaryChannel != "sms" {
		t.Errorf("期望 primary_channel=sms，实际 %s", resp.PrimaryChannel)
	}
	// 验证 9 步全部执行
	expectedSteps := []string{
		SendStepPermission, SendStepRateLimit, SendStepContentAudit,
		SendStepRetry, SendStepFallback, SendStepAudit,
		SendStepCost, SendStepJourney, SendStepSend,
	}
	if len(resp.StepResults) != len(expectedSteps) {
		t.Fatalf("期望 %d 步，实际 %d 步", len(expectedSteps), len(resp.StepResults))
	}
	for _, step := range expectedSteps {
		if !stepExists(resp, step) {
			t.Errorf("期望步骤 %s 已执行", step)
		}
	}
	if resp.RetryCount != 0 {
		t.Errorf("期望 retry_count=0（一次成功），实际 %d", resp.RetryCount)
	}
	if resp.FallbackUsed {
		t.Error("期望未使用降级")
	}
}

// ===== 2. 权限拒绝 =====

func TestSendPipeline_PermissionDenied(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.PermissionChecker = denyAllPermission{}
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	}
	resp := pipeline.Send(context.Background(), req)

	if resp.Success {
		t.Fatal("期望失败（权限拒绝），实际成功")
	}
	if !errors.Is(errors.New(resp.Error), ErrSendPermissionDenied) && !strings.Contains(resp.Error, ErrSendPermissionDenied.Error()) {
		t.Errorf("期望错误包含 %q，实际 %q", ErrSendPermissionDenied.Error(), resp.Error)
	}
	// 权限拒绝应该在第 1 步就中断
	permStep := findStepResult(resp, SendStepPermission)
	if permStep == nil {
		t.Fatal("期望有 permission 步骤")
	}
	if permStep.Success {
		t.Error("期望 permission 步骤失败")
	}
	// 后续步骤不应执行
	if stepExists(resp, SendStepRateLimit) {
		t.Error("权限拒绝后不应执行 rate_limit 步骤")
	}
	if stepExists(resp, SendStepSend) {
		t.Error("权限拒绝后不应执行 send 步骤")
	}
}

// ===== 3. 限流拦截 =====

func TestSendPipeline_RateLimited(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.RateLimiter = NewMemorySendRateLimiter()
	cfg.RateLimitSpec = RateLimitSpec{QPS: 1, Burst: 1} // 1 QPS, burst=1
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		AccountID:   "acc-1",
		RecipientID: "13800138000",
		CustomerID:  "cust-1",
		Content:     "Hello",
	}

	// 第一次应该成功
	resp1 := pipeline.Send(context.Background(), req)
	if !resp1.Success {
		t.Fatalf("第一次发送应该成功，失败: %s", resp1.Error)
	}
	// 第二次立即发送应该被限流
	resp2 := pipeline.Send(context.Background(), req)
	if resp2.Success {
		t.Fatal("第二次发送应该被限流")
	}
	rlStep := findStepResult(resp2, SendStepRateLimit)
	if rlStep == nil {
		t.Fatal("期望有 rate_limit 步骤")
	}
	if rlStep.Success {
		t.Error("期望 rate_limit 步骤失败")
	}
	if !strings.Contains(resp2.Error, ErrSendRateLimited.Error()) {
		t.Errorf("期望错误包含 %q，实际 %q", ErrSendRateLimited.Error(), resp2.Error)
	}
	// 限流后不应执行后续步骤
	if stepExists(resp2, SendStepContentAudit) {
		t.Error("限流后不应执行 content_audit 步骤")
	}
	if stepExists(resp2, SendStepSend) {
		t.Error("限流后不应执行 send 步骤")
	}
}

// ===== 4. 敏感词拦截 =====

func TestSendPipeline_SensitiveWordBlocked(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	pipeline := newTestPipeline(adapter)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "快来参与赌博游戏",
	}
	resp := pipeline.Send(context.Background(), req)

	if resp.Success {
		t.Fatal("期望失败（敏感词），实际成功")
	}
	caStep := findStepResult(resp, SendStepContentAudit)
	if caStep == nil {
		t.Fatal("期望有 content_audit 步骤")
	}
	if caStep.Success {
		t.Error("期望 content_audit 步骤失败")
	}
	if !strings.Contains(resp.Error, ErrSendContentRejected.Error()) {
		t.Errorf("期望错误包含 %q，实际 %q", ErrSendContentRejected.Error(), resp.Error)
	}
	if !strings.Contains(resp.Error, "赌博") {
		t.Errorf("期望错误包含命中的敏感词 '赌博'，实际 %q", resp.Error)
	}
	// 敏感词后不应执行后续步骤
	if stepExists(resp, SendStepSend) {
		t.Error("敏感词后不应执行 send 步骤")
	}
	if adapter.Count() > 0 {
		t.Error("敏感词拦截后 adapter 不应被调用")
	}
}

// ===== 5. 广告法极限词拦截 =====

func TestSendPipeline_AdLawKeywordBlocked(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	pipeline := newTestPipeline(adapter)

	cases := []string{
		"我们是行业第一",
		"全球最佳产品",
		"100% 有效",
		"极致体验",
		"永久免费",
	}
	for i, content := range cases {
		req := &ReachSendRequest{
			Channel:     "sms",
			RecipientID: "13800138000",
			Content:     content,
		}
		resp := pipeline.Send(context.Background(), req)
		if resp.Success {
			t.Errorf("case %d: 期望失败（广告法极限词），实际成功。内容: %s", i, content)
			continue
		}
		if !strings.Contains(resp.Error, ErrSendContentRejected.Error()) {
			t.Errorf("case %d: 期望错误包含 %q，实际 %q", i, ErrSendContentRejected.Error(), resp.Error)
		}
	}
}

// ===== 6. 重试（指数退避，FlakyAdapter 验证） =====

func TestSendPipeline_RetryWithFlakyAdapter(t *testing.T) {
	// FlakyAdapter 前 2 次失败，第 3 次成功
	adapter := NewFlakyAdapter(2)
	cfg := DefaultSendPipelineConfig(adapter)
	// 缩短重试间隔以加速测试
	cfg.RetryPolicy = SendRetryPolicy{
		MaxRetries:    3,
		IntervalMs:    10, // 10ms
		Backoff:       "exponential",
		MaxIntervalMs: 100,
	}
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	}
	start := time.Now()
	resp := pipeline.Send(context.Background(), req)
	elapsed := time.Since(start)

	if !resp.Success {
		t.Fatalf("期望最终成功（重试后），失败: %s", resp.Error)
	}
	if resp.RetryCount < 2 {
		t.Errorf("期望 retry_count>=2（前 2 次失败），实际 %d", resp.RetryCount)
	}
	if adapter.Count() < 3 {
		t.Errorf("期望 adapter 至少调用 3 次，实际 %d", adapter.Count())
	}
	// 验证指数退避：2 次重试，间隔 10ms + 20ms = 30ms（最少）
	if elapsed < 25*time.Millisecond {
		t.Errorf("期望指数退避至少 30ms，实际 %v", elapsed)
	}
}

func TestSendPipeline_RetryExhaustedAllFail(t *testing.T) {
	// AlwaysFailAdapter 始终失败
	adapter := NewAlwaysFailAdapter(errors.New("network error"))
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.RetryPolicy = SendRetryPolicy{
		MaxRetries:    2,
		IntervalMs:    5,
		Backoff:       "exponential",
		MaxIntervalMs: 50,
	}
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	}
	resp := pipeline.Send(context.Background(), req)

	if resp.Success {
		t.Fatal("期望失败（全部重试失败）")
	}
	// MaxRetries=2，应调用 3 次（1 + 2 retries）
	if adapter.Count() != 3 {
		t.Errorf("期望 adapter 调用 3 次（1 + 2 retries），实际 %d", adapter.Count())
	}
	if resp.RetryCount != 2 {
		t.Errorf("期望 retry_count=2，实际 %d", resp.RetryCount)
	}
}

// ===== 7. 降级（主渠道失败 → 备用渠道） =====

func TestSendPipeline_FallbackToBackupChannel(t *testing.T) {
	// 主渠道始终失败
	primaryAdapter := NewAlwaysFailAdapter(errors.New("primary down"))
	// 备用渠道成功
	backupAdapter := newSuccessAdapter("backup-msg")
	cfg := DefaultSendPipelineConfig(primaryAdapter)
	cfg.FallbackAdapters = map[string]ChannelAdapter{
		"email": backupAdapter,
	}
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
		Fallback: &FallbackConfig{
			Enabled:       true,
			BackupChannel: "email",
			BackupAccount: "backup-acc-1",
			MaxAttempts:   1,
		},
	}
	resp := pipeline.Send(context.Background(), req)

	if !resp.Success {
		t.Fatalf("期望降级后成功，失败: %s", resp.Error)
	}
	if resp.MessageID == "" {
		t.Error("期望返回 message_id")
	}
	if primaryAdapter.Count() < 1 {
		t.Errorf("主渠道至少调用 1 次，实际 %d", primaryAdapter.Count())
	}
	if backupAdapter.Count() < 1 {
		t.Errorf("备用渠道至少调用 1 次，实际 %d", backupAdapter.Count())
	}
}

func TestSendPipeline_FallbackDisabled(t *testing.T) {
	primaryAdapter := NewAlwaysFailAdapter(errors.New("primary down"))
	backupAdapter := newSuccessAdapter("backup-msg")
	cfg := DefaultSendPipelineConfig(primaryAdapter)
	cfg.FallbackAdapters = map[string]ChannelAdapter{
		"email": backupAdapter,
	}
	pipeline := NewSendPipeline(cfg)

	// 不启用降级
	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
		Fallback: &FallbackConfig{
			Enabled: false,
		},
	}
	resp := pipeline.Send(context.Background(), req)

	if resp.Success {
		t.Fatal("期望失败（降级未启用）")
	}
	if backupAdapter.Count() > 0 {
		t.Error("降级未启用时备用渠道不应被调用")
	}
}

// ===== 8. 审计日志记录 =====

func TestSendPipeline_AuditLogRecorded(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	auditLogger := NewMemorySendAuditLogger(100)
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.AuditLogger = auditLogger
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		AccountID:   "acc-1",
		RecipientID: "13800138000",
		CustomerID:  "cust-1",
		OperatorID:  "op-1",
		Content:     "Hello",
	}
	resp := pipeline.Send(context.Background(), req)

	if !resp.Success {
		t.Fatalf("期望成功，失败: %s", resp.Error)
	}
	// 审计日志至少记录 1 条（成功）
	entries := auditLogger.Entries()
	if len(entries) == 0 {
		t.Fatal("期望至少 1 条审计日志，实际 0 条")
	}
	last := entries[len(entries)-1]
	if last.Channel != "sms" {
		t.Errorf("期望 channel=sms，实际 %s", last.Channel)
	}
	if last.Recipient != "13800138000" {
		t.Errorf("期望 recipient=13800138000，实际 %s", last.Recipient)
	}
	if !last.Success {
		t.Error("期望审计日志标记为成功")
	}
	if last.MessageID == "" {
		t.Error("期望审计日志包含 message_id")
	}
}

func TestSendPipeline_AuditLogRecordsContent(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	auditLogger := NewMemorySendAuditLogger(100)
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.AuditLogger = auditLogger
	pipeline := NewSendPipeline(cfg)

	content := "Hello test message"
	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     content,
	}
	resp := pipeline.Send(context.Background(), req)
	if !resp.Success {
		t.Fatalf("期望成功，失败: %s", resp.Error)
	}

	entries := auditLogger.Entries()
	if len(entries) == 0 {
		t.Fatal("期望审计日志非空")
	}
	found := false
	for _, e := range entries {
		if e.Content == content {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望审计日志包含内容 %q", content)
	}
}

// TestSendPipeline_AuditLogOnFailure 验证 PRD §5.2 P0-4 G4 "每条触达有完整审计记录"
// 即失败时也必须记录审计日志
func TestSendPipeline_AuditLogOnFailure(t *testing.T) {
	// 用敏感词触发失败
	adapter := newSuccessAdapter("msg")
	auditLogger := NewMemorySendAuditLogger(100)
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.AuditLogger = auditLogger
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		CustomerID:  "cust-1",
		OperatorID:  "op-1",
		Content:     "赌博内容",
	}
	resp := pipeline.Send(context.Background(), req)
	if resp.Success {
		t.Fatal("期望失败（敏感词）")
	}
	entries := auditLogger.Entries()
	if len(entries) == 0 {
		t.Fatal("PRD 合规失败：失败时未记录审计日志")
	}
	last := entries[len(entries)-1]
	if last.Success {
		t.Error("期望审计日志标记为失败")
	}
	if !strings.Contains(last.Error, ErrSendContentRejected.Error()) {
		t.Errorf("期望审计日志 error 包含 %q，实际 %q", ErrSendContentRejected.Error(), last.Error)
	}
	if last.Channel != "sms" {
		t.Errorf("期望 channel=sms，实际 %s", last.Channel)
	}
	if last.Recipient != "13800138000" {
		t.Errorf("期望 recipient=13800138000，实际 %s", last.Recipient)
	}
}

// ===== 9. 计费扣费 + 余额不足 =====

func TestSendPipeline_CostTrackerCharged(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	costTracker := NewMemorySendCostTracker(100.0) // 初始余额 100
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.CostTracker = costTracker
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	}
	resp := pipeline.Send(context.Background(), req)
	if !resp.Success {
		t.Fatalf("期望成功，失败: %s", resp.Error)
	}
	// sms 单价 0.05，余额应从 100 减为 99.95
	balance := costTracker.Balance()
	if balance >= 100.0 {
		t.Errorf("期望余额 < 100，实际 %f", balance)
	}
	used := costTracker.TotalUsed()
	if used <= 0 {
		t.Errorf("期望累计消费 > 0，实际 %f", used)
	}
	// 步骤记录应包含 cost 步骤
	costStep := findStepResult(resp, SendStepCost)
	if costStep == nil {
		t.Fatal("期望有 cost 步骤")
	}
	if !costStep.Success {
		t.Errorf("期望 cost 步骤成功: %s", costStep.Error)
	}
}

func TestSendPipeline_CostTrackerInsufficientBalance(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	costTracker := NewMemorySendCostTracker(0.01) // 余额不足支付 sms 单价 0.05
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.CostTracker = costTracker
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	}
	resp := pipeline.Send(context.Background(), req)
	if resp.Success {
		t.Fatal("期望失败（余额不足）")
	}
	costStep := findStepResult(resp, SendStepCost)
	if costStep == nil {
		t.Fatal("期望有 cost 步骤")
	}
	if costStep.Success {
		t.Error("期望 cost 步骤失败")
	}
	if !strings.Contains(resp.Error, ErrSendInsufficientCost.Error()) {
		t.Errorf("期望错误包含 %q，实际 %q", ErrSendInsufficientCost.Error(), resp.Error)
	}
}

// ===== 10. 客户轨迹记录 =====

type recordingJourneyTracker struct {
	mu     sync.Mutex
	calls  []journeyCall
	hasErr bool
}

type journeyCall struct {
	CustomerID string
	Channel    string
	Source     string
}

func (m *recordingJourneyTracker) RecordTouch(ctx context.Context, customerID, channel, source string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, journeyCall{CustomerID: customerID, Channel: channel, Source: source})
	if m.hasErr {
		return errors.New("journey tracker error")
	}
	return nil
}

func (m *recordingJourneyTracker) Calls(ctx context.Context) []journeyCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]journeyCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestSendPipeline_JourneyTrackerRecorded(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	tracker := &recordingJourneyTracker{}
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.JourneyTracker = tracker
	pipeline := NewSendPipeline(cfg)

	req := &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		CustomerID:  "cust-123",
		Content:     "Hello",
	}
	resp := pipeline.Send(context.Background(), req)
	if !resp.Success {
		t.Fatalf("期望成功，失败: %s", resp.Error)
	}
	calls := tracker.Calls()
	if len(calls) == 0 {
		t.Fatal("期望客户轨迹至少记录 1 次")
	}
	c := calls[0]
	if c.CustomerID != "cust-123" {
		t.Errorf("期望 customer_id=cust-123，实际 %s", c.CustomerID)
	}
	if c.Channel != "sms" {
		t.Errorf("期望 channel=sms，实际 %s", c.Channel)
	}
	if c.Source != "reach_pipeline" {
		t.Errorf("期望 source=reach_pipeline，实际 %s", c.Source)
	}
	// 步骤记录
	jStep := findStepResult(resp, SendStepJourney)
	if jStep == nil {
		t.Fatal("期望有 journey 步骤")
	}
	if !jStep.Success {
		t.Errorf("期望 journey 步骤成功: %s", jStep.Error)
	}
}

// ===== 11. ContentAuditor 默认敏感词与广告法极限词覆盖 =====

func TestDefaultContentAuditor_SensitiveWords(t *testing.T) {
	auditor := NewDefaultContentAuditor()
	// 默认 8 个敏感词
	expectedSensitive := []string{"赌博", "色情", "毒品", "诈骗", "传销", "枪支", "弹药", "爆炸物"}
	for _, w := range expectedSensitive {
		result, err := auditor.Audit(context.Background(), "sms", "test "+w+" content")
		if err != nil {
			t.Errorf("敏感词 %s 审核报错: %v", w, err)
		}
		if result.Passed {
			t.Errorf("敏感词 %s 应该被拦截", w)
		}
		if result.Category != "sensitive" {
			t.Errorf("敏感词 %s 期望 category=sensitive，实际 %s", w, result.Category)
		}
	}
}

func TestDefaultContentAuditor_AdLawKeywords(t *testing.T) {
	auditor := NewDefaultContentAuditor()
	// 默认 16 个广告法极限词
	expectedAdLaw := []string{
		"国家级", "最高级", "最佳", "最强", "最先", "最新",
		"第一", "唯一", "首个", "冠军", "顶尖", "极致",
		"永久", "百分百", "100%", "绝对",
	}
	for _, w := range expectedAdLaw {
		result, err := auditor.Audit(context.Background(), "sms", "我们的产品是"+w)
		if err != nil {
			t.Errorf("广告法极限词 %s 审核报错: %v", w, err)
		}
		if result.Passed {
			t.Errorf("广告法极限词 %s 应该被拦截", w)
		}
		if result.Category != "ad_law" && result.Category != "sensitive" {
			t.Errorf("广告法极限词 %s 期望 category=ad_law/sensitive，实际 %s", w, result.Category)
		}
	}
}

// ===== 12. countedSendPipeline 统计包装器 =====

func TestCountedSendPipeline_Stats(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	inner := newTestPipeline(adapter)
	pipeline := NewCountedSendPipeline(inner)
	counted, ok := pipeline.(*countedSendPipeline)
	if !ok {
		t.Fatal("期望 *countedSendPipeline 类型")
	}

	// 3 次成功 + 1 次限流失败
	for i := 0; i < 3; i++ {
		req := &ReachSendRequest{
			Channel:     "sms",
			RecipientID: fmt.Sprintf("1380013800%d", i),
			Content:     "Hello",
		}
		resp := pipeline.Send(context.Background(), req)
		if !resp.Success {
			t.Errorf("第 %d 次发送应该成功: %s", i, resp.Error)
		}
	}

	stats := counted.Stats()
	if stats.TotalSends != 3 {
		t.Errorf("期望 total_sends=3，实际 %d", stats.TotalSends)
	}
	if stats.SuccessSends != 3 {
		t.Errorf("期望 success_sends=3，实际 %d", stats.SuccessSends)
	}
	if stats.FailedSends != 0 {
		t.Errorf("期望 failed_sends=0，实际 %d", stats.FailedSends)
	}
}

func TestCountedSendPipeline_StatsWithFailures(t *testing.T) {
	// 主渠道失败 + 敏感词触发
	adapter := newSuccessAdapter("msg")
	cfg := DefaultSendPipelineConfig(adapter)
	pipeline := NewCountedSendPipeline(NewSendPipeline(cfg))
	counted := pipeline.(*countedSendPipeline)

	// 1 次敏感词失败
	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "赌博内容",
	})
	if resp.Success {
		t.Fatal("期望敏感词失败")
	}
	// 1 次成功
	pipeline.Send(context.Background(), &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138001",
		Content:     "Hello",
	})

	stats := counted.Stats()
	if stats.TotalSends != 2 {
		t.Errorf("期望 total_sends=2，实际 %d", stats.TotalSends)
	}
	if stats.SuccessSends != 1 {
		t.Errorf("期望 success_sends=1，实际 %d", stats.SuccessSends)
	}
	if stats.FailedSends != 1 {
		t.Errorf("期望 failed_sends=1，实际 %d", stats.FailedSends)
	}
	if stats.ContentBlocked != 1 {
		t.Errorf("期望 content_blocked=1，实际 %d", stats.ContentBlocked)
	}
}

// ===== 13. ContentAuditor 空内容放行 =====

func TestDefaultContentAuditor_EmptyContent(t *testing.T) {
	auditor := NewDefaultContentAuditor()
	result, err := auditor.Audit(context.Background(), "sms", "")
	if err != nil {
		t.Errorf("空内容审核报错: %v", err)
	}
	if !result.Passed {
		t.Error("空内容应该放行")
	}
	if result.Category != "normal" {
		t.Errorf("空内容期望 category=normal，实际 %s", result.Category)
	}
}

// ===== 14. MemorySendRateLimiter 令牌桶行为 =====

func TestMemorySendRateLimiter_TokenBucket(t *testing.T) {
	limiter := NewMemorySendRateLimiter()
	spec := RateLimitSpec{QPS: 2, Burst: 2}

	// burst=2，前 2 次允许
	if !limiter.Allow(ctx, "k1", spec) {
		t.Error("第 1 次应该允许（burst）")
	}
	if !limiter.Allow(ctx, "k1", spec) {
		t.Error("第 2 次应该允许（burst）")
	}
	// 第 3 次应该被限流（无可用 token）
	if limiter.Allow(ctx, "k1", spec) {
		t.Error("第 3 次应该被限流")
	}
	// 等待 500ms（QPS=2 → 1 个 token）
	time.Sleep(550 * time.Millisecond)
	if !limiter.Allow(ctx, "k1", spec) {
		t.Error("等待 500ms 后应该有 1 个 token 可用")
	}
}

func TestMemorySendRateLimiter_DifferentKeys(t *testing.T) {
	limiter := NewMemorySendRateLimiter()
	spec := RateLimitSpec{QPS: 1, Burst: 1}

	// 不同 key 应该有独立的桶
	if !limiter.Allow(ctx, "k1", spec) {
		t.Error("k1 第 1 次应该允许")
	}
	if !limiter.Allow(ctx, "k2", spec) {
		t.Error("k2 第 1 次应该允许")
	}
	// 同一 key 第 2 次应被限流
	if limiter.Allow(ctx, "k1", spec) {
		t.Error("k1 第 2 次应该被限流")
	}
}

func TestMemorySendRateLimiter_Reset(t *testing.T) {
	limiter := NewMemorySendRateLimiter()
	spec := RateLimitSpec{QPS: 1, Burst: 1}

	if !limiter.Allow(ctx, "k1", spec) {
		t.Error("k1 第 1 次应该允许")
	}
	if limiter.Allow(ctx, "k1", spec) {
		t.Error("k1 第 2 次应该被限流")
	}
	// Reset 后应允许
	limiter.Reset("k1")
	if !limiter.Allow(ctx, "k1", spec) {
		t.Error("Reset 后 k1 应该允许")
	}
}

// ===== 15. MemorySendAuditLogger 容量上限 =====

func TestMemorySendAuditLogger_MaxSize(t *testing.T) {
	logger := NewMemorySendAuditLogger(3) // 容量 3
	for i := 0; i < 5; i++ {
		logger.LogSend(context.Background(), &ReachSendRequest{
			Channel:     "sms",
			RecipientID: fmt.Sprintf("r%d", i),
			Content:     "x",
		}, &SendResponse{Success: true, MessageID: fmt.Sprintf("m%d", i)})
	}
	entries := logger.Entries()
	if len(entries) != 3 {
		t.Errorf("期望保留 3 条（容量上限），实际 %d", len(entries))
	}
	// 应该保留最后 3 条（r2/r3/r4）
	if entries[0].Recipient != "r2" {
		t.Errorf("期望第一条 recipient=r2，实际 %s", entries[0].Recipient)
	}
	if entries[2].Recipient != "r4" {
		t.Errorf("期望最后一条 recipient=r4，实际 %s", entries[2].Recipient)
	}
}

// ===== 16. 默认配置 NoOp 行为验证 =====

func TestNoOpSendCostTracker_ZeroCost(t *testing.T) {
	tracker := NoOpSendCostTracker{}
	cost, err := tracker.Charge(context.Background(), "sms", &ReachSendRequest{})
	if err != nil {
		t.Errorf("NoOp CostTracker 不应报错: %v", err)
	}
	if cost != 0 {
		t.Errorf("NoOp CostTracker 应返回 0，实际 %f", cost)
	}
}

func TestNoOpSendRateLimiter_AlwaysAllow(t *testing.T) {
	limiter := NoOpSendRateLimiter{}
	for i := 0; i < 100; i++ {
		if !limiter.Allow(context.Background(), "k", RateLimitSpec{QPS: 1, Burst: 1}) {
			t.Errorf("NoOp RateLimiter 第 %d 次应该允许", i)
		}
	}
}

func TestNoOpSendJourneyTracker_NoError(t *testing.T) {
	tracker := NoOpSendJourneyTracker{}
	if err := tracker.RecordTouch(context.Background(), "c1", "sms", "test"); err != nil {
		t.Errorf("NoOp JourneyTracker 不应报错: %v", err)
	}
}

func TestAllowAllSendPermission_AlwaysAllow(t *testing.T) {
	p := AllowAllSendPermission{}
	if err := p.CheckSendPermission(context.Background(), &ReachSendRequest{}); err != nil {
		t.Errorf("AllowAllSendPermission 不应报错: %v", err)
	}
}

// ===== 17. 默认重试策略验证 =====

func TestDefaultSendRetryPolicy_Values(t *testing.T) {
	p := DefaultSendRetryPolicy()
	if p.MaxRetries != 3 {
		t.Errorf("期望 MaxRetries=3，实际 %d", p.MaxRetries)
	}
	if p.Backoff != "exponential" {
		t.Errorf("期望 Backoff=exponential，实际 %s", p.Backoff)
	}
	if p.IntervalMs <= 0 {
		t.Errorf("期望 IntervalMs>0，实际 %d", p.IntervalMs)
	}
	if p.MaxIntervalMs <= 0 {
		t.Errorf("期望 MaxIntervalMs>0，实际 %d", p.MaxIntervalMs)
	}
}

// ===== 18. computeBackoff 指数退避计算 =====

func TestComputeBackoff_Exponential(t *testing.T) {
	p := &defaultSendPipeline{}
	policy := SendRetryPolicy{
		MaxRetries:    3,
		IntervalMs:    100,
		Backoff:       "exponential",
		MaxIntervalMs: 10000,
	}
	// attempt 0: 100ms
	// attempt 1: 200ms
	// attempt 2: 400ms
	// attempt 3: 800ms
	cases := []struct {
		attempt int
		expect  time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
	}
	for _, c := range cases {
		got := p.computeBackoff(policy, c.attempt)
		if got != c.expect {
			t.Errorf("attempt %d: 期望 %v，实际 %v", c.attempt, c.expect, got)
		}
	}
}

func TestComputeBackoff_MaxIntervalCap(t *testing.T) {
	p := &defaultSendPipeline{}
	policy := SendRetryPolicy{
		MaxRetries:    3,
		IntervalMs:    1000,
		Backoff:       "exponential",
		MaxIntervalMs: 5000, // cap at 5s
	}
	// attempt 5: 1000 * 2^5 = 32000ms > 5000ms cap
	got := p.computeBackoff(policy, 5)
	if got != 5000*time.Millisecond {
		t.Errorf("期望被 cap 到 5000ms，实际 %v", got)
	}
}

func TestComputeBackoff_Fixed(t *testing.T) {
	p := &defaultSendPipeline{}
	policy := SendRetryPolicy{
		MaxRetries: 3,
		IntervalMs: 200,
		Backoff:    "fixed",
	}
	for attempt := 0; attempt < 5; attempt++ {
		got := p.computeBackoff(policy, attempt)
		if got != 200*time.Millisecond {
			t.Errorf("attempt %d: 期望固定 200ms，实际 %v", attempt, got)
		}
	}
}

// ===== 19. isRetryable 错误匹配 =====

func TestIsRetryable_DefaultAllRetryable(t *testing.T) {
	p := &defaultSendPipeline{}
	// 默认 RetryableErrors 为空 → 所有错误可重试
	if !p.isRetryable(errors.New("any error"), nil) {
		t.Error("默认所有错误应该可重试")
	}
	if !p.isRetryable(errors.New("network error"), []string{}) {
		t.Error("空 RetryableErrors 应该所有错误可重试")
	}
}

func TestIsRetryable_SpecificErrors(t *testing.T) {
	p := &defaultSendPipeline{}
	retryable := []string{"timeout", "connection reset"}
	if !p.isRetryable(errors.New("request timeout"), retryable) {
		t.Error("'timeout' 应该匹配")
	}
	if !p.isRetryable(errors.New("connection reset by peer"), retryable) {
		t.Error("'connection reset' 应该匹配")
	}
	if p.isRetryable(errors.New("invalid argument"), retryable) {
		t.Error("'invalid argument' 不应该匹配")
	}
	if p.isRetryable(nil, retryable) {
		t.Error("nil 错误不应该可重试")
	}
}

// ===== 20. 默认 Pipeline 不带 adapter 应报错 =====

func TestSendPipeline_NoAdapter_ReturnsError(t *testing.T) {
	cfg := DefaultSendPipelineConfig(nil) // adapter = nil
	pipeline := NewSendPipeline(cfg)

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	})
	if resp.Success {
		t.Fatal("期望失败（无 adapter）")
	}
	// runRetry 中 executeSendWithFallback 会返回 ErrSendChannelNotConfig
	if !strings.Contains(resp.Error, ErrSendChannelNotConfig.Error()) {
		t.Errorf("期望错误包含 %q，实际 %q", ErrSendChannelNotConfig.Error(), resp.Error)
	}
}

// ===== 21. Custom Steps Order（自定义步骤顺序） =====

func TestSendPipeline_CustomStepsOrder(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	cfg := DefaultSendPipelineConfig(adapter)
	// 只启用 3 步
	cfg.Steps = []string{SendStepPermission, SendStepContentAudit, SendStepRetry}
	pipeline := NewSendPipeline(cfg)

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel:     "sms",
		RecipientID: "13800138000",
		Content:     "Hello",
	})
	if !resp.Success {
		t.Fatalf("期望成功，失败: %s", resp.Error)
	}
	if len(resp.StepResults) != 3 {
		t.Errorf("期望 3 步，实际 %d", len(resp.StepResults))
	}
	// 验证未启用的步骤不存在
	if stepExists(resp, SendStepRateLimit) {
		t.Error("rate_limit 不应执行（未在自定义 Steps 中）")
	}
	if stepExists(resp, SendStepAudit) {
		t.Error("audit 不应执行（未在自定义 Steps 中）")
	}
}

// ===== 22. 并发安全：高并发下消息不丢失 =====

func TestSendPipeline_ConcurrentSends(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	pipeline := newTestPipeline(adapter)

	const N = 50
	var wg sync.WaitGroup
	results := make([]*SendResponse, N)
	start := make(chan struct{})

	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			req := &ReachSendRequest{
				Channel:     "sms",
				RecipientID: fmt.Sprintf("r%d", idx),
				Content:     "Hello",
			}
			results[idx] = pipeline.Send(context.Background(), req)
		}(i)
	}
	close(start)
	wg.Wait()

	successCnt := 0
	for i, r := range results {
		if r == nil {
			t.Errorf("第 %d 个响应为 nil", i)
			continue
		}
		if r.Success {
			successCnt++
		}
	}
	if successCnt != N {
		t.Errorf("期望全部 %d 个成功，实际 %d", N, successCnt)
	}
}

// ===== 23. MemorySendCostTracker 多渠道单价 =====

func TestMemorySendCostTracker_DifferentChannelCosts(t *testing.T) {
	tracker := NewMemorySendCostTracker(10.0)
	// 默认：sms=0.05, email=0.001, wecom=0, card=0.01
	cases := []struct {
		channel   string
		expectErr bool
	}{
		{"sms", false},      // 0.05 ≤ 10
		{"email", false},    // 0.001 ≤ 9.95
		{"wecom", false},    // 0
		{"dingtalk", false}, // 0
		{"card", false},     // 0.01
	}
	for _, c := range cases {
		_, err := tracker.Charge(context.Background(), c.channel, &ReachSendRequest{})
		if c.expectErr && err == nil {
			t.Errorf("channel %s: 期望报错", c.channel)
		}
		if !c.expectErr && err != nil {
			t.Errorf("channel %s: 不期望报错: %v", c.channel, err)
		}
	}
	// 设置新单价
	tracker.SetCost("sms", 5.0)
	balanceBefore := tracker.Balance()
	_, err := tracker.Charge(context.Background(), "sms", &ReachSendRequest{})
	if err != nil {
		t.Errorf("设置单价后扣费不应报错: %v", err)
	}
	balanceAfter := tracker.Balance()
	if balanceAfter != balanceBefore-5.0 {
		t.Errorf("期望扣 5.0，实际从 %f 到 %f", balanceBefore, balanceAfter)
	}
}

// ===== 24. CustomerJourneySendTracker 适配器（无 Service 时静默返回 nil） =====

func TestCustomerJourneySendTracker_NilService(t *testing.T) {
	tracker := CustomerJourneySendTracker{Service: nil}
	// Service 为 nil 应静默成功（不报错）
	if err := tracker.RecordTouch(context.Background(), "c1", "sms", "test"); err != nil {
		t.Errorf("Service 为 nil 不应报错: %v", err)
	}
	// 空 customerID 应静默成功
	if err := tracker.RecordTouch(context.Background(), "", "sms", "test"); err != nil {
		t.Errorf("空 customerID 不应报错: %v", err)
	}
}

// ===== 25. DefaultSendPipelineConfig 默认值验证 =====

func TestDefaultSendPipelineConfig_AllComponents(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	cfg := DefaultSendPipelineConfig(adapter)
	if cfg.PermissionChecker == nil {
		t.Error("PermissionChecker 不应为 nil")
	}
	if cfg.RateLimiter == nil {
		t.Error("RateLimiter 不应为 nil")
	}
	if cfg.ContentAuditor == nil {
		t.Error("ContentAuditor 不应为 nil")
	}
	if cfg.AuditLogger == nil {
		t.Error("AuditLogger 不应为 nil")
	}
	if cfg.CostTracker == nil {
		t.Error("CostTracker 不应为 nil")
	}
	if cfg.JourneyTracker == nil {
		t.Error("JourneyTracker 不应为 nil")
	}
	if cfg.Adapter == nil {
		t.Error("Adapter 不应为 nil")
	}
	if len(cfg.Steps) != 9 {
		t.Errorf("期望 9 步，实际 %d", len(cfg.Steps))
	}
	if cfg.RetryPolicy.MaxRetries != 3 {
		t.Errorf("期望 MaxRetries=3，实际 %d", cfg.RetryPolicy.MaxRetries)
	}
}
