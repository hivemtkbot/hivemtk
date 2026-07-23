package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SystemUserRole 系统用户角色常量（P1-5 文档化：详见 docs/architecture/DUAL_ROLE_MODEL.md）
//
// 私域独立部署下，SystemUser 仅有两种角色：
//   - admin：部署超管，由 system_init 初始化向导创建，每实例唯一
//   - user ：系统普通用户（保留值，扩展位）
//
// 注意：SystemUser.admin 与 TeamUser.admin 是不同实体的角色，权限层级不同。
const (
	SystemUserRoleAdmin = "admin"
	SystemUserRoleUser  = "user"
)

// IsValidSystemUserRole 校验系统用户角色是否合法
func IsValidSystemUserRole(role string) bool {
	return role == SystemUserRoleAdmin || role == SystemUserRoleUser
}

// DataScope 数据范围枚举（P1-4 行级权限）
//
// 控制用户能查询到的数据范围：
//   - all        : 全部数据（仅 admin）
//   - department : 本部门数据
//   - team       : 本团队数据
//   - self       : 仅自己创建的数据
const (
	DataScopeAll        = "all"
	DataScopeDepartment = "department"
	DataScopeTeam       = "team"
	DataScopeSelf       = "self"
)

// IsValidDataScope 校验数据范围是否合法
func IsValidDataScope(scope string) bool {
	return scope == DataScopeAll ||
		scope == DataScopeDepartment ||
		scope == DataScopeTeam ||
		scope == DataScopeSelf
}

// DefaultDataScopeForRole 根据角色返回默认数据范围
// admin → all, 其他 → self
func DefaultDataScopeForRole(role string) string {
	if role == SystemUserRoleAdmin {
		return DataScopeAll
	}
	return DataScopeSelf
}

// SystemUser 系统用户模型
type SystemUser struct {
	ID                 uint       `json:"id" gorm:"primaryKey"`
	Username           string     `json:"username" gorm:"size:50;uniqueIndex;not null"`     // 用户名
	Password           string     `json:"-" gorm:"size:100;not null"`                     // 密码，不在JSON中返回
	Email              string     `json:"email" gorm:"size:100"`                          // 邮箱
	Phone              string     `json:"phone" gorm:"size:20;index"`                     // 手机号（P2-X：用于 Profile 资料维护 + 短信验证）
	RealName           string     `json:"real_name" gorm:"size:50"`                       // 真实姓名
	Role               string     `json:"role" gorm:"size:20;default:'staff'"`            // 角色：admin(超管) / customer_service(客服) / staff(员工)
	Status             int        `json:"status" gorm:"default:1"`                        // 状态：1-启用，0-禁用（审计保留）
	Enabled            bool       `json:"enabled" gorm:"column:enabled;default:true;not null"` // 启用/禁用开关（阶段 1 新增）
	LastLogin          *time.Time `json:"last_login"`                                       // 最后登录时间
	MustChangePassword bool       `json:"must_change_password" gorm:"default:false"`        // 是否必须修改密码（首次登录）
	DataScope          string     `json:"data_scope" gorm:"size:20;default:'self'"`       // P1-4：数据范围 all/department/team/self
	DepartmentID       uint       `json:"department_id" gorm:"index;default:0"`           // P1-4：所属部门 ID（0=未分配）
	TeamID             uint       `json:"team_id" gorm:"index;default:0"`                 // P1-4：所属团队 ID（0=未分配）
	CreatedAt          time.Time  `json:"created_at" gorm:"autoCreateTime"`               // 创建时间
	UpdatedAt          time.Time  `json:"updated_at" gorm:"autoUpdateTime"`               // 更新时间
}

// TableName 返回表名
func (SystemUser) TableName() string {
	return "system_users"
}

// BeforeCreate GORM钩子，在创建前加密密码
func (u *SystemUser) BeforeCreate(tx *gorm.DB) error {
	// P1-4：未显式指定 data_scope 时，根据 role 设置默认值
	if u.DataScope == "" {
		u.DataScope = DefaultDataScopeForRole(u.Role)
	}
	return HashSystemUserPassword(u)
}

// HashSystemUserPassword 加密密码（包级函数，避免 model 上挂业务方法）
func HashSystemUserPassword(u *SystemUser) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

// CheckSystemUserPassword 验证密码（包级函数）
func CheckSystemUserPassword(u *SystemUser, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// IsSystemUserAdmin 检查是否为管理员（P1-5：使用角色常量而非硬编码）
func IsSystemUserAdmin(u *SystemUser) bool {
	return u.Role == SystemUserRoleAdmin
}
