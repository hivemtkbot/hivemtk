package repository

import (
	"context"
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
		"phone":           true,
		"email":           true,
		"wechat_open_id":  true,
		"douyin_open_id":  true,
		"xiaohongshu_id":  true,
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
		"SELECT COUNT(*) FROM customers WHERE "+expr+" >= 2",
	).Scan(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
