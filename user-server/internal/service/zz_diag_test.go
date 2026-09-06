package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/identity"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

func TestDiagUnifiedIDLookup(t *testing.T) {
	database := testutil.NewTestDB(t, &model.Customer{}, &model.CustomerSession{}, &model.CustomerEvent{})
	db.SetTestDB(database)
	svc := NewCustomerIdentityService()
	phone := "13900001111"
	c, err := svc.IdentifyOrCreate(context.Background(), identity.Identifiers{Phone: phone})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("created unified_id=%q id=%q", c.UnifiedID, c.ID)
	repo := repository.NewCustomerRepository()

	got, gerr := repo.GetByUnifiedID(context.Background(), c.UnifiedID)
	t.Logf("GetByUnifiedID err=%v got=%v", gerr, got)
	if got == nil {
		t.Fatalf("GetByUnifiedID miss after create")
	}
}
