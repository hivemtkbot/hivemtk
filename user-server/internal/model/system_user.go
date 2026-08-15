package model

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SystemUserRole 系统用户角色常量（文档化：详见 docs/architecture/DUAL_ROLE_MODEL.md）
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

// DataScope 数据范围枚举（行级权限）
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
	ID           uint       `json:"id" gorm:"primaryKey"`
	Username     string     `json:"username" gorm:"size:50;uniqueIndex;not null"`        
	Password     string     `json:"-" gorm:"size:100;not null"`                          
	Email        string     `json:"email" gorm:"size:100"`                               
	Phone        string     `json:"phone" gorm:"size:20;index"`                          
	RealName     string     `json:"real_name" gorm:"size:50"`                            
	Role         string     `json:"role" gorm:"size:20;default:'staff'"`                 
	Status       int        `json:"status" gorm:"default:1"`                             
	Enabled      bool       `json:"enabled" gorm:"column:enabled;default:true;not null"` 
	LastLogin    *time.Time `json:"last_login"`                                          
	DataScope    string     `json:"data_scope" gorm:"size:20;default:'self'"`            
	DepartmentID uint       `json:"department_id" gorm:"index;default:0"`                
	TeamID       uint       `json:"team_id" gorm:"index;default:0"`                      
	CreatedAt    time.Time  `json:"created_at" gorm:"autoCreateTime"`                    
	UpdatedAt    time.Time  `json:"updated_at" gorm:"autoUpdateTime"`                    
}

// TableName 返回表名
func (SystemUser) TableName() string {
	return "system_users"
}

// BeforeCreate GORM钩子，在创建前加密密码
func (u *SystemUser) BeforeCreate(tx *gorm.DB) error {
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

// IsSystemUserAdmin 检查是否为管理员（：使用角色常量而非硬编码）
func IsSystemUserAdmin(u *SystemUser) bool {
	return u.Role == SystemUserRoleAdmin
}

