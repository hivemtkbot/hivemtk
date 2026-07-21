package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
)

// CustomerRepository defines the interface for customer data access
type CustomerRepository interface {
	Create(customer *model.Customer) error
	GetByID(id string) (*model.Customer, error)
	GetByUnifiedID(unifiedID string) (*model.Customer, error)
	GetByPhone(phone string) (*model.Customer, error)
	GetByEmail(email string) (*model.Customer, error)
	GetByWechatOpenID(openID string) (*model.Customer, error)
	GetByDouyinOpenID(openID string) (*model.Customer, error)
	Update(customer *model.Customer) error
	Delete(id string) error
	List(page, limit int) ([]*model.Customer, int64, error)
	FindByIdentity(phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error)
}

// customerRepository implements CustomerRepository
type customerRepository struct{}

// NewCustomerRepository creates a new CustomerRepository instance
func NewCustomerRepository() CustomerRepository {
	return &customerRepository{}
}

// Create creates a new customer
func (r *customerRepository) Create(customer *model.Customer) error {
	return _db.GetDB().Create(customer).Error
}

// GetByID retrieves a customer by ID
func (r *customerRepository) GetByID(id string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().First(&customer, "id = ?", id).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByUnifiedID retrieves a customer by unified ID
func (r *customerRepository) GetByUnifiedID(unifiedID string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().First(&customer, "unified_id = ?", unifiedID).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByPhone retrieves a customer by phone
func (r *customerRepository) GetByPhone(phone string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().First(&customer, "phone = ?", phone).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByEmail retrieves a customer by email
func (r *customerRepository) GetByEmail(email string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().First(&customer, "email = ?", email).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByWechatOpenID retrieves a customer by Wechat OpenID
func (r *customerRepository) GetByWechatOpenID(openID string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().First(&customer, "wechat_open_id = ?", openID).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// GetByDouyinOpenID retrieves a customer by Douyin OpenID
func (r *customerRepository) GetByDouyinOpenID(openID string) (*model.Customer, error) {
	var customer model.Customer
	if err := _db.GetDB().First(&customer, "douyin_open_id = ?", openID).Error; err != nil {
		return nil, nil
	}
	return &customer, nil
}

// Update updates an existing customer
func (r *customerRepository) Update(customer *model.Customer) error {
	return _db.GetDB().Save(customer).Error
}

// Delete deletes a customer by ID
func (r *customerRepository) Delete(id string) error {
	return _db.GetDB().Delete(&model.Customer{}, "id = ?", id).Error
}

// List retrieves customers with pagination
func (r *customerRepository) List(page, limit int) ([]*model.Customer, int64, error) {
	var customers []*model.Customer
	var total int64

	offset := (page - 1) * limit

	if err := _db.GetDB().Model(&model.Customer{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := _db.GetDB().Offset(offset).Limit(limit).Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	return customers, total, nil
}

// FindByIdentity finds a customer by any identity field
func (r *customerRepository) FindByIdentity(phone, email, wechatOpenID, douyinOpenID string) (*model.Customer, error) {
	var customer model.Customer
	query := _db.GetDB()

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
