package service

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// DomainHealthService 域名健康度服务
// G 域 ：定时探测（DNS / HTTP HEAD / 平台黑名单）+ 健康度评分 + 自动切换
//
// 设计要点：
//   - 探测与评分完全同步执行（不引入后台 goroutine），由调用方触发；
//   - 自动切换遵循"评分 < 阈值 且 连续失败次数 >= N"才切换，避免抖动；
//   - 切换目标：评分最高且未在黑名单的可用域名（ListAvailable）；
//   - 所有操作都会写入 DomainHealthLog，便于事后追查；
//   - 私域独立部署：无 merchant_id 字段。
type DomainHealthService interface {
	// CheckOne 探测单个域名，写入健康度与日志
	CheckOne(ctx context.Context, domainID int) (*HealthCheckResult, error)
	// CheckAll 探测所有域名
	CheckAll(ctx context.Context) ([]*HealthCheckResult, error)
	// SwitchActive 手动切换到指定域名
	SwitchActive(ctx context.Context, domainID int, reason string) error
	// SwitchToBest 自动切换到评分最高的可用域名
	SwitchToBest(ctx context.Context, reason string) (*model.DomainPool, error)
	// GetActiveDomain 获取当前活跃域名
	GetActiveDomain(ctx context.Context) (*model.DomainPool, error)
	// ListAvailable 列出可用域名
	ListAvailable(ctx context.Context, minScore int) ([]*model.DomainPool, error)
	// ListHealthLogs 查询最近 N 条健康度日志
	ListHealthLogs(ctx context.Context, domainID int, limit int) ([]*model.DomainHealthLog, error)
	// ProbeOnce 一次性探测（不写库），用于单元测试 / API 调试
	ProbeOnce(ctx context.Context, domain string) (*HealthCheckResult, error)
}

// HealthCheckResult 一次健康度探测的完整结果
type HealthCheckResult struct {
	DomainID     int       `json:"domain_id"`
	Domain       string    `json:"domain"`
	Port         int       `json:"port"`
	CheckedAt    time.Time `json:"checked_at"`
	DNSOK        bool      `json:"dns_ok"`
	DNSError     string    `json:"dns_error,omitempty"`
	HTTPOk       bool      `json:"http_ok"`
	HTTPStatus   int       `json:"http_status"`
	HTTPLatency  int       `json:"http_latency_ms"`
	HTTPErrorMsg string    `json:"http_error,omitempty"`
	OnBlacklist  bool      `json:"on_blacklist"`
	BlacklistSrc string    `json:"blacklist_source,omitempty"`
	HealthScore  int       `json:"health_score"`
	ActionTaken  string    `json:"action_taken"` // none / mark_unhealthy / switch_over
}

// 评分阈值（导出常量，供测试断言使用）
const (
	HealthScoreSwitchThreshold = 30 // 评分低于此值触发自动切换
	HealthScoreHealthy         = 80
	HealthScoreWarn            = 60
	ConsecutiveFailureLimit    = 3 // 连续失败 3 次触发切换
	HealthCheckTimeout         = 5 * time.Second
	HealthLogRetentionDays     = 30
)

// domainHealthService 实现
type domainHealthService struct {
	repo        repository.DomainPoolRepository
	logRepo     *repository.DomainHealthLogRepository
	blacklistR  *repository.DomainBlacklistRepository
	httpClient  *http.Client
	probeTarget string // 可选：HTTP 探测时的 Path，默认 "/"
	mu          sync.Mutex
}

// NewDomainHealthService 创建域名健康度服务
func NewDomainHealthService(db interface{}, repo repository.DomainPoolRepository) DomainHealthService {
	// db 形参保留扩展位：未来可基于 db 注入其他仓储；当前统一通过仓储层封装 DB 入口
	_ = db
	blRepo := repository.NewDomainBlacklistRepository(nil)
	return &domainHealthService{
		repo:        repo,
		logRepo:     repository.NewDomainHealthLogRepository(nil),
		blacklistR:  blRepo,
		httpClient:  &http.Client{Timeout: HealthCheckTimeout},
		probeTarget: "/",
	}
}

// NewDomainHealthServiceWithDeps 显式注入 db（测试用）
func NewDomainHealthServiceWithDeps(repo repository.DomainPoolRepository, logRepo *repository.DomainHealthLogRepository, blRepo *repository.DomainBlacklistRepository) DomainHealthService {
	if logRepo == nil {
		logRepo = repository.NewDomainHealthLogRepository(nil)
	}
	if blRepo == nil {
		blRepo = repository.NewDomainBlacklistRepository(nil)
	}
	return &domainHealthService{
		repo:        repo,
		logRepo:     logRepo,
		blacklistR:  blRepo,
		httpClient:  &http.Client{Timeout: HealthCheckTimeout},
		probeTarget: "/",
	}
}

// CheckOne 探测单个域名
func (s *domainHealthService) CheckOne(ctx context.Context, domainID int) (*HealthCheckResult, error) {
	dp, err := s.repo.GetByID(ctx, domainID)
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}
	if dp == nil {
		return nil, errors.New("域名不存在")
	}
	return s.checkAndPersist(ctx, dp), nil
}

// CheckAll 探测所有域名
func (s *domainHealthService) CheckAll(ctx context.Context) ([]*HealthCheckResult, error) {
	rows, _, err := s.repo.List(ctx, 1, 1000, "", 0)
	if err != nil {
		return nil, err
	}
	results := make([]*HealthCheckResult, 0, len(rows))
	for _, dp := range rows {
		results = append(results, s.checkAndPersist(ctx, dp))
	}
	return results, nil
}

// checkAndPersist 实际执行探测并落库
func (s *domainHealthService) checkAndPersist(ctx context.Context, dp *model.DomainPool) *HealthCheckResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	probe, err := s.ProbeOnce(ctx, dp.Domain)
	if err != nil {
		logger.Errorf("探测域名失败 domain=%s: %v", dp.Domain, err)
	}

	// 端口修正：若 port=0 用 http=80，https=443
	port := dp.Port
	if port == 0 {
		if probe.HTTPOk && probe.HTTPStatus > 0 {
			port = 80
		} else {
			port = 443
		}
	}

	// 黑名单查询
	blacklisted, blEntry, blErr := s.blacklistR.IsBlacklisted(ctx, dp.Domain)
	if blErr != nil {
		logger.Errorf("查询黑名单失败 domain=%s: %v", dp.Domain, blErr)
	}
	blSrc := ""
	if blEntry != nil {
		blSrc = blEntry.Source + ":" + blEntry.Platform
	}

	// 计算评分
	score, failures := calcHealthScore(dp.HealthScore, dp.ConsecutiveFailures, probe, blacklisted)

	// 决定动作
	action := "none"
	newStatus := dp.Status
	if !probe.DNSOK || !probe.HTTPOk || blacklisted || score == 0 {
		// 不可用
		newStatus = 2
		action = "mark_unhealthy"
	} else if score >= HealthScoreHealthy && newStatus != 1 {
		// 恢复
		newStatus = 1
	}

	// 写健康度
	blacklistAt := time.Time{}
	if blacklisted && blEntry != nil && !blEntry.CreatedAt.IsZero() {
		blacklistAt = blEntry.CreatedAt
	}
	blacklistNote := ""
	if blEntry != nil {
		blacklistNote = blEntry.Reason
	}
	if err := s.repo.UpdateHealth(ctx, dp.ID, score, failures, probe.DNSOK, probe.DNSError,
		probe.HTTPStatus, probe.HTTPLatency, blacklisted, blacklistAt, blacklistNote); err != nil {
		logger.Errorf("更新域名健康度失败 id=%d: %v", dp.ID, err)
	}
	if err := s.repo.UpdateStatus(ctx, dp.ID, newStatus); err != nil {
		logger.Errorf("更新域名状态失败 id=%d: %v", dp.ID, err)
	}

	// 写日志
	logRow := &model.DomainHealthLog{
		DomainID:     dp.ID,
		Domain:       dp.Domain,
		CheckedAt:    time.Now(),
		DNSOK:        probe.DNSOK,
		DNSError:     probe.DNSError,
		HTTPOk:       probe.HTTPOk,
		HTTPStatus:   probe.HTTPStatus,
		HTTPLatency:  probe.HTTPLatency,
		HTTPErrorMsg: probe.HTTPErrorMsg,
		OnBlacklist:  blacklisted,
		BlacklistSrc: blSrc,
		HealthScore:  score,
		ActionTaken:  action,
	}
	if err := s.logRepo.Create(ctx, logRow); err != nil {
		logger.Errorf("写入健康度日志失败 id=%d: %v", dp.ID, err)
	}

	// 自动切换判定
	if dp.AutoSwitchEnabled {
		// 评分过低 或 连续失败超限 且 当前活跃
		if dp.IsActive && (score < HealthScoreSwitchThreshold || failures >= ConsecutiveFailureLimit) {
			if _, err := s.SwitchToBest(ctx, fmt.Sprintf("健康度自动切换: score=%d failures=%d", score, failures)); err != nil {
				logger.Errorf("自动切换失败: %v", err)
			} else {
				action = "switch_over"
			}
		}
	}

	probe.ActionTaken = action
	probe.HealthScore = score
	probe.DomainID = dp.ID
	probe.Port = port
	return probe
}

// calcHealthScore 综合评分
//   - 起始 100
//   - DNS 失败：-30
//   - HTTP 非 2xx/3xx：-50
//   - 黑名单：直接归零
//   - 每次连续失败：-10，封顶 -50
//   - 最终 clamp 在 [0, 100]
func calcHealthScore(prevScore, prevFailures int, probe *HealthCheckResult, blacklisted bool) (int, int) {
	if blacklisted {
		return 0, prevFailures + 1
	}
	score := 100
	if !probe.DNSOK {
		score -= 30
	}
	if !probe.HTTPOk {
		score -= 50
	}
	// 累加连续失败：探测失败 +1，否则归零
	failures := prevFailures
	if !probe.DNSOK || !probe.HTTPOk {
		failures++
		if failures > 10 {
			failures = 10
		}
		score -= failures * 5
	} else {
		failures = 0
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, failures
}

// ProbeOnce 不写库的纯探测（用于单点调试 / 单元测试）
func (s *domainHealthService) ProbeOnce(ctx context.Context, domain string) (*HealthCheckResult, error) {
	res := &HealthCheckResult{
		Domain:    domain,
		CheckedAt: time.Now(),
	}

	// 1) DNS 解析
	ips, dnsErr := net.LookupHost(domain)
	if dnsErr != nil || len(ips) == 0 {
		res.DNSError = dnsErr.Error()
		if res.DNSError == "" {
			res.DNSError = "no such host"
		}
	} else {
		res.DNSOK = true
	}

	// 2) HTTP HEAD 探测（先 http，再 https）
	urls := []string{
		"http://" + domain + s.probeTarget,
		"https://" + domain + s.probeTarget,
	}
	for _, u := range urls {
		ok, status, latency, errMsg := s.doHTTPHead(ctx, u)
		if ok {
			res.HTTPOk = true
			res.HTTPStatus = status
			res.HTTPLatency = int(latency / time.Millisecond)
			break
		}
		res.HTTPErrorMsg = errMsg
		// 继续尝试下一个 scheme
	}

	// 评分（用于 ProbeOnce 返回结果，方便 controller 直接展示）
	score, _ := calcHealthScore(100, 0, res, false)
	res.HealthScore = score
	return res, nil
}

func (s *domainHealthService) doHTTPHead(ctx context.Context, url string) (bool, int, time.Duration, string) {
	start := time.Now()
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false, 0, time.Since(start), err.Error()
	}
	req.Header.Set("User-Agent", "mtk-domain-health/1.0")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, 0, time.Since(start), err.Error()
	}
	defer resp.Body.Close()
	latency := time.Since(start)
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, resp.StatusCode, latency, ""
	}
	return false, resp.StatusCode, latency, "HTTP " + strconv.Itoa(resp.StatusCode)
}

// SwitchActive 手动切换
func (s *domainHealthService) SwitchActive(ctx context.Context, domainID int, reason string) error {
	dp, err := s.repo.GetByID(ctx, domainID)
	if err != nil {
		return err
	}
	if dp == nil {
		return errors.New("域名不存在")
	}
	if dp.OnBlacklist {
		return errors.New("域名在黑名单中，禁止切换")
	}
	if dp.HealthScore < HealthScoreHealthy {
		return fmt.Errorf("域名健康度过低（%d），禁止切换", dp.HealthScore)
	}
	if err := s.repo.DeactivateAll(ctx); err != nil {
		return err
	}
	now := time.Now()
	return s.repo.UpdateActive(ctx, dp.ID, true, &now, 0)
}

// SwitchToBest 自动切换到评分最高可用域名
func (s *domainHealthService) SwitchToBest(ctx context.Context, reason string) (*model.DomainPool, error) {
	candidates, err := s.repo.ListAvailable(ctx, HealthScoreHealthy)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("无可用域名（健康度 >= " + strconv.Itoa(HealthScoreHealthy) + " 且未在黑名单）")
	}
	best := candidates[0]
	// 查找当前活跃域名
	actives, _ := s.repo.ListActive(ctx)
	prevID := 0
	for _, a := range actives {
		prevID = a.ID
		break
	}
	if err := s.repo.DeactivateAll(ctx); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.repo.UpdateActive(ctx, best.ID, true, &now, prevID); err != nil {
		return nil, err
	}
	logger.Infof("[domain-health] 自动切换完成 target=%s reason=%s", best.Domain, reason)
	return best, nil
}

// GetActiveDomain 获取当前活跃域名（无则返回 nil）
func (s *domainHealthService) GetActiveDomain(ctx context.Context) (*model.DomainPool, error) {
	actives, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	if len(actives) == 0 {
		return nil, nil
	}
	return actives[0], nil
}

// ListAvailable 列出可用域名
func (s *domainHealthService) ListAvailable(ctx context.Context, minScore int) ([]*model.DomainPool, error) {
	if minScore <= 0 {
		minScore = HealthScoreHealthy
	}
	return s.repo.ListAvailable(ctx, minScore)
}

// ListHealthLogs 查询日志
func (s *domainHealthService) ListHealthLogs(ctx context.Context, domainID int, limit int) ([]*model.DomainHealthLog, error) {
	return s.logRepo.ListByDomain(ctx, domainID, limit)
}
