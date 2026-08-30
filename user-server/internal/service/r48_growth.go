// r48_growth.go R48 T6-T12 综合实现（竞品吸收：Webhook Out/自定义属性/保存视图/报表订阅/转录导出/UTM/AI绩效）
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ==================== T6: Webhook Out 事件订阅 ====================

// WebhookSubscription 出站 Webhook 订阅
type WebhookSubscription struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	URL       string    `gorm:"type:varchar(500);not null" json:"url"`
	Events    string    `gorm:"type:varchar(500);not null" json:"events"` // 逗号分隔: message.created,session.created
	Secret    string    `gorm:"type:varchar(120);not null" json:"secret"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (WebhookSubscription) TableName() string { return "webhook_subscriptions" }

// Webhook 事件常量
const (
	WebhookEventMessageCreated = "message.created"
	WebhookEventSessionCreated = "session.created"
	WebhookEventSessionClosed  = "session.closed"
)

// webhookOutClient 出站 HTTP 客户端（装配处可注入测试双）
var webhookOutClient = &http.Client{Timeout: 5 * time.Second}

// PublishWebhookEvent 事件发布（fire-and-forget，失败仅日志，绝不阻塞主链路）
func PublishWebhookEvent(ctx context.Context, event string, payload map[string]any) {
	g := db.GetDB()
	var subs []model.WebhookSubscription
	if err := g.WithContext(ctx).
		Where("enabled = ? AND (events LIKE ? OR events LIKE ?)", true, "%"+event+"%", "%all%").
		Limit(20).Find(&subs).Error; err != nil || len(subs) == 0 {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"event":     event,
		"timestamp": time.Now().Format(time.RFC3339),
		"data":      payload,
	})
	for _, sub := range subs {
		go func(sub model.WebhookSubscription, body []byte) {
			defer func() { _ = recover() }()
			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, sub.URL, strings.NewReader(string(body)))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			mac := hmac.New(sha256.New, []byte(sub.Secret))
			mac.Write(body)
			req.Header.Set("X-Hivemtk-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
			resp, err := webhookOutClient.Do(req)
			if err != nil {
				slog.Warn("[WebhookOut] 投递失败", "url", sub.URL, "event", event, "err", err)
				return
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			if resp.StatusCode >= 400 {
				slog.Warn("[WebhookOut] 远端非 2xx", "url", sub.URL, "status", resp.StatusCode)
			}
		}(sub, body)
	}
}

// WebhookSubService CRUD 服务
type WebhookSubService struct{}

// NewWebhookSubService 构造
func NewWebhookSubService() *WebhookSubService { return &WebhookSubService{} }

// Create 创建订阅（URL 必须 http(s)，secret 自动生成）
func (s *WebhookSubService) Create(ctx context.Context, url, events string) (*model.WebhookSubscription, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("url 必须以 http(s):// 开头")
	}
	if strings.TrimSpace(events) == "" {
		events = "all"
	}
	secret := "whsec_" + fmt.Sprintf("%d", time.Now().UnixNano())
	sub := &model.WebhookSubscription{URL: url, Events: events, Secret: secret, Enabled: true}
	if err := db.GetDB().WithContext(ctx).Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

// List 订阅列表
func (s *WebhookSubService) List(ctx context.Context) ([]*model.WebhookSubscription, error) {
	var list []*model.WebhookSubscription
	err := db.GetDB().WithContext(ctx).Order("id ASC").Find(&list).Error
	return list, err
}

// Delete 删除
func (s *WebhookSubService) Delete(ctx context.Context, id uint) error {
	return db.GetDB().WithContext(ctx).Delete(&model.WebhookSubscription{}, id).Error
}

// ==================== T7: 联系人自定义属性 ====================

// SetCustomAttributes 更新客户自定义属性（JSONB merge）
func (s *CustomerServicePlusService) SetCustomAttributes(ctx context.Context, customerID string, attrs map[string]any) (map[string]any, error) {
	g := db.GetDB()
	var curStr string
	if err := g.WithContext(ctx).Table("customers").
		Select("COALESCE(NULLIF(custom_attributes::text,''), '{}')").
		Where("id = ?", customerID).Scan(&curStr).Error; err != nil {
		return nil, err
	}
	merged := map[string]any{}
	_ = json.Unmarshal([]byte(curStr), &merged)
	for k, v := range attrs {
		merged[k] = v
	}
	raw, _ := json.Marshal(merged)
	res := g.WithContext(ctx).Table("customers").
		Where("id = ?", customerID).
		Update("custom_attributes", string(raw))
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return merged, nil
}

// ==================== T8: 保存的自定义视图 ====================

// SavedView 保存视图
type SavedView struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Route     string    `gorm:"type:varchar(100);not null" json:"route"`
	Filter    string    `gorm:"type:text" json:"filter"` // 过滤条件 JSON
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SavedView) TableName() string { return "saved_views" }

// CreateSavedView 创建视图（同名覆盖）
func (s *CustomerServicePlusService) CreateSavedView(ctx context.Context, userID uint, name, route, filter string) (*model.SavedView, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(route) == "" {
		return nil, fmt.Errorf("name/route 必填")
	}
	g := db.GetDB()
	_ = g.WithContext(ctx).
		Where("user_id = ? AND name = ? AND route = ?", userID, name, route).
		Delete(&model.SavedView{}).Error
	v := &model.SavedView{UserID: userID, Name: name, Route: route, Filter: filter}
	if err := g.WithContext(ctx).Create(v).Error; err != nil {
		return nil, err
	}
	return v, nil
}

// ListSavedViews 视图列表（按用户+路由）
func (s *CustomerServicePlusService) ListSavedViews(ctx context.Context, userID uint, route string) ([]*model.SavedView, error) {
	var list []*model.SavedView
	q := db.GetDB().WithContext(ctx).Where("user_id = ?", userID)
	if route != "" {
		q = q.Where("route = ?", route)
	}
	err := q.Order("id ASC").Limit(100).Find(&list).Error
	return list, err
}

// DeleteSavedView 删除视图
func (s *CustomerServicePlusService) DeleteSavedView(ctx context.Context, id, userID uint) error {
	res := db.GetDB().WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&model.SavedView{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ==================== T9: 定时邮件报表订阅 ====================

// ReportSubscription 报表订阅
type ReportSubscription struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Email     string    `gorm:"type:varchar(200);not null;uniqueIndex" json:"email"`
	Schedule  string    `gorm:"type:varchar(20);default:'daily'" json:"schedule"` // daily/weekly
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	LastSent  *time.Time `json:"last_sent,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (ReportSubscription) TableName() string { return "report_subscriptions" }

// CreateReportSubscription 订阅报表
func (s *CustomerServicePlusService) CreateReportSubscription(ctx context.Context, email, schedule string) (*model.ReportSubscription, error) {
	if !strings.Contains(email, "@") {
		return nil, fmt.Errorf("邮箱格式无效")
	}
	if schedule != "daily" && schedule != "weekly" {
		schedule = "daily"
	}
	g := db.GetDB()
	// 幂等：同邮箱覆盖
	_ = g.WithContext(ctx).Where("email = ?", email).Delete(&model.ReportSubscription{}).Error
	sub := &model.ReportSubscription{Email: email, Schedule: schedule, Enabled: true}
	if err := g.WithContext(ctx).Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

// ListReportSubscriptions 订阅列表
func (s *CustomerServicePlusService) ListReportSubscriptions(ctx context.Context) ([]*model.ReportSubscription, error) {
	var list []*model.ReportSubscription
	err := db.GetDB().WithContext(ctx).Order("id ASC").Find(&list).Error
	return list, err
}

// DeleteReportSubscription 退订
func (s *CustomerServicePlusService) DeleteReportSubscription(ctx context.Context, id uint) error {
	return db.GetDB().WithContext(ctx).Delete(&model.ReportSubscription{}, id).Error
}

// SendScheduledReports cron 入口：给全部启用订阅发送昨日汇总（汇总 CSV 内存生成→SMTP）
func (s *CustomerServicePlusService) SendScheduledReports(ctx context.Context) (int, error) {
	g := db.GetDB()
	var subs []model.ReportSubscription
	if err := g.WithContext(ctx).Where("enabled = ?", true).Find(&subs).Error; err != nil {
		return 0, err
	}
	if len(subs) == 0 {
		return 0, nil
	}
	// 昨日汇总（会话量/消息量/新客户）
	type summaryRow struct {
		Metric string `gorm:"column:metric"`
		Value  int64  `gorm:"column:value"`
	}
	var rows []summaryRow
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	err := g.WithContext(ctx).Raw(`
		SELECT '新会话' AS metric, COUNT(*) AS value FROM customer_sessions WHERE created_at::date = ?
		UNION ALL SELECT '新消息', COUNT(*) FROM session_messages WHERE created_at::date = ?
		UNION ALL SELECT '新客户', COUNT(*) FROM customers WHERE created_at::date = ?
		UNION ALL SELECT '发送消息', COUNT(*) FROM message_hub WHERE direction='outbound' AND sent_at::date = ?`,
		yesterday, yesterday, yesterday, yesterday).Scan(&rows).Error
	if err != nil {
		return 0, err
	}
	var csv strings.Builder
	csv.WriteString("指标,数量\n")
	for _, r := range rows {
		csv.WriteString(fmt.Sprintf("%s,%d\n", r.Metric, r.Value))
	}
	// 发送（复用 EmailService.Send，取首个 active SMTP）
	emailSvc := NewEmailService(g)
	sent := 0
	for _, sub := range subs {
		subject := fmt.Sprintf("每日数据报表 %s", yesterday)
		html := "<pre style='font-family:monospace'>" + csv.String() + "</pre>"
		if _, err := emailSvc.Send(ctx, 0, sub.Email, subject, html, nil); err != nil {
			slog.Warn("[ReportCron] 报表发送失败", "email", sub.Email, "err", err)
			continue
		}
		g.WithContext(ctx).Model(&model.ReportSubscription{}).Where("id = ?", sub.ID).Update("last_sent", time.Now())
		sent++
	}
	return sent, nil
}

// ==================== T10: 会话转录导出 ====================

// SessionTranscript 导出转录（csv=true 时返回 CSV 两列）
func (s *CustomerServicePlusService) SessionTranscript(ctx context.Context, sessionID string, csv bool) (string, string, error) {
	type msgRow struct {
		SenderType string `gorm:"column:sender_type"`
		SenderName string `gorm:"column:sender_name"`
		Content    string `gorm:"column:content"`
		CreatedAt  time.Time `gorm:"column:created_at"`
	}
	var msgs []transcriptMsgRow
	// R51 业务修复: 转录排除内部备注（is_internal 仅坐席可见）
	if err := db.GetDB().WithContext(ctx).
		Table("session_messages").
		Select("sender_type, COALESCE(sender_name,'') AS sender_name, content, created_at").
		Where("session_id = ? AND is_internal = ?", sessionID, false).
		Order("created_at ASC").Limit(2000).Scan(&msgs).Error; err != nil {
		return "", "", err
	}
	if len(msgs) == 0 {
		return "", "", gorm.ErrRecordNotFound
	}
	if csv {
		var b strings.Builder
		b.WriteString("时间,发送方,内容\n")
		for _, m := range msgs {
			line := strings.ReplaceAll(m.Content, "\"", "\"\"")
			line = strings.ReplaceAll(line, "\n", " ")
			fmt.Fprintf(&b, "%s,%s,\"%s\"\n", m.CreatedAt.Format("2006-01-02 15:04:05"), whoOf(m), line)
		}
		return "text/csv", b.String(), nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("会话转录 %s\n导出时间 %s\n====================\n\n",
		sessionID, time.Now().Format("2006-01-02 15:04")))
	for _, m := range msgs {
		fmt.Fprintf(&b, "[%s] %s:\n%s\n\n", m.CreatedAt.Format("15:04:05"), whoOf(m), m.Content)
	}
	return "text/plain", b.String(), nil
}

type transcriptMsgRow struct {
	SenderType string    `gorm:"column:sender_type"`
	SenderName string    `gorm:"column:sender_name"`
	Content    string    `gorm:"column:content"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func whoOf(m transcriptMsgRow) string {
	if m.SenderName != "" {
		return m.SenderName
	}
	switch m.SenderType {
	case "customer", "user":
		return "客户"
	case "staff", "agent":
		return "坐席"
	case "ai", "bot":
		return "AI"
	}
	return m.SenderType
}

// ==================== T12: AI 代理绩效报表 ====================

// AIPerformanceResult 自动化率漏斗
type AIPerformanceResult struct {
	Window        string         `json:"window"`
	TotalSessions int64          `json:"total_sessions"`
	AIHandled     int64          `json:"ai_handled"`
	HumanHandled  int64          `json:"human_handled"`
	ClosedByAI    int64          `json:"closed_by_ai"`
	AutoRate      float64        `json:"automation_rate"` // AI 处理占比
	LLMCalls      int64          `json:"llm_calls"`
	LLMCost       float64        `json:"llm_cost"`
	Breakdown     map[string]int64 `json:"llm_by_scenario,omitempty"`
}

// AIPerformance AI 代理绩效（窗口天）
func (s *EmailGapService) AIPerformance(ctx context.Context, days int) (*AIPerformanceResult, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	g := db.GetDB()
	since := time.Now().AddDate(0, 0, -days)
	res := &AIPerformanceResult{Window: fmt.Sprintf("%dd", days)}
	_ = g.WithContext(ctx).Table("customer_sessions").
		Where("created_at >= ?", since).Count(&res.TotalSessions).Error
	_ = g.WithContext(ctx).Table("customer_sessions").
		Where("created_at >= ? AND status = ?", since, "closed").Count(&res.ClosedByAI).Error
	// AI 处理 = ai_agent_id/agent 归属 AI 的会话（诚实口径: status ai_handling/已关闭未转人工）
	_ = g.WithContext(ctx).Table("customer_sessions").
		Where("created_at >= ? AND (agent_id IS NULL OR agent_id = '')", since).Count(&res.AIHandled).Error
	res.HumanHandled = res.TotalSessions - res.AIHandled
	if res.TotalSessions > 0 {
		res.AutoRate = float64(res.AIHandled) * 100 / float64(res.TotalSessions)
	}
	// LLM 调用与成本
	type lr struct {
		Scenario string  `gorm:"column:scenario"`
		Cnt      int64   `gorm:"column:cnt"`
		Cost     float64 `gorm:"column:cost"`
	}
	var lrs []lr
	if err := g.WithContext(ctx).Table("llm_routing_logs").
		Select("scenario, COUNT(*) AS cnt, COALESCE(SUM(cost),0) AS cost").
		Where("created_at >= ?", since).
		Group("scenario").Scan(&lrs).Error; err == nil {
		res.Breakdown = map[string]int64{}
		for _, r := range lrs {
			res.LLMCalls += r.Cnt
			res.LLMCost += r.Cost
			res.Breakdown[r.Scenario] = r.Cnt
		}
	}
	return res, nil
}
