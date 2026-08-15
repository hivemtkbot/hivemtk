package service


import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"hivemtk-user/internal/model"
)

// CustomerProfileItem 客户档案列表项
type CustomerProfileItem struct {
	ID        string   `json:"id"`
	UnifiedID string   `json:"unified_id"`
	Name      string   `json:"name"`
	Phone     string   `json:"phone"`
	Email     string   `json:"email"`
	Tags      []string `json:"tags"`
	RFMScore  int      `json:"rfm_score"`
	ChurnRisk string   `json:"churn_risk"`
	Platforms    []string `json:"platforms"`
	SessionCount int      `json:"session_count"`
	MessageCount int      `json:"message_count"`
	LastActiveAt string   `json:"last_active_at"`
	CreatedAt    string   `json:"created_at"`
	Status string `json:"status"`
}

// ListCustomerProfiles 分页查询客户档案列表。
// keyword 在服务端匹配 phone / email / unified_id。
func (s *Customer360Service) ListCustomerProfiles(ctx context.Context, page, pageSize int, keyword string) ([]*CustomerProfileItem, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	customers, total, err := s.customerRepo.List(ctx, page, pageSize, strings.TrimSpace(keyword))
	if err != nil {
		return nil, 0, err
	}

	sessionsByCustomer := s.batchResolveSessions(ctx, customers)

	now := time.Now()
	items := make([]*CustomerProfileItem, 0, len(customers))
	for _, c := range customers {
		item := &CustomerProfileItem{
			ID:        c.ID,
			UnifiedID: c.UnifiedID,
			Phone:     c.Phone,
			Email:     c.Email,
			Tags:      parseCustomerTags(c.Tags),
			RFMScore:  c.RFMScore,
			ChurnRisk: c.ChurnRisk,
			Platforms: []string{},
			CreatedAt: c.CreatedAt.Format("2006-01-02 15:04:05"),
			Status:    "new",
		}

		sessions := sessionsByCustomer[c.ID]
		item.SessionCount = len(sessions)
		var lastActive time.Time
		platformSeen := map[string]struct{}{}
		for _, sess := range sessions {
			item.MessageCount += sess.MessageCount
			if item.Name == "" && strings.TrimSpace(sess.UserName) != "" {
				item.Name = sess.UserName
			}
			if p := string(sess.Platform); p != "" {
				if _, ok := platformSeen[p]; !ok {
					platformSeen[p] = struct{}{}
					item.Platforms = append(item.Platforms, p)
				}
			}
			ts := sess.CreatedAt
			if sess.LastMessageAt != nil && sess.LastMessageAt.After(ts) {
				ts = *sess.LastMessageAt
			}
			if ts.After(lastActive) {
				lastActive = ts
			}
		}

		if !lastActive.IsZero() {
			item.LastActiveAt = lastActive.Format("2006-01-02 15:04:05")
			switch days := now.Sub(lastActive).Hours() / 24; {
			case days <= 7:
				item.Status = "active"
			case days <= 30:
				item.Status = "idle"
			default:
				item.Status = "inactive"
			}
		}

		if item.Name == "" {
			item.Name = fallbackCustomerName(c)
		}
		items = append(items, item)
	}
	return items, total, nil
}

// customerSessionKeys 返回该客户可用于匹配会话的候选键。
//
// 真实数据形态（生产库实测）：customer_sessions.one_id 绝大多数为空，且已填的值存的是平台
// openid（如 5e4f75e3...），并不等于 customers.unified_id（如 lm:xiaohongshu:5e4f75e3...）。
// 真正稳定成立的关联是：unified_id 的最后一段 == customer_sessions.user_id
// （visitor:default:sim_xxx ↔ sim_xxx、lm:xiaohongshu:<openid> ↔ <openid>）。
// 因此同时按 one_id 与 user_id 两条路径解析，任一命中即算该客户的会话。
func customerSessionKeys(c *model.Customer) (userIDs []string, oneIDs []string) {
	if c == nil {
		return nil, nil
	}
	if c.UnifiedID != "" {
		oneIDs = append(oneIDs, c.UnifiedID)
		userIDs = append(userIDs, c.UnifiedID)
		if idx := strings.LastIndex(c.UnifiedID, ":"); idx >= 0 && idx+1 < len(c.UnifiedID) {
			tail := c.UnifiedID[idx+1:]
			oneIDs = append(oneIDs, tail)
			userIDs = append(userIDs, tail)
		}
	}
	for _, v := range []string{c.Phone, c.Email, c.WechatOpenID, c.DouyinOpenID, c.XiaohongshuID} {
		if v != "" {
			userIDs = append(userIDs, v)
		}
	}
	return userIDs, oneIDs
}

// batchResolveSessions 批量解析客户 → 会话，返回 map[customerID][]session（已去重）
func (s *Customer360Service) batchResolveSessions(ctx context.Context, customers []*model.Customer) map[string][]*model.CustomerSession {
	result := make(map[string][]*model.CustomerSession, len(customers))
	if len(customers) == 0 {
		return result
	}
	allUserIDs := make([]string, 0, len(customers)*2)
	allOneIDs := make([]string, 0, len(customers))
	ownerByUserID := map[string]string{}
	ownerByOneID := map[string]string{}
	for _, c := range customers {
		userIDs, oneIDs := customerSessionKeys(c)
		for _, k := range userIDs {
			if _, exists := ownerByUserID[k]; !exists {
				ownerByUserID[k] = c.ID
				allUserIDs = append(allUserIDs, k)
			}
		}
		for _, k := range oneIDs {
			if _, exists := ownerByOneID[k]; !exists {
				ownerByOneID[k] = c.ID
				allOneIDs = append(allOneIDs, k)
			}
		}
	}

	seen := map[string]map[uint]struct{}{}
	attach := func(customerID string, sess *model.CustomerSession) {
		if customerID == "" || sess == nil {
			return
		}
		if seen[customerID] == nil {
			seen[customerID] = map[uint]struct{}{}
		}
		if _, dup := seen[customerID][sess.ID]; dup {
			return
		}
		seen[customerID][sess.ID] = struct{}{}
		result[customerID] = append(result[customerID], sess)
	}

	if byOne, err := s.sessionRepo.ListByOneIDsBatch(ctx, allOneIDs); err == nil {
		for key, list := range byOne {
			for _, sess := range list {
				attach(ownerByOneID[key], sess)
			}
		}
	}
	if byUser, err := s.sessionRepo.ListByUserIDsBatch(ctx, allUserIDs); err == nil {
		for key, list := range byUser {
			for _, sess := range list {
				attach(ownerByUserID[key], sess)
			}
		}
	}
	return result
}

// resolveSessionsForCustomer 解析单个客户的关联会话（详情页复用同一套关联规则）
func (s *Customer360Service) resolveSessionsForCustomer(ctx context.Context, c *model.Customer) []*model.CustomerSession {
	if c == nil {
		return nil
	}
	return s.batchResolveSessions(ctx, []*model.Customer{c})[c.ID]
}

// parseCustomerTags 解析 customers.tags（JSON 数组字符串），非法内容按逗号分隔兜底
func parseCustomerTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err == nil {
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	out := []string{}
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// fallbackCustomerName 无会话昵称时的展示名：手机号 > 邮箱 > OneID > 主键
func fallbackCustomerName(c *model.Customer) string {
	switch {
	case c.Phone != "":
		return c.Phone
	case c.Email != "":
		return c.Email
	case c.UnifiedID != "":
		return c.UnifiedID
	default:
		return c.ID
	}
}


