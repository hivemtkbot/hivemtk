package service

// script_template_sync.go T-5 双体系桥接：管理端模板库 → 运行时话术库同步任务。
//
// 定位（详见 script_template.go 头注释）：
//   - content/script_templates = 管理端 CRUD 模板库（单一内容生产源）
//   - model.ScriptLibrary      = 运行时引擎话术库（objection_handler/SalesEngine 检索）
//
// 同步语义：按 (category, title) 幂等 upsert —— 已存在则更新内容字段，
// 不存在则创建；usage/转化等运行时统计只存在于 library 侧，同步不会覆盖。

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	contentmodel "hivemtk-user/internal/content/model"
	"hivemtk-user/internal/content/repository"
	mainmodel "hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// ScriptTemplateSyncService 话术模板 → 运行时话术库 同步服务
type ScriptTemplateSyncService struct {
	templateRepo *repository.ScriptTemplateRepository
	db           *gorm.DB
}

// NewScriptTemplateSyncService 创建同步服务（使用全局 DB）
func NewScriptTemplateSyncService() *ScriptTemplateSyncService {
	return &ScriptTemplateSyncService{
		templateRepo: repository.NewScriptTemplateRepository(),
		db:           _db.GetDB(),
	}
}

// NewScriptTemplateSyncServiceWithDB 测试/内嵌注入用构造器
func NewScriptTemplateSyncServiceWithDB(db *gorm.DB, templateRepo *repository.ScriptTemplateRepository) *ScriptTemplateSyncService {
	return &ScriptTemplateSyncService{templateRepo: templateRepo, db: db}
}

// SyncToLibraryStats 同步结果统计
type SyncToLibraryStats struct {
	Scanned int `json:"scanned"` // 读取的管理端模板总数
	Created int `json:"created"` // 新建到 script_library 的条数
	Updated int `json:"updated"` // 更新的条数
	Skipped int `json:"skipped"` // 跳过（title/content 为空）的条数
}

// SyncToLibrary 执行 template → library 单向同步（幂等，可重复执行）。
//
// 读取管理端全部话术模板，逐条按 (category, title) upsert 到 script_library：
//   - 未命中：创建（Tags 由逗号分隔字符串转 JSONArray，Scenario 取 JourneyStage）
//   - 已命中：更新 Content/Scenario/Tags，保留 library 侧 usage_count 等运行时统计
func (s *ScriptTemplateSyncService) SyncToLibrary(ctx context.Context) (*SyncToLibraryStats, error) {
	if s == nil || s.db == nil || s.templateRepo == nil {
		return nil, errors.New("script template sync: service not initialized")
	}
	templates, _, err := s.templateRepo.GetAll("", 1, 1<<20)
	if err != nil {
		return nil, err
	}

	stats := &SyncToLibraryStats{Scanned: len(templates)}
	for _, t := range templates {
		if strings.TrimSpace(t.Title) == "" || strings.TrimSpace(t.Content) == "" {
			stats.Skipped++
			continue
		}
		created, err := s.upsertOne(t)
		if err != nil {
			return stats, err
		}
		if created {
			stats.Created++
		} else {
			stats.Updated++
		}
	}
	return stats, nil
}

// upsertOne 按 (category, title) 幂等 upsert 单条；返回是否新建
func (s *ScriptTemplateSyncService) upsertOne(t *contentmodel.ScriptTemplate) (bool, error) {
	var lib mainmodel.ScriptLibrary
	err := s.db.Where("category = ? AND title = ?", t.Category, t.Title).First(&lib).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		rec := templateToLibrary(t)
		return true, s.db.Create(rec).Error
	case err != nil:
		return false, err
	default:
		lib.Content = t.Content
		lib.Scenario = t.JourneyStage
		lib.Tags = parseTagsToJSONArray(t.Tags)
		return false, s.db.Save(&lib).Error
	}
}

// templateToLibrary 管理端模板 → 运行时话术库 记录映射（不含运行时统计字段）
func templateToLibrary(t *contentmodel.ScriptTemplate) *mainmodel.ScriptLibrary {
	return &mainmodel.ScriptLibrary{
		Category: t.Category,
		Title:    t.Title,
		Content:  t.Content,
		Scenario: t.JourneyStage,
		Tags:     parseTagsToJSONArray(t.Tags),
	}
}

// parseTagsToJSONArray 管理端 Tags（逗号分隔字符串）→ ScriptLibrary.JSONArray
func parseTagsToJSONArray(s string) mainmodel.JSONArray {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	})
	arr := make(mainmodel.JSONArray, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			arr = append(arr, p)
		}
	}
	return arr
}
