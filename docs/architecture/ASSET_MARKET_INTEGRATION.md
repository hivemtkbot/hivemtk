# HivemTK 用户端 · 资产市场集成文档

> **配套**:`hivemtk-platform/docs/architecture/ASSET_MARKET_DESIGN.md`(总设计)
> **本端**:用户端(`hivemtk` 仓库)
> **核心改造**:**把原代码里写死的 Prompt/人设/SOP/AB/工作流 5 类常量改为从 PostgreSQL 加载**(代码默认作兜底)
> **核心原则**:**同源同构**——平台购买资产与商户自建资产,数据模型、CRUD、UI **完全统一**,仅 `source` 标识不同

---

## 〇、核心设计原则(必须严格遵守)

### 原则 1:同源同构(Same-Source, Same-Structure)

> **平台购买资产**与**商户自建资产**是**同一个东西**——同一张表 `local_assets`、同一组 CRUD、同一套 UI、同一套业务加载逻辑。
> **唯一区别**:记录里的 `source` 字段(`purchased` / `manual` / `synced` / `imported`)。

| 维度 | 平台购买 | 商户自建 |
|---|---|---|
| 存储表 | `local_assets` | `local_assets` |
| 加载方式 | Loader 读 DB | Loader 读 DB |
| CRUD API | 同一组 | 同一组 |
| 详情 UI | 同一组件 | 同一组件 |
| 编辑器 UI | 同一组件(可编辑) | 同一组件 |
| 业务生效 | 自动(下次调用 Loader) | 自动(下次调用 Loader) |
| 删除 | 软删(`is_active=false`) | 软删 |
| 唯一差异 | 多一个「同步到最新版本」按钮 | 无 |

> **禁止**为"购买"和"自建"建两张表、写两套 API、做两个页面。

### 原则 2:五层架构(Handler → Service → Domain → Repository → Model)

```
┌──────────────────────────────────────────────┐
│  Handler(Controller)层                       │  HTTP 请求/响应、参数校验、调用 Service
├──────────────────────────────────────────────┤
│  Service 层                                   │  业务逻辑、事务编排、跨实体操作
├──────────────────────────────────────────────┤
│  Domain 层                                    │  业务实体、领域规则、不依赖 GORM/JSON
├──────────────────────────────────────────────┤
│  Repository 层                                │  数据访问抽象、SQL 拼装、DB 隔离
├──────────────────────────────────────────────┤
│  Model 层                                     │  GORM PO、DB Schema、JSON Tag
└──────────────────────────────────────────────┘
```

依赖方向:**Handler → Service → Domain → Repository → Model**(单向,严禁反向依赖/跨层调用)

### 原则 3:Repository Pattern + 依赖注入

- 所有 DB 操作**只允许在 Repository 层出现**
- Service 通过接口依赖 Repository,便于测试
- Controller 通过 wire/manual 注入 Service

### 原则 4:统一错误处理

- **业务错误码**统一常量定义,不允许散落 magic number
- **错误链**:`fmt.Errorf("xxx 失败: %w", err)`,统一用 `errors.Is` / `errors.As` 判断
- **响应结构**:`{ code: 0, message: "ok", data: ... }`(成功)/ `{ code: xxxx, message: "..." }`(失败)

### 原则 5:配置驱动

- 业务参数(平台端地址、超时、分页大小、缓存 TTL)**只允许从 config 读**,不允许硬编码
- 配置文件分三层:`configs/config.yaml` / `.env` / `configs/config.local.yaml`(本地覆盖,git 忽略)

### 原则 6:日志规范

- 使用 `slog`(Go 1.21+)或 `zap`,结构化输出
- 日志字段:`asset_id` / `asset_type` / `merchant_key` / `action` / `latency_ms`
- 关键路径必须有 INFO 级别日志

### 原则 7:可测试性

- Service 层 100% 单元测试覆盖(目标)
- Loader 必须有"DB 命中 / DB 未命中 / DB 异常 / JSON 损坏"4 个用例
- 集成测试覆盖端到端主流程

---

## 一、本端改造范围

| 模块 | 类型 | 关键性 |
|---|---|---|
| 资产市场菜单 + 4 个页面 | 前端新增 | ⭐⭐⭐ |
| `/api/v1/asset-market/*` API | 后端新增 | ⭐⭐⭐ |
| `local_assets` / `local_asset_data` / `local_asset_sync_log` 3 张表 | DB 新增 | ⭐⭐⭐ |
| **5 个 Loader + 业务调用方改造** | **关键改造** | ⭐⭐⭐⭐⭐ |
| 原代码默认值兜底保留 | 重构 | ⭐⭐⭐⭐⭐ |
| 平台端代理(AppKey 鉴权) | 新增 | ⭐⭐⭐ |
| 同步服务 | 新增 | ⭐⭐⭐ |

---

## 二、目录结构(严格遵循五层架构)

```
hivemtk/
├── user-server/
│   ├── internal/
│   │   ├── handler/                              ← 第 1 层:HTTP Handler
│   │   │   └── asset_market_handler.go           ← 仅做参数解析、调用 Service、格式化响应
│   │   ├── service/                              ← 第 2 层:业务逻辑
│   │   │   ├── asset_market_service.go           ← 资产市场业务编排
│   │   │   ├── local_asset_service.go            ← 本地资产 CRUD 业务
│   │   │   ├── agent_loader.go                   ← 智能体 Loader(DB → 默认兜底)
│   │   │   ├── sop_loader.go                     ← SOP Loader
│   │   │   ├── abtest_loader.go                  ← AB Loader
│   │   │   ├── workflow_loader.go                ← 工作流 Loader
│   │   │   └── script_loader.go                  ← 话术 Loader
│   │   ├── domain/                               ← 第 3 层:领域实体/规则
│   │   │   ├── asset/
│   │   │   │   ├── entity.go                     ← Asset 实体(平台/本地统一)
│   │   │   │   ├── value_object.go               ← AssetType / Source / Status 等值对象
│   │   │   │   └── validator.go                  ← 5 类资产 schema 校验逻辑
│   │   │   └── errors/
│   │   │       └── codes.go                      ← 业务错误码常量
│   │   ├── repository/                           ← 第 4 层:数据访问抽象
│   │   │   ├── local_asset_repo.go               ← LocalAsset CRUD
│   │   │   ├── local_asset_data_repo.go          ← LocalAssetData CRUD
│   │   │   ├── sync_log_repo.go                  ← SyncLog 写入/查询
│   │   │   └── platform_api_client.go            ← 平台端 HTTP 客户端(走 AppKey)
│   │   ├── model/                                ← 第 5 层:持久化对象
│   │   │   ├── local_asset.go                    ← GORM PO
│   │   │   ├── local_asset_data.go
│   │   │   └── local_asset_sync_log.go
│   │   ├── router/
│   │   │   └── router.go                         ← 路由注册
│   │   └── di/
│   │       └── wire.go                           ← 依赖注入(手动 wire)
│   └── migrations/024_asset_market.sql
└── user-web/                                     ← 前端(Vue3 + JS)
    └── src/
        ├── api/asset/                            ← API 封装
        │   ├── index.js                          ← 统一 export
        │   ├── market.js                         ← 市场相关 API
        │   └── local.js                          ← 本地资产 CRUD API
        ├── components/asset/                     ← 通用 UI 组件(同源同构)
        │   ├── AssetFormDialog.vue               ← 资产编辑对话框(统一)
        │   ├── AssetTypePicker.vue               ← 5 类资产选择器
        │   ├── JsonSchemaEditor.vue              ← JSON 编辑器(带 schema 校验)
        │   ├── IndustrySelector.vue              ← 5 行业选择器
        │   ├── SourceTag.vue                     ← 来源标签(购买/自建/同步/导入)
        │   └── SyncVersionButton.vue             ← 同步按钮(仅 purchased 来源显示)
        ├── views/asset/                          ← 页面
        │   ├── Market.vue                        ← 资产市场浏览(平台端列表)
        │   ├── Detail.vue                        ← 资产详情(平台)
        │   ├── MyAssets.vue                      ← 我的资产(平台+自建 统一列表)★关键
        │   ├── AssetEdit.vue                     ← 编辑/创建资产(统一)★关键
        │   └── SyncLog.vue
        ├── stores/asset.js                       ← Pinia store
        └── router/index.js
```

---

## 三、数据库迁移

文件:`hivemtk/user-server/migrations/024_asset_market.sql`

```sql
-- 1. 已购资产主表
CREATE TABLE local_assets (
    id BIGSERIAL PRIMARY KEY,
    asset_id VARCHAR(64) UNIQUE NOT NULL,        -- 平台端业务 ID
    asset_type VARCHAR(32) NOT NULL,             -- agent_persona/sales_script/ab_test_plan/marketing_workflow/industry_sop
    industry VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    version VARCHAR(16) NOT NULL,
    source VARCHAR(16) DEFAULT 'purchased',      -- purchased/manual/synced
    is_active BOOLEAN DEFAULT TRUE,
    purchase_id BIGINT,                          -- 平台端购买记录 ID
    purchased_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_local_assets_type ON local_assets(asset_type);
CREATE INDEX idx_local_assets_industry ON local_assets(industry);
CREATE INDEX idx_local_assets_active ON local_assets(is_active);

-- 2. 资产实际数据(JSONB)
CREATE TABLE local_asset_data (
    id BIGSERIAL PRIMARY KEY,
    local_asset_id BIGINT NOT NULL REFERENCES local_assets(id) ON DELETE CASCADE,
    data JSONB NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(local_asset_id)
);
CREATE INDEX idx_local_asset_data_gin ON local_asset_data USING GIN(data);

-- 3. 同步日志
CREATE TABLE local_asset_sync_log (
    id BIGSERIAL PRIMARY KEY,
    asset_id VARCHAR(64) NOT NULL,
    action VARCHAR(32) NOT NULL,                 -- pull/sync/activate/deactivate/delete
    status VARCHAR(16) NOT NULL,                 -- success/failed
    error_msg TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_sync_log_asset ON local_asset_sync_log(asset_id);
CREATE INDEX idx_sync_log_created ON local_asset_sync_log(created_at DESC);
```

---

## 四、统一资产模型(同源同构)★核心

### 4.1 数据模型层(`model/local_asset.go`)

```go
package model

import (
    "time"
    "gorm.io/gorm"
)

// AssetSource 资产来源(同源同构的"源"标识)
type AssetSource string

const (
    AssetSourcePurchased AssetSource = "purchased"  // 平台购买同步
    AssetSourceManual    AssetSource = "manual"     // 商户自建
    AssetSourceSynced    AssetSource = "synced"     // 平台运营分发(预留)
    AssetSourceImported  AssetSource = "imported"   // 文件导入(预留)
)

type LocalAsset struct {
    ID          int64       `gorm:"primaryKey" json:"id"`
    AssetID     string      `gorm:"size:64;uniqueIndex" json:"asset_id"`
    AssetType   string      `gorm:"size:32;index" json:"asset_type"`
    Industry    string      `gorm:"size:32;index" json:"industry"`
    Name        string      `gorm:"size:128" json:"name"`
    Version     string      `gorm:"size:16" json:"version"`
    Source      AssetSource `gorm:"size:16;index" json:"source"`     // ★ 核心:同源标识
    IsActive    bool        `gorm:"index" json:"is_active"`
    PurchaseID  *int64      `json:"purchase_id,omitempty"`           // 仅 purchased 来源有
    PurchasedAt *time.Time  `json:"purchased_at,omitempty"`
    SyncedAt    time.Time   `json:"synced_at"`
    UpdatedAt   time.Time   `json:"updated_at"`
    CreatedAt   time.Time   `gorm:"autoCreateTime" json:"created_at"`
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (LocalAsset) TableName() string { return "local_assets" }
```

### 4.2 领域层(`domain/asset/entity.go`)★统一

```go
package asset

import "time"

// Asset 资产领域实体(同源同构:不区分平台/本地来源)
type Asset struct {
    ID          int64
    AssetID     string       // 业务 ID(平台:平台端 ID;自建:UUID)
    AssetType   AssetType    // 5 类枚举
    Industry    Industry     // 5 行业枚举
    Name        string
    Version     string
    Source      AssetSource  // ★ 来源标识
    IsActive    bool
    Data        []byte       // 资产 JSON 数据
    PurchaseID  *int64
    PurchasedAt *time.Time
    SyncedAt    time.Time
    UpdatedAt   time.Time
}

// AssetType 5 类资产(强类型)
type AssetType string

const (
    AssetTypeAgentPersona    AssetType = "agent_persona"
    AssetTypeSalesScript     AssetType = "sales_script"
    AssetTypeABTestPlan      AssetType = "ab_test_plan"
    AssetTypeMarketingFlow   AssetType = "marketing_workflow"
    AssetTypeIndustrySOP     AssetType = "industry_sop"
)

func (t AssetType) Valid() bool {
    switch t {
    case AssetTypeAgentPersona, AssetTypeSalesScript,
        AssetTypeABTestPlan, AssetTypeMarketingFlow, AssetTypeIndustrySOP:
        return true
    }
    return false
}

func (t AssetType) Label() string {
    return map[AssetType]string{
        AssetTypeAgentPersona:  "智能体角色",
        AssetTypeSalesScript:   "销冠话术",
        AssetTypeABTestPlan:    "AB 测试方案",
        AssetTypeMarketingFlow: "自动化工作流",
        AssetTypeIndustrySOP:   "行业 SOP 模板",
    }[t]
}

// Industry 5 行业
type Industry string

const (
    IndustryMeiZhuang Industry = "美妆"
    IndustryJiaoPei   Industry = "教培"
    IndustryYiMei     Industry = "医美"
    IndustryQiChe     Industry = "汽车"
    IndustryJinRong   Industry = "金融"
)

func (i Industry) Valid() bool {
    switch i {
    case IndustryMeiZhuang, IndustryJiaoPei, IndustryYiMei, IndustryQiChe, IndustryJinRong:
        return true
    }
    return false
}

// AssetSource 来源
type AssetSource string

const (
    SourcePurchased AssetSource = "purchased"
    SourceManual    AssetSource = "manual"
    SourceSynced    AssetSource = "synced"
    SourceImported  AssetSource = "imported"
)

func (s AssetSource) Label() string {
    return map[AssetSource]string{
        SourcePurchased: "平台购买",
        SourceManual:    "自建",
        SourceSynced:    "平台分发",
        SourceImported:  "导入",
    }[s]
}

func (s AssetSource) IsFromPlatform() bool {
    return s == SourcePurchased || s == SourceSynced
}
```

### 4.3 Domain 校验层(`domain/asset/validator.go`)

```go
package asset

import (
    "encoding/json"
    "errors"
    "github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
    validate = validator.New()
}

// ValidateAssetData 校验资产 JSON 数据
// (5 类资产共用,按 asset_type 走不同 schema)
func ValidateAssetData(t AssetType, data []byte) error {
    var m map[string]interface{}
    if err := json.Unmarshal(data, &m); err != nil {
        return errors.New("资产 JSON 格式错误")
    }

    assetType, _ := m["asset_type"].(string)
    if AssetType(assetType) != t {
        return errors.New("asset_type 字段与路径不一致")
    }

    industry, _ := m["industry"].(string)
    if !Industry(industry).Valid() {
        return errors.New("industry 必须是 5 个行业之一")
    }

    // 按类型走具体校验
    switch t {
    case AssetTypeAgentPersona:
        return validateAgentPersona(m)
    case AssetTypeSalesScript:
        return validateSalesScript(m)
    case AssetTypeABTestPlan:
        return validateABTestPlan(m)
    case AssetTypeMarketingFlow:
        return validateMarketingFlow(m)
    case AssetTypeIndustrySOP:
        return validateIndustrySOP(m)
    }
    return errors.New("未知资产类型")
}

func validateAgentPersona(m map[string]interface{}) error {
    // 必填字段、类型、长度等
    required := []string{"schema_version", "name", "system_prompt", "persona"}
    for _, k := range required {
        if _, ok := m[k]; !ok {
            return errors.New("缺少必填字段: " + k)
        }
    }
    return nil
}
// validateSalesScript / validateABTestPlan / validateMarketingFlow / validateIndustrySOP 同模式
```

### 4.4 Repository 层(`repository/local_asset_repo.go`)

```go
package repository

import (
    "context"
    "gorm.io/gorm"
    "hivemtk/user-server/internal/model"
)

// LocalAssetRepository 严格抽象接口
type LocalAssetRepository interface {
    Create(ctx context.Context, m *model.LocalAsset) error
    Update(ctx context.Context, m *model.LocalAsset) error
    SoftDelete(ctx context.Context, id int64) error
    FindByID(ctx context.Context, id int64) (*model.LocalAsset, error)
    FindByAssetID(ctx context.Context, assetID string) (*model.LocalAsset, error)
    List(ctx context.Context, filter LocalAssetFilter) ([]*model.LocalAsset, int64, error)
    ToggleActive(ctx context.Context, id int64, active bool) error
    ListByTypeAndActive(ctx context.Context, assetType string) ([]*model.LocalAsset, error)
}

type LocalAssetFilter struct {
    AssetType string
    Industry  string
    Source    string
    IsActive  *bool
    Keyword   string
    Page      int
    Size      int
}

type localAssetRepo struct {
    db *gorm.DB
}

func NewLocalAssetRepository(db *gorm.DB) LocalAssetRepository {
    return &localAssetRepo{db: db}
}

func (r *localAssetRepo) Create(ctx context.Context, m *model.LocalAsset) error {
    return r.db.WithContext(ctx).Create(m).Error
}

func (r *localAssetRepo) Update(ctx context.Context, m *model.LocalAsset) error {
    return r.db.WithContext(ctx).Save(m).Error
}

func (r *localAssetRepo) SoftDelete(ctx context.Context, id int64) error {
    return r.db.WithContext(ctx).Delete(&model.LocalAsset{}, id).Error
}

func (r *localAssetRepo) FindByID(ctx context.Context, id int64) (*model.LocalAsset, error) {
    var m model.LocalAsset
    err := r.db.WithContext(ctx).First(&m, id).Error
    return &m, err
}

func (r *localAssetRepo) FindByAssetID(ctx context.Context, assetID string) (*model.LocalAsset, error) {
    var m model.LocalAsset
    err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).First(&m).Error
    return &m, err
}

func (r *localAssetRepo) List(ctx context.Context, f LocalAssetFilter) ([]*model.LocalAsset, int64, error) {
    var list []*model.LocalAsset
    var total int64
    q := r.db.WithContext(ctx).Model(&model.LocalAsset{})
    if f.AssetType != "" { q = q.Where("asset_type = ?", f.AssetType) }
    if f.Industry != ""  { q = q.Where("industry = ?", f.Industry) }
    if f.Source != ""    { q = q.Where("source = ?", f.Source) }
    if f.IsActive != nil { q = q.Where("is_active = ?", *f.IsActive) }
    if f.Keyword != ""   { q = q.Where("name ILIKE ?", "%"+f.Keyword+"%") }
    q.Count(&total)
    err := q.Order("synced_at DESC").Offset((f.Page - 1) * f.Size).Limit(f.Size).Find(&list).Error
    return list, total, err
}

func (r *localAssetRepo) ToggleActive(ctx context.Context, id int64, active bool) error {
    return r.db.WithContext(ctx).Model(&model.LocalAsset{}).
        Where("id = ?", id).
        Update("is_active", active).Error
}

func (r *localAssetRepo) ListByTypeAndActive(ctx context.Context, assetType string) ([]*model.LocalAsset, error) {
    var list []*model.LocalAsset
    err := r.db.WithContext(ctx).Where("asset_type = ? AND is_active = ?", assetType, true).Find(&list).Error
    return list, err
}
```

### 4.5 错误码(`domain/errors/codes.go`)

```go
package errors

const (
    CodeSuccess         = 0
    CodeParamInvalid    = 4000
    CodeUnauthorized    = 4001
    CodeForbidden       = 4003
    CodeNotFound        = 4004
    CodeConflict        = 4009
    CodeInternal        = 5000
    CodePlatformUnavail = 5001
    CodeAssetNotFound   = 6001
    CodeAssetDup        = 6002
    CodeAssetInvalid    = 6003  // JSON 校验失败
    CodeSyncFailed      = 6004
    CodeLoaderFallback  = 6005  // Loader 回退(非错,日志)
)

type BizError struct {
    Code    int
    Message string
    Cause   error
}

func (e *BizError) Error() string {
    if e.Cause != nil { return e.Message + ": " + e.Cause.Error() }
    return e.Message
}

func (e *BizError) Unwrap() error { return e.Cause }

func New(code int, msg string) *BizError {
    return &BizError{Code: code, Message: msg}
}

func Wrap(code int, msg string, cause error) *BizError {
    return &BizError{Code: code, Message: msg, Cause: cause}
}
```

---

## 五、统一 CRUD 服务(★关键:同源同构)

文件:`hivemtk/user-server/internal/service/local_asset_service.go`

> **核心思想**:**平台购买**与**商户自建**走**同一组 CRUD 方法**。
> 业务层只关心"是什么资产",不关心"从哪来"。

```go
package service

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "hivemtk/user-server/internal/domain/asset"
    bizerr "hivemtk/user-server/internal/domain/errors"
    "hivemtk/user-server/internal/model"
    "hivemtk/user-server/internal/repository"
)

type LocalAssetService struct {
    assetRepo      repository.LocalAssetRepository
    dataRepo       repository.LocalAssetDataRepository
    syncLogRepo    repository.SyncLogRepository
    platformClient repository.PlatformAPIClient
    db             *gorm.DB
}

func NewLocalAssetService(
    ar repository.LocalAssetRepository,
    dr repository.LocalAssetDataRepository,
    sr repository.SyncLogRepository,
    pc repository.PlatformAPIClient,
    db *gorm.DB,
) *LocalAssetService {
    return &LocalAssetService{assetRepo: ar, dataRepo: dr, syncLogRepo: sr, platformClient: pc, db: db}
}

// ==================== 平台购买(来源=SourcePurchased) ====================

// PurchaseAndSync 平台购买 + 同步到本地(原子事务)
func (s *LocalAssetService) PurchaseAndSync(ctx context.Context, platformAssetID string) error {
    // 1. 检查本地是否已存在
    existing, err := s.assetRepo.FindByAssetID(ctx, platformAssetID)
    if err == nil && existing != nil {
        return bizerr.New(bizerr.CodeAssetDup, "资产已存在,请直接点击「同步到最新版本」")
    }

    // 2. 调用平台端购买
    if err := s.platformClient.Purchase(ctx, platformAssetID); err != nil {
        return bizerr.Wrap(bizerr.CodePlatformUnavail, "平台购买失败", err)
    }

    // 3. 拉取最新数据
    payload, err := s.platformClient.PullData(ctx, platformAssetID)
    if err != nil {
        return bizerr.Wrap(bizerr.CodeSyncFailed, "拉取平台数据失败", err)
    }

    // 4. 校验
    if err := asset.ValidateAssetData(asset.AssetType(payload.AssetType), payload.Data); err != nil {
        return bizerr.Wrap(bizerr.CodeAssetInvalid, "平台数据校验失败", err)
    }

    // 5. 落本地(事务)
    return s.db.Transaction(func(tx *gorm.DB) error {
        la := &model.LocalAsset{
            AssetID:     payload.AssetID,
            AssetType:   payload.AssetType,
            Industry:    payload.Industry,
            Name:        payload.Name,
            Version:     payload.Version,
            Source:      model.AssetSourcePurchased,  // ★ 标识来源
            IsActive:    true,
            PurchaseID:  &payload.PurchaseID,
            PurchasedAt: &payload.PurchasedAt,
            SyncedAt:    time.Now(),
        }
        if err := tx.Create(la).Error; err != nil {
            return bizerr.Wrap(bizerr.CodeInternal, "保存资产主表失败", err)
        }
        lad := &model.LocalAssetData{LocalAssetID: la.ID, Data: payload.Data, UpdatedAt: time.Now()}
        if err := tx.Create(lad).Error; err != nil {
            return bizerr.Wrap(bizerr.CodeInternal, "保存资产数据失败", err)
        }
        // 同步日志
        tx.Create(&model.LocalAssetSyncLog{AssetID: la.AssetID, Action: "purchase_sync", Status: "success"})
        return nil
    })
}

// SyncFromPlatform 仅同步(已购买的资产拉最新版本)
func (s *LocalAssetService) SyncFromPlatform(ctx context.Context, assetID string) error {
    la, err := s.assetRepo.FindByAssetID(ctx, assetID)
    if err != nil {
        return bizerr.Wrap(bizerr.CodeAssetNotFound, "资产不存在", err)
    }
    if la.Source != model.AssetSourcePurchased && la.Source != model.AssetSourceSynced {
        return bizerr.New(bizerr.CodeForbidden, "仅平台来源资产支持同步")
    }

    payload, err := s.platformClient.PullData(ctx, assetID)
    if err != nil {
        s.syncLogRepo.Create(ctx, &model.LocalAssetSyncLog{AssetID: assetID, Action: "sync", Status: "failed", ErrorMsg: err.Error()})
        return bizerr.Wrap(bizerr.CodeSyncFailed, "拉取失败", err)
    }

    return s.db.Transaction(func(tx *gorm.DB) error {
        la.Version = payload.Version
        la.SyncedAt = time.Now()
        if err := tx.Save(la).Error; err != nil { return err }
        // upsert data
        return tx.Exec(`
            INSERT INTO local_asset_data (local_asset_id, data, updated_at)
            VALUES (?, ?, NOW())
            ON CONFLICT (local_asset_id) DO UPDATE SET data = ?, updated_at = NOW()
        `, la.ID, payload.Data, payload.Data).Error
    })
}

// ==================== 商户自建(来源=SourceManual) ====================

// CreateManual 商户自建资产
func (s *LocalAssetService) CreateManual(ctx context.Context, in *CreateAssetInput) (*model.LocalAsset, error) {
    // 1. 校验
    if !asset.AssetType(in.AssetType).Valid() {
        return nil, bizerr.New(bizerr.CodeParamInvalid, "asset_type 非法")
    }
    if !asset.Industry(in.Industry).Valid() {
        return nil, bizerr.New(bizerr.CodeParamInvalid, "industry 必须是 5 行业之一")
    }
    if err := asset.ValidateAssetData(asset.AssetType(in.AssetType), in.Data); err != nil {
        return nil, bizerr.Wrap(bizerr.CodeAssetInvalid, "资产 JSON 校验失败", err)
    }

    // 2. 生成业务 ID(自建用 UUID)
    if in.AssetID == "" {
        in.AssetID = "manual-" + uuid.NewString()
    }
    // 3. 重名检查
    if existing, _ := s.assetRepo.FindByAssetID(ctx, in.AssetID); existing != nil {
        return nil, bizerr.New(bizerr.CodeAssetDup, "asset_id 已存在")
    }

    // 4. 落库
    la := &model.LocalAsset{
        AssetID:   in.AssetID,
        AssetType: in.AssetType,
        Industry:  in.Industry,
        Name:      in.Name,
        Version:   "1.0.0",
        Source:    model.AssetSourceManual,  // ★ 标识来源
        IsActive:  true,
        SyncedAt:  time.Now(),
    }
    if err := s.assetRepo.Create(ctx, la); err != nil {
        return nil, bizerr.Wrap(bizerr.CodeInternal, "创建资产失败", err)
    }
    lad := &model.LocalAssetData{LocalAssetID: la.ID, Data: in.Data, UpdatedAt: time.Now()}
    if err := s.dataRepo.Create(ctx, lad); err != nil {
        return nil, bizerr.Wrap(bizerr.CodeInternal, "保存资产数据失败", err)
    }
    return la, nil
}

// ==================== 通用 CRUD(同源同构 ★关键) ====================

// Update 编辑(平台/自建都走这里)
func (s *LocalAssetService) Update(ctx context.Context, id int64, in *UpdateAssetInput) error {
    la, err := s.assetRepo.FindByID(ctx, id)
    if err != nil {
        return bizerr.Wrap(bizerr.CodeAssetNotFound, "资产不存在", err)
    }
    if !asset.AssetType(in.AssetType).Valid() {
        return bizerr.New(bizerr.CodeParamInvalid, "asset_type 非法")
    }
    if err := asset.ValidateAssetData(asset.AssetType(in.AssetType), in.Data); err != nil {
        return bizerr.Wrap(bizerr.CodeAssetInvalid, "资产 JSON 校验失败", err)
    }

    return s.db.Transaction(func(tx *gorm.DB) error {
        la.Name = in.Name
        la.AssetType = in.AssetType
        la.Industry = in.Industry
        la.UpdatedAt = time.Now()
        if err := tx.Save(la).Error; err != nil { return err }
        // upsert data
        return tx.Exec(`
            INSERT INTO local_asset_data (local_asset_id, data, updated_at)
            VALUES (?, ?, NOW())
            ON CONFLICT (local_asset_id) DO UPDATE SET data = ?, updated_at = NOW()
        `, la.ID, in.Data, in.Data).Error
    })
}

// List 通用列表(同源同构,只是查询条件变化)
func (s *LocalAssetService) List(ctx context.Context, f repository.LocalAssetFilter) ([]*model.LocalAsset, int64, error) {
    return s.assetRepo.List(ctx, f)
}

// Get 详情(含 data)
func (s *LocalAssetService) Get(ctx context.Context, id int64) (*model.LocalAsset, []byte, error) {
    la, err := s.assetRepo.FindByID(ctx, id)
    if err != nil { return nil, nil, err }
    lad, err := s.dataRepo.FindByLocalAssetID(ctx, la.ID)
    if err != nil { return la, nil, err }
    return la, lad.Data, nil
}

// SoftDelete 软删(平台/自建都走这里)
func (s *LocalAssetService) SoftDelete(ctx context.Context, id int64) error {
    return s.assetRepo.SoftDelete(ctx, id)
}

// ToggleActive 启停(平台/自建都走这里)
func (s *LocalAssetService) ToggleActive(ctx context.Context, id int64, active bool) error {
    return s.assetRepo.ToggleActive(ctx, id, active)
}

// ==================== Loader 加载层(消费方,无差别) ====================

// LoadByType Loader 调这个方法,无差别加载所有来源
func (s *LocalAssetService) LoadByType(ctx context.Context, assetType string) ([]*model.LocalAsset, [][]byte, error) {
    list, err := s.assetRepo.ListByTypeAndActive(ctx, assetType)
    if err != nil { return nil, nil, err }
    var datas [][]byte
    for _, la := range list {
        lad, err := s.dataRepo.FindByLocalAssetID(ctx, la.ID)
        if err == nil { datas = append(datas, lad.Data) }
    }
    return list, datas, nil
}

// ==================== DTO(输入参数) ====================

type CreateAssetInput struct {
    AssetID   string // 可选,自建可自动生成
    AssetType string
    Industry  string
    Name      string
    Data      []byte
}

type UpdateAssetInput struct {
    AssetType string
    Industry  string
    Name      string
    Data      []byte
}
```

---

## 六、Handler 层(同源同构 API)

文件:`hivemtk/user-server/internal/handler/asset_market_handler.go`

> **关键**:所有 API 都是**通用的**——不区分"购买"和"自建",**根据请求参数自动识别行为**。

```go
package handler

import (
    "errors"
    "io"
    "net/http"
    "strconv"
    "github.com/gin-gonic/gin"
    "hivemtk/user-server/internal/service"
    bizerr "hivemtk/user-server/internal/domain/errors"
)

type AssetMarketHandler struct {
    localSvc *service.LocalAssetService
    marketSvc *service.AssetMarketService
}

func NewAssetMarketHandler(ls *service.LocalAssetService, ms *service.AssetMarketService) *AssetMarketHandler {
    return &AssetMarketHandler{localSvc: ls, marketSvc: ms}
}

// 统一响应
func ok(c *gin.Context, data interface{}) {
    c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func fail(c *gin.Context, err error) {
    var be *bizerr.BizError
    if errors.As(err, &be) {
        c.JSON(http.StatusOK, gin.H{"code": be.Code, "message": be.Message})
        return
    }
    c.JSON(http.StatusInternalServerError, gin.H{"code": 5000, "message": err.Error()})
}

// ============= 资产市场(平台端) =============

// GET /api/v1/asset-market/list
func (h *AssetMarketHandler) ListMarket(c *gin.Context) {
    list, total, err := h.marketSvc.ListMarketAssets(c.Request.Context(),
        c.Query("asset_type"), c.Query("industry"),
        atoi(c.DefaultQuery("page", "1")), atoi(c.DefaultQuery("size", "20")))
    if err != nil { fail(c, err); return }
    ok(c, gin.H{"list": list, "total": total})
}

// GET /api/v1/asset-market/detail/:asset_id
func (h *AssetMarketHandler) MarketDetail(c *gin.Context) {
    detail, err := h.marketSvc.GetMarketAssetDetail(c.Request.Context(), c.Param("asset_id"))
    if err != nil { fail(c, err); return }
    ok(c, detail)
}

// POST /api/v1/asset-market/purchase  (平台购买 + 同步,一气呵成)
func (h *AssetMarketHandler) Purchase(c *gin.Context) {
    var body struct{ AssetID string `json:"asset_id" binding:"required"` }
    if err := c.ShouldBindJSON(&body); err != nil { fail(c, bizerr.New(bizerr.CodeParamInvalid, "参数错误")); return }
    if err := h.localSvc.PurchaseAndSync(c.Request.Context(), body.AssetID); err != nil { fail(c, err); return }
    ok(c, gin.H{"message": "购买并同步成功"})
}

// POST /api/v1/asset-market/sync  (仅平台来源)
func (h *AssetMarketHandler) Sync(c *gin.Context) {
    var body struct{ AssetID string `json:"asset_id" binding:"required"` }
    if err := c.ShouldBindJSON(&body); err != nil { fail(c, bizerr.New(bizerr.CodeParamInvalid, "参数错误")); return }
    if err := h.localSvc.SyncFromPlatform(c.Request.Context(), body.AssetID); err != nil { fail(c, err); return }
    ok(c, gin.H{"message": "同步成功"})
}

// ============= 本地资产 CRUD(★同源同构,平台/自建 同一组 API) =============

// GET /api/v1/local-assets
func (h *AssetMarketHandler) ListLocal(c *gin.Context) {
    list, total, err := h.localSvc.List(c.Request.Context(), service.LocalAssetFilter{
        AssetType: c.Query("asset_type"),
        Industry:  c.Query("industry"),
        Source:    c.Query("source"),       // 可选过滤: purchased / manual
        Keyword:   c.Query("keyword"),
        Page:      atoi(c.DefaultQuery("page", "1")),
        Size:      atoi(c.DefaultQuery("size", "20")),
    })
    if err != nil { fail(c, err); return }
    ok(c, gin.H{"list": list, "total": total})
}

// GET /api/v1/local-assets/:id
func (h *AssetMarketHandler) GetLocal(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    la, data, err := h.localSvc.Get(c.Request.Context(), id)
    if err != nil { fail(c, err); return }
    ok(c, gin.H{"asset": la, "data": data})
}

// POST /api/v1/local-assets  (商户自建)
func (h *AssetMarketHandler) CreateLocal(c *gin.Context) {
    body, _ := io.ReadAll(c.Request.Body)
    var in service.CreateAssetInput
    if err := bindAndValidate(body, &in); err != nil { fail(c, err); return }
    la, err := h.localSvc.CreateManual(c.Request.Context(), &in)
    if err != nil { fail(c, err); return }
    ok(c, la)
}

// PUT /api/v1/local-assets/:id  (编辑,平台/自建 都可)
func (h *AssetMarketHandler) UpdateLocal(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    body, _ := io.ReadAll(c.Request.Body)
    var in service.UpdateAssetInput
    if err := bindAndValidate(body, &in); err != nil { fail(c, err); return }
    if err := h.localSvc.Update(c.Request.Context(), id, &in); err != nil { fail(c, err); return }
    ok(c, gin.H{"message": "更新成功"})
}

// DELETE /api/v1/local-assets/:id  (软删,平台/自建 都可)
func (h *AssetMarketHandler) DeleteLocal(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    if err := h.localSvc.SoftDelete(c.Request.Context(), id); err != nil { fail(c, err); return }
    ok(c, gin.H{"message": "删除成功"})
}

// PUT /api/v1/local-assets/:id/toggle-active
func (h *AssetMarketHandler) ToggleActive(c *gin.Context) {
    id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    var body struct{ Active bool `json:"active"` }
    if err := c.ShouldBindJSON(&body); err != nil { fail(c, err); return }
    if err := h.localSvc.ToggleActive(c.Request.Context(), id, body.Active); err != nil { fail(c, err); return }
    ok(c, gin.H{"message": "操作成功"})
}

// GET /api/v1/local-assets/sync-log
func (h *AssetMarketHandler) SyncLog(c *gin.Context) {
    list, err := h.localSvc.GetSyncLog(c.Request.Context(),
        c.Query("asset_id"), atoi(c.DefaultQuery("limit", "50")))
    if err != nil { fail(c, err); return }
    ok(c, list)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }
func bindAndValidate(body []byte, in interface{}) error {
    if err := json.Unmarshal(body, in); err != nil { return bizerr.New(bizerr.CodeParamInvalid, "JSON 解析失败") }
    return nil
}
```

路由注册(`router/router.go` 增量):

```go
// 平台端(市场)
market := apiV1.Group("/asset-market")
{
    market.GET("/list",          h.ListMarket)
    market.GET("/detail/:id",    h.MarketDetail)
    market.POST("/purchase",     h.Purchase)
    market.POST("/sync",         h.Sync)
}

// 本地(同源同构 ★)
local := apiV1.Group("/local-assets")
{
    local.GET("",                  h.ListLocal)
    local.GET("/sync-log",         h.SyncLog)
    local.GET("/:id",              h.GetLocal)
    local.POST("",                 h.CreateLocal)
    local.PUT("/:id",              h.UpdateLocal)
    local.DELETE("/:id",           h.DeleteLocal)
    local.PUT("/:id/toggle-active", h.ToggleActive)
}
```

---

## 五、关键架构改造:5 个 Loader

> **核心原则**:**业务代码改动最小**,只改"赋值来源",不改业务逻辑。
> **代码默认值仍在代码里**,保证没买资产时也能跑。
> **资产同步后自动激活**,无需重启代码(下次调用 loader 时自动拿到新值)。

### 5.1 通用 Loader 接口

```go
// internal/service/asset_loader_base.go
package service

import (
    "context"
    "encoding/json"
    "log"
    "gorm.io/gorm"
)

// LoadFromDB 通用 DB 加载器
// 返回:data(原始 JSON bytes),found(是否找到),err
func LoadAssetFromDB(db *gorm.DB, assetType, assetID string) ([]byte, bool, error) {
    var row struct {
        Data json.RawMessage
    }
    err := db.Table("local_assets la").
        Joins("JOIN local_asset_data lad ON lad.local_asset_id = la.id").
        Where("la.asset_id = ? AND la.asset_type = ? AND la.is_active = ?", assetID, assetType, true).
        Select("lad.data").
        Scan(&row).Error

    if err != nil {
        log.Printf("[Loader] DB error for %s/%s: %v, fallback to default", assetType, assetID, err)
        return nil, false, nil
    }
    if len(row.Data) == 0 {
        return nil, false, nil
    }
    return row.Data, true, nil
}
```

### 5.2 Loader 1:`agent_loader.go`(智能体角色)

```go
package service

import (
    "context"
    "encoding/json"
    "errors"
    "log"
    "gorm.io/gorm"
)

type AgentPersona struct {
    ID            string                 `json:"id"`
    Name          string                 `json:"name"`
    Industry      string                 `json:"industry"`
    SystemPrompt  string                 `json:"system_prompt"`
    Persona       map[string]interface{} `json:"persona"`
    GreetingTemplates []string           `json:"greeting_templates"`
    ObjectionHandlers []map[string]string `json:"objection_handlers"`
    ToolWhitelist []string               `json:"tool_whitelist"`
    KBRefs        []string               `json:"kb_refs"`
    DefaultTuning map[string]interface{} `json:"default_tuning"`
}

type AgentLoader struct{ db *gorm.DB }

func NewAgentLoader(db *gorm.DB) *AgentLoader { return &AgentLoader{db: db} }

// LoadPersona 优先 DB,失败回退代码默认
func (l *AgentLoader) LoadPersona(ctx context.Context, personaID string) (*AgentPersona, error) {
    // 1. 查 DB
    if data, found, _ := LoadAssetFromDB(l.db, "agent_persona", personaID); found {
        var p AgentPersona
        if err := json.Unmarshal(data, &p); err == nil {
            p.ID = personaID
            log.Printf("[AgentLoader] loaded from DB: %s", personaID)
            return &p, nil
        }
    }

    // 2. 回退代码默认
    log.Printf("[AgentLoader] fallback to default: %s", personaID)
    return loadDefaultAgentPersona(personaID)
}

// ListAllPersonas 列出所有人设(DB ∪ 代码默认)
func (l *AgentLoader) ListAllPersonas(ctx context.Context) ([]*AgentPersona, error) {
    var dbPersonas []*AgentPersona

    var rows []struct {
        AssetID string
        Name    string
        Data    json.RawMessage
    }
    l.db.Table("local_assets la").
        Joins("JOIN local_asset_data lad ON lad.local_asset_id = la.id").
        Where("la.asset_type = ? AND la.is_active = ?", "agent_persona", true).
        Select("la.asset_id, la.name, lad.data").
        Scan(&rows)
    for _, r := range rows {
        var p AgentPersona
        if err := json.Unmarshal(r.Data, &p); err == nil {
            p.ID = r.AssetID
            p.Name = r.Name
            dbPersonas = append(dbPersonas, &p)
        }
    }

    // 合并代码默认
    seen := map[string]bool{}
    for _, p := range dbPersonas { seen[p.ID] = true }
    for id, p := range defaultAgentPersonas() {
        if !seen[id] { dbPersonas = append(dbPersonas, p) }
    }
    return dbPersonas, nil
}

// loadDefaultAgentPersona 代码默认(从原 service/agent/agent.go 迁移过来)
func loadDefaultAgentPersona(personaID string) (*AgentPersona, error) {
    defaults := defaultAgentPersonas()
    if p, ok := defaults[personaID]; ok { return p, nil }
    return nil, errors.New("agent persona not found: " + personaID)
}

func defaultAgentPersonas() map[string]*AgentPersona {
    return map[string]*AgentPersona{
        "default-sales": {
            ID: "default-sales",
            Name: "默认销售助手",
            SystemPrompt: "你是一位专业的销售助手,擅长倾听客户需求,提供准确的产品信息。",
            Persona: map[string]interface{}{
                "tone": "专业、热情",
                "expertise": []string{"产品咨询", "价格答疑"},
            },
            ToolWhitelist: []string{"query_product", "recommend_sku", "check_stock"},
            DefaultTuning: map[string]interface{}{
                "temperature": 0.7, "max_tokens": 512, "react_max_rounds": 5,
            },
        },
        "default-service": {
            ID: "default-service",
            Name: "默认客服",
            SystemPrompt: "你是一位专业的客服,耐心解答用户问题。",
            Persona: map[string]interface{}{
                "tone": "耐心、亲切",
                "expertise": []string{"售后问题", "使用指导"},
            },
            ToolWhitelist: []string{"query_order", "check_status"},
            DefaultTuning: map[string]interface{}{
                "temperature": 0.5, "max_tokens": 512, "react_max_rounds": 3,
            },
        },
    }
}
```

### 7.3 Loader 2-5:同样模式(脚本/AB/SOP/工作流)

**关键点统一**:

| Loader | DB 查询 | 代码默认位置(原文件) | 改造点 |
|---|---|---|---|
| `script_loader.go` | `local_assets WHERE type='sales_script'` | `service/auto_reply/scripts.go` | 销冠话术从 DB 读 step 列表 |
| `sop_loader.go` | `local_assets WHERE type='industry_sop'` | `service/sop_service.go` 的 `DefaultSOPs` map | SOP 步骤从 DB 读 |
| `abtest_loader.go` | `local_assets WHERE type='ab_test_plan'` | `service/sop_abtest.go` 的 `DefaultABTests` | AB 测试方案从 DB 读 |
| `workflow_loader.go` | `local_assets WHERE type='marketing_workflow'` | `service/marketing_flow.go` 的 `DefaultWorkflows` | 工作流从 DB 读 |

每个 Loader 提供:
- `LoadXXX(ctx, id) (*XXX, error)` — 单个加载
- `ListAllXXX(ctx) ([]*XXX, error)` — 全部加载(DB ∪ 默认)
- `loadDefaultXXX(id)` — 代码默认兜底
- `defaultXXXs()` — 全部代码默认

---

## 八、业务调用方改造(示例)

### 8.1 改造前 `internal/service/agent/agent.go`

```go
// 原代码:硬编码
const DefaultSystemPrompt = "你是一位销售助手..."
var DefaultPersonaConfig = map[string]interface{}{...}

func NewAgent() *Agent {
    return &Agent{
        SystemPrompt: DefaultSystemPrompt,   // ← 写死
        Persona: DefaultPersonaConfig,        // ← 写死
        // ...
    }
}
```

### 6.2 改造后 `internal/service/agent/agent.go`

```go
// 新代码:动态加载
type Agent struct {
    SystemPrompt  string
    Persona       map[string]interface{}
    ToolWhitelist []string
    DefaultTuning map[string]interface{}
}

// NewAgent 构造时从 loader 加载
func NewAgent(ctx context.Context, loader *service.AgentLoader, personaID string) (*Agent, error) {
    persona, err := loader.LoadPersona(ctx, personaID)
    if err != nil { return nil, err }

    return &Agent{
        SystemPrompt:  persona.SystemPrompt,
        Persona:       persona.Persona,
        ToolWhitelist: persona.ToolWhitelist,
        DefaultTuning: persona.DefaultTuning,
    }, nil
}
```

### 8.3 改造原则

1. **业务逻辑零改动**,只改"赋值来源"
2. **默认值保留在代码**,没买资产时自动用默认
3. **每次调用 loader 都查最新 DB**,无需重启即可生效
4. **DB 异常自动回退**,try-catch 默认值
5. **支持热切换**:用户可在 user-web 切换"使用哪个人设"

---

## 七、前端资产市场模块

### 7.1 路由(增量)

```js
// user-web/src/router/index.js
{
  path: 'asset-market',
  meta: { title: '资产市场', icon: 'Shop' },
  children: [
    { path: '',              component: () => import('@/views/assetMarket/Market.vue'),   meta: { title: '市场首页' } },
    { path: 'detail/:id',    component: () => import('@/views/assetMarket/Detail.vue'),   meta: { title: '资产详情' } },
    { path: 'my-assets',     component: () => import('@/views/assetMarket/MyAssets.vue'), meta: { title: '我的资产' } },
    { path: 'sync-log',      component: () => import('@/views/assetMarket/SyncLog.vue'),  meta: { title: '同步日志' } }
  ]
}
```

### 9.2 API 封装(同源同构)

```js
// user-web/src/api/assetMarket.js
import request from '@/utils/request'

export const listAssets      = (params) => request.get('/api/v1/asset-market/list', { params })
export const assetDetail     = (id)     => request.get(`/api/v1/asset-market/detail/${id}`)
export const purchaseAsset   = (data)   => request.post('/api/v1/asset-market/purchase', data)
export const syncAsset       = (data)   => request.post('/api/v1/asset-market/sync', data)
export const myAssets        = ()       => request.get('/api/v1/asset-market/my-purchases')
export const syncLog         = (params) => request.get('/api/v1/asset-market/sync-log', { params })
```

### 7.3 `Market.vue`(完整)

```vue
<template>
  <div class="asset-market">
    <el-card class="filter-bar">
      <el-form inline>
        <el-form-item label="类型">
          <el-select v-model="filter.asset_type" placeholder="全部" clearable>
            <el-option label="智能体角色"     value="agent_persona" />
            <el-option label="销冠话术"       value="sales_script" />
            <el-option label="AB 测试方案"    value="ab_test_plan" />
            <el-option label="自动化工作流"   value="marketing_workflow" />
            <el-option label="行业 SOP"       value="industry_sop" />
          </el-select>
        </el-form-item>
        <el-form-item label="行业">
          <el-select v-model="filter.industry" placeholder="全部" clearable>
            <el-option v-for="i in industries" :key="i" :label="i" :value="i" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="fetchList">查询</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <div class="asset-grid" v-loading="loading">
      <el-card v-for="a in list" :key="a.asset_id" class="asset-card" shadow="hover"
               @click="$router.push(`/asset-market/detail/${a.asset_id}`)">
        <div class="cover-wrap">
          <img v-if="a.cover_url" :src="a.cover_url" class="cover" />
          <div v-else class="cover-placeholder">{{ typeLabel(a.asset_type)[0] }}</div>
        </div>
        <h3 class="title">{{ a.name }}</h3>
        <div class="tags">
          <el-tag size="small" effect="plain">{{ a.industry }}</el-tag>
          <el-tag size="small" :type="typeColor(a.asset_type)">{{ typeLabel(a.asset_type) }}</el-tag>
        </div>
        <p class="desc">{{ a.description }}</p>
        <div class="footer">
          <div class="meta">
            <el-rate v-model="a.rating_avg" disabled size="small" />
            <span class="downloads">↓ {{ a.download_count }}</span>
          </div>
          <el-tag v-if="a.purchased" type="success" size="small">已购</el-tag>
          <el-button v-else type="primary" size="small" @click.stop="handlePurchase(a)">免费试用</el-button>
        </div>
      </el-card>
    </div>

    <el-pagination
      v-model:current-page="filter.page"
      v-model:page-size="filter.size"
      :total="total"
      layout="total, prev, pager, next"
      @current-change="fetchList"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { listAssets, purchaseAsset } from '@/api/assetMarket'
import { ElMessage, ElMessageBox } from 'element-plus'

const industries = ['美妆', '教培', '医美', '汽车', '金融']
const filter = ref({ asset_type: '', industry: '', page: 1, size: 20 })
const list = ref([])
const total = ref(0)
const loading = ref(false)

const typeLabel = (t) => ({
  agent_persona: '智能体角色',
  sales_script: '销冠话术',
  ab_test_plan: 'AB 测试',
  marketing_workflow: '工作流',
  industry_sop: '行业 SOP'
}[t] || t)

const typeColor = (t) => ({
  agent_persona: '',
  sales_script: 'success',
  ab_test_plan: 'warning',
  marketing_workflow: 'info',
  industry_sop: 'danger'
}[t] || '')

const fetchList = async () => {
  loading.value = true
  try {
    const resp = await listAssets(filter.value)
    list.value = resp.data || []
    total.value = resp.total || 0
  } finally { loading.value = false }
}

const handlePurchase = async (asset) => {
  try {
    await ElMessageBox.confirm(
      `确认「免费试用」资产「${asset.name}」?试用后将自动同步到本地数据库。`,
      '免费试用', { type: 'info' }
    )
  } catch { return }
  await purchaseAsset({ asset_id: asset.asset_id })
  ElMessage.success('试用成功,请到"我的资产"中点击"同步到本地"')
  fetchList()
}

onMounted(fetchList)
</script>

<style scoped lang="scss">
.asset-market {
  padding: 16px;
  .filter-bar { margin-bottom: 16px; }
  .asset-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: 16px;
  }
  .asset-card {
    cursor: pointer;
    transition: transform 0.2s;
    &:hover { transform: translateY(-4px); }
    .cover-wrap {
      height: 140px;
      overflow: hidden;
      border-radius: 4px;
      margin-bottom: 12px;
      .cover { width: 100%; height: 100%; object-fit: cover; }
      .cover-placeholder {
        width: 100%; height: 100%;
        display: flex; align-items: center; justify-content: center;
        background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        color: white; font-size: 48px; font-weight: bold;
      }
    }
    .title { font-size: 16px; margin: 0 0 8px; }
    .tags { margin-bottom: 8px; .el-tag { margin-right: 4px; } }
    .desc { color: #666; font-size: 13px; min-height: 40px; margin: 0 0 12px; }
    .footer { display: flex; justify-content: space-between; align-items: center; }
    .meta { display: flex; align-items: center; gap: 8px; color: #999; font-size: 12px; }
  }
  .el-pagination { margin-top: 16px; justify-content: center; }
}
</style>
```

### 7.4 `Detail.vue` / `MyAssets.vue` / `SyncLog.vue` 概要

#### Detail.vue
- 资产详情页:封面/名称/描述/类型/行业/作者/版本/SHA256
- JSON 预览(monaco-editor 或简单 `<pre>`)
- "免费试用"按钮
- "查看文档"链接(如果有)

#### MyAssets.vue
- 我的已购列表
- 列:资产名/类型/行业/版本/购买时间/同步时间/状态(已同步/未同步)
- 操作:同步到本地/查看/删除/停用
- 空状态:提示去资产市场浏览

#### SyncLog.vue
- 同步日志列表
- 列:时间/资产/操作/状态/错误信息
- 筛选:按资产 ID、按状态

---

## 十、配置 & 环境变量

文件:`hivemtk/user-server/configs/config.yaml`(增量)

```yaml
asset_market:
  enabled: true
  platform_api:
    base_url: ${PLATFORM_API_BASE_URL:http://platform-server:8083}
    app_key: ${PLATFORM_APP_KEY:default-key}
    app_secret: ${PLATFORM_APP_SECRET:default-secret}
    timeout_seconds: 30
  cache:
    list_ttl_seconds: 300      # 列表缓存 5 分钟
  log:
    enable: true
```

---

## 十一、单元测试(关键)

文件:`hivemtk/user-server/internal/service/agent_loader_test.go`

```go
package service_test

import (
    "context"
    "encoding/json"
    "testing"
    "github.com/DATA-DOG/go-sqlmock"
    "github.com/stretchr/testify/assert"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    "hivemtk/user-server/internal/service"
)

func TestAgentLoader_LoadPersona_FromDB(t *testing.T) {
    // 模拟 DB 返回资产数据
    // 验证 loader 优先使用 DB
}

func TestAgentLoader_LoadPersona_FallbackToDefault(t *testing.T) {
    // 模拟 DB 无数据
    // 验证 loader 自动回退代码默认
}

func TestAgentLoader_LoadPersona_DBError_FallbackToDefault(t *testing.T) {
    // 模拟 DB 异常
    // 验证 loader 自动回退代码默认(不报错)
}

func TestAgentLoader_LoadPersona_InvalidJSON_FallbackToDefault(t *testing.T) {
    // 模拟 DB 有数据但 JSON 损坏
    // 验证 loader 自动回退代码默认
}
```

---

## 十二、迁移 & 升级保障

### 10.1 老用户升级

**场景**:已有部署的商户升级到新版本。

**保障**:
1. **migration 自动创建新表**(`local_assets` / `local_asset_data` / `local_asset_sync_log`),不影响老数据
2. **5 个 Loader 在 DB 无数据时自动用代码默认**,所有老业务 0 改动
3. **业务调用方只改"赋值来源"**,逻辑不变,回归测试覆盖即可
4. **逐步灰度**:可加 `ASSET_LOADER_ENABLED` 开关,默认 true,异常时切回代码默认

### 10.2 兜底优先级

```
Loader 加载资产时:
1. local_assets WHERE asset_id = ? AND is_active = true  → 用 DB
2. local_assets 查询异常 / 无数据 / JSON 损坏             → 用代码默认
3. 代码默认也不存在                                     → 返回 error(上层处理)
```

### 12.3 性能影响

- DB 查询加了索引(asset_id、type、is_active),毫秒级
- 加载结果可加内存缓存(5 分钟,资产变更时主动失效)
- 业务调用频次高的 Loader(agent_loader)建议加 sync.Map 缓存

---

## 十三、风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| 资产 JSON 损坏导致业务崩溃 | 高 | 加载器 try-catch,损坏自动回退默认 |
| 老用户升级后 Loader 找不到表 | 中 | migration 自动创建 + 加载前检查表存在 |
| 业务代码改动引发回归 | 中高 | 代码默认兜底 + 单元测试覆盖 + 灰度发布 |
| 用户删除资产后业务报错 | 中 | Loader 优先用 DB,删了自动回退默认 |
| 平台端不可达 | 中 | 资产市场功能降级为"浏览本地已购",不阻断核心业务 |

---

## 十二、关键里程碑

| 里程碑 | 时间 | 验收标准 |
|---|---|---|
| M1:用户端资产市场 API + 前端 | Week 4 | 可浏览平台端资产,可免费试用,可同步到本地 |
| M2:5 个 Loader 改造完成 | Week 6 | DB 有数据用 DB,无数据用默认,异常自动回退 |
| M3:25 个演示资产同步 | Week 6 | 5 行业 × 5 类全部可购买可同步 |
| M4:端到端联调通过 | Week 7 | 上传 → 审核 → 购买 → 同步 → 重启 → 业务生效 |
| M5:回归测试通过 | Week 8 | 老用户无资产也能正常运行,新用户可加载资产 |

---

## 十五、一句话总结

> **用户端 = 100% MIT 开源 + 加一个"资产市场"菜单 + 5 个 Loader 把代码常量改为从 DB 加载(代码默认兜底)**;**前期商户免费试用**,**25 个演示资产作为冷启动**,**老用户升级 0 感知**。

---

## 十六、五层架构实施标准(★强制)

### 16.1 严格分层规则

| 层 | 路径 | 允许依赖 | 禁止 |
|---|---|---|---|
| Handler | `internal/handler/` | Service | ❌ 直接调 Repository / Model / GORM |
| Service | `internal/service/` | Domain, Repository(接口) | ❌ 直接调 GORM、❌ 跨层调 Handler |
| Domain | `internal/domain/` | (无) | ❌ 任何外部依赖(GORM/JSON/HTTP 都不行) |
| Repository | `internal/repository/` | Model, GORM | ❌ 业务逻辑、❌ HTTP/JSON |
| Model | `internal/model/` | GORM, JSON tag | ❌ 业务逻辑、❌ 跨实体操作 |

> **关键**:**GORM 只允许出现在 Repository 层和 Model 层**。Service 层通过接口依赖 Repository。

### 16.2 依赖注入(`internal/di/wire.go`)

```go
package di

import (
    "gorm.io/gorm"
    "hivemtk/user-server/internal/handler"
    "hivemtk/user-server/internal/repository"
    "hivemtk/user-server/internal/service"
)

type Container struct {
    DB                *gorm.DB
    LocalAssetHandler *handler.AssetMarketHandler
    AssetMarketService *service.AssetMarketService
    LocalAssetService  *service.LocalAssetService
}

func NewContainer(db *gorm.DB) *Container {
    // 1. Repository(底层先建)
    localAssetRepo := repository.NewLocalAssetRepository(db)
    dataRepo := repository.NewLocalAssetDataRepository(db)
    syncLogRepo := repository.NewSyncLogRepository(db)
    platformClient := repository.NewPlatformAPIClient(
        os.Getenv("PLATFORM_API_BASE_URL"),
        os.Getenv("PLATFORM_APP_KEY"),
        os.Getenv("PLATFORM_APP_SECRET"),
    )

    // 2. Service
    marketSvc := service.NewAssetMarketService(platformClient, syncLogRepo)
    localSvc := service.NewLocalAssetService(localAssetRepo, dataRepo, syncLogRepo, platformClient, db)

    // 3. Handler
    h := handler.NewAssetMarketHandler(localSvc, marketSvc)

    return &Container{
        DB:                db,
        LocalAssetHandler: h,
        AssetMarketService: marketSvc,
        LocalAssetService:  localSvc,
    }
}
```

### 16.3 单向依赖检查(强制)

```bash
# 禁止反向依赖检查脚本 (scripts/check-deps.sh)
#!/bin/bash
echo "检查 Handler 不能依赖 Repository/Model..."
grep -r "repository\." internal/handler/ && exit 1
grep -r "model\." internal/handler/ && exit 1

echo "检查 Service 不能直接调 GORM..."
grep -r "\.Debug()\|\.Session(&" internal/service/ && exit 1
grep -r "gorm\." internal/service/ && exit 1

echo "检查 Domain 不能依赖外部..."
grep -r "gorm\." internal/domain/ && exit 1
grep -r "http\." internal/domain/ && exit 1
grep -r "gin\." internal/domain/ && exit 1

echo "通过 ✓"
```

CI 流水线必须执行此检查,失败则拒绝合并。

### 16.4 分层代码模板

每个新功能必须按以下模板:

```go
// 1. model/entity.go - 第 5 层
type Xxx struct {
    ID int64 `gorm:"primaryKey"`
    // ...
}

// 2. domain/xxx/entity.go - 第 3 层
type XxxDomain struct {
    ID int64
    // 业务规则方法
    func (x *XxxDomain) Validate() error { ... }
}

// 3. repository/xxx_repo.go - 第 4 层
type XxxRepository interface {
    Create(ctx, *model.Xxx) error
    // ...
}

// 4. service/xxx_service.go - 第 2 层
type XxxService struct {
    repo repository.XxxRepository
}
func (s *XxxService) DoSomething(ctx) error { ... }

// 5. handler/xxx_handler.go - 第 1 层
func (h *XxxHandler) Handle(c *gin.Context) {
    s := h.service.DoSomething(...)
    // 格式化响应
}
```

---

## 十七、代码编码规范(★强制)

### 17.1 命名规范

| 类型 | 规范 | 示例 |
|---|---|---|
| 包名 | 小写,无下划线 | `asset`, `localasset` |
| 文件名 | 小写下划线 | `local_asset.go`, `agent_loader.go` |
| 类型 | PascalCase | `LocalAsset`, `AgentPersona` |
| 接口 | 行为命名 | `Repository`, `Loader` |
| 方法 | PascalCase / camelCase | `GetByID` / `getDefaultValue` |
| 变量 | camelCase | `assetID`, `userKey` |
| 常量 | PascalCase 或全大写下划线 | `CodeSuccess` / `MAX_RETRY_COUNT` |
| DB 表 | snake_case 复数 | `local_assets`, `sync_logs` |
| DB 字段 | snake_case | `asset_id`, `is_active` |
| JSON 字段 | snake_case | `"asset_id": "..."` |
| URL 路径 | kebab-case | `/api/v1/asset-market/list` |
| 路由分组 | 复数名词 | `/local-assets`, `/asset-market` |
| 环境变量 | UPPER_SNAKE | `PLATFORM_API_BASE_URL` |

> **严禁**:`v1` / `v2` / `ext` / `stub` / 时间戳后缀(如 `*_2026-07-22`)

### 17.2 错误处理规范

```go
// ❌ 错误示范
func (s *Service) DoX() error {
    if err := s.repo.Save(x); err != nil {
        return err  // 丢失上下文
    }
}

// ✅ 正确示范
func (s *Service) DoX(ctx context.Context) error {
    if err := s.repo.Save(ctx, x); err != nil {
        return bizerr.Wrap(bizerr.CodeInternal, "保存资产失败", err)
    }
    return nil
}

// 上层判断
if errors.Is(err, gorm.ErrRecordNotFound) { ... }
var be *bizerr.BizError
if errors.As(err, &be) {
    // be.Code / be.Message 可读
}
```

### 17.3 日志规范(使用 slog)

```go
import "log/slog"

slog.Info("资产购买成功",
    "asset_id", assetID,
    "asset_type", assetType,
    "merchant_key", merchantKey,
    "source", "purchased",
    "latency_ms", time.Since(start).Milliseconds(),
)

slog.Warn("资产 JSON 损坏,回退默认",
    "asset_id", assetID,
    "error", err.Error(),
    "fallback", "code_default",
)

slog.Error("平台端不可达",
    "asset_id", assetID,
    "endpoint", endpoint,
    "error", err.Error(),
)
```

**禁止**:
- ❌ `fmt.Println` 用于业务日志
- ❌ `log.Printf("...%v", err)` 不带字段
- ❌ 在 hot path 打 INFO 日志(用 DEBUG)

### 17.4 注释规范

```go
// Package service 实现资产业务逻辑
//
// 严格遵守五层架构:
// - 本包只依赖 Domain / Repository(接口)
// - 不允许直接调 GORM
// - 所有外部 IO 必须通过 context.Context 传递
package service

// LoadPersona 从 DB 加载智能体人设,失败回退代码默认
//
// 加载策略(三段式):
//  1. 查 local_assets(asset_type=agent_persona AND is_active=true)
//  2. 校验 JSON
//  3. 失败回退 loadDefaultAgentPersona(从代码常量)
//
// 返回的 AgentPersona.ID 一定是传入的 personaID,不会自动转换为 DB 中的 asset_id。
func (l *AgentLoader) LoadPersona(ctx context.Context, personaID string) (*AgentPersona, error) {
    // ...
}
```

### 17.5 测试规范

| 层 | 测试类型 | 覆盖率目标 | 关键用例 |
|---|---|---|---|
| Handler | HTTP 集成 | 80% | 4xx/5xx 响应、业务错误码 |
| Service | 单元测试 | 100% | 主路径 + 异常路径 + 边界 |
| Domain | 单元测试 | 100% | 校验规则、值对象方法 |
| Repository | 集成测试 | 80% | CRUD + 复杂查询 |
| Loader | 单元测试 | 100% | 4 象限:DB有/无/异常/JSON坏 |

### 17.6 Lint 与格式化

```bash
# Go
gofmt -w .
goimports -w .
golangci-lint run  # 启用 errcheck / govet / staticcheck

# 前端
npm run lint
npm run format  # prettier
```

### 17.7 Git Commit 规范

```
feat(asset): 新增资产市场同源同构 CRUD
fix(loader): 修复 JSON 损坏导致业务崩溃
refactor(repo): 抽离 LocalAssetRepository 接口
docs(design): 更新 ASSET_MARKET_INTEGRATION 同源同构章节
test(loader): 补充 4 象限单元测试
chore(deps): 升级 uuid 依赖到 v1.6.0
```

---

## 十八、同源同构 UI 复用设计(★关键)

### 18.1 核心思想

> **`MyAssets.vue` 不区分"我买的"和"我建的"**——同源同构,一张表展示所有本地资产,
> **唯一的差异是 `source` 字段**(`purchased` 显示「同步」按钮,`manual` 不显示)。

### 18.2 通用组件 `AssetFormDialog.vue`

**所有资产的"创建/编辑"对话框共用同一个组件**。

```vue
<!-- user-web/src/components/asset/AssetFormDialog.vue -->
<template>
  <el-dialog
    :model-value="visible"
    :title="isEdit ? '编辑资产' : '新建资产'"
    width="900px"
    @close="$emit('update:visible', false)"
  >
    <el-form :model="form" :rules="rules" ref="formRef" label-width="120px">
      <el-form-item label="来源" v-if="isEdit">
        <SourceTag :source="form.source" />  <!-- 平台购买/自建 -->
      </el-form-item>

      <el-form-item label="资产类型" prop="asset_type">
        <AssetTypePicker
          v-model="form.asset_type"
          :disabled="isEdit"
        />
      </el-form-item>

      <el-form-item label="行业" prop="industry">
        <IndustrySelector v-model="form.industry" />
      </el-form-item>

      <el-form-item label="名称" prop="name">
        <el-input v-model="form.name" maxlength="64" show-word-limit />
      </el-form-item>

      <el-form-item label="资产数据" prop="data">
        <JsonSchemaEditor
          v-model="form.data"
          :schema-type="form.asset_type"
          :readonly="form.source === 'purchased' && !form.local_modified"
        />
        <div class="form-tip">
          <el-icon><InfoFilled /></el-icon>
          编辑已购买的资产会保存为本地副本(原平台版本不受影响)
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:visible', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="handleSave">
        {{ isEdit ? '保存' : '创建' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { createLocalAsset, updateLocalAsset } from '@/api/asset/local'
import SourceTag from './SourceTag.vue'
import AssetTypePicker from './AssetTypePicker.vue'
import IndustrySelector from './IndustrySelector.vue'
import JsonSchemaEditor from './JsonSchemaEditor.vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  asset: { type: Object, default: null }   // null = 新建,非 null = 编辑
})
const emit = defineEmits(['update:visible', 'success'])

const form = ref({ asset_type: '', industry: '', name: '', data: '{}' })
const formRef = ref(null)
const saving = ref(false)

const isEdit = computed(() => !!props.asset)

watch(() => props.asset, (a) => {
  if (a) form.value = { ...a, data: JSON.stringify(a.data || {}, null, 2) }
  else   form.value = { asset_type: '', industry: '', name: '', data: '{}' }
}, { immediate: true })

const rules = {
  asset_type: [{ required: true, message: '请选择资产类型' }],
  industry:   [{ required: true, message: '请选择行业' }],
  name:       [{ required: true, message: '请输入名称' }, { min: 2, max: 64 }],
  data:       [{ required: true, validator: (rule, value, cb) => {
    try { JSON.parse(value); cb() } catch (e) { cb(new Error('JSON 格式错误')) }
  } }]
}

const handleSave = async () => {
  await formRef.value.validate()
  saving.value = true
  try {
    const payload = {
      asset_type: form.value.asset_type,
      industry:   form.value.industry,
      name:       form.value.name,
      data:       JSON.parse(form.value.data)
    }
    if (isEdit.value) {
      await updateLocalAsset(props.asset.id, payload)
      ElMessage.success('更新成功')
    } else {
      await createLocalAsset(payload)
      ElMessage.success('创建成功')
    }
    emit('success')
    emit('update:visible', false)
  } finally { saving.value = false }
}
</script>
```

### 18.3 `SourceTag.vue` 来源标签

```vue
<!-- user-web/src/components/asset/SourceTag.vue -->
<template>
  <el-tag :type="type" size="small" effect="plain">
    <el-icon><component :is="icon" /></el-icon>
    {{ label }}
  </el-tag>
</template>

<script setup>
import { computed } from 'vue'
import { Shop, EditPen, Download, FolderOpened } from '@element-plus/icons-vue'

const props = defineProps({ source: { type: String, required: true } })

const map = {
  purchased: { label: '平台购买', type: 'success', icon: Shop },
  manual:    { label: '自建',     type: 'info',    icon: EditPen },
  synced:    { label: '平台分发', type: 'warning', icon: Download },
  imported:  { label: '导入',     type: '',        icon: FolderOpened }
}
const label = computed(() => map[props.source]?.label || props.source)
const type  = computed(() => map[props.source]?.type || '')
const icon  = computed(() => map[props.source]?.icon || FolderOpened)
</script>
```

### 18.4 `MyAssets.vue` ★关键:同源同构统一列表

```vue
<!-- user-web/src/views/asset/MyAssets.vue -->
<template>
  <div class="my-assets">
    <!-- 顶部操作栏 -->
    <el-card class="toolbar">
      <el-form inline>
        <el-form-item label="来源">
          <el-select v-model="filter.source" placeholder="全部" clearable>
            <el-option label="全部" value="" />
            <el-option label="平台购买" value="purchased" />
            <el-option label="自建"     value="manual" />
            <el-option label="平台分发" value="synced" />
            <el-option label="导入"     value="imported" />
          </el-select>
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="filter.asset_type" placeholder="全部" clearable>
            <el-option label="智能体角色"     value="agent_persona" />
            <el-option label="销冠话术"       value="sales_script" />
            <el-option label="AB 测试方案"    value="ab_test_plan" />
            <el-option label="自动化工作流"   value="marketing_workflow" />
            <el-option label="行业 SOP"       value="industry_sop" />
          </el-select>
        </el-form-item>
        <el-form-item label="行业">
          <el-select v-model="filter.industry" placeholder="全部" clearable>
            <el-option v-for="i in industries" :key="i" :label="i" :value="i" />
          </el-select>
        </el-form-item>
        <el-form-item label="关键词">
          <el-input v-model="filter.keyword" placeholder="搜索资产名" clearable />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="fetchList">查询</el-button>
          <el-button type="success" @click="handleCreate">
            <el-icon><Plus /></el-icon>新建资产
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 资产表格(★同源同构:平台购买 + 自建 同一张表) -->
    <el-table :data="list" v-loading="loading" border stripe>
      <el-table-column prop="name" label="名称" min-width="200">
        <template #default="{ row }">
          <el-link type="primary" @click="handleView(row)">{{ row.name }}</el-link>
        </template>
      </el-table-column>
      <el-table-column prop="source" label="来源" width="100">
        <template #default="{ row }">
          <SourceTag :source="row.source" />
        </template>
      </el-table-column>
      <el-table-column prop="asset_type" label="类型" width="120">
        <template #default="{ row }">{{ typeLabel(row.asset_type) }}</template>
      </el-table-column>
      <el-table-column prop="industry" label="行业" width="80" />
      <el-table-column prop="version" label="版本" width="100" />
      <el-table-column prop="is_active" label="状态" width="80">
        <template #default="{ row }">
          <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
            {{ row.is_active ? '已启用' : '已停用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="synced_at" label="同步/更新时间" width="170">
        <template #default="{ row }">{{ formatDate(row.synced_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button size="small" @click="handleView(row)">查看</el-button>
          <el-button size="small" type="primary" @click="handleEdit(row)">编辑</el-button>
          <!-- ★ 关键:仅平台来源显示「同步」按钮 -->
          <SyncVersionButton
            v-if="row.source === 'purchased' || row.source === 'synced'"
            :asset-id="row.asset_id"
            @success="fetchList"
          />
          <el-button size="small" :type="row.is_active ? 'warning' : 'success'"
                     @click="handleToggle(row)">
            {{ row.is_active ? '停用' : '启用' }}
          </el-button>
          <el-button size="small" type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-model:current-page="filter.page"
      v-model:page-size="filter.size"
      :total="total"
      layout="total, prev, pager, next"
      @current-change="fetchList"
    />

    <!-- ★ 同源同构:编辑对话框(create + edit 同一组件) -->
    <AssetFormDialog
      v-model:visible="dialogVisible"
      :asset="currentAsset"
      @success="fetchList"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { listLocalAssets, deleteLocalAsset, toggleLocalAsset } from '@/api/asset/local'
import SourceTag from '@/components/asset/SourceTag.vue'
import SyncVersionButton from '@/components/asset/SyncVersionButton.vue'
import AssetFormDialog from '@/components/asset/AssetFormDialog.vue'

const industries = ['美妆', '教培', '医美', '汽车', '金融']
const filter = ref({ source: '', asset_type: '', industry: '', keyword: '', page: 1, size: 20 })
const list = ref([])
const total = ref(0)
const loading = ref(false)
const dialogVisible = ref(false)
const currentAsset = ref(null)  // null=新建,非null=编辑

const typeLabel = (t) => ({
  agent_persona: '智能体角色', sales_script: '销冠话术',
  ab_test_plan: 'AB 测试', marketing_workflow: '工作流', industry_sop: '行业 SOP'
}[t] || t)

const formatDate = (d) => d ? new Date(d).toLocaleString('zh-CN') : '-'

const fetchList = async () => {
  loading.value = true
  try {
    const { list: data, total: t } = await listLocalAssets(filter.value)
    list.value = data
    total.value = t
  } finally { loading.value = false }
}

const handleCreate = () => { currentAsset.value = null; dialogVisible.value = true }
const handleView   = (row) => { /* 跳详情或弹详情 */ }
const handleEdit   = (row) => { currentAsset.value = row; dialogVisible.value = true }
const handleToggle = async (row) => {
  await toggleLocalAsset(row.id, !row.is_active)
  ElMessage.success('操作成功')
  fetchList()
}
const handleDelete = async (row) => {
  await ElMessageBox.confirm(`确认删除「${row.name}」?删除后将从本地移除(平台端不受影响)`, '提示',
    { type: 'warning' })
  await deleteLocalAsset(row.id)
  ElMessage.success('删除成功')
  fetchList()
}

onMounted(fetchList)
</script>
```

### 18.5 关键:前后端 API 形态(同源同构)

**前端 API 封装** `user-web/src/api/asset/local.js`:

```js
import request from '@/utils/request'

// ★ 同一组 API 同时支持「自建」和「平台购买后编辑」
export const listLocalAssets     = (params) => request.get('/api/v1/local-assets', { params })
export const getLocalAsset       = (id)     => request.get(`/api/v1/local-assets/${id}`)
export const createLocalAsset    = (data)   => request.post('/api/v1/local-assets', data)
export const updateLocalAsset    = (id, d)  => request.put(`/api/v1/local-assets/${id}`, d)
export const deleteLocalAsset    = (id)     => request.delete(`/api/v1/local-assets/${id}`)
export const toggleLocalAsset    = (id, a)  => request.put(`/api/v1/local-assets/${id}/toggle-active`, { active: a })
```

**关键点**:
- **同一组 API**(6 个端点)同时服务"自建"和"购买"
- 后端 Service 不区分来源,统一处理
- 前端 UI 用 `SourceTag` 显示来源,操作按钮根据 `source` 条件显示
- `AssetFormDialog` 同一个组件用于"创建"和"编辑"
- **禁止**为"自建"和"购买"写两套 API/页面/组件

### 18.6 同源同构带来的好处

| 维度 | 旧模式(分两套) | 新模式(同源同构) |
|---|---|---|
| 数据库 | 2 张表 | 1 张表(加 `source` 字段) |
| 后端 API | 2 套 CRUD | 1 套 CRUD |
| 前端组件 | 2 套页面/表单 | 1 套页面/表单 |
| 编辑能力 | 购买后不能编辑 | **购买后也能编辑,保存为本地副本** |
| 用户心智 | "这个是买的,那个是自建的" | "都是我的资产,只是来源不同" |
| 代码量 | ×2 | ×1 |
| Bug 率 | ×2 | ×1 |
| 维护成本 | 高 | **低** |

---

## 十九、与上一版报告的关键差异(锁定)

| 维度 | 第二版(无同源) | **本版(同源同构)** |
|---|---|---|
| 资产表 | 1 张(`local_assets`)| **同 1 张**(已正确) |
| 自建 vs 购买 | 设计上未明确 | **明确同源同构,仅 `source` 标识** |
| 商户自建能力 | 未设计 | **完整 CRUD,与购买共用** |
| 编辑能力 | 购买资产不能编辑 | **购买资产也能编辑,保存为本地副本** |
| UI 复用 | 未强调 | **AssetFormDialog 一个组件,create/edit 通用** |
| 五层架构 | 未明确 | **强制 Handler/Service/Domain/Repository/Model** |
| 编码规范 | 未明确 | **命名/错误/日志/注释/测试/Lint 全套** |
| 依赖注入 | 未设计 | **手 wire,Container 模式** |
| 分层检查 | 无 | **CI 强制 `check-deps.sh` 拒绝合并** |

---

## 二十、最终一句话总结(终版)

> **HivemTK 资产市场 = 同源同构 + 五层架构 + 编码规范三位一体**:
> **平台购买与商户自建走同一张表 `local_assets`、同一组 CRUD、同一套 UI,仅 `source` 字段区分**;
> **后端严格五层(handler → service → domain → repository → model),GORM 只在 repo 出现**;
> **代码规范强制命名/错误/日志/测试/Lint,CI 拒绝违规**;
> **业务效果:用户可购买、可编辑、可自建、可同步,所有操作 0 心智负担**。
