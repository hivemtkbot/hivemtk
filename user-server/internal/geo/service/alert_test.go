package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/geo/model"
)

type memAlertRepo struct {
	alerts []*model.GeoAlert
}

func (r *memAlertRepo) Create(_ context.Context, a *model.GeoAlert) error {
	r.alerts = append(r.alerts, a)
	return nil
}

func (r *memAlertRepo) List(_ context.Context, alertType, level string, page, limit int) ([]*model.GeoAlert, int64, error) {
	var filtered []*model.GeoAlert
	for _, a := range r.alerts {
		if alertType != "" && a.Type != alertType {
			continue
		}
		if level != "" && a.Level != level {
			continue
		}
		filtered = append(filtered, a)
	}
	total := int64(len(filtered))
	start := (page - 1) * limit
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (r *memAlertRepo) MarkNotified(_ context.Context, id uint) error {
	for _, a := range r.alerts {
		if a.ID == id {
			a.Notified = true
		}
	}
	return nil
}

func (r *memAlertRepo) DeleteBefore(_ context.Context, _ time.Time) error { return nil }

func (r *memAlertRepo) CountUnread(_ context.Context) (int64, error) {
	var n int64
	for _, a := range r.alerts {
		if !a.Notified {
			n++
		}
	}
	return n, nil
}

func (r *memAlertRepo) Delete(_ context.Context, id uint) error {
	for i, a := range r.alerts {
		if a.ID == id {
			r.alerts = append(r.alerts[:i], r.alerts[i+1:]...)
			return nil
		}
	}
	return nil
}

func newTestAlertService() (*AlertService, *memAlertRepo) {
	repo := &memAlertRepo{
		alerts: []*model.GeoAlert{
			{ID: 1, Type: "negative_monitor", Level: "warning", Notified: false},
			{ID: 2, Type: "sov_drop", Level: "critical", Notified: false},
			{ID: 3, Type: "negative_monitor", Level: "info", Notified: true},
		},
	}
	return NewAlertService(repo), repo
}

func TestAlertListFilterAndPaging(t *testing.T) {
	svc, _ := newTestAlertService()
	ctx := context.Background()

	res, err := svc.ListAlerts(ctx, AlertQuery{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("ListAlerts: %v", err)
	}
	if res.Total != 3 || len(res.List) != 3 {
		t.Fatalf("期望 3 条，got total=%d len=%d", res.Total, len(res.List))
	}

	res, err = svc.ListAlerts(ctx, AlertQuery{Type: "negative_monitor", Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("ListAlerts filter: %v", err)
	}
	if res.Total != 2 {
		t.Fatalf("类型过滤期望 2 条，got %d", res.Total)
	}

	res, err = svc.ListAlerts(ctx, AlertQuery{Level: "critical", Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("ListAlerts level: %v", err)
	}
	if res.Total != 1 || res.List[0].Level != "critical" {
		t.Fatalf("级别过滤异常: total=%d", res.Total)
	}
}

func TestAlertAckAndUnread(t *testing.T) {
	svc, _ := newTestAlertService()
	ctx := context.Background()

	n, _ := svc.CountUnread(ctx)
	if n != 2 {
		t.Fatalf("初始未读期望 2，got %d", n)
	}
	if err := svc.MarkNotified(ctx, 1); err != nil {
		t.Fatalf("MarkNotified: %v", err)
	}
	n, _ = svc.CountUnread(ctx)
	if n != 1 {
		t.Fatalf("确认后未读期望 1，got %d", n)
	}
}

func TestAlertDelete(t *testing.T) {
	svc, _ := newTestAlertService()
	ctx := context.Background()

	if err := svc.DeleteAlert(ctx, 2); err != nil {
		t.Fatalf("DeleteAlert: %v", err)
	}
	res, _ := svc.ListAlerts(ctx, AlertQuery{Page: 1, Limit: 10})
	if res.Total != 2 {
		t.Fatalf("删除后期望 2 条，got %d", res.Total)
	}
}
