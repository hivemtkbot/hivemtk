package tooluse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"
	"marketing/internal/service"
)

// business_tools.go 业务工具实现（PRD §5.2 P0-3 G3）
//
// 6 个业务工具：
//   1. order.create       - 创建订单（自动生成 UUID + 默认 pending 状态）
//   2. order.query        - 查询订单（支持按 ID/account_id/tg_id/status 多条件）
//   3. coupon.apply       - 应用优惠券（核销 + 计算折扣价）
//   4. follow_task.create - 创建跟进任务（联动 FollowUpService + 客户旅程）
//   5. follow_task.update - 更新跟进任务（完成/取消/重新安排，联动客户旅程推进）
//   6. payment.create     - 创建支付（生成支付 URL + 关联订单）

// ===== 业务工具依赖 =====

// BusinessToolDeps 业务工具依赖
type BusinessToolDeps struct {
	OrderService    *service.OrderService
	FollowUpService *service.FollowUpService
	JourneyService  *service.CustomerJourneyService
	DB              *gorm.DB // 用于 coupon.apply 直接读写 coupons / coupon_records 表
}

// NewBusinessToolDeps 创建业务工具依赖（使用全局 DB）
func NewBusinessToolDeps() BusinessToolDeps {
	return BusinessToolDeps{
		OrderService:    service.NewOrderService(),
		FollowUpService: service.NewFollowUpService(service.NewCustomerJourneyService()),
		JourneyService:  service.NewCustomerJourneyService(),
		DB:              db.GetDB(),
	}
}

// NewBusinessToolDepsWithDB 创建业务工具依赖（带 DB，用于测试）
func NewBusinessToolDepsWithDB(gdb *gorm.DB) BusinessToolDeps {
	return BusinessToolDeps{
		OrderService:    service.NewOrderServiceWithDB(gdb),
		FollowUpService: service.NewFollowUpService(service.NewCustomerJourneyService()),
		JourneyService:  service.NewCustomerJourneyService(),
		DB:              gdb,
	}
}

// RegisterBusinessTools 注册所有 6 个业务工具到 registry
func RegisterBusinessTools(registry *ToolRegistry, deps BusinessToolDeps) error {
	tools := []Tool{
		NewOrderCreateTool(deps),
		NewOrderQueryTool(deps),
		NewCouponApplyTool(deps),
		NewFollowTaskCreateTool(deps),
		NewFollowTaskUpdateTool(deps),
		NewPaymentCreateTool(deps),
	}
	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return fmt.Errorf("注册业务工具 %s 失败：%w", t.Name(), err)
		}
	}
	return nil
}

// MustRegisterBusinessTools 注册所有业务工具，出错 panic
func MustRegisterBusinessTools(registry *ToolRegistry, deps BusinessToolDeps) {
	if err := RegisterBusinessTools(registry, deps); err != nil {
		panic(err)
	}
}

// ===== 优惠券模型（私域独立部署，无 merchant_id） =====

// Coupon 优惠券模型
type Coupon struct {
	ID           string    `gorm:"type:varchar(36);primaryKey;not null" json:"id"`
	Code         string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"` // 优惠码
	Name         string    `gorm:"type:varchar(128);not null" json:"name"`
	Type         string    `gorm:"type:varchar(16);not null;default:'fixed'" json:"type"` // fixed（满减）/ percent（折扣）
	Value        string    `gorm:"type:varchar(32);not null" json:"value"`                // fixed: 金额（元）；percent: 百分比（0-100）
	MinAmount    string    `gorm:"type:varchar(32);default:'0'" json:"min_amount"`        // 满减门槛
	TotalQuota   int       `gorm:"default:0" json:"total_quota"`                          // 发放总量（0=不限）
	UsedCount    int       `gorm:"default:0" json:"used_count"`                           // 已核销次数
	PerUserLimit int       `gorm:"default:1" json:"per_user_limit"`                       // 每用户限用次数
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Status       string    `gorm:"type:varchar(16);default:'active';index" json:"status"` // active / expired / disabled
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (Coupon) TableName() string {
	return "coupons"
}

// CouponRecord 优惠券核销记录
type CouponRecord struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	CouponID       string    `gorm:"type:varchar(36);index;not null" json:"coupon_id"`
	CouponCode     string    `gorm:"type:varchar(64);index;not null" json:"coupon_code"`
	CustomerID     string    `gorm:"type:varchar(64);index" json:"customer_id"`
	OrderID        string    `gorm:"type:varchar(36);index" json:"order_id"`
	OriginalAmount string    `gorm:"type:varchar(32)" json:"original_amount"` // 原价
	DiscountAmount string    `gorm:"type:varchar(32)" json:"discount_amount"` // 抵扣金额
	FinalAmount    string    `gorm:"type:varchar(32)" json:"final_amount"`    // 实付金额
	Operator       string    `gorm:"type:varchar(64)" json:"operator"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 表名
func (CouponRecord) TableName() string {
	return "coupon_records"
}

// ===== 工具 1：order.create =====

// OrderCreateTool 创建订单工具
type OrderCreateTool struct {
	BaseTool
	deps BusinessToolDeps
}

// NewOrderCreateTool 创建订单创建工具
func NewOrderCreateTool(deps BusinessToolDeps) *OrderCreateTool {
	return &OrderCreateTool{
		BaseTool: BaseTool{
			NameVal:        "order.create",
			CategoryVal:    CategoryBusiness,
			DescriptionVal: "创建新订单（默认 pending 待支付状态）。用于客户确定购买意向后由智能体自动创建订单，或由销售在对话中生成订单。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "客户账号 ID（必填）"},
					"price":      {Type: "string", Description: "订单金额（元，字符串避免精度丢失，如 \"99.00\"）"},
					"tg_id":      {Type: "integer", Description: "Telegram 用户 ID（可选，用于跨系统关联）"},
				},
				Required: []string{"account_id", "price"},
			},
		},
		deps: deps,
	}
}

// Execute 执行创建订单
func (t *OrderCreateTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "price"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	if t.deps.OrderService == nil {
		return ErrorResult(t.Name(), errors.New("order.create 工具需要 OrderService 依赖")), errors.New("order.create 工具需要 OrderService 依赖")
	}

	accountID := getArgString(args, "account_id")
	if accountID == "" {
		return ErrorResult(t.Name(), errors.New("account_id 不能为空")), errors.New("account_id 不能为空")
	}
	priceStr := getArgString(args, "price")
	if priceStr == "" {
		return ErrorResult(t.Name(), errors.New("price 不能为空")), errors.New("price 不能为空")
	}
	price, err := decimal.NewFromString(priceStr)
	if err != nil {
		return ErrorResult(t.Name(), fmt.Errorf("price 格式错误：%v", err)), fmt.Errorf("price 格式错误：%v", err)
	}
	if price.LessThanOrEqual(decimal.Zero) {
		return ErrorResult(t.Name(), errors.New("price 必须大于 0")), errors.New("price 必须大于 0")
	}

	var tgID int64
	if v, ok := GetIntArgSafe(args, "tg_id"); ok {
		tgID = int64(v)
	}

	order := &model.Order{
		AccountID: accountID,
		Price:     price.String(),
		TgID:      tgID,
		Status:    _type.OrderStatusPending,
	}
	created, err := t.deps.OrderService.CreateOrderFromRequest(order)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}

	return SuccessResult(t.Name(), map[string]any{
		"order_id":    created.ID,
		"account_id":  created.AccountID,
		"price":       created.Price,
		"status":      int(created.Status),
		"status_desc": "pending",
		"create_time": created.CreateTime,
		"message":     "订单创建成功，待支付",
	}), nil
}

// ===== 工具 2：order.query =====

// OrderQueryTool 查询订单工具
type OrderQueryTool struct {
	BaseTool
	deps BusinessToolDeps
}

// NewOrderQueryTool 创建查询订单工具
func NewOrderQueryTool(deps BusinessToolDeps) *OrderQueryTool {
	return &OrderQueryTool{
		BaseTool: BaseTool{
			NameVal:        "order.query",
			CategoryVal:    CategoryBusiness,
			DescriptionVal: "查询订单。支持按订单 ID 单条查询，或按 account_id/tg_id/status 多条件列表查询。用于客服查询订单状态、销售跟进订单进度。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"order_id":   {Type: "string", Description: "订单 ID（指定时单条查询，忽略其他条件）"},
					"account_id": {Type: "string", Description: "客户账号 ID（列表查询时使用）"},
					"tg_id":      {Type: "integer", Description: "Telegram 用户 ID（列表查询时使用）"},
					"status":     {Type: "integer", Description: "订单状态：0=待支付 / 1=已支付 / 2=强制成功 / -1=超时 / -2=关闭", Enum: []string{"0", "1", "2", "-1", "-2"}},
					"page":       {Type: "integer", Description: "页码（默认 1）", Default: 1},
					"page_size":  {Type: "integer", Description: "每页数量（默认 20，最大 100）", Default: 20},
				},
			},
		},
		deps: deps,
	}
}

// Execute 执行查询订单
func (t *OrderQueryTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if t.deps.OrderService == nil {
		return ErrorResult(t.Name(), errors.New("order.query 工具需要 OrderService 依赖")), errors.New("order.query 工具需要 OrderService 依赖")
	}

	// 1. 按订单 ID 单条查询
	if orderID := getArgString(args, "order_id"); orderID != "" {
		order, err := t.deps.OrderService.GetOrderByID(orderID)
		if err != nil {
			return ErrorResult(t.Name(), err), err
		}
		return SuccessResult(t.Name(), map[string]any{
			"order":       order,
			"status_desc": statusDesc(order.Status),
		}), nil
	}

	// 2. 列表查询
	page, _ := GetIntArg(args, "page")
	if page <= 0 {
		page = 1
	}
	pageSize, _ := GetIntArg(args, "page_size")
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	accountID := getArgString(args, "account_id")
	tgID, tgIDOk := GetIntArgSafe(args, "tg_id")
	statusStr := getArgString(args, "status")

	// 走 OrderRepository 直接查询（多条件）
	repo := repository.NewOrderRepository()
	// 通过 account_id + tg_id 查询最近订单
	if accountID != "" && tgIDOk {
		orders, err := repo.GetGetLastOrder(accountID, int64(tgID))
		if err != nil {
			return ErrorResult(t.Name(), err), err
		}
		return SuccessResult(t.Name(), map[string]any{
			"orders":      []*model.Order{orders},
			"total":       1,
			"status_desc": statusDesc(orders.Status),
		}), nil
	}

	// 默认走分页列表
	orders, total, err := t.deps.OrderService.GetOrderList(page, pageSize)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}

	// 内存中按 status 过滤
	filtered := orders
	if statusStr != "" {
		statusVal, perr := parseStatusArg(statusStr)
		if perr != nil {
			return ErrorResult(t.Name(), perr), perr
		}
		filtered = make([]*model.Order, 0, len(orders))
		for _, o := range orders {
			if o.Status == statusVal {
				filtered = append(filtered, o)
			}
		}
	}

	return SuccessResult(t.Name(), map[string]any{
		"orders":    filtered,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}), nil
}

// statusDesc 订单状态描述
func statusDesc(status _type.OrderStatusType) string {
	switch status {
	case _type.OrderStatusPending:
		return "pending"
	case _type.OrderStatusSuccess:
		return "success"
	case _type.OrderStatusForceSuccess:
		return "force_success"
	case _type.OrderStatusTimeout:
		return "timeout"
	case _type.OrderStatusForceClose:
		return "force_closed"
	default:
		return "unknown"
	}
}

// parseStatusArg 解析状态参数
func parseStatusArg(s string) (_type.OrderStatusType, error) {
	switch s {
	case "0":
		return _type.OrderStatusPending, nil
	case "1":
		return _type.OrderStatusSuccess, nil
	case "2":
		return _type.OrderStatusForceSuccess, nil
	case "-1":
		return _type.OrderStatusTimeout, nil
	case "-2":
		return _type.OrderStatusForceClose, nil
	default:
		return 0, fmt.Errorf("status 必须是 0/1/2/-1/-2，实际：%s", s)
	}
}

// ===== 工具 3：coupon.apply =====

// CouponApplyTool 应用优惠券工具
type CouponApplyTool struct {
	BaseTool
	deps BusinessToolDeps
}

// NewCouponApplyTool 创建应用优惠券工具
func NewCouponApplyTool(deps BusinessToolDeps) *CouponApplyTool {
	return &CouponApplyTool{
		BaseTool: BaseTool{
			NameVal:        "coupon.apply",
			CategoryVal:    CategoryBusiness,
			DescriptionVal: "应用优惠券到指定订单。校验优惠券有效性（有效期/总量/每用户限用）+ 计算折扣价 + 核销 + 写入 coupon_records 记录。用于销售/智能体在订单创建后自动应用折扣。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"coupon_code": {Type: "string", Description: "优惠券码（必填）"},
					"order_id":    {Type: "string", Description: "订单 ID（必填）"},
					"customer_id": {Type: "string", Description: "客户 ID（必填，用于校验每用户限用次数）"},
					"operator":    {Type: "string", Description: "操作人（可选，AI Agent 名称或用户 ID）"},
				},
				Required: []string{"coupon_code", "order_id", "customer_id"},
			},
		},
		deps: deps,
	}
}

// Execute 执行应用优惠券
func (t *CouponApplyTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"coupon_code", "order_id", "customer_id"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	if t.deps.DB == nil {
		return ErrorResult(t.Name(), errors.New("coupon.apply 工具需要 DB 依赖")), errors.New("coupon.apply 工具需要 DB 依赖")
	}

	couponCode := getArgString(args, "coupon_code")
	orderID := getArgString(args, "order_id")
	customerID := getArgString(args, "customer_id")
	operator := getArgString(args, "operator")
	if operator == "" {
		operator = "ai_agent"
	}

	// 1. 查询优惠券
	var coupon Coupon
	if err := t.deps.DB.WithContext(ctx).Where("code = ?", couponCode).First(&coupon).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrorResult(t.Name(), fmt.Errorf("优惠券 %s 不存在", couponCode)), fmt.Errorf("优惠券 %s 不存在", couponCode)
		}
		return ErrorResult(t.Name(), err), err
	}

	// 2. 校验有效性
	if coupon.Status != "active" {
		return ErrorResult(t.Name(), fmt.Errorf("优惠券 %s 状态为 %s，不可用", couponCode, coupon.Status)), fmt.Errorf("优惠券 %s 状态为 %s，不可用", couponCode, coupon.Status)
	}
	now := time.Now()
	if now.Before(coupon.StartTime) {
		return ErrorResult(t.Name(), fmt.Errorf("优惠券 %s 尚未生效（生效时间 %s）", couponCode, coupon.StartTime.Format(time.RFC3339))), fmt.Errorf("优惠券 %s 尚未生效", couponCode)
	}
	if !coupon.EndTime.IsZero() && now.After(coupon.EndTime) {
		return ErrorResult(t.Name(), fmt.Errorf("优惠券 %s 已过期（过期时间 %s）", couponCode, coupon.EndTime.Format(time.RFC3339))), fmt.Errorf("优惠券 %s 已过期", couponCode)
	}
	if coupon.TotalQuota > 0 && coupon.UsedCount >= coupon.TotalQuota {
		return ErrorResult(t.Name(), fmt.Errorf("优惠券 %s 已发完（%d/%d）", couponCode, coupon.UsedCount, coupon.TotalQuota)), fmt.Errorf("优惠券 %s 已发完", couponCode)
	}

	// 3. 校验每用户限用次数
	if coupon.PerUserLimit > 0 {
		var userUsedCount int64
		if err := t.deps.DB.WithContext(ctx).Model(&CouponRecord{}).
			Where("coupon_code = ? AND customer_id = ?", couponCode, customerID).
			Count(&userUsedCount).Error; err != nil {
			return ErrorResult(t.Name(), err), err
		}
		if int(userUsedCount) >= coupon.PerUserLimit {
			return ErrorResult(t.Name(), fmt.Errorf("客户 %s 已使用优惠券 %s %d 次（上限 %d）", customerID, couponCode, userUsedCount, coupon.PerUserLimit)), fmt.Errorf("客户已达到优惠券使用上限")
		}
	}

	// 4. 查询订单
	orderSvc := t.deps.OrderService
	if orderSvc == nil {
		orderSvc = service.NewOrderServiceWithDB(t.deps.DB)
	}
	order, err := orderSvc.GetOrderByID(orderID)
	if err != nil {
		return ErrorResult(t.Name(), fmt.Errorf("订单 %s 不存在：%v", orderID, err)), fmt.Errorf("订单 %s 不存在", orderID)
	}
	if order.Status != _type.OrderStatusPending {
		return ErrorResult(t.Name(), fmt.Errorf("订单 %s 状态为 %s，仅待支付订单可应用优惠券", orderID, statusDesc(order.Status))), fmt.Errorf("仅待支付订单可应用优惠券")
	}

	// 5. 计算折扣
	originalAmount, err := decimal.NewFromString(order.Price)
	if err != nil {
		return ErrorResult(t.Name(), fmt.Errorf("订单金额格式错误：%v", err)), fmt.Errorf("订单金额格式错误")
	}
	minAmount, _ := decimal.NewFromString(coupon.MinAmount)
	if minAmount.GreaterThan(originalAmount) {
		return ErrorResult(t.Name(), fmt.Errorf("订单金额 %s 不满足满减门槛 %s", order.Price, coupon.MinAmount)), fmt.Errorf("不满足满减门槛")
	}

	var discountAmount decimal.Decimal
	switch coupon.Type {
	case "fixed":
		discountAmount, err = decimal.NewFromString(coupon.Value)
		if err != nil {
			return ErrorResult(t.Name(), fmt.Errorf("优惠券固定金额格式错误：%v", err)), fmt.Errorf("优惠券金额格式错误")
		}
	case "percent":
		percentVal, err := decimal.NewFromString(coupon.Value)
		if err != nil {
			return ErrorResult(t.Name(), fmt.Errorf("优惠券折扣百分比格式错误：%v", err)), fmt.Errorf("优惠券折扣格式错误")
		}
		if percentVal.LessThan(decimal.Zero) || percentVal.GreaterThan(decimal.NewFromInt(100)) {
			return ErrorResult(t.Name(), errors.New("优惠券折扣百分比必须在 0-100 之间")), errors.New("优惠券折扣百分比非法")
		}
		discountAmount = originalAmount.Mul(percentVal).Div(decimal.NewFromInt(100))
	default:
		return ErrorResult(t.Name(), fmt.Errorf("未知优惠券类型：%s", coupon.Type)), fmt.Errorf("未知优惠券类型")
	}

	// 折扣不能超过原价
	if discountAmount.GreaterThan(originalAmount) {
		discountAmount = originalAmount
	}
	finalAmount := originalAmount.Sub(discountAmount)

	// 6. 事务：核销优惠券 + 更新订单价格 + 写入核销记录
	txErr := t.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 核销优惠券（used_count + 1）
		if err := tx.Model(&Coupon{}).Where("id = ?", coupon.ID).
			UpdateColumn("used_count", gorm.Expr("used_count + 1")).Error; err != nil {
			return err
		}
		// 更新订单价格
		if err := tx.Model(&model.Order{}).Where("id = ?", order.ID).
			Update("price", finalAmount.String()).Error; err != nil {
			return err
		}
		// 写入核销记录
		record := &CouponRecord{
			CouponID:       coupon.ID,
			CouponCode:     coupon.Code,
			CustomerID:     customerID,
			OrderID:        order.ID,
			OriginalAmount: originalAmount.String(),
			DiscountAmount: discountAmount.String(),
			FinalAmount:    finalAmount.String(),
			Operator:       operator,
		}
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		return ErrorResult(t.Name(), txErr), txErr
	}

	return SuccessResult(t.Name(), map[string]any{
		"coupon_code":     coupon.Code,
		"coupon_name":     coupon.Name,
		"coupon_type":     coupon.Type,
		"order_id":        order.ID,
		"customer_id":     customerID,
		"original_amount": originalAmount.String(),
		"discount_amount": discountAmount.String(),
		"final_amount":    finalAmount.String(),
		"remaining_quota": coupon.TotalQuota - coupon.UsedCount - 1,
		"applied":         true,
		"message":         "优惠券已应用，订单价格已更新",
	}), nil
}

// ===== 工具 4：follow_task.create =====

// FollowTaskCreateTool 创建跟进任务工具
type FollowTaskCreateTool struct {
	BaseTool
	deps BusinessToolDeps
}

// NewFollowTaskCreateTool 创建跟进任务创建工具
func NewFollowTaskCreateTool(deps BusinessToolDeps) *FollowTaskCreateTool {
	return &FollowTaskCreateTool{
		BaseTool: BaseTool{
			NameVal:        "follow_task.create",
			CategoryVal:    CategoryBusiness,
			DescriptionVal: "创建客户跟进任务（提醒）。联动客户旅程阶段自动推进 + 销售仪表盘。用于智能体识别意向后自动排程跟进、销售手动安排回访。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"customer_id":    {Type: "string", Description: "客户 ID（必填）"},
					"owner_id":       {Type: "string", Description: "负责销售 ID（可选，默认 'ai_agent'）"},
					"type":           {Type: "string", Description: "跟进类型：first_contact（首次跟进）/ quote_followup（报价后跟进）/ after_sale_care（售后回访）/ repurchase（复购提醒）/ reactivation（沉睡激活）/ birthday（生日祝福）/ custom（自定义）", Enum: []string{"first_contact", "quote_followup", "after_sale_care", "repurchase", "reactivation", "birthday", "custom"}, Default: "custom"},
					"due_in_minutes": {Type: "integer", Description: "多少分钟后到期（默认 1440=24小时）", Default: 1440},
					"title":          {Type: "string", Description: "跟进标题（可选，默认使用 type）"},
					"description":    {Type: "string", Description: "跟进描述（可选）"},
					"priority":       {Type: "integer", Description: "优先级：0=低 / 1=普通 / 2=高 / 3=紧急", Default: 1},
					"sop_name":       {Type: "string", Description: "关联 SOP 名称（可选）"},
					"auto_handle":    {Type: "boolean", Description: "是否由 AI 自动处理（默认 false）", Default: false},
					"channel":        {Type: "string", Description: "触达渠道（可选，如 wecom/sms/email）"},
				},
				Required: []string{"customer_id"},
			},
		},
		deps: deps,
	}
}

// Execute 执行创建跟进任务
func (t *FollowTaskCreateTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"customer_id"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	if t.deps.FollowUpService == nil {
		return ErrorResult(t.Name(), errors.New("follow_task.create 工具需要 FollowUpService 依赖")), errors.New("follow_task.create 工具需要 FollowUpService 依赖")
	}

	customerID := getArgString(args, "customer_id")
	ownerID := getArgString(args, "owner_id")
	if ownerID == "" {
		ownerID = "ai_agent"
	}
	rTypeStr := getArgString(args, "type")
	if rTypeStr == "" {
		rTypeStr = "custom"
	}
	var rType service.ReminderType
	switch rTypeStr {
	case "first_contact":
		rType = service.ReminderFirstContact
	case "quote_followup":
		rType = service.ReminderQuoteFollowup
	case "after_sale_care":
		rType = service.ReminderAfterSaleCare
	case "repurchase":
		rType = service.ReminderRepurchase
	case "reactivation":
		rType = service.ReminderReactivation
	case "birthday":
		rType = service.ReminderBirthday
	case "custom":
		rType = service.ReminderCustom
	default:
		return ErrorResult(t.Name(), fmt.Errorf("type 必须是 first_contact/quote_followup/after_sale_care/repurchase/reactivation/birthday/custom，实际：%s", rTypeStr)), fmt.Errorf("type 非法：%s", rTypeStr)
	}

	dueInMinutes, _ := GetIntArg(args, "due_in_minutes")
	if dueInMinutes <= 0 {
		dueInMinutes = 1440
	}
	dueIn := time.Duration(dueInMinutes) * time.Minute

	title := getArgString(args, "title")
	description := getArgString(args, "description")
	sopName := getArgString(args, "sop_name")
	channel := getArgString(args, "channel")

	priorityVal, priorityOk := GetIntArgSafe(args, "priority")
	var priority service.ReminderPriority
	if priorityOk {
		switch priorityVal {
		case 0:
			priority = service.PriorityLow
		case 1:
			priority = service.PriorityNormal
		case 2:
			priority = service.PriorityHigh
		case 3:
			priority = service.PriorityUrgent
		default:
			priority = service.PriorityNormal
		}
	} else {
		priority = service.PriorityNormal
	}

	autoHandle := false
	if v, ok := args["auto_handle"].(bool); ok {
		autoHandle = v
	}

	opts := &service.ScheduleOptions{
		Title:       title,
		Description: description,
		Priority:    priority,
		SOPName:     sopName,
		AutoHandle:  autoHandle,
		Channel:     channel,
	}

	reminder, err := t.deps.FollowUpService.Schedule(ctx, customerID, ownerID, rType, dueIn, opts)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}

	return SuccessResult(t.Name(), map[string]any{
		"reminder_id": reminder.ID,
		"customer_id": reminder.CustomerID,
		"owner_id":    reminder.OwnerID,
		"type":        string(reminder.Type),
		"priority":    int(reminder.Priority),
		"title":       reminder.Title,
		"description": reminder.Description,
		"due_at":      reminder.DueAt.Format(time.RFC3339),
		"status":      reminder.Status,
		"sop_name":    reminder.SOPName,
		"auto_handle": reminder.AutoHandle,
		"channel":     reminder.Channel,
		"created_at":  reminder.CreatedAt.Format(time.RFC3339),
		"message":     "跟进任务已创建",
	}), nil
}

// ===== 工具 5：follow_task.update =====

// FollowTaskUpdateTool 更新跟进任务工具
type FollowTaskUpdateTool struct {
	BaseTool
	deps BusinessToolDeps
}

// NewFollowTaskUpdateTool 创建跟进任务更新工具
func NewFollowTaskUpdateTool(deps BusinessToolDeps) *FollowTaskUpdateTool {
	return &FollowTaskUpdateTool{
		BaseTool: BaseTool{
			NameVal:        "follow_task.update",
			CategoryVal:    CategoryBusiness,
			DescriptionVal: "更新跟进任务状态：完成（带结果，自动推进客户旅程）/ 取消。完成时必须提供 result 参数，系统将自动推进客户旅程阶段并更新销售仪表盘。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"reminder_id": {Type: "string", Description: "跟进任务 ID（必填）"},
					"action":      {Type: "string", Description: "操作：complete（完成）/ cancel（取消）", Enum: []string{"complete", "cancel"}, Default: "complete"},
					"result":      {Type: "string", Description: "跟进结果（action=complete 时必填）：contacted（已联系）/ interested（有兴趣）/ quoted（已报价）/ converted（已成交）/ rejected（已拒绝）/ lost（已流失）/ no_response（未响应）", Enum: []string{"contacted", "interested", "quoted", "converted", "rejected", "lost", "no_response"}},
					"note":        {Type: "string", Description: "跟进备注（可选）"},
				},
				Required: []string{"reminder_id", "action"},
			},
		},
		deps: deps,
	}
}

// Execute 执行更新跟进任务
func (t *FollowTaskUpdateTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"reminder_id", "action"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	if t.deps.FollowUpService == nil {
		return ErrorResult(t.Name(), errors.New("follow_task.update 工具需要 FollowUpService 依赖")), errors.New("follow_task.update 工具需要 FollowUpService 依赖")
	}

	reminderID := getArgString(args, "reminder_id")
	if reminderID == "" {
		return ErrorResult(t.Name(), errors.New("reminder_id 不能为空")), errors.New("reminder_id 不能为空")
	}
	action := getArgString(args, "action")
	note := getArgString(args, "note")

	switch action {
	case "complete":
		resultStr := getArgString(args, "result")
		if resultStr == "" {
			return ErrorResult(t.Name(), errors.New("action=complete 时 result 必填")), errors.New("action=complete 时 result 必填")
		}
		var result service.FollowUpResult
		switch resultStr {
		case "contacted":
			result = service.FollowUpResultContacted
		case "interested":
			result = service.FollowUpResultInterested
		case "quoted":
			result = service.FollowUpResultQuoted
		case "converted":
			result = service.FollowUpResultConverted
		case "rejected":
			result = service.FollowUpResultRejected
		case "lost":
			result = service.FollowUpResultLost
		case "no_response":
			result = service.FollowUpResultNoResponse
		default:
			return ErrorResult(t.Name(), fmt.Errorf("result 必须是 contacted/interested/quoted/converted/rejected/lost/no_response，实际：%s", resultStr)), fmt.Errorf("result 非法")
		}
		if err := t.deps.FollowUpService.CompleteWithResult(reminderID, result, note); err != nil {
			return ErrorResult(t.Name(), err), err
		}
		// 计算旅程阶段推进
		stageInfo, ok := service.FollowUpResultInfo[result]
		targetStage := ""
		if ok {
			targetStage = string(stageInfo.TargetStage)
		}
		return SuccessResult(t.Name(), map[string]any{
			"reminder_id":    reminderID,
			"action":         "complete",
			"result":         resultStr,
			"note":           note,
			"target_stage":   targetStage,
			"is_positive":    ok && stageInfo.IsPositive,
			"journey_pushed": ok,
			"message":        "跟进已完成，客户旅程已自动推进",
		}), nil

	case "cancel":
		if err := t.deps.FollowUpService.Cancel(reminderID); err != nil {
			return ErrorResult(t.Name(), err), err
		}
		return SuccessResult(t.Name(), map[string]any{
			"reminder_id": reminderID,
			"action":      "cancel",
			"status":      "canceled",
			"message":     "跟进任务已取消",
		}), nil

	default:
		return ErrorResult(t.Name(), fmt.Errorf("action 必须是 complete/cancel，实际：%s", action)), fmt.Errorf("action 必须是 complete/cancel")
	}
}

// ===== 工具 6：payment.create =====

// PaymentCreateTool 创建支付工具
type PaymentCreateTool struct {
	BaseTool
	deps BusinessToolDeps
}

// NewPaymentCreateTool 创建支付创建工具
func NewPaymentCreateTool(deps BusinessToolDeps) *PaymentCreateTool {
	return &PaymentCreateTool{
		BaseTool: BaseTool{
			NameVal:        "payment.create",
			CategoryVal:    CategoryBusiness,
			DescriptionVal: "为指定客户创建支付订单 + 生成支付 URL。返回 pay_url 与 order_id。用于智能体在客户确认购买后自动发起收款、销售发送支付链接。",
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"account_id": {Type: "string", Description: "客户账号 ID（必填）"},
					"price":      {Type: "string", Description: "支付金额（元，字符串如 \"99.00\"）"},
					"tg_id":      {Type: "integer", Description: "Telegram 用户 ID（可选）"},
				},
				Required: []string{"account_id", "price"},
			},
		},
		deps: deps,
	}
}

// Execute 执行创建支付
func (t *PaymentCreateTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if err := ValidateRequired(args, []string{"account_id", "price"}); err != nil {
		return ErrorResult(t.Name(), err), err
	}
	if t.deps.OrderService == nil {
		return ErrorResult(t.Name(), errors.New("payment.create 工具需要 OrderService 依赖")), errors.New("payment.create 工具需要 OrderService 依赖")
	}

	accountID := getArgString(args, "account_id")
	if accountID == "" {
		return ErrorResult(t.Name(), errors.New("account_id 不能为空")), errors.New("account_id 不能为空")
	}
	priceStr := getArgString(args, "price")
	if priceStr == "" {
		return ErrorResult(t.Name(), errors.New("price 不能为空")), errors.New("price 不能为空")
	}
	price, err := decimal.NewFromString(priceStr)
	if err != nil {
		return ErrorResult(t.Name(), fmt.Errorf("price 格式错误：%v", err)), fmt.Errorf("price 格式错误：%v", err)
	}
	if price.LessThanOrEqual(decimal.Zero) {
		return ErrorResult(t.Name(), errors.New("price 必须大于 0")), errors.New("price 必须大于 0")
	}

	var tgID int64
	if v, ok := GetIntArgSafe(args, "tg_id"); ok {
		tgID = int64(v)
	}

	// 调用 OrderService.CreatePayAndReturn（内部会创建 pending 订单 + 返回支付 URL）
	payURL, orderID, err := t.deps.OrderService.CreatePayAndReturn(accountID, price, tgID)
	if err != nil {
		return ErrorResult(t.Name(), err), err
	}

	// 生成完整支付链接（如果 payURL 是相对路径，拼接为完整 URL）
	fullPayURL := payURL
	if strings.HasPrefix(payURL, "/") {
		// 生产环境应从配置读取 base URL；这里保持相对路径，由前端拼接
		fullPayURL = payURL
	}

	// 记录支付创建审计（写入 payment_configs 表的 last_used 或独立审计日志）
	// 此处简化：仅返回结果，不写额外审计表（避免侵入现有模型）

	return SuccessResult(t.Name(), map[string]any{
		"order_id":    orderID,
		"account_id":  accountID,
		"price":       price.String(),
		"pay_url":     fullPayURL,
		"status":      "pending",
		"status_desc": "pending",
		"expire_at":   time.Now().Add(30 * time.Minute).Format(time.RFC3339), // 默认 30 分钟过期
		"message":     "支付订单已创建，请引导客户完成支付",
	}), nil
}
