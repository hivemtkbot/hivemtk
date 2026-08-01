package model

import "time"

// LLMRoutingRule 持久化场景路由规则。
//
// 此前场景路由(ScenarioRoute)只存在于内存，由代码 seed(registerLocalFirstRoutes) 生成，
// 运营在后台「LLM路由」页面做的配置(主 provider / 兜底链 / 灰度权重 / canary)重启即丢，
// 且多实例各持一份导致行为分裂。
//
// 改造后：通过 UpdateStrategies(API) 调用 dispatcher.SetRouteWithAudit 时，
// 路由本体经 UpsertRouteToDB 落库到此表；user-server 启动时 LoadRoutesFromDB 覆盖内存种子，
// 实现「可视化配置 → 落库 → 容器重启不丢、多实例一致」的闭环。
//
// 路由以 JSON 整存(route_json)，天然兼容 ScenarioRoute 的嵌套字段(CanaryRoute)与后续扩展，
// scenario 为全局唯一键，与 dispatcher.DispatchScenario 一一对应。
type LLMRoutingRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Scenario  string    `gorm:"column:scenario;uniqueIndex;size:64" json:"scenario"` // intent / sop / objection ...
	RouteJSON string    `gorm:"column:route_json;type:text" json:"route_json"`        // 完整 ScenarioRoute JSON
	Version   int       `gorm:"column:version;default:1" json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (LLMRoutingRule) TableName() string { return "llm_routing_rules" }
