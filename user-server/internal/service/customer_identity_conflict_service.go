package service

import (
	"marketing/internal/model"
	"marketing/internal/repository"
)

// IdentityConflict 身份冲突记录
// 表示同一身份标识(手机号/邮箱/各平台 OpenID)被多个客户记录持有
type IdentityConflict struct {
	IdentityType  string             `json:"identity_type"`  // phone / email / wechat_open_id / douyin_open_id / xiaohongshu_id
	IdentityValue string             `json:"identity_value"` // 身份标识值
	CustomerIDs   []string           `json:"customer_ids"`   // 关联到的客户ID列表
	Customers     []ConflictCustomer `json:"customers"`
	MatchScore    float64            `json:"match_score"` // 冲突置信度 0-1，恒为1表示精确命中
}

// ConflictCustomer 冲突中涉及的客户摘要
type ConflictCustomer struct {
	ID        string `json:"id"`
	UnifiedID string `json:"unified_id"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

// 通过对每种身份字段分组统计，找出被多个客户持有的身份标识。
// 返回分页后的冲突列表与总数。
func DetectIdentityConflicts(repo repository.CustomerRepository, page, pageSize int) ([]*IdentityConflict, int64) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	all, _, err := repo.List(1, 1000)
	if err != nil || len(all) == 0 {
		return []*IdentityConflict{}, 0
	}

	// 按各身份字段聚合
	type bucket struct {
		value  string
		owners []*model.Customer
	}
	phoneMap := map[string][]*model.Customer{}
	emailMap := map[string][]*model.Customer{}
	wechatMap := map[string][]*model.Customer{}
	douyinMap := map[string][]*model.Customer{}
	xhsMap := map[string][]*model.Customer{}

	add := func(m map[string][]*model.Customer, key string, c *model.Customer) {
		if key == "" {
			return
		}
		m[key] = append(m[key], c)
	}
	for _, c := range all {
		add(phoneMap, c.Phone, c)
		add(emailMap, c.Email, c)
		add(wechatMap, c.WechatOpenID, c)
		add(douyinMap, c.DouyinOpenID, c)
		add(xhsMap, c.XiaohongshuID, c)
	}

	var conflicts []*IdentityConflict
	collect := func(m map[string][]*model.Customer, idType string) {
		for val, owners := range m {
			if len(owners) < 2 {
				continue
			}
			ids := make([]string, 0, len(owners))
			summary := make([]ConflictCustomer, 0, len(owners))
			seen := map[string]bool{}
			for _, o := range owners {
				if seen[o.ID] {
					continue
				}
				seen[o.ID] = true
				ids = append(ids, o.ID)
				summary = append(summary, ConflictCustomer{
					ID:        o.ID,
					UnifiedID: o.UnifiedID,
					Phone:     o.Phone,
					Email:     o.Email,
					CreatedAt: o.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			if len(ids) < 2 {
				continue
			}
			conflicts = append(conflicts, &IdentityConflict{
				IdentityType:  idType,
				IdentityValue: val,
				CustomerIDs:   ids,
				Customers:     summary,
				MatchScore:    1.0,
			})
		}
	}
	collect(phoneMap, "phone")
	collect(emailMap, "email")
	collect(wechatMap, "wechat_open_id")
	collect(douyinMap, "douyin_open_id")
	collect(xhsMap, "xiaohongshu_id")

	// 稳定排序：按冲突客户数降序
	for i := 0; i < len(conflicts); i++ {
		for j := i + 1; j < len(conflicts); j++ {
			if len(conflicts[j].CustomerIDs) > len(conflicts[i].CustomerIDs) {
				conflicts[i], conflicts[j] = conflicts[j], conflicts[i]
			}
		}
	}

	total := int64(len(conflicts))
	start := (page - 1) * pageSize
	if start >= len(conflicts) {
		return []*IdentityConflict{}, total
	}
	end := start + pageSize
	if end > len(conflicts) {
		end = len(conflicts)
	}
	return conflicts[start:end], total
}

type OneIDCustomer struct {
	ID            string  `json:"id"`
	UnifiedID     string  `json:"unified_id"`
	Phone         string  `json:"phone"`
	Email         string  `json:"email"`
	WechatOpenID  string  `json:"wechat_open_id"`
	DouyinOpenID  string  `json:"douyin_open_id"`
	XiaohongshuID string  `json:"xiaohongshu_id"`
	IdentityCount int     `json:"identity_count"`
	MatchScore    float64 `json:"match_score"` // 身份完整度：已绑定身份数 / 5
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

// ListOneIDCustomers 列出 OneID 客户
func ListOneIDCustomers(repo repository.CustomerRepository, page, pageSize int, keyword string) ([]*OneIDCustomer, int64) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	list, total, err := repo.List(page, pageSize)
	if err != nil {
		return []*OneIDCustomer{}, 0
	}
	result := make([]*OneIDCustomer, 0, len(list))
	for _, c := range list {
		oc := &OneIDCustomer{
			ID:            c.ID,
			UnifiedID:     c.UnifiedID,
			Phone:         c.Phone,
			Email:         c.Email,
			WechatOpenID:  c.WechatOpenID,
			DouyinOpenID:  c.DouyinOpenID,
			XiaohongshuID: c.XiaohongshuID,
			CreatedAt:     c.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:     c.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		count := 0
		if c.Phone != "" {
			count++
		}
		if c.Email != "" {
			count++
		}
		if c.WechatOpenID != "" {
			count++
		}
		if c.DouyinOpenID != "" {
			count++
		}
		if c.XiaohongshuID != "" {
			count++
		}
		oc.IdentityCount = count
		oc.MatchScore = float64(count) / 5.0
		result = append(result, oc)
	}
	_ = keyword
	return result, total
}
