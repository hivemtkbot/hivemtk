# Go 五层架构编码规范(强制)

> **最高规则级别**:⭐⭐⭐ 项目级硬性约束  
> **适用范围**:所有 Go 后端代码(`user-server`、`platform-server`、未来新增 Go 服务)  
> **生效日期**:2026-07-22  
> **强制要求**:任何偏离本文档的代码,**必须**提交 ADR 经技术委员会审批  
> **配套文档**:`ARCHITECTURE_DIAGRAM.md` 描述高层架构;本文档描述 **代码级落地规范**

---

## 〇、本文档存在的意义

在过去 6 个月的项目演进中,以下违反五层架构的问题反复出现:
- Controller 直接 `db.GetDB()` 查表,绕过 Service
- Service 直接调 `db.GetDB()` 执行 SQL,绕过 Repository
- Model 层包含业务校验、跨表查询、调用 Service
- DTO 引用 Service / Model,反向耦合
- Router 写业务逻辑
- 工具方法/常量散落在 controller 包

**本文档面向所有 AI 编码 Agent 与人类开发者**,必须无条件遵守。任何"图方便"、"反正只有一个调用方"都不能成为破坏架构的理由。

---

## 一、五层总览

```
┌────────────────────────────────────────────────────────────────────┐
│  L1  cmd/                   进程入口 / 装配 (Wiring)              │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 加载配置 → 初始化 DB/Redis/Logger → 装配全局单例            │
│        → 注册路由 → 启动 HTTP 服务                                  │
│  禁止: 任何业务逻辑、SQL、Redis 调用                                │
├────────────────────────────────────────────────────────────────────┤
│  L2  internal/router        路由声明 (Route Registry)              │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 路径 → Controller 方法 绑定;中间件挂载                       │
│  禁止: 任何业务逻辑;不允许在路由闭包里写业务                        │
│  仅允许: HTTP 方法、URL、参数解析、调用 Controller.XXX              │
├────────────────────────────────────────────────────────────────────┤
│  L3  internal/controller    表现层 (Thin Handler)                  │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 解析入参 (ShouldBindJSON/Query/Param) → 调 Service         │
│        → 序列化响应 (response.Success/Error)                        │
│  禁止: 业务逻辑、SQL、Redis、跨 Service 编排、事务控制              │
│  禁止: 直接引用 repository / model / db.GetDB()                    │
├────────────────────────────────────────────────────────────────────┤
│  L4  internal/service       业务层 (Business Orchestration)        │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 业务规则、跨实体编排、事务控制 (tx.Begin)、缓存策略          │
│        调用多个 Repository / 工具包 / aiagent                      │
│  允许: 引用 model (作为入参/出参)、repository (数据访问)            │
│  禁止: 直接调 db.GetDB() / redis / SQL                             │
│  禁止: 引用 controller / router / DTO 写入端以外的东西              │
├────────────────────────────────────────────────────────────────────┤
│  L5  internal/repository    数据访问层 (Data Access)                │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 封装 GORM / pgvector / Redis 操作;返回 model 实体            │
│  允许: 引用 model、_db (db.GetDB)                                  │
│  禁止: 业务逻辑、跨表事务编排、调用 service                         │
├────────────────────────────────────────────────────────────────────┤
│  横向  internal/model         数据模型 (GORM Entity + Hook)         │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 表结构映射、字段约束 (gorm tag)、UUID 生成、密码哈希 Hook     │
│  允许: GORM Hook (BeforeCreate/BeforeUpdate)、TableName()          │
│  禁止: 任何业务方法、跨表查询、调用其他层                           │
├────────────────────────────────────────────────────────────────────┤
│  横向  internal/dto           数据传输对象 (Request/Response)       │
│  ────────────────────────────────────────────────────────────────  │
│  职责: 入参校验 (binding tag)、响应序列化结构                       │
│  允许: 引用 _type 枚举、引用其他 dto (嵌套响应)                    │
│  禁止: 引用 service / repository / model 业务方法 / db             │
│  禁止: 包含业务逻辑 (校验放 binding tag,不写方法体)                │
└────────────────────────────────────────────────────────────────────┘
```

**依赖方向(强制)**:

```
cmd ──▶ router ──▶ controller ──▶ service ──▶ repository ──▶ model
                                                  │
                                                  ▼
                                                db (GORM)
```

> ⛔ **任何反向依赖、跨层调用、绕层调用都是违规**

---

## 二、目录布局(强约束)

### 2.1 user-server 标准布局

```
hivemtk/user-server/
├── cmd/
│   ├── api/main.go            # L1 入口,只做装配
│   ├── perf/                  # 压测工具
│   ├── tool/                  # 运维工具子命令
│   └── verify-fix/            # 修复验证脚本
├── config/                    # 配置文件 + 平台授权 yaml
├── docs/swagger/              # 由 swag generate 生成
├── internal/
│   ├── router/                # L2 路由声明,只放路由
│   │   ├── router.go          #   Setup() 装配
│   │   ├── *_routes.go        #   按业务域拆分 (auth_routes.go / sms_routes.go ...)
│   │   └── health.go          #   /health /healthz /readyz
│   │
│   ├── controller/            # L3 表现层,薄
│   │   ├── user.go            #   一个业务域一个文件
│   │   ├── sms.go
│   │   └── helpers.go         #   跨 controller 复用辅助
│   │
│   ├── service/               # L4 业务层,厚
│   │   ├── user.go            #   interface + 实现
│   │   ├── sms.go
│   │   └── *_test.go
│   │
│   ├── repository/            # L5 数据访问层
│   │   ├── user.go            #   interface + 实现
│   │   ├── generic.go         #   通用 CRUD 基类(可选)
│   │   └── *_test.go
│   │
│   ├── model/                 # 横向:GORM 实体
│   │   ├── user.go            #   一个表一个文件
│   │   └── *_test.go
│   │
│   ├── dto/                   # 横向:数据传输对象
│   │   ├── user.go            #   CreateUserRequest/UpdateUserRequest/UserResponse
│   │   └── dto_test.go
│   │
│   ├── middleware/            # 横向:Gin 中间件
│   │   ├── auth.go
│   │   ├── audit.go
│   │   ├── jwt.go
│   │   └── ...
│   │
│   ├── config/                # 横向:平台配置加载
│   ├── cache/                 # 横向:全局缓存
│   ├── event/                 # 横向:事件总线
│   ├── platform/              # 横向:平台同步 SDK
│   ├── migration/             # 横向:数据库迁移服务
│   ├── websocket/             # 横向:WebSocket Hub
│   │
│   ├── aiagent/               # 能力层(独立模块,见 §六)
│   │   ├── agent/             #   决策引擎
│   │   ├── llm/               #   LLM 路由
│   │   ├── rag/               #   RAG 检索
│   │   ├── embedding/         #   向量化
│   │   ├── vector/            #   pgvector
│   │   └── knowledge/         #   知识库资产
│   │
│   └── pkg/                   # 通用工具(无业务含义)
│       ├── i18n/
│       ├── metrics/
│       ├── trace/
│       ├── testutil/
│       └── utils/
│           ├── db/            #   GORM 初始化、GetDB
│           ├── logger/
│           ├── jwt/
│           ├── bcrypt/
│           ├── response/      #   response.Success / response.Error
│           ├── pagination/    #   分页参数解析
│           ├── mail/
│           ├── cron/
│           └── type/          #   枚举类型定义
│
├── tests/                     # 集成 / E2E 测试
├── migrations/                # SQL 迁移文件(非 Go)
├── go.mod
└── go.sum
```

### 2.2 关键命名规则

| 文件类型 | 命名 | 示例 |
|---|---|---|
| Controller 文件 | `<domain>.go` | `sms.go`, `user.go` |
| Service 文件 | `<domain>.go` | `sms.go`, `user.go` |
| Repository 文件 | `<domain>.go` | `sms.go`, `user.go` |
| Model 文件 | `<table_name>.go` | `user.go`, `sms_template.go` |
| DTO 文件 | `<domain>.go` | `user.go`, `sms.go` |
| 路由文件 | `<domain>_routes.go` | `sms_routes.go` |
| 测试文件 | `<name>_test.go` | `user_test.go` |

> ⛔ **禁止**:`utils.go`、`common.go`、`helpers.go`(除跨业务复用的明确辅助外)  
> ⛔ **禁止**:`<name>_v1.go` / `<name>_v2.go` / `<name>_new.go` 等版本后缀  
> ⛔ **禁止**:`<name>_stub.go` / `<name>_ext.go` / `<name>_2026-07-22.go`

---

## 三、每层详解 + 编码模板

### 3.1 L1 - `cmd/api/main.go`(Wiring)

**唯一职责**:装配。**禁止**业务逻辑、SQL、Redis 调用。

```go
package main

import (
    "context"
    "log"
    "marketing/internal/middleware"
    "marketing/internal/pkg/utils/config"
    "marketing/internal/pkg/utils/db"
    "marketing/internal/pkg/utils/logger"
    "marketing/internal/router"
    "github.com/gin-gonic/gin"
)

func main() {
    // 1. 加载配置
    cfg, err := config.Load("config.yaml")
    if err != nil { log.Fatal(err) }

    // 2. 初始化日志
    logger.InitLogger(cfg.Logging)

    // 3. 初始化 DB
    if err := db.InitDB(cfg.DB); err != nil { log.Fatal(err) }
    defer db.Close()

    // 4. 启动迁移(异步)
    go migration.Run(cfg.Version)

    // 5. 装配 Gin
    r := gin.Default()
    r.Use(middleware.TraceMiddleware())
    r.Use(middleware.SensitiveLogMiddleware())
    router.Setup(r)

    // 6. 启动
    if err := r.Run(cfg.Server.Addr); err != nil { log.Fatal(err) }
}
```

✅ DO:装配顺序固定为 config → logger → db → migration → middleware → router  
✅ DO:全局单例的初始化放 main,如 `cache.InitGlobalCache()`、`llm.InitGlobalDispatcher()`  
⛔ DON'T:在 main 里写业务逻辑  
⛔ DON'T:在 main 里直接 `db.GetDB().Create(...)` 插入数据

---

### 3.2 L2 - `internal/router/`

**唯一职责**:URL ↔ Controller 绑定 + 中间件挂载。

**文件名约定**:`<domain>_routes.go`,`Setup()` 在 `router.go`。

```go
// router.go
func Setup(r *gin.Engine) {
    // 1. 全局中间件
    r.Use(gin.Recovery())
    r.Use(middleware.TraceMiddleware())
    r.Use(middleware.SensitiveLogMiddleware())
    r.Use(middleware.RateLimitMiddleware(...))
    r.Use(middleware.AuditMiddleware())

    // 2. 健康检查(公开)
    r.GET("/health", HealthCheck)
    r.GET("/healthz", LivenessCheck)
    r.GET("/readyz", ReadinessCheck)

    // 3. 公开路由组
    public := r.Group("/api")
    setupPublicRoutes(public)

    // 4. 鉴权路由组
    auth := r.Group("/api")
    auth.Use(middleware.InitGuard())
    auth.Use(middleware.JWTAuthMiddleware())
    {
        setupUserRoutes(auth)
        setupSMSRoutes(auth)
        setupWeComRoutes(auth)
        // ...
    }

    // 5. 平台路由
    platform := r.Group("/api/platform")
    platform.Use(middleware.AdminAuthMiddleware())
    setupPlatformRoutes(platform)
}
```

```go
// sms_routes.go
func setupSMSRoutes(rg *gin.RouterGroup) {
    c := controller.NewSMSController()
    sms := rg.Group("/sms")
    {
        sms.GET("/templates", c.ListTemplates)
        sms.POST("/templates", c.CreateTemplate)
        sms.PUT("/templates/:id", c.UpdateTemplate)
        sms.DELETE("/templates/:id", c.DeleteTemplate)
        sms.POST("/send", c.Send)
        sms.GET("/jobs", c.ListJobs)
    }
}
```

✅ DO:每个业务域单独一个 `<domain>_routes.go` 文件  
✅ DO:Controller 实例化在 routes 文件内(或 `controller.New*Controller(db.GetDB())` 注入)  
✅ DO:中间件按职责拆分,`Setup()` 仅做组合  
⛔ DON'T:在 routes 里写业务判断,如 `if user.IsAdmin { ... }`  
⛔ DON'T:在 routes 里直接调 `db.GetDB()`  
⛔ DON'T:把业务逻辑写在闭包里,例如:
```go
// ❌ 错误示范
sms.GET("/send", func(c *gin.Context) {
    var req struct{ Phone string `json:"phone"` }
    c.ShouldBindJSON(&req)
    if len(req.Phone) != 11 {
        c.JSON(400, gin.H{"error": "invalid phone"})
        return
    }
    // 业务逻辑应该全部在 c.Send → Controller.Send → Service.SendSMS
    db.GetDB().Create(&model.SMSRecord{Phone: req.Phone})
    c.JSON(200, gin.H{"ok": true})
})
```

---

### 3.3 L3 - `internal/controller/`

**职责**:薄。解析参数 → 调 Service → 响应。

**模板**:
```go
package controller

import (
    "marketing/internal/dto"
    "marketing/internal/pkg/utils/pagination"
    "marketing/internal/pkg/utils/response"
    "marketing/internal/service"
    "net/http"
    "github.com/gin-gonic/gin"
)

// ✅ 标准:以 Controller 命名的 struct,持有 Service 接口
type SMSController struct {
    svc service.SMSService
}

// ✅ 构造函数接收 db 注入(便于测试 mock)
func NewSMSController(db *gorm.DB) *SMSController {
    return &SMSController{svc: service.NewSMSService(db)}
}

// ✅ 命名规范:动作 + 资源,PascalCase
func (c *SMSController) ListTemplates(ctx *gin.Context) {
    page, size, err := pagination.Parse(ctx)
    if err != nil {
        response.Error(ctx, http.StatusBadRequest, err.Error())
        return
    }
    result, err := c.svc.ListTemplates(ctx, page, size)
    if err != nil {
        response.Error(ctx, http.StatusInternalServerError, err.Error())
        return
    }
    response.Success(ctx, result, "ok")
}

func (c *SMSController) CreateTemplate(ctx *gin.Context) {
    var req dto.CreateSMSTemplateRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
        return
    }
    out, err := c.svc.CreateTemplate(ctx, &req)
    if err != nil {
        response.Error(ctx, http.StatusInternalServerError, err.Error())
        return
    }
    response.Success(ctx, out, "ok")
}
```

**强约束清单**:

| ✅ 必须 | ⛔ 禁止 |
|---|---|
| 通过 `ctx` 透传 `request_id` / `user_id` | 直接调 `db.GetDB()` 或任何 GORM 操作 |
| 业务校验放 DTO `binding` tag | 业务校验写在 controller 方法体内(除非涉及多字段交叉) |
| 所有 Service 调用传 `ctx` | 调用其他 Controller 方法 |
| 用 `response.Success/Error` 返回 | 调 `c.JSON()`、`c.AbortWithStatusJSON()` 直接写响应 |
| 命名 `CreateXxx / UpdateXxx / GetXxx / ListXxxs / DeleteXxx` | 命名 `HandleXxx / ProcessXxx / DoXxx`(含义模糊) |
| 文件按业务域拆 | 一个文件超过 500 行(超过则按子域拆) |

**反例**(❌ 绝对禁止):

```go
// ❌ 错误 1:Controller 直接操作 DB
func (c *UserController) GetUser(ctx *gin.Context) {
    var user model.User
    db.GetDB().Where("id = ?", ctx.Param("id")).First(&user)  // 越层
    ctx.JSON(200, user)
}

// ❌ 错误 2:Controller 写业务逻辑
func (c *OrderController) CreateOrder(ctx *gin.Context) {
    var req dto.CreateOrderRequest
    ctx.ShouldBindJSON(&req)
    if req.Quantity <= 0 || req.Quantity > 100 {       // 业务规则下沉到 Service
        ctx.JSON(400, gin.H{"error": "invalid quantity"})
        return
    }
    // ...
}

// ❌ 错误 3:Controller 调其他 Service 做编排
func (c *OrderController) Pay(ctx *gin.Context) {
    order, _ := c.orderSvc.GetOrder(ctx, ctx.Param("id"))
    c.userSvc.DeductBalance(ctx, order.UserID, order.Amount)  // 跨 Service 编排应在下个 OrderService.Pay
    c.notifySvc.Send(ctx, order.UserID, "已扣款")              // 编排下沉
    ctx.JSON(200, gin.H{"ok": true})
}

// ❌ 错误 4:Controller 直接返回 Service 内部错误原始信息
err := c.svc.Create(ctx, &req)
ctx.JSON(500, gin.H{"error": err.Error()})  // 错误处理也走 response.Error
```

---

### 3.4 L4 - `internal/service/`

**职责**:业务规则、跨实体编排、事务、缓存。

**模板**:
```go
package service

import (
    "context"
    "errors"
    "marketing/internal/dto"
    "marketing/internal/model"
    "marketing/internal/repository"
    "gorm.io/gorm"
)

// ✅ 必须:导出 interface + 小写 struct 实现 (面向接口编程)
type SMSService interface {
    ListTemplates(ctx context.Context, page, size int) (*dto.ListSMSTemplatesResponse, error)
    CreateTemplate(ctx context.Context, req *dto.CreateSMSTemplateRequest) (*dto.SMSTemplateResponse, error)
    UpdateTemplate(ctx context.Context, id string, req *dto.UpdateSMSTemplateRequest) error
    DeleteTemplate(ctx context.Context, id string) error
    Send(ctx context.Context, req *dto.SendSMSRequest) (*dto.SendSMSResponse, error)
}

type smsService struct {
    tmplRepo repository.SMSTemplateRepository
    jobRepo  repository.SMSJobRepository
    reach    // 触达执行器(可注入)
    cache    Cache  // 缓存抽象
}

// ✅ 构造函数返回 interface
func NewSMSService(db *gorm.DB) SMSService {
    return &smsService{
        tmplRepo: repository.NewSMSTemplateRepository(db),
        jobRepo:  repository.NewSMSJobRepository(db),
        reach:    reach.NewExecutor(db),
        cache:    cache.NewSMSCache(),
    }
}

func (s *smsService) CreateTemplate(ctx context.Context, req *dto.CreateSMSTemplateRequest) (*dto.SMSTemplateResponse, error) {
    // ✅ 业务校验
    if req.Content == "" {
        return nil, errors.New("模板内容不能为空")
    }

    // ✅ 业务编排:多 Repo 协调
    exists, err := s.tmplRepo.ExistsByName(ctx, req.Name)
    if err != nil { return nil, err }
    if exists { return nil, errors.New("模板名称已存在") }

    tmpl := &model.SMSTemplate{
        Name:    req.Name,
        Content: req.Content,
    }
    if err := s.tmplRepo.Create(ctx, tmpl); err != nil {
        return nil, err
    }

    // ✅ 缓存失效
    s.cache.InvalidateTemplateList(ctx)

    return dto.ToSMSTemplateResponse(tmpl), nil
}

func (s *smsService) Send(ctx context.Context, req *dto.SendSMSRequest) (*dto.SendSMSResponse, error) {
    // ✅ 事务:Job + ReachRecord 必须原子
    return s.tx.Run(ctx, func(tx *gorm.DB) (interface{}, error) {
        tmpl, err := s.tmplRepo.GetByIDWithTx(ctx, tx, req.TemplateID)
        if err != nil { return nil, err }

        // ✅ 渲染
        content, err := renderTemplate(tmpl.Content, req.Vars)
        if err != nil { return nil, err }

        // ✅ 触达执行
        return s.reach.Send(ctx, tx, &reach.SMSPayload{
            Phone:   req.Phone,
            Content: content,
        })
    })
}
```

**强约束清单**:

| ✅ 必须 | ⛔ 禁止 |
|---|---|
| **导出 interface,小写 struct 实现** | 把 struct 设为大写导出 |
| 所有公开方法第一个参数为 `ctx context.Context` | 在 Service 里手动 `gin.Context` |
| 业务校验在此层(dto 做格式校验,service 做业务规则) | 业务校验在 controller |
| 跨表事务用 `tx.Begin()` / `s.tx.Run(...)` | 事务散落在各 Repo |
| 调用 Repository 时把 `ctx` 透传 | Repository 方法忽略 ctx |
| 错误用 `errors.New` / `fmt.Errorf("...%w", err)` | 返回字符串裸错误 |
| DTO ↔ Model 转换用 `dto.ToXxxResponse(m)` 函数 | 在 Service 内拼装结构体 |
| 业务事件通过 `event.GetGlobalBus().Publish(...)` 触发 | 跨 Service 直接调用 |

**事务模板**(Repository 必须支持 Tx):
```go
// service 层
func (s *xxxService) ComplexOp(ctx context.Context) error {
    return s.db.Transaction(func(tx *gorm.DB) error {
        if err := s.repoA.CreateWithTx(ctx, tx, ...); err != nil { return err }
        if err := s.repoB.UpdateWithTx(ctx, tx, ...); err != nil { return err }
        return nil
    })
}
```

**反例**(❌ 绝对禁止):

```go
// ❌ 错误 1:Service 直接调 db.GetDB()
func (s *userService) GetUser(id string) (*model.User, error) {
    var u model.User
    db.GetDB().First(&u, "id = ?", id)   // 越层,应该调 userRepo.GetByID
    return &u, nil
}

// ❌ 错误 2:Service 拼装响应结构
func (s *userService) GetUser(id string) (*dto.UserResponse, error) {
    u, _ := s.repo.GetByID(id)
    return &dto.UserResponse{            // 转换应在 dto.ToUserResponse
        ID: u.ID, Name: u.Name, ...
    }, nil
}

// ❌ 错误 3:Service 之间循环依赖
// ssoService → userService → ssoService  ❌ 通过 event bus 解耦
```

---

### 3.5 L5 - `internal/repository/`

**职责**:封装 GORM/Redis 操作,返回 model。

**模板**:
```go
package repository

import (
    "context"
    "marketing/internal/model"
    "gorm.io/gorm"
)

// ✅ 必须:导出 interface + 小写 struct
type UserRepository interface {
    Create(ctx context.Context, user *model.User) error
    GetByID(ctx context.Context, id string) (*model.User, error)
    GetByUsername(ctx context.Context, username string) (*model.User, error)
    List(ctx context.Context, page, size int) ([]*model.User, int64, error)
    Update(ctx context.Context, user *model.User) error
    Delete(ctx context.Context, id string) error
    // ✅ 事务版本
    CreateWithTx(ctx context.Context, tx *gorm.DB, user *model.User) error
    UpdateWithTx(ctx context.Context, tx *gorm.DB, user *model.User) error
}

type userRepo struct {
    db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
    return &userRepo{db: db}
}

func (r *userRepo) Create(ctx context.Context, user *model.User) error {
    return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
    var user model.User
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
    if err != nil { return nil, err }
    return &user, nil
}

func (r *userRepo) List(ctx context.Context, page, size int) ([]*model.User, int64, error) {
    var users []*model.User
    var total int64
    if err := r.db.WithContext(ctx).Model(&model.User{}).Count(&total).Error; err != nil {
        return nil, 0, err
    }
    err := r.db.WithContext(ctx).Offset((page-1)*size).Limit(size).Order("create_time DESC").Find(&users).Error
    return users, total, err
}
```

**强约束清单**:

| ✅ 必须 | ⛔ 禁止 |
|---|---|
| **导出 interface,小写 struct** | struct 大写导出 |
| 所有方法第一个参数 `ctx context.Context` | 忽略 ctx(并发追踪失效) |
| `r.db.WithContext(ctx)` 链式调用 | 裸 `r.db.Where(...)`(丢失 ctx 取消信号) |
| 提供 `*WithTx(ctx, tx, ...)` 事务版本 | Service 自行开 tx 后强传 db(应该走 WithTx) |
| 返回 `*model.Xxx` 或 `([]*model.Xxx, error)` | 返回 `*dto.Xxx`、返回 `map[string]any` |
| 表名小写 + 下划线复数 | 中文表名、驼峰表名 |
| 复杂查询封装为方法(如 `ListActiveUsers`) | 在 Service 里写 50 行 GORM 链 |
| 软删除 `Where("deleted_at IS NULL")` 默认 | 硬删除非显式声明 |

**反例**(❌ 绝对禁止):

```go
// ❌ 错误 1:Repo 写业务
func (r *userRepo) RegisterWithValidation(u *model.User) error {
    if u.Username == "" { return errors.New("username required") }  // 业务校验在 Service
    return r.db.Create(u).Error
}

// ❌ 错误 2:Repo 返回 DTO
func (r *userRepo) GetUserDTO(id string) (*dto.UserResponse, error) {  // Repo 只返回 Model
    var u model.User
    r.db.First(&u, "id = ?", id)
    return dto.ToUserResponse(&u), nil
}

// ❌ 错误 3:Repo 调 Service
func (r *orderRepo) CreateOrder(o *model.Order) error {
    r.db.Create(o)
    s.userSvc.DeductBalance(...)  // 越层
    return nil
}
```

---

### 3.6 横向 - `internal/model/`

**职责**:表结构 + GORM 映射 + Hook。

**模板**:
```go
package model

import (
    "time"
    "marketing/internal/pkg/utils/type"
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type SMSTemplate struct {
    ID         string                  `gorm:"type:varchar(36);primaryKey" json:"id"`
    Name       string                  `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
    Content    string                  `gorm:"type:text;not null" json:"content"`
    Status     type.SMSTemplateStatus  `gorm:"type:varchar(20);default:'active';index" json:"status"`
    CreateTime int64                   `gorm:"autoCreateTime" json:"create_time"`
    UpdateTime int64                   `gorm:"autoUpdateTime" json:"update_time"`
    // ✅ 软删除
    DeletedAt  gorm.DeletedAt          `gorm:"index" json:"-"`
}

func (SMSTemplate) TableName() string { return "sms_templates" }

// ✅ Hook:只做"模型自身的事"
func (t *SMSTemplate) BeforeCreate(tx *gorm.DB) error {
    if t.ID == "" { t.ID = uuid.New().String() }
    return nil
}
```

**强约束清单**:

| ✅ 必须 | ⛔ 禁止 |
|---|---|
| 主键 `varchar(36)` UUID | 业务字段当主键 |
| 必填字段加 `not null` | 全部可空(失去约束) |
| 时间字段 `autoCreateTime` / `autoUpdateTime` | 手动赋值 `time.Now()` |
| 软删除字段 `DeletedAt gorm.DeletedAt` | 硬删除非显式声明 |
| 枚举用 `type.XxxStatus` 类型(在 `pkg/utils/type` 定义) | 直接 `int` 表示状态 |
| 敏感字段 `json:"-"`(如 Password) | 密码 / token 进 JSON |
| 唯一索引用 `uniqueIndex` | 业务层去重 |
| 仅 `TableName()` + GORM Hook | 自定义业务方法(如 `func (u *User) CanSend() bool`) |

**反例**(❌ 绝对禁止):

```go
// ❌ 错误 1:Model 含业务方法
func (u *User) CanSendSMS() bool {       // 业务规则在 Service
    return u.Status == 1 && u.SMSQuota > 0
}

// ❌ 错误 2:Model 跨表查
func (u *User) GetOrders() []Order {     // 跨表查询在 Repository
    db.GetDB().Where("user_id = ?", u.ID).Find(&orders)
    return orders
}

// ❌ 错误 3:Model 调 Service
func (o *Order) Notify() {
    notifySvc.Send(o.UserID, "订单创建")  // Service 编排下沉
}
```

---

### 3.7 横向 - `internal/dto/`

**职责**:入参 / 出参 / 校验规则 / 序列化。

**模板**:
```go
package dto

import "marketing/internal/pkg/utils/type"

// ✅ 入参:仅含 binding 校验
type CreateSMSTemplateRequest struct {
    Name    string `json:"name" binding:"required,min=2,max=100"`
    Content string `json:"content" binding:"required,min=1,max=500"`
    Status  type.SMSTemplateStatus `json:"status" binding:"omitempty,oneof=active inactive"`
}

// ✅ 出参:纯结构,无方法
type SMSTemplateResponse struct {
    ID         string                  `json:"id"`
    Name       string                  `json:"name"`
    Content    string                  `json:"content"`
    Status     type.SMSTemplateStatus  `json:"status"`
    CreateTime int64                   `json:"create_time"`
}

// ✅ 列表响应:固定包含 items + total
type ListSMSTemplatesResponse struct {
    Total int64                `json:"total"`
    Items []*SMSTemplateResponse `json:"items"`
}

// ✅ 转换函数:在 dto 包内集中
func ToSMSTemplateResponse(t *model.SMSTemplate) *SMSTemplateResponse {
    if t == nil { return nil }
    return &SMSTemplateResponse{
        ID: t.ID, Name: t.Name, Content: t.Content,
        Status: t.Status, CreateTime: t.CreateTime,
    }
}

func ToSMSTemplateResponses(ts []*model.SMSTemplate) []*SMSTemplateResponse {
    out := make([]*SMSTemplateResponse, 0, len(ts))
    for _, t := range ts {
        out = append(out, ToSMSTemplateResponse(t))
    }
    return out
}
```

**强约束清单**:

| ✅ 必须 | ⛔ 禁止 |
|---|---|
| 入参 `binding` 标签做格式校验 | 在 Service / Controller 写 `if req.Xxx == ""` |
| 转换函数命名 `ToXxxResponse(m *model.Xxx) *XxxResponse` | 在 Service 里直接拼装 |
| 出参结构命名 `XxxResponse` / `XxxListResponse` | 命名 `XxxVO` / `XxxOut` / `XxxResult` |
| 入参命名 `CreateXxxRequest` / `UpdateXxxRequest` | 命名 `XxxParam` / `XxxInput` |
| 嵌套响应引用其他 DTO | 引用 Service / Repository |
| 列表响应含 `total` 字段 | 仅返回数组(分页信息丢失) |
| `omitempty` 用于可选字段 | 所有字段 omitempty(强制字段丢失) |

**反例**(❌ 绝对禁止):

```go
// ❌ 错误 1:DTO 调 Service
type OrderDTO struct {
    User *UserDTO
    _    *service.UserService  // 禁止
}

// ❌ 错误 2:DTO 写方法
func (r *UserResponse) MaskPhone() {  // 数据脱敏在 Service / 序列化层
    r.Phone = r.Phone[:3] + "****"
}
```

---

## 四、依赖注入与全局单例

### 4.1 构造函数注入(推荐)

```go
// Service 持有 Repo
type userService struct {
    userRepo repository.UserRepository  // interface
    jwtUtils *utils.JWTUtils
}

func NewUserService(db *gorm.DB) UserService {
    return &userService{
        userRepo: repository.NewUserRepository(db),  // 注入
        jwtUtils: utils.NewJWTUtils(...),
    }
}

// Controller 持有 Service
type UserController struct {
    svc service.UserService
}

func NewUserController(db *gorm.DB) *UserController {
    return &UserController{svc: service.NewUserService(db)}
}
```

### 4.2 全局单例(仅限确实跨模块共享)

```go
// 允许作为全局单例:
// - Logger
// - DB / Redis Client
// - Cache Manager
// - LLM Dispatcher
// - Event Bus
// - Sales Engine(AI 销冠主流程)
// - SSE Hub
// - Trace Bus
// - Tool Executor(智能体工具)

var (
    globalDB      *gorm.DB
    globalCache   cache.Cache
    globalBus     *event.Bus
    globalEngine  *agent.Engine
)

func InitGlobalCache(c *redis.Client) Cache { ... }
func GetGlobalCache() Cache { ... }
```

✅ DO:全局单例提供 `InitGlobal*(...)` 和 `GetGlobal*()` 一对函数  
✅ DO:`Get*()` 在未 Init 时返回 nil 并日志警告,不允许 panic  
⛔ DON'T:在 Service / Controller / Repository 内部 `InitGlobal*`(应在 main)  
⛔ DON'T:把非真正全局的对象设为全局(如 `GetGlobalUserService()` ❌)

---

## 五、错误处理

### 5.1 错误构造

```go
import "errors"
import "fmt"

// ✅ 简单错误
return errors.New("模板内容不能为空")

// ✅ 包装错误
if err != nil {
    return fmt.Errorf("查询模板失败: %w", err)
}

// ✅ 自定义错误类型
type NotFoundError struct{ Resource string; ID string }
func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s 不存在: %s", e.Resource, e.ID)
}

// ✅ 业务错误码(配合统一响应)
type BizError struct {
    Code    int
    Message string
    Cause   error
}
func (e *BizError) Error() string { return e.Message }
func (e *BizError) Unwrap() error { return e.Cause }
```

### 5.2 错误传递

| 层 | 行为 |
|---|---|
| Repository | 包装底层错误 `fmt.Errorf("DB op failed: %w", err)`,不返回业务错误 |
| Service | 抛出业务错误(BizError)或包装 Repo 错误,做错误分类 |
| Controller | `response.Error(ctx, code, bizErr.Message)`,不暴露堆栈 |

---

## 六、aiagent 能力层(特殊)

**aiagent** 是 L4 业务层之上、能力层的独立模块,内部仍遵循分层:

```
internal/aiagent/
├── agent/            # 决策引擎(ReAct Loop)
│   ├── runtime/      #   AgentRuntime
│   ├── sales/        #   销冠引擎
│   └── sop/          #   SOP 状态机
├── llm/              # LLM 路由
│   ├── dispatcher/   #   多厂商调度
│   ├── provider/     #   6 大厂商
│   └── failover/     #   熔断 + 降级
├── rag/              # RAG 检索
│   ├── retriever/    #   召回
│   ├── reranker/     #   重排
│   └── incremental/  #   增量索引
├── embedding/        # 向量化(本地)
├── vector/           # pgvector 操作
└── knowledge/        # 知识库资产
```

**aiagent 内部依赖规则**:
- `agent/` 可调所有子模块
- `llm/` 可调 `vector/`
- `embedding/` 可调 `vector/`
- `knowledge/` 仅持有静态资产(纯函数)
- ✅ aiagent **被** Service 调用
- ⛔ aiagent **不调**业务 Service(避免循环)

---

## 七、AI 编码 Agent 自检清单

每次提交 Go 代码前,AI 必须逐项勾选:

### 7.1 L1 cmd
- [ ] main.go **未**直接调 `db.GetDB().Create/Query/...`
- [ ] main.go **未**写 if/else 业务判断
- [ ] 全局单例的 `Init*` 在 main 集中

### 7.2 L2 router
- [ ] 路由文件命名 `<domain>_routes.go`
- [ ] `Setup()` 在 `router.go`
- [ ] 路由闭包**不**含业务代码
- [ ] 中间件按职责拆分,组合在 `Setup()`

### 7.3 L3 controller
- [ ] 通过 `ctx` 接收,无 `gin.Context` 字段
- [ ] 用 `dto.XxxRequest` + `ctx.ShouldBindJSON`
- [ ] 用 `response.Success / response.Error`
- [ ] **未**调 `db.GetDB()`、Repository、Model 方法
- [ ] **未**调 `c.JSON()` 直接写响应
- [ ] 业务校验在 DTO `binding` tag 或 Service,**不**在 Controller

### 7.4 L4 service
- [ ] 导出 `interface` + 小写 struct
- [ ] 所有方法第一个参数 `ctx context.Context`
- [ ] 业务校验在此层
- [ ] 事务用 `tx.Begin()` / `s.db.Transaction(func(tx)...)`
- [ ] DTO ↔ Model 转换调 `dto.ToXxxResponse()`
- [ ] **未**调 `db.GetDB()`、Controller
- [ ] **未**写 SQL / GORM 链
- [ ] 跨服务通信通过 `event.GetGlobalBus()`

### 7.5 L5 repository
- [ ] 导出 `interface` + 小写 struct
- [ ] 所有方法第一个参数 `ctx context.Context`
- [ ] `r.db.WithContext(ctx)` 链式调用
- [ ] 提供 `*WithTx(ctx, tx, ...)` 事务版本
- [ ] 返回 `*model.Xxx` 或 `([]*model.Xxx, int64, error)`
- [ ] **未**返回 DTO
- [ ] **未**调 Service
- [ ] **未**写业务校验

### 7.6 Model
- [ ] 主键 `varchar(36)` UUID
- [ ] 必填字段 `not null`
- [ ] 时间字段 `autoCreateTime` / `autoUpdateTime`
- [ ] 软删除 `gorm.DeletedAt`
- [ ] 敏感字段 `json:"-"`
- [ ] **未**含业务方法
- [ ] **未**跨表查询
- [ ] **未**调 Service

### 7.7 DTO
- [ ] 入参 `binding` 标签做校验
- [ ] 转换函数 `ToXxxResponse(*model.Xxx) *XxxResponse`
- [ ] 列表响应含 `total`
- [ ] **未**引用 Service / Repository
- [ ] **未**含方法

### 7.8 命名
- [ ] 文件名按 §2.2 规范
- [ ] 无 `v1` / `v2` / `ext` / `stub` / 日期 后缀
- [ ] 无 `utils.go` / `common.go`(除明确的跨业务辅助)

### 7.9 测试
- [ ] Service 单元测试覆盖核心业务规则
- [ ] Repository 测试用 `testutil.NewTestDB()`
- [ ] Controller 测试用 `httptest`
- [ ] 测试文件 `<name>_test.go` 与源码同包

---

## 八、违规检查脚本(`scripts/check-architecture.sh`)

> 维护一个 CI 跑的脚本,自动检测违规。可在 PR 检查时阻断合并。

```bash
#!/usr/bin/env bash
# 检查 controller 包是否引用 repository/model/db
# 检查 service 包是否引用 controller/router
# 检查 model 是否包含方法(除 TableName + Hook)
# 检查文件命名是否符合规范

set -e

echo "[1/6] 检查 controller 是否绕过 service 直访 repository..."
if grep -rn "marketing/internal/repository" hivemtk/user-server/internal/controller/; then
  echo "❌ 违规:controller 引用了 repository"
  exit 1
fi

echo "[2/6] 检查 controller 是否直接调 db..."
if grep -rn "db.GetDB()" hivemtk/user-server/internal/controller/; then
  echo "❌ 违规:controller 直接调 db"
  exit 1
fi

echo "[3/6] 检查 service 是否绕过 repository 直访 db..."
if grep -rn "db.GetDB()" hivemtk/user-server/internal/service/; then
  echo "❌ 违规:service 直接调 db"
  exit 1
fi

echo "[4/6] 检查 model 是否包含业务方法..."
# 检测 func (xxx *Xxx) 不在 TableName/BeforeCreate/BeforeUpdate 列表中的方法
for f in $(find hivemtk/user-server/internal/model -name "*.go"); do
  funcs=$(grep -E "^func \(.+\*?[A-Z][a-zA-Z]+\)" "$f" | grep -v "TableName\|BeforeCreate\|BeforeUpdate\|AfterCreate\|AfterUpdate\|AfterFind\|AfterDelete" || true)
  if [ -n "$funcs" ]; then
    echo "❌ $f 包含非 GORM Hook 方法:"
    echo "$funcs"
    exit 1
  fi
done

echo "[5/6] 检查文件命名..."
for f in $(find hivemtk/user-server/internal -name "*_v[0-9]*.go" -o -name "*_stub*.go" -o -name "*_ext*.go" -o -name "*_2026-*.go" -o -name "utils.go" -o -name "common.go"); do
  if [ -n "$f" ]; then
    echo "❌ 文件命名违规: $f"
    exit 1
  fi
done

echo "[6/6] 检查 dto 是否引用 service..."
if grep -rn "marketing/internal/service\|marketing/internal/repository" hivemtk/user-server/internal/dto/; then
  echo "❌ 违规:dto 引用了 service/repository"
  exit 1
fi

echo "✅ 架构检查通过"
```

集成到 CI:
```yaml
# .github/workflows/user-server-ci.yml
- name: 架构合规检查
  run: bash scripts/check-architecture.sh
```

---

## 九、迁移与重构指引

### 9.1 把现有违规代码迁回架构

**场景 A**:Controller 写了业务
```go
// ❌ Before
func (c *XController) Y(ctx *gin.Context) {
    // 业务逻辑
    if x.A < 10 {
        ctx.JSON(400, ...)
        return
    }
    db.GetDB().Create(...)
}

// ✅ After
// 1. 业务规则下沉到 service.XService.Y
// 2. DB 操作下沉到 repository.XRepository.Create
// 3. controller 变为薄层
func (c *XController) Y(ctx *gin.Context) {
    var req dto.YRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        response.Error(ctx, 400, err.Error())
        return
    }
    out, err := c.svc.Y(ctx, &req)
    if err != nil {
        response.Error(ctx, 500, err.Error())
        return
    }
    response.Success(ctx, out, "ok")
}
```

**场景 B**:Service 调 db
```go
// ❌ Before
func (s *xService) List() ([]*model.X, error) {
    var xs []*model.X
    db.GetDB().Find(&xs)
    return xs, nil
}

// ✅ After
func (s *xService) List(ctx context.Context) ([]*model.X, error) {
    return s.xRepo.List(ctx, 1, 1000)  // 通过 repo
}
```

### 9.2 新增业务域的步骤模板

1. **数据建模**:`internal/model/<table>.go` 定义 GORM 实体
2. **数据传输**:`internal/dto/<domain>.go` 定义 Request / Response
3. **数据访问**:`internal/repository/<domain>.go` 定义 Repository interface + 实现
4. **业务逻辑**:`internal/service/<domain>.go` 定义 Service interface + 实现
5. **HTTP 接口**:`internal/controller/<domain>.go` 定义 Controller
6. **路由注册**:`internal/router/<domain>_routes.go` 注册路径
7. **装配**:在 `router.go` 的 `Setup()` 调用 `setup<Domain>Routes`
8. **测试**:每个层单独测试 + 端到端测试
9. **迁移**:`migrations/<seq>_<name>.sql` + `internal/migration/migrations/<name>.go`

---

## 十、附录:常见反模式汇编(禁止)

| 反模式 | 正确做法 |
|---|---|
| Controller 内 `db.GetDB().Create(...)` | 调 `c.svc.Create(ctx, &req)` |
| Service 内 `db.GetDB().Where(...).Find(...)` | 调 `s.repo.List(ctx, ...)` |
| Model 写 `func (u *User) CanXxx() bool` | 移到 `service/user.go` |
| DTO 内 `func (r *Resp) Mask()` | 脱敏在序列化层或 Service |
| Router 闭包写 if-else 业务 | 业务下沉到 Controller → Service |
| Service 返回 `(*dto.XxxResponse, error)` 在方法内拼装 | 返回 model,转换在 Controller(或 dto 包函数) |
| Repository 写 `if validation...` | 业务校验在 Service |
| `func NewXService()` 无参数 | 显式注入 `db / cache / 其他` |
| 全局变量持有业务对象(如 `var globalUserService`) | 用 `InitGlobal*` + `Get*` 一对函数,且仅限真正跨模块共享 |
| 一个 service 跨 5 个 controller 调用 | 编排下沉到 `service.XService.OrchestrateY` |
| 文件超过 500 行未拆分 | 按子域拆 `<domain>_<sub>.go` |
| `import "marketing/..."` 出现反向依赖 | 重新审查依赖方向 |
| 手动开 goroutine 处理业务 | 异步任务统一通过 `event.Bus.Publish` |
| 错误用 `return "", errors.New("...")` 后不携带上下文 | `fmt.Errorf("操作 X 失败: %w", err)` |
| Controller 直接 `c.JSON(200, data)` | 用 `response.Success(ctx, data, "ok")` |
| SQL 在 Service 字符串拼接 | 复杂查询封装为 Repository 方法 |
| 测试用真实 DB 连接 | 用 `internal/pkg/utils/testutil.NewTestDB()` |
| 时间字段手动 `time.Now().Unix()` | GORM tag `autoCreateTime` |
| 表名中文 | 英文小写下划线 |
| 模型方法返回 Service 调用结果 | Service 编排下沉 |

---

## 十一、版本

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-22 | 首次发布。从 `ARCHITECTURE_DIAGRAM.md` 抽出代码级规范,补全 7 层详解、模板、反例、自检清单、CI 脚本。 |
