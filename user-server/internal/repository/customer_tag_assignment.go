package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// CustomerTagAssignmentRepository defines the interface for customer tag assignment data access
type CustomerTagAssignmentRepository interface {
	GetByCustomerAndTag(ctx context.Context, customerID, tag string) (*model.CustomerTagAssignment, error)
	ListByCustomerID(ctx context.Context, customerID string) ([]*model.CustomerTagAssignment, error)
	Create(ctx context.Context, assignment *model.CustomerTagAssignment) error
	Update(ctx context.Context, assignment *model.CustomerTagAssignment) error
	DeleteByCustomerAndTag(ctx context.Context, customerID, tag string) error
}

// customerTagAssignmentRepository implements CustomerTagAssignmentRepository
type customerTagAssignmentRepository struct{}

// NewCustomerTagAssignmentRepository creates a new CustomerTagAssignmentRepository instance
func NewCustomerTagAssignmentRepository() CustomerTagAssignmentRepository {
	return &customerTagAssignmentRepository{}
}

func assignmentDB() (*gorm.DB, error) {
	database := _db.GetDB()
	if database == nil {
		return nil, gorm.ErrInvalidDB
	}
	return database, nil
}

// GetByCustomerAndTag retrieves a single assignment by customer ID and tag name.
// Returns (nil, nil) when not found (与 customer_tag.go GetByID 的容错风格一致).
func (r *customerTagAssignmentRepository) GetByCustomerAndTag(ctx context.Context, customerID, tag string) (*model.CustomerTagAssignment, error) {
	database, err := assignmentDB()
	if err != nil {
		return nil, err
	}
	var assignment model.CustomerTagAssignment
	if err := database.First(&assignment, "customer_id = ? AND tag = ?", customerID, tag).Error; err != nil {
		return nil, nil
	}
	return &assignment, nil
}

func (r *customerTagAssignmentRepository) ListByCustomerID(ctx context.Context, customerID string) ([]*model.CustomerTagAssignment, error) {
	database, err := assignmentDB()
	if err != nil {
		return nil, err
	}
	var assignments []*model.CustomerTagAssignment
	if err := database.
		Where("customer_id = ?", customerID).
		Order("created_at").
		Find(&assignments).Error; err != nil {
		return nil, err
	}
	return assignments, nil
}

// Create creates a new customer tag assignment
func (r *customerTagAssignmentRepository) Create(ctx context.Context, assignment *model.CustomerTagAssignment) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Create(assignment).Error
}

// Update updates an existing customer tag assignment
func (r *customerTagAssignmentRepository) Update(ctx context.Context, assignment *model.CustomerTagAssignment) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Save(assignment).Error
}

// DeleteByCustomerAndTag deletes an assignment by customer ID and tag name
func (r *customerTagAssignmentRepository) DeleteByCustomerAndTag(ctx context.Context, customerID, tag string) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Delete(&model.CustomerTagAssignment{}, "customer_id = ? AND tag = ?", customerID, tag).Error
}
