package asset

import "time"

// AssetType 5 类资产
type AssetType string

const (
	AssetTypeAgentPersona  AssetType = "agent_persona"
	AssetTypeSalesScript   AssetType = "sales_script"
	AssetTypeABTestPlan    AssetType = "ab_test_plan"
	AssetTypeMarketingFlow AssetType = "marketing_workflow"
	AssetTypeIndustrySOP   AssetType = "industry_sop"
)

func (t AssetType) Valid() bool {
	switch t {
	case AssetTypeAgentPersona, AssetTypeSalesScript,
		AssetTypeABTestPlan, AssetTypeMarketingFlow, AssetTypeIndustrySOP:
		return true
	}
	return false
}

func (t AssetType) Label() string {
	return map[AssetType]string{
		AssetTypeAgentPersona:  "智能体角色",
		AssetTypeSalesScript:   "销冠话术",
		AssetTypeABTestPlan:    "AB 测试方案",
		AssetTypeMarketingFlow: "自动化工作流",
		AssetTypeIndustrySOP:   "行业 SOP 模板",
	}[t]
}

// Industry 行业（原 5 行业 + 7 行业资产包扩展）
type Industry string

const (
	IndustryMeiZhuang Industry = "美妆"
	IndustryJiaoPei   Industry = "教培"
	IndustryYiMei     Industry = "医美"
	IndustryQiChe     Industry = "汽车"
	IndustryJinRong   Industry = "金融"
	IndustryECig      Industry = "电子烟"
	IndustryAdult     Industry = "成人用品"
	IndustrySexHealth Industry = "两性健康"
	IndustryCarRent   Industry = "租车"
	IndustryHomestay  Industry = "民宿"
	IndustryFreight   Industry = "货代"
	IndustryImmigra   Industry = "移民"
)

func (i Industry) Valid() bool {
	switch i {
	case IndustryMeiZhuang, IndustryJiaoPei, IndustryYiMei, IndustryQiChe, IndustryJinRong,
		IndustryECig, IndustryAdult, IndustrySexHealth, IndustryCarRent,
		IndustryHomestay, IndustryFreight, IndustryImmigra:
		return true
	}
	return false
}

// AssetSource 来源
type AssetSource string

const (
	SourcePurchased AssetSource = "purchased"
	SourceManual    AssetSource = "manual"
	SourceSynced    AssetSource = "synced"
	SourceImported  AssetSource = "imported"
)

func (s AssetSource) Label() string {
	return map[AssetSource]string{
		SourcePurchased: "平台购买",
		SourceManual:    "自建",
		SourceSynced:    "平台分发",
		SourceImported:  "导入",
	}[s]
}

func (s AssetSource) IsFromPlatform() bool {
	return s == SourcePurchased || s == SourceSynced
}

// Asset 领域实体
type Asset struct {
	ID          int64
	AssetID     string
	AssetType   AssetType
	Industry    Industry
	Name        string
	Version     string
	Source      AssetSource
	IsActive    bool
	Data        []byte
	PurchaseID  *int64
	PurchasedAt *time.Time
	SyncedAt    time.Time
	UpdatedAt   time.Time
}

