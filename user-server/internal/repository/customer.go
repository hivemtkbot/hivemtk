package repository

import (
	"context"
	"fmt"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
)

// CustomerRepository defines the interface for customer data access
type CustomerRepository interface {
	Create(ctx context.Context, customer *model.Customer) error
	GetByID(ctx context.Context, id string) (*model.Customer, error)
	GetByUnifiedID(ctx context.Context, unifiedID string) (*model.Customer, error)
	GetByPhone(ctx context.Context, phone string) (*model.Customer, error)
	GetByEmail(ctx context.Context, email string) (*model.Customer, error)
	GetByWechatOpenID(ctx context.Context, openID string) (*model.Customer, error)
	GetByDouyinOpenID(ctx context.Context, openID string) (*model.Customer, error)
	Update(ctx context.Context, customer *model.Customer) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, page, limit int) ([]*model.Customer, int64, error)
	FindByIdentity(ctx context.Context, phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error)
	CountNotEmpty(ctx context.Context, fieldName string) (int64, error)
	CountMultiIdentity(ctx context.Context) (int64, error)
	// ListByIDs 批量按 ID 拉取客户，返回按 ID 索引的 map（CC-P2 N+1 优化）
	ListByIDs(ctx context.Context, ids []string) (map[string]*model.Customer, error)
	// GetByXiaohongshuID 按小红书 ID 查询客户
	// 五层架构修复：tooluse 包不可直接访问 DB，由 repository 提供 query 接口
	GetByXiaohongshuID(ctx context.Context, xhsID string) (*model.Customer, error)
	// SearchByFilter 按过滤条件分页查询客户（用于 customer.segment 工具）
	// 五层架构修复：将原 tooluse 层的 t.deps.DB.Model().Where() 链下沉到 repository
	SearchByFilter(ctx context.Context, filter CustomerSearchFilter) (items []*model.Customer, total int64, err error)
}

// CustomerSearchFilter 客户搜索过滤条件（CustomerRepository.SearchByFilter 入参）
//
// 字段语义对应 customer.segment 工具的 args：
//   - Tag: tags::jsonb @> '["tag"]' 匹配（PostgreSQL JSON 数组包含）
//   - RFMMin/RFMMax: rfm_score 范围（含两端）
//   - ChurnRisk: churn_risk 等值匹配（low/medium/high）
//   - CreatedAfter/CreatedBefore: created_at 范围（RFC3339 字符串）
//   - Page/PageSize: 分页参数（1-based，PageSize 上限 100）
type CustomerSearchFilter struct {
	Tag           string
	RFMMin        int
	RFMMax        int
	HasRFMMin     bool // 区分零值与未设置
	HasRFMMax     bool
	ChurnRisk     string
	CreatedAfter  string
	CreatedBefore string
	Page          int
	PageSize      int
}

// customerRepository implements CustomerRepository
type customerRepository struct{}

// NewCustomerRepository creates a new CustomerRepository instance
func NewCustomerRepository() CustomerRepository {
	return &customerRepository{}
}

// Create creates a new customer
func (r *customerRepository) Create(ctx context.Context, customer *model.Customer) error {
	return _db.GetDB().WithContext(ctx).Create(customer).Error
}

// GetByID retrieves a customer by ID
func (r *customerRepository) GetByID(ctx context.Context, id string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).First(&customer, "id = ?", id).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByUnifiedID retrieves a customer by unified ID
func (r *customerRepository) GetByUnifiedID(ctx context.Context, unifiedID string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).First(&customer, "unified_id = ?", unifiedID).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByPhone retrieves a customer by phone
func (r *customerRepository) GetByPhone(ctx context.Context, phone string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).First(&customer, "phone = ?", phone).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByEmail retrieves a customer by email
func (r *customerRepository) GetByEmail(ctx context.Context, email string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).First(&customer, "email = ?", email).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByWechatOpenID retrieves a customer by Wechat OpenID
func (r *customerRepository) GetByWechatOpenID(ctx context.Context, openID string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).First(&customer, "wechat_open_id = ?", openID).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByDouyinOpenID retrieves a customer by Douyin OpenID
func (r *customerRepository) GetByDouyinOpenID(ctx context.Context, openID string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).First(&customer, "douyin_open_id = ?", openID).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// Update updates an existing customer
func (r *customerRepository) Update(ctx context.Context, customer *model.Customer) error {
	return _db.GetDB().WithContext(ctx).Save(customer).Error
}

// Delete deletes a customer by ID
func (r *customerRepository) Delete(ctx context.Context, id string) error {
	return _db.GetDB().WithContext(ctx).Delete(&model.Customer{}, "id = ?", id).Error
}

// List retrieves customers with pagination
func (r *customerRepository) List(ctx context.Context, page, limit int) ([]*model.Customer, int64, error) {
	var customers []*model.Customer
	var total int64

	offset := (page - 1) * limit

	if err := _db.GetDB().WithContext(ctx).Model(&model.Customer{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := _db.GetDB().WithContext(ctx).Offset(offset).Limit(limit).Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}

// FindByIdentity finds a customer by any identity field
func (r *customerRepository) FindByIdentity(ctx context.Context, phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error) {
	var customer model.Customer
	query := _db.GetDB().WithContext(ctx)

	// Build OR conditions for identity fields
	conditions := ""
	args := []any{}

	if phone != "" {
		conditions += "phone = ? OR "
		args = append(args, phone)
	}
	if email != "" {
		conditions += "email = ? OR "
		args = append(args, email)
	}
	if wechatOpenID != "" {
		conditions += "wechat_open_id = ? OR "
		args = append(args, wechatOpenID)
	}
	if douyinOpenID != "" {
		conditions += "douyin_open_id = ? OR "
		args = append(args, douyinOpenID)
	}

	if conditions == "" {
		return nil, nil
	}

	// Remove trailing " OR "
	conditions = conditions[:len(conditions)-4]

	if err := query.Where(conditions, args...).First(&customer).Error; err != nil {
		return nil, nil
	}

	return &customer, nil
}

// CountNotEmpty 统计指定字段非空的客户数
// fieldName: phone / email / wechat_open_id / douyin_open_id / xiaohongshu_id
func (r *customerRepository) CountNotEmpty(ctx context.Context, fieldName string) (int64, error) {
	if fieldName == "" {
		return 0, nil
	}
	var n int64
	// 列名直接拼接：仅允许白名单字段（防 SQL 注入）
	allowed := map[string]bool{
		"phone":          true,
		"email":          true,
		"wechat_open_id": true,
		"douyin_open_id": true,
		"xiaohongshu_id": true,
	}
	if !allowed[fieldName] {
		return 0, nil
	}
	stmt := fieldName + " <> '' AND " + fieldName + " IS NOT NULL"
	if err := _db.GetDB().WithContext(ctx).Model(&model.Customer{}).Where(stmt).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountMultiIdentity 统计具有 2 个及以上身份标识的客户数
// 多身份：phone+email / phone+openid 等任意两种以上
func (r *customerRepository) CountMultiIdentity(ctx context.Context) (int64, error) {
	// 计算每个客户的"已绑定身份数"，筛选 >= 2 的
	expr := "(CASE WHEN phone IS NOT NULL AND phone <> '' THEN 1 ELSE 0 END) + " +
		"(CASE WHEN email IS NOT NULL AND email <> '' THEN 1 ELSE 0 END) + " +
		"(CASE WHEN wechat_open_id IS NOT NULL AND wechat_open_id <> '' THEN 1 ELSE 0 END) + " +
		"(CASE WHEN douyin_open_id IS NOT NULL AND douyin_open_id <> '' THEN 1 ELSE 0 END) + " +
		"(CASE WHEN xiaohongshu_id IS NOT NULL AND xiaohongshu_id <> '' THEN 1 ELSE 0 END)"
	var n int64
	if err := _db.GetDB().WithContext(ctx).Raw(
		"SELECT COUNT(*) FROM customers WHERE " + expr + " >= 2",
	).Scan(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListByIDs 批量按 ID 拉取客户，返回按 ID 索引的 map（CC-P2 N+1 优化）
//
// 单次 SQL：SELECT * FROM customers WHERE id IN (...)。
// 入参 ids 去重 + 跳过空串；未命中的 ID 不会出现在结果 map 中。
// 用于 ComputeAll / ListCustomersWithProfile 等"先 List 再 GetByID"场景，
// 把 N 次 IO 收敛为 1 次。
func (r *customerRepository) ListByIDs(ctx context.Context, ids []string) (map[string]*model.Customer, error) {
	result := make(map[string]*model.Customer, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return result, nil
	}
	var customers []*model.Customer
	if err := _db.GetDB().WithContext(ctx).Where("id IN ?", unique).Find(&customers).Error; err != nil {
		return nil, err
	}
	for _, c := range customers {
		result[c.ID] = c
	}
	return result, nil
}

// GetByXiaohongshuID 按小红书 ID 查询客户
//
// 五层架构修复：tooluse 包 customer.search 工具原直接调用 t.deps.DB.Where("xiaohongshu_id = ?"),
// 违反"service/tooluse 不可直接访问 DB"约束。本方法将查询下沉到 repository 层。
//
// 返回：
//   - 找到: 返回客户指针
//   - 未找到: 返回 (nil, nil)（与 GetByPhone 等同身份查询接口保持一致）
func (r *customerRepository) GetByXiaohongshuID(ctx context.Context, xhsID string) (*model.Customer, error) {
	if xhsID == "" {
		return nil, nil
	}
	var customer model.Customer
	if err := _db.GetDB().WithContext(ctx).Where("xiaohongshu_id = ?", xhsID).First(&customer).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// SearchByFilter 按过滤条件分页查询客户
//
// 五层架构修复：tooluse 包 customer.segment 工具原直接构造 t.deps.DB.Model().Where() 链,
// 违反"service/tooluse 不可直接访问 DB"约束。本方法将整条查询链下沉到 repository 层。
//
// 支持的过滤条件（与 customer.segment 工具 args 一一对应）：
//   - filter.Tag: tags::jsonb @> '["tag"]'
//   - filter.RFMMin/RFMMax（需配合 HasRFMMin/HasRFMMax 才生效）: rfm_score 范围
//   - filter.ChurnRisk: churn_risk 等值
//   - filter.CreatedAfter/CreatedBefore: created_at 范围
//   - filter.Page/filter.PageSize: 分页（1-based，PageSize 上限 100）
//
// 返回：切片 + 总数 + 错误
func (r *customerRepository) SearchByFilter(ctx context.Context, filter CustomerSearchFilter) ([]*model.Customer, int64, error) {
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	q := _db.GetDB().WithContext(ctx).Model(&model.Customer{})

	if filter.Tag != "" {
		q = q.Where("tags::jsonb @> ?", fmt.Sprintf(`["%s"]`, escapeJSONString(filter.Tag)))
	}
	if filter.HasRFMMin {
		q = q.Where("rfm_score >= ?", filter.RFMMin)
	}
	if filter.HasRFMMax {
		q = q.Where("rfm_score <= ?", filter.RFMMax)
	}
	if filter.ChurnRisk != "" {
		q = q.Where("churn_risk = ?", filter.ChurnRisk)
	}
	if filter.CreatedAfter != "" {
		q = q.Where("created_at >= ?", filter.CreatedAfter)
	}
	if filter.CreatedBefore != "" {
		q = q.Where("created_at <= ?", filter.CreatedBefore)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var customers []*model.Customer
	if err := q.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&customers).Error; err != nil {
		return nil, 0, err
	}
	return customers, total, nil
}

// escapeJSONString 转义 JSON 字符串（防止 tag 注入）
// 内部使用：仅供 SearchByFilter 拼 JSON 数组字面量时转义 tag 值
func escapeJSONString(s string) string {
	// 简单转义：双引号和反斜杠
	out := make([]byte, 0, len(s)+2)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		default:
			out = append(out, c)
		}
	}
	return string(out)
}
