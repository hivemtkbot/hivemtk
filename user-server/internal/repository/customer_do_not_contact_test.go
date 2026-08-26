package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// dncTestEnv 全局退订标志位测试环境
type dncTestEnv struct {
	DB      *gorm.DB
	DNCRepo *customerDoNotContactRepo
}

// setupDoNotContactTestDB 设置全局退订标志位测试数据库
func setupDoNotContactTestDB(t *testing.T) *dncTestEnv {
	database := testutil.NewTestDB(t,
		&model.CustomerDoNotContact{},
	)
	repo := NewCustomerDoNotContactRepository(database).(*customerDoNotContactRepo)
	return &dncTestEnv{DB: database, DNCRepo: repo}
}

// TestDoNotContact_Create_Idempotent 测试写入幂等：唯一索引(one_id, channel)冲突跳过
func TestDoNotContact_Create_Idempotent(t *testing.T) {
	env := setupDoNotContactTestDB(t)
	ctx := context.Background()

	rec := &model.CustomerDoNotContact{OneID: "one-1", Channel: "", Source: model.DNCSourceManual}
	created, err := env.DNCRepo.Create(ctx, rec)
	if err != nil {
		t.Fatalf("首次 Create 失败: %v", err)
	}
	if !created {
		t.Fatal("Expected created=true on first insert")
	}

	created, err = env.DNCRepo.Create(ctx, &model.CustomerDoNotContact{OneID: "one-1", Channel: "", Source: model.DNCSourceSMSKeyword})
	if err != nil {
		t.Fatalf("重复 Create 不应报错: %v", err)
	}
	if created {
		t.Error("Expected created=false on duplicate (conflict skipped)")
	}

	var count int64
	env.DB.Model(&model.CustomerDoNotContact{}).Where("one_id = ?", "one-1").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 row (idempotent), got %d", count)
	}
}

// TestDoNotContact_ExistsByOneIDAndChannels 测试三态查询语义
func TestDoNotContact_ExistsByOneIDAndChannels(t *testing.T) {
	env := setupDoNotContactTestDB(t)
	ctx := context.Background()

	// 未命中
	exists, err := env.DNCRepo.ExistsByOneIDAndChannels(ctx, "one-2", []string{"sms", ""})
	if err != nil {
		t.Fatalf("ExistsByOneIDAndChannels 失败: %v", err)
	}
	if exists {
		t.Error("Expected not blocked before any block")
	}

	// 全局行(channel="")命中
	if _, err := env.DNCRepo.Create(ctx, &model.CustomerDoNotContact{OneID: "one-2", Channel: "", Source: model.DNCSourceImport}); err != nil {
		t.Fatalf("Create 全局行失败: %v", err)
	}
	exists, _ = env.DNCRepo.ExistsByOneIDAndChannels(ctx, "one-2", []string{"sms", ""})
	if !exists {
		t.Error("Expected blocked via global row (channel='')")
	}

	// 精确渠道行命中
	if _, err := env.DNCRepo.Create(ctx, &model.CustomerDoNotContact{OneID: "one-3", Channel: "email", Source: model.DNCSourceWebhook}); err != nil {
		t.Fatalf("Create 渠道行失败: %v", err)
	}
	exists, _ = env.DNCRepo.ExistsByOneIDAndChannels(ctx, "one-3", []string{"email", ""})
	if !exists {
		t.Error("Expected blocked via channel-exact row")
	}
	exists, _ = env.DNCRepo.ExistsByOneIDAndChannels(ctx, "one-3", []string{"sms", ""})
	if exists {
		t.Error("Expected not blocked on other channels")
	}
}

// TestDoNotContact_Delete_List 测试删除与列表
func TestDoNotContact_Delete_List(t *testing.T) {
	env := setupDoNotContactTestDB(t)
	ctx := context.Background()

	for _, ch := range []string{"", "sms"} {
		if _, err := env.DNCRepo.Create(ctx, &model.CustomerDoNotContact{OneID: "one-4", Channel: ch}); err != nil {
			t.Fatalf("Create(%q) 失败: %v", ch, err)
		}
	}

	rows, err := env.DNCRepo.ListByOneID(ctx, "one-4")
	if err != nil {
		t.Fatalf("ListByOneID 失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(rows))
	}

	// 删除精确渠道行
	if err := env.DNCRepo.DeleteByOneIDAndChannel(ctx, "one-4", "sms"); err != nil {
		t.Fatalf("DeleteByOneIDAndChannel 失败: %v", err)
	}
	rows, _ = env.DNCRepo.ListByOneID(ctx, "one-4")
	if len(rows) != 1 || rows[0].Channel != "" {
		t.Errorf("Expected only global row left, got %+v", rows)
	}

	// 删除不存在的行幂等
	if err := env.DNCRepo.DeleteByOneIDAndChannel(ctx, "one-4", "sms"); err != nil {
		t.Errorf("删除不存在的行应幂等返回 nil, got %v", err)
	}
}
