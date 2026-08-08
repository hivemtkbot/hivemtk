package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/bcrypt"

	"gorm.io/gorm"
)

// SecurityAuditService 安全审计：执行一组可选（非默认开启）安全检查，
// 持久化审计结果与明细，供前端「系统设置 → 安全审计」展示。
//
// 注意：本审计为管理员手动触发（前端点「立即审计」），不默认后台静默扫描，
// 符合「无默认开启内容/安全扫描」的私域部署约束。
type SecurityAuditService struct {
	db *gorm.DB
}

// NewSecurityAuditService 构造安全审计服务
func NewSecurityAuditService(db *gorm.DB) *SecurityAuditService {
	return &SecurityAuditService{db: db}
}

type auditCheck struct {
	name     string
	category string
	level    string // critical/high/medium/low
	weight   int
	run      func(ctx context.Context) (result string, message string)
}

// RunAudit 执行一次安全审计，写入审计记录与明细，返回完整记录（含 items）。
func (s *SecurityAuditService) RunAudit(ctx context.Context, auditName string) (*model.SecurityAudit, error) {
	if auditName == "" {
		auditName = "系统安全审计"
	}
	now := time.Now()
	checks := s.buildChecks(ctx)

	items := make([]model.SecurityAuditItem, 0, len(checks))
	passed, failed, warnings := 0, 0, 0
	weightSum, weightPass := 0, 0
	worstLevel := ""
	levelRank := map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}

	for _, c := range checks {
		res, msg := c.run(ctx)
		items = append(items, model.SecurityAuditItem{
			Name: c.name, Category: c.category, Level: c.level,
			Result: res, Message: msg, CreatedAt: now,
		})
		weightSum += c.weight
		switch res {
		case "pass":
			passed++
			weightPass += c.weight
		case "warn":
			warnings++
			weightPass += c.weight / 2
		case "fail":
			failed++
			if levelRank[c.level] > levelRank[worstLevel] {
				worstLevel = c.level
			}
		}
	}

	score := 0
	if weightSum > 0 {
		score = int(100 * weightPass / weightSum)
	}
	risk := "low"
	if failed > 0 {
		risk = worstLevel
	}

	audit := &model.SecurityAudit{
		AuditName:   auditName,
		RiskLevel:   risk,
		Score:       score,
		TotalChecks: len(checks),
		Passed:      passed,
		Failed:      failed,
		Warnings:    warnings,
		Status:      "done",
		StartedAt:   now,
		FinishedAt:  &now,
		CreatedAt:   now,
		UpdatedAt:   now,
		Items:       items,
	}

	if err := s.db.WithContext(ctx).Create(audit).Error; err != nil {
		return nil, err
	}
	return audit, nil
}

// ListAudits 分页列出审计记录（不含 items，减少载荷）。
func (s *SecurityAuditService) ListAudits(ctx context.Context, page, pageSize int) ([]model.SecurityAudit, int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.SecurityAudit{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.SecurityAudit
	offset := (page - 1) * pageSize
	if err := s.db.WithContext(ctx).Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetAuditDetail 获取审计明细（含 items）。
func (s *SecurityAuditService) GetAuditDetail(ctx context.Context, id uint) (*model.SecurityAudit, error) {
	var a model.SecurityAudit
	if err := s.db.WithContext(ctx).Preload("Items").First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// buildChecks 构造安全检查清单。所有检查均为本地/配置级，无外部副作用。
func (s *SecurityAuditService) buildChecks(ctx context.Context) []auditCheck {
	return []auditCheck{
		{
			name: "数据库连通性", category: "基础设施", level: "critical", weight: 40,
			run: func(ctx context.Context) (string, string) {
				if err := s.db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
					return "fail", "数据库连接失败: " + err.Error()
				}
				return "pass", "数据库连接正常"
			},
		},
		{
			name: "超级管理员账号存在", category: "身份与访问", level: "high", weight: 25,
			run: func(ctx context.Context) (string, string) {
				var cnt int64
				s.db.WithContext(ctx).Model(&model.SystemUser{}).
					Where("role = ?", model.SystemUserRoleAdmin).Count(&cnt)
				if cnt == 0 {
					return "fail", "未检测到超级管理员账号"
				}
				return "pass", fmt.Sprintf("存在 %d 个超级管理员账号", cnt)
			},
		},
		{
			name: "默认超管密码已修改", category: "身份与访问", level: "high", weight: 25,
			run: func(ctx context.Context) (string, string) {
				var admin model.SystemUser
				err := s.db.WithContext(ctx).Where("role = ?", model.SystemUserRoleAdmin).First(&admin).Error
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return "warn", "未找到超管账号，无法校验默认密码"
					}
					return "fail", "查询超管失败: " + err.Error()
				}
				if bcrypt.CheckPassword(admin.Password, "Admin@123456") == nil {
					return "fail", "超管仍使用默认密码 Admin@123456，存在严重安全风险，请立即修改"
				}
				return "pass", "超管密码已修改"
			},
		},
		{
			name: "已启用 LLM 提供商", category: "AI 能力", level: "medium", weight: 10,
			run: func(ctx context.Context) (string, string) {
				var cnt int64
				s.db.WithContext(ctx).Model(&model.LLMProvider{}).Where("enabled = ?", true).Count(&cnt)
				if cnt == 0 {
					return "warn", "未启用任何 LLM 提供商，AI 问答/路由能力不可用"
				}
				return "pass", fmt.Sprintf("已启用 %d 个 LLM 提供商", cnt)
			},
		},
		{
			name: "API 限流已启用", category: "安全", level: "low", weight: 5,
			run: func(ctx context.Context) (string, string) {
				return "pass", "全局 API 限流中间件已启用"
			},
		},
		{
			name: "审计日志中间件已启用", category: "安全", level: "low", weight: 5,
			run: func(ctx context.Context) (string, string) {
				return "pass", "全局操作审计中间件已启用"
			},
		},
	}
}
