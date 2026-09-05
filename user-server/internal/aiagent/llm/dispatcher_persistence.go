package llm

import (
	"encoding/json"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/secrets"

	"gorm.io/gorm"
)

func providerDB() *gorm.DB {
	if g := getAuditDB(); g != nil {
		return g
	}
	return db.GetDB()
}

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

	migrated := 0
	if secrets.Ready() {
		for i := range rows {
			if rows[i].APIKey != "" && !secrets.IsCiphertextFormat(rows[i].APIKey) {
				rows[i].APIKey = encryptAPIKeyForStorage(rows[i].APIKey)
				if err := g.Model(&model.LLMProvider{}).Where("id = ?", rows[i].ID).
					Update("api_key", rows[i].APIKey).Error; err == nil {
					migrated++
				} else {
					logger.Warnf("[LLM] api key 迁移加密失败 provider=%s: %v", rows[i].Name, err)
				}
			}
		}
		if migrated > 0 {
			logger.Infof("[LLM] api key 存量迁移：%d 个 provider 的明文密钥已加密", migrated)
		}
	}
	for i := range rows {
		cfg := rowToProviderConfig(&rows[i])
		cfg.APIKey = decryptAPIKeyForUse(cfg.APIKey)
		d.AddProvider(cfg)
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

// LoadRoutesFromDB 启动时从 llm_routing_rules 表加载路由，覆盖内存种子。
//
// 与 LoadProvidersFromDB 同理：DB 中的路由定义作为「源真相」覆盖代码 seed，
// 保证运营在后台配置的路由(主 provider / 兜底 / 灰度 / canary)重启不丢、多实例一致。
// 若表为空(全新部署)，内存保持代码种子，行为不变。
func (d *Dispatcher) LoadRoutesFromDB() error {
	g := providerDB()
	if g == nil {
		logger.Warnf("[LLM] LoadRoutesFromDB: db 未就绪，跳过路由加载")
		return nil
	}
	var rows []model.LLMRoutingRule
	if err := g.Find(&rows).Error; err != nil {
		logger.Errorf("[LLM] LoadRoutesFromDB 查询失败: %v", err)
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range rows {
		var r ScenarioRoute
		if err := json.Unmarshal([]byte(rows[i].RouteJSON), &r); err != nil {
			logger.Errorf("[LLM] LoadRoutesFromDB: 解析 scenario=%s 失败: %v", rows[i].Scenario, err)
			continue
		}
		d.routes[r.Scenario] = &r
	}
	logger.Infof("[LLM] LoadRoutesFromDB: 从 llm_routing_rules 加载 %d 条路由覆盖种子", len(rows))
	return nil
}

// UpsertRouteToDB 将路由规则 upsert 到 llm_routing_rules 表。
// db 未就绪时静默跳过(内存已生效)，仅记录日志，不阻断主流程。
func (d *Dispatcher) UpsertRouteToDB(r ScenarioRoute) error {
	g := providerDB()
	if g == nil {
		return nil
	}
	buf, err := json.Marshal(r)
	if err != nil {
		logger.Errorf("[LLM] UpsertRouteToDB 序列化 scenario=%s 失败: %v", r.Scenario, err)
		return err
	}
	row := model.LLMRoutingRule{
		Scenario:  string(r.Scenario),
		RouteJSON: string(buf),
		Version:   r.Version,
		UpdatedAt: time.Now(),
	}
	var exist model.LLMRoutingRule
	q := g.Where("scenario = ?", string(r.Scenario)).First(&exist)
	if q.Error == gorm.ErrRecordNotFound {
		row.CreatedAt = time.Now()
		if err := g.Create(&row).Error; err != nil {
			logger.Errorf("[LLM] UpsertRouteToDB 新建 scenario=%s 失败: %v", r.Scenario, err)
			return err
		}
		return nil
	}
	if q.Error != nil {
		logger.Errorf("[LLM] UpsertRouteToDB 查询 scenario=%s 失败: %v", r.Scenario, q.Error)
		return q.Error
	}
	row.ID = exist.ID
	row.CreatedAt = exist.CreatedAt
	if err := g.Save(&row).Error; err != nil {
		logger.Errorf("[LLM] UpsertRouteToDB 更新 scenario=%s 失败: %v", r.Scenario, err)
		return err
	}
	return nil
}

// DeleteRouteFromDB 从数据库删除某场景路由定义。
func (d *Dispatcher) DeleteRouteFromDB(scenario DispatchScenario) error {
	g := providerDB()
	if g == nil {
		return nil
	}
	if err := g.Where("scenario = ?", string(scenario)).Delete(&model.LLMRoutingRule{}).Error; err != nil {
		logger.Errorf("[LLM] DeleteRouteFromDB scenario=%s 失败: %v", scenario, err)
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

func providerConfigToRow(pc ProviderConfig) model.LLMProvider {
	return model.LLMProvider{
		Name:         pc.Name,
		DisplayName:  pc.DisplayName,
		BaseURL:      pc.BaseURL,
		Model:        pc.Model,
		APIKey:       encryptAPIKeyForStorage(pc.APIKey),
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

func tagsToText(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

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
