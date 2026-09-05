package service

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
	Scanned int `json:"scanned"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
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

func templateToLibrary(t *contentmodel.ScriptTemplate) *mainmodel.ScriptLibrary {
	return &mainmodel.ScriptLibrary{
		Category: t.Category,
		Title:    t.Title,
		Content:  t.Content,
		Scenario: t.JourneyStage,
		Tags:     parseTagsToJSONArray(t.Tags),
	}
}

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
