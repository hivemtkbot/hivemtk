package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TeamUserStatus 用户状态
type TeamUserStatus int

const (
	TeamUserStatusInactive TeamUserStatus = 0 // 禁用
	TeamUserStatusActive   TeamUserStatus = 1 // 启用
)

// TeamDataScope 团队用户数据范围（A 域 P1-4）
//
// 与 team_user.data_scope 列对应：1=全部 2=本部门 3=本人 4=自定义
const (
	TeamDataScopeAll        TeamUserStatus = 1 // 全部
	TeamDataScopeDepartment TeamUserStatus = 2 // 本部门
	TeamDataScopeSelf       TeamUserStatus = 3 // 本人（默认值）
	TeamDataScopeCustom     TeamUserStatus = 4 // 自定义（custom_dept_ids 白名单）
)

// TeamUser 团队用户模型
type TeamUser struct {
	ID            uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Username      string         `gorm:"type:varchar(50);not null" json:"username"`
	Password      string         `gorm:"type:varchar(255);not null" json:"-"`
	Name          string         `gorm:"type:varchar(50)" json:"name"`
	Email         string         `gorm:"type:varchar(100)" json:"email"`
	Phone         string         `gorm:"type:varchar(20)" json:"phone"`
	Avatar        string         `gorm:"type:varchar(255)" json:"avatar"`
	Role          string         `gorm:"type:varchar(20);default:'viewer'" json:"role"` // admin, manager, viewer
	Status        TeamUserStatus `gorm:"default:1" json:"status"`
	DataScope     int            `gorm:"type:smallint;default:3" json:"data_scope"`  // A 域 P1-4：数据范围 1=全部 2=本部门 3=本人 4=自定义
	DepartmentID  uint           `gorm:"index;default:0" json:"department_id"`       // A 域 P1-4：所属部门 ID（0=未分配）
	TeamID        uint           `gorm:"index;default:0" json:"team_id"`             // A 域 P1-4：所属团队 ID（0=未分配）
	CustomDeptIDs string         `gorm:"type:text" json:"custom_dept_ids,omitempty"` // A 域 P1-4：data_scope=4 时的部门白名单（JSON 数组）
	LastLoginAt   *time.Time     `json:"last_login_at"`
	LastLoginIP   string         `gorm:"type:varchar(50)" json:"last_login_ip"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (TeamUser) TableName() string {
	return "team_users"
}

// BeforeCreate 创建前钩子
func (u *TeamUser) BeforeCreate(tx *gorm.DB) error {
	return nil
}

// TeamRole 团队角色模型
type TeamRole struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Code        string    `gorm:"type:varchar(20);not null" json:"code"` // admin, manager, viewer, custom
	Name        string    `gorm:"type:varchar(50);not null" json:"name"`
	Permissions string    `gorm:"type:text" json:"permissions"` // JSON 格式存储权限列表
	IsSystem    bool      `gorm:"default:false" json:"is_system"`
	Status      int       `gorm:"default:1" json:"status"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (TeamRole) TableName() string {
	return "team_roles"
}

// BeforeCreate 创建前钩子
func (r *TeamRole) BeforeCreate(tx *gorm.DB) error {
	return nil
}

// OperationLog 操作日志模型
type OperationLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	Username   string    `gorm:"type:varchar(50)" json:"username"`
	Action     string    `gorm:"type:varchar(50);not null" json:"action"` // create, update, delete, login, logout, anomaly_login_detected, etc.
	Module     string    `gorm:"type:varchar(50);not null" json:"module"` // user, role, card, shortlink, etc.
	Resource   string    `gorm:"type:varchar(50)" json:"resource"`        // 资源类型
	ResourceID string    `gorm:"type:varchar(50)" json:"resource_id"`     // 资源ID
	Detail     string    `gorm:"type:text" json:"detail"`                 // 操作详情 JSON
	OldValue   string    `gorm:"type:text" json:"old_value"`              // 旧值 JSON
	NewValue   string    `gorm:"type:text" json:"new_value"`              // 新值 JSON
	IP         string    `gorm:"type:varchar(50)" json:"ip"`
	UserAgent  string    `gorm:"type:varchar(255)" json:"user_agent"`
	CreatedAt  time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (OperationLog) TableName() string {
	return "operation_logs"
}

// 系统预定义角色
var SystemRoles = []TeamRole{
	{Code: "admin", Name: "管理员", Permissions: `["*"]`, IsSystem: true},
	{Code: "manager", Name: "运营经理", Permissions: `["cards.*","shortlinks.*","clues.*","autoreply.*"]`, IsSystem: true},
	{Code: "viewer", Name: "查看者", Permissions: `["cards.view","shortlinks.view","clues.view"]`, IsSystem: true},
}

// 系统预定义权限
var SystemPermissions = map[string]string{
	// 用户管理
	"users.view":   "查看用户列表",
	"users.create": "创建用户",
	"users.update": "编辑用户",
	"users.delete": "删除用户",

	// 角色管理
	"roles.view":   "查看角色列表",
	"roles.create": "创建角色",
	"roles.update": "编辑角色",
	"roles.delete": "删除角色",

	// 卡片管理
	"cards.view":   "查看卡片",
	"cards.create": "创建卡片",
	"cards.update": "编辑卡片",
	"cards.delete": "删除卡片",
	"cards.*":      "卡片全部权限",

	// 短链管理
	"shortlinks.view":   "查看短链",
	"shortlinks.create": "创建短链",
	"shortlinks.update": "编辑短链",
	"shortlinks.delete": "删除短链",
	"shortlinks.*":      "短链全部权限",

	// 活码管理
	"livecodes.view":   "查看活码",
	"livecodes.create": "创建活码",
	"livecodes.update": "编辑活码",
	"livecodes.delete": "删除活码",
	"livecodes.*":      "活码全部权限",

	// 线索管理
	"clues.view":   "查看线索",
	"clues.create": "创建线索",
	"clues.update": "编辑线索",
	"clues.delete": "删除线索",
	"clues.*":      "线索全部权限",

	// 自动回复
	"autoreply.view":   "查看自动回复",
	"autoreply.create": "创建自动回复规则",
	"autoreply.update": "编辑自动回复规则",
	"autoreply.delete": "删除自动回复规则",
	"autoreply.*":      "自动回复全部权限",

	// 系统设置
	"system.config": "系统配置",
	"system.logs":   "查看日志",
	"system.backup": "备份管理",

	// 全部权限
	"*": "全部权限",
}

// GenerateUUID 生成UUID
func GenerateUUID() string {
	return uuid.New().String()
}
