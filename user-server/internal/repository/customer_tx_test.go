package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	dbUtil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
)

// TestWithTransaction_Rollback_Participation 回归：OPT-ARC-06 假事务修复。
// WithTransaction 写入 ctx 的 tx 必须被仓储方法（dbFromCtx）真实消费：
// 事务内 Update 生效、fn 返回 error 后必须整体回滚（此前 tx 无人读取，
// 所有写入逐条自动提交，回滚承诺完全失效）。
func TestWithTransaction_Rollback_Participation(t *testing.T) {
	db := testutil.NewTestDB(t, &model.Customer{})
	prev := dbUtil.GetDB()
	dbUtil.SetTestDB(db)
	defer dbUtil.SetTestDB(prev)

	repo := NewCustomerRepository()

	cust := &model.Customer{Name: "tx-part", UnifiedID: "tx:part:1"}
	if err := db.Create(cust).Error; err != nil {
		t.Fatalf("seed 失败: %v", err)
	}

	ctx := context.Background()
	err := repo.WithTransaction(ctx, func(txCtx context.Context) error {
		cust.Name = "changed-in-tx"
		if err := repo.Update(txCtx, cust); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("期望 fn 错误上抛")
	}

	after, err := repo.GetByID(ctx, cust.ID)
	if err != nil || after == nil {
		t.Fatalf("回查失败: %v", err)
	}
	if after.Name != "tx-part" {
		t.Fatalf("事务内写入应被回滚, 实际 Name=%q (假事务回归!)", after.Name)
	}

	err = repo.WithTransaction(ctx, func(txCtx context.Context) error {
		cust.Name = "committed"
		return repo.Update(txCtx, cust)
	})
	if err != nil {
		t.Fatalf("正常提交失败: %v", err)
	}
	after2, _ := repo.GetByID(ctx, cust.ID)
	if after2 == nil || after2.Name != "committed" {
		t.Fatalf("提交后变更应可见, 实际 %+v", after2)
	}
}

// TestReassignCustomerID_Move 验证事件迁移为 UPDATE 移动语义（P1-2 修复）。
func TestReassignCustomerID_Move(t *testing.T) {
	db := testutil.NewTestDB(t, &model.CustomerEvent{})
	prev := dbUtil.GetDB()
	dbUtil.SetTestDB(db)
	defer dbUtil.SetTestDB(prev)

	repo := NewCustomerEventRepository()

	for i := 0; i < 3; i++ {
		if err := repo.Record(context.Background(), &model.CustomerEvent{
			CustomerID: "sec-1",
			EventType:  model.EventTypeClick,
		}); err != nil {
			t.Fatalf("seed 失败: %v", err)
		}
	}

	moved, err := repo.ReassignCustomerID(context.Background(), "sec-1", "main-1")
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if moved != 3 {
		t.Fatalf("期望迁移 3 条, 实际 %d", moved)
	}

	var remainSec, totalMain int64
	db.Model(&model.CustomerEvent{}).Where("customer_id = ?", "sec-1").Count(&remainSec)
	db.Model(&model.CustomerEvent{}).Where("customer_id = ?", "main-1").Count(&totalMain)
	if remainSec != 0 {
		t.Fatalf("旧客户事件应为 0(移动语义非复制), 实际 %d", remainSec)
	}
	if totalMain != 3 {
		t.Fatalf("主客户事件应为 3, 实际 %d", totalMain)
	}
}
