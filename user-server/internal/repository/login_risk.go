package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// LoginRiskRepository 登录风险评估数据仓储
//
// 封装 login_events / security_alerts / notifications 三张表,
// 避免 service 层直接调 db.GetDB()。
type LoginRiskRepository interface {
	CountRecentFailures(ctx context.Context, userID uint, username string, since time.Time) (int64, error)
	GetLastSuccessLocation(ctx context.Context, userID uint) (location, ip string, found bool, err error)
	CountDeviceFingerprintSince(ctx context.Context, userID uint, fingerprint string, since time.Time) (int64, error)
	CreateLoginEvent(ctx context.Context, event *model.LoginEvent) (*model.LoginEvent, error)
	ListLoginEvents(ctx context.Context, userID uint, page, pageSize int) ([]*model.LoginEvent, int64, error)

	CreateSecurityAlert(ctx context.Context, alert *model.SecurityAlert) (*model.SecurityAlert, error)
	MarkAlertNotified(ctx context.Context, alertID uint) error
	ListSecurityAlerts(ctx context.Context, userID uint, status string, page, pageSize int) ([]*model.SecurityAlert, int64, error)
	ResolveSecurityAlert(ctx context.Context, alertID, resolverUserID uint, note string, now time.Time, status string) error

	CreateNotification(ctx context.Context, notif *model.Notification) error
}

type loginRiskRepo struct {
	db *gorm.DB
}

// NewLoginRiskRepository 创建登录风险仓储
func NewLoginRiskRepository() LoginRiskRepository {
	return &loginRiskRepo{db: _db.GetDB()}
}

// NewLoginRiskRepositoryWithDB 通过 *gorm.DB 创建(用于测试)
func NewLoginRiskRepositoryWithDB(gormDB *gorm.DB) LoginRiskRepository {
	return &loginRiskRepo{db: gormDB}
}

func (r *loginRiskRepo) CountRecentFailures(ctx context.Context, userID uint, username string, since time.Time) (int64, error) {
	q := r.db.WithContext(ctx).Model(&model.LoginEvent{}).
		Where("success = ? AND login_at >= ?", false, since)
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	} else if username != "" {
		q = q.Where("username = ?", username)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *loginRiskRepo) GetLastSuccessLocation(ctx context.Context, userID uint) (location, ip string, found bool, err error) {
	if userID == 0 {
		return "", "", false, nil
	}
	var prev model.LoginEvent
	if err := r.db.WithContext(ctx).Where("user_id = ? AND success = ?", userID, true).
		Order("login_at DESC").First(&prev).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return prev.Location, prev.IP, true, nil
}

func (r *loginRiskRepo) CountDeviceFingerprintSince(ctx context.Context, userID uint, fingerprint string, since time.Time) (int64, error) {
	if userID == 0 || fingerprint == "" {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.LoginEvent{}).
		Where("user_id = ? AND device_fingerprint = ? AND login_at >= ?", userID, fingerprint, since).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *loginRiskRepo) CreateLoginEvent(ctx context.Context, event *model.LoginEvent) (*model.LoginEvent, error) {
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func (r *loginRiskRepo) ListLoginEvents(ctx context.Context, userID uint, page, pageSize int) ([]*model.LoginEvent, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var (
		events []*model.LoginEvent
		total  int64
	)
	q := r.db.WithContext(ctx).Model(&model.LoginEvent{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := q.Order("login_at DESC").Offset(offset).Limit(pageSize).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func (r *loginRiskRepo) CreateSecurityAlert(ctx context.Context, alert *model.SecurityAlert) (*model.SecurityAlert, error) {
	if err := r.db.WithContext(ctx).Create(alert).Error; err != nil {
		return nil, err
	}
	return alert, nil
}

func (r *loginRiskRepo) MarkAlertNotified(ctx context.Context, alertID uint) error {
	return r.db.WithContext(ctx).Model(&model.SecurityAlert{}).
		Where("id = ?", alertID).
		Update("notified", true).Error
}

func (r *loginRiskRepo) ListSecurityAlerts(ctx context.Context, userID uint, status string, page, pageSize int) ([]*model.SecurityAlert, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var (
		alerts []*model.SecurityAlert
		total  int64
	)
	q := r.db.WithContext(ctx).Model(&model.SecurityAlert{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&alerts).Error; err != nil {
		return nil, 0, err
	}
	return alerts, total, nil
}

func (r *loginRiskRepo) ResolveSecurityAlert(ctx context.Context, alertID, resolverUserID uint, note string, now time.Time, status string) error {
	return r.db.WithContext(ctx).Model(&model.SecurityAlert{}).
		Where("id = ?", alertID).
		Updates(map[string]any{
			"status":       status,
			"resolved_at":  now,
			"resolved_by":  resolverUserID,
			"resolve_note": note,
		}).Error
}

func (r *loginRiskRepo) CreateNotification(ctx context.Context, notif *model.Notification) error {
	return r.db.WithContext(ctx).Create(notif).Error
}

var _ LoginRiskRepository = (*loginRiskRepo)(nil)
