package response

// 错误消息常量
// 用于统一 API 响应中的错误消息文本
const (
	// 通用错误
	ErrSuccess          = "操作成功"
	ErrInvalidParams    = "无效的请求参数"
	ErrInvalidIDFormat  = "无效的 ID 格式"
	ErrIDMismatch       = "ID 不一致"
	ErrResourceNotFound = "资源不存在"
	ErrInternalError    = "服务器内部错误"

	// 创建操作
	ErrCreateFailed  = "创建失败"
	ErrCreateSuccess = "创建成功"

	// 更新操作
	ErrUpdateFailed  = "更新失败"
	ErrUpdateSuccess = "更新成功"

	// 删除操作
	ErrDeleteFailed  = "删除失败"
	ErrDeleteSuccess = "删除成功"

	// 获取操作
	ErrGetFailed     = "获取失败"
	ErrGetSuccess    = "获取成功"
	ErrGetListFailed = "获取列表失败"

	// 认证相关
	ErrUnauthorized     = "未授权访问"
	ErrTokenInvalid     = "无效的令牌"
	ErrTokenExpired     = "令牌已过期"
	ErrPermissionDenied = "权限不足"

	// 业务相关
	ErrBusinessError = "业务处理失败"
)

// 卡片管理相关错误消息
const (
	ErrCardNotFound     = "卡片不存在"
	ErrCardCreateFailed = "创建卡片失败"
	ErrCardUpdateFailed = "更新卡片失败"
	ErrCardDeleteFailed = "删除卡片失败"
)

// 短链管理相关错误消息
const (
	ErrShortLinkNotFound     = "短链不存在"
	ErrShortLinkCreateFailed = "创建短链失败"
	ErrShortLinkUpdateFailed = "更新短链失败"
	ErrShortLinkDeleteFailed = "删除短链失败"
)

// 自动回复相关错误消息
const (
	ErrAutoReplyNotFound     = "自动回复规则不存在"
	ErrAutoReplyCreateFailed = "创建自动回复失败"
	ErrAutoReplyUpdateFailed = "更新自动回复失败"
	ErrAutoReplyDeleteFailed = "删除自动回复失败"
)

// 用户管理相关错误消息
const (
	ErrUserNotFound       = "用户不存在"
	ErrUserCreateFailed   = "创建用户失败"
	ErrUserUpdateFailed   = "更新用户失败"
	ErrUserDeleteFailed   = "删除用户失败"
	ErrUserAlreadyExists  = "用户已存在"
	ErrInvalidCredentials = "用户名或密码错误"
)

// 系统配置相关错误消息
const (
	ErrConfigNotFound   = "配置不存在"
	ErrConfigSaveFailed = "保存配置失败"

	// 初始化相关
	ErrSystemAlreadyInitialized = "系统已初始化，禁止重复创建超管"
)
