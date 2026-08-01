package llm

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
)

// providerDB 返回用于 provider 持久化的 *gorm.DB：
// 优先用注入的 auditDB，回退到全局 db.GetDB()，
// 避免 auditDB 未注入时静默失败（与审计日志一致：db 未就绪时跳过）。
func providerDB() *gorm.DB {
	if g := getAuditDB(); g != nil {
		return g
	}
	return db.GetDB()
}

// LoadProvidersFromDB 启动时从 llm_providers 表加载所有 provider 到内存路由表。
// DB 定义覆盖同名的 config 占位 provider（如 deepseek 在库里有真实密钥则启用）。
func (d *Dispatcher) LoadProvidersFromDB() error {
	g := providerDB()
	if g == nil {
		logger.Warnf("[LLM] LoadProvidersFromDB: db 未就绪，跳过从库加载 provider")
		return nil
	}
	var rows []model.LLMProvider
	if err := g.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		d.AddProvider(rowToProviderConfig(&rows[i]))
	}
	logger.Infof("[LLM] LoadProvidersFromDB: 从 llm_providers 加载 %d 个 provider", len(rows))
	return nil
}

// UpsertProviderToDB 写入/更新单个 provider 到 llm_providers（Name 为唯一键）。
// 直接镜像内存态 pc（含 APIKey）：与 dispatcher 内存保持一致。
// 清空密钥由调用方通过 APIKeyClearSentinel 解析后传 "" 实现。
func (d *Dispatcher) UpsertProviderToDB(pc ProviderConfig) error {
	g := providerDB()
	if g == nil {
		return nil
	}
	row := providerConfigToRow(pc)
	row.UpdatedAt = time.Now()

	var exist model.LLMProvider
	err := g.Where("name = ?", pc.Name).First(&exist).Error
	if err == gorm.ErrRecordNotFound {
		if err := g.Create(&row).Error; err != nil {
			logger.Errorf("[LLM] UpsertProviderToDB 新建 %s 失败: %v", pc.Name, err)
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	row.ID = exist.ID
	row.CreatedAt = exist.CreatedAt
	if err := g.Save(&row).Error; err != nil {
		logger.Errorf("[LLM] UpsertProviderToDB 更新 %s 失败: %v", pc.Name, err)
		return err
	}
	return nil
}

// DeleteProviderFromDB 从 llm_providers 删除指定 provider。
func (d *Dispatcher) DeleteProviderFromDB(name string) error {
	g := providerDB()
	if g == nil {
		return nil
	}
	if err := g.Where("name = ?", name).Delete(&model.LLMProvider{}).Error; err != nil {
		logger.Errorf("[LLM] DeleteProviderFromDB %s 失败: %v", name, err)
		return err
	}
	return nil
}

// providerConfigToRow 将内存配置转为 DB 行
func providerConfigToRow(pc ProviderConfig) model.LLMProvider {
	return model.LLMProvider{
		Name:         pc.Name,
		DisplayName:  pc.DisplayName,
		BaseURL:      pc.BaseURL,
		Model:        pc.Model,
		APIKey:       pc.APIKey,
		APIType:      pc.APIType,
		Enabled:      pc.Enabled,
		QualityScore: pc.QualityScore,
		MaxRPM:       pc.MaxRPM,
		MaxTPM:       pc.MaxTPM,
		CostPer1k:    pc.CostPer1k,
		AvgLatencyMs: pc.AvgLatencyMs,
		NoFC:         pc.NoFC,
		Vendor:       pc.Vendor,
		Tags:         tagsToText(pc.Tags),
	}
}

// rowToProviderConfig 将 DB 行转为内存配置
func rowToProviderConfig(row *model.LLMProvider) ProviderConfig {
	return ProviderConfig{
		Name:         row.Name,
		DisplayName:  row.DisplayName,
		APIKey:       row.APIKey,
		BaseURL:      row.BaseURL,
		Model:        row.Model,
		APIType:      row.APIType,
		Enabled:      row.Enabled,
		QualityScore: row.QualityScore,
		MaxRPM:       row.MaxRPM,
		MaxTPM:       row.MaxTPM,
		CostPer1k:    row.CostPer1k,
		AvgLatencyMs: row.AvgLatencyMs,
		NoFC:         row.NoFC,
		Vendor:       row.Vendor,
		Tags:         tagsFromText(row.Tags),
	}
}

// tagsToText 标签切片序列化为 JSON 文本
func tagsToText(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// tagsFromText 解析标签文本，兼容 JSON 数组与逗号分隔旧格式
func tagsFromText(s string) []string {
	if s == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(s), &tags); err == nil {
		return tags
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
