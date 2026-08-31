package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- helpers ----------

func setupAfterSaleRepo(t *testing.T) (*AfterSaleRepository, context.Context) {
	t.Helper()
	db := testutil.NewTestDB(t, &model.AfterSale{})
	repo := NewAfterSaleRepository()
	repo.SetDB(context.Background(), db)
	return repo, context.Background()
}

func newTestAfterSale() *model.AfterSale {
	return &model.AfterSale{
		Platform:      "taobao",
		OrderID:       "TB-20260831-001",
		CustomerPhone: "13800138000",
		CustomerName:  "张三",
		Type:          "refund",
		Reason:        "商品与描述不符",
		Amount:        9900,
		Status:        "pending",
		ExternalID:    "EXT-9999",
	}
}

// ---------- NewAfterSaleRepository / SetDB ----------

func TestAfterSale_NewAndSetDB(t *testing.T) {
	db := testutil.NewTestDB(t, &model.AfterSale{})
	ctx := context.Background()

	repo := NewAfterSaleRepository()
	// SetDB 注入真实 DB
	repo.SetDB(ctx, db)

	// SetDB 传入 nil 不应覆盖已有 DB（源码 if db != nil）
	repo.SetDB(ctx, nil)

	// 后续 Create 正常通过即可证明 DB 已注入
	err := repo.Create(ctx, newTestAfterSale())
	require.NoError(t, err)
}

// ---------- Create ----------

func TestAfterSale_Create(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	err := repo.Create(ctx, as)
	require.NoError(t, err)
	assert.NotZero(t, as.ID, "Create 后 ID 应被自动填充")

	// 再次读取验证持久化
	got, err := repo.GetByID(ctx, as.ID)
	require.NoError(t, err)
	assert.Equal(t, as.Platform, got.Platform)
	assert.Equal(t, as.OrderID, got.OrderID)
	assert.Equal(t, as.CustomerPhone, got.CustomerPhone)
	assert.Equal(t, as.Amount, got.Amount)
	assert.Equal(t, "pending", got.Status)
}

// ---------- GetByID ----------

func TestAfterSale_GetByID_Found(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	require.NoError(t, repo.Create(ctx, as))

	got, err := repo.GetByID(ctx, as.ID)
	require.NoError(t, err)
	assert.Equal(t, as.ID, got.ID)
	assert.Equal(t, "TB-20260831-001", got.OrderID)
}

func TestAfterSale_GetByID_NotFound(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	got, err := repo.GetByID(ctx, 99999)
	assert.Error(t, err)
	assert.Nil(t, got)
}

// ---------- Update ----------

func TestAfterSale_Update(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	require.NoError(t, repo.Create(ctx, as))

	as.Status = "processing"
	as.ExternalID = "EXT-8888"
	err := repo.Update(ctx, as)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, as.ID)
	require.NoError(t, err)
	assert.Equal(t, "processing", got.Status)
	assert.Equal(t, "EXT-8888", got.ExternalID)
}

func TestAfterSale_Update_NonExistent(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	as := newTestAfterSale()
	as.ID = 99999
	// Save 一个不存在的记录会 INSERT 一条新记录（GORM Save 行为），这里只验证不 panic
	err := repo.Update(ctx, as)
	assert.NoError(t, err)
}

// ---------- ListByOrder ----------

func TestAfterSale_ListByOrder_Match(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	// 创建 2 条同平台同订单，和 1 条不同平台的
	a1 := newTestAfterSale()
	a2 := newTestAfterSale()
	a3 := newTestAfterSale()
	a3.Platform = "jd"
	a3.OrderID = "JD-123"
	require.NoError(t, repo.Create(ctx, a1))
	require.NoError(t, repo.Create(ctx, a2))
	require.NoError(t, repo.Create(ctx, a3))

	list, err := repo.ListByOrder(ctx, "taobao", "TB-20260831-001")
	require.NoError(t, err)
	assert.Len(t, list, 2)
	// Order("id DESC") 验证排序
	assert.GreaterOrEqual(t, list[0].ID, list[1].ID)
}

func TestAfterSale_ListByOrder_NoMatch(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	list, err := repo.ListByOrder(ctx, "taobao", "NOT-EXIST")
	require.NoError(t, err)
	assert.Len(t, list, 0)
}

// ---------- ListByCustomer ----------

func TestAfterSale_ListByCustomer_WithPhone(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	a1 := newTestAfterSale()
	a2 := newTestAfterSale()
	a3 := newTestAfterSale()
	a3.CustomerPhone = "13900139000"
	require.NoError(t, repo.Create(ctx, a1))
	require.NoError(t, repo.Create(ctx, a2))
	require.NoError(t, repo.Create(ctx, a3))

	list, err := repo.ListByCustomer(ctx, "13800138000")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestAfterSale_ListByCustomer_EmptyPhone_ReturnAll(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	require.NoError(t, repo.Create(ctx, newTestAfterSale()))
	other := newTestAfterSale()
	other.CustomerPhone = "13900139000"
	require.NoError(t, repo.Create(ctx, other))

	// phone 为空 → 源码不追加 WHERE，返回全表
	list, err := repo.ListByCustomer(ctx, "")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestAfterSale_ListByCustomer_NoMatch(t *testing.T) {
	repo, ctx := setupAfterSaleRepo(t)

	require.NoError(t, repo.Create(ctx, newTestAfterSale()))

	list, err := repo.ListByCustomer(ctx, "19900199000")
	require.NoError(t, err)
	assert.Len(t, list, 0)
}
