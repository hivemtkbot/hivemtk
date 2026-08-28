package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CustomerTagAssignmentRepository interface {
	GetByCustomerAndTag(ctx context.Context, customerID, tag string) (*model.CustomerTagAssignment, error)
	ListByCustomerID(ctx context.Context, customerID string) ([]*model.CustomerTagAssignment, error)
	Create(ctx context.Context, assignment *model.CustomerTagAssignment) error
	Update(ctx context.Context, assignment *model.CustomerTagAssignment) error
	DeleteByCustomerAndTag(ctx context.Context, customerID, tag string) error
	Upsert(ctx context.Context, assignment *model.CustomerTagAssignment) error
}

type customerTagAssignmentRepository struct{}

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

func (r *customerTagAssignmentRepository) Create(ctx context.Context, assignment *model.CustomerTagAssignment) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Create(assignment).Error
}

func (r *customerTagAssignmentRepository) Update(ctx context.Context, assignment *model.CustomerTagAssignment) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Save(assignment).Error
}

func (r *customerTagAssignmentRepository) DeleteByCustomerAndTag(ctx context.Context, customerID, tag string) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Delete(&model.CustomerTagAssignment{}, "customer_id = ? AND tag = ?", customerID, tag).Error
}

func (r *customerTagAssignmentRepository) Upsert(ctx context.Context, assignment *model.CustomerTagAssignment) error {
	database, err := assignmentDB()
	if err != nil {
		return err
	}
	return database.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "customer_id"},
			{Name: "tag"},
		},
		DoUpdates: clause.Assignments(map[string]any{
				"category": assignment.Category,
				"source":   assignment.Source,
				// ON CONFLICT DO UPDATE 的 SET 右值中裸列名会触发 PG "column reference is ambiguous"，
				// 必须以表名限定引用既有行的 confidence（与 EXCLUDED 的新值取 GREATEST）
				"confidence": gorm.Expr("GREATEST(customer_tag_assignments.confidence, ?)", assignment.Confidence),
			}),
	}).Create(assignment).Error
}
