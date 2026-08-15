package portcontract

import (
	"errors"

	"hivemtk-user/internal/model"
)


// ErrCustomerNotFound 客户不存在错误（sentinel）
//
// 工具层（tooluse）依赖此 sentinel 区分"客户不存在"与其他错误，
// 不再 import service 包。CustomerPort 实现方在底层 service 返回
// service.ErrCustomerNotFound 时应将其映射为本 sentinel。
var ErrCustomerNotFound = errors.New("客户不存在")

// CustomerIdentity 创建/更新客户的身份字段投影。
//
// 设计动机：与历史 service.CustomerDTO 字段对齐，工具层无需 import service 包
// 也能传入最小可识别身份子集（手机/邮箱/各平台 openID）。
type CustomerIdentity struct {
	Phone         string
	Email         string
	WechatOpenID  string
	DouyinOpenID  string
	XiaohongshuID string
}

// CustomerProfileView 客户 360° 视图（工具层投影）。
//
// 字段含义：
//   - Customer：客户主表记录
//   - RecentEvents：最近 N 条客户行为事件（用于上下文）
//   - Tags：长期画像标签
type CustomerProfileView struct {
	Customer     *model.Customer        `json:"customer"`
	RecentEvents []*model.CustomerEvent `json:"recent_events,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
}

// CustomerPort 客户领域能力端口。
//
// 实现方：service.CustomerService（见 service/tool_ports_adapter.go:CustomerPortAdapter）
// 消费方：tooluse/customer_tools.go 等客户相关工具
type CustomerPort interface {
	GetCustomerProfile(customerID string) (*CustomerProfileView, error)
	CreateOrUpdate(identity *CustomerIdentity) (*model.Customer, error)
	MergeCustomers(primaryID, secondaryID string) error
	AddTags(customerID string, tags []string) error
	RemoveTags(customerID string, tags []string) error
}

