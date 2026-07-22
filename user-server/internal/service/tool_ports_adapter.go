package service

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"marketing/internal/aiagent/agent/portcontract"
	"marketing/internal/model"
)

// ============================================================================
// tooluse Port 适配器（L4 依赖反转）
// ----------------------------------------------------------------------------
// tooluse 只依赖 portcontract.CustomerPort / SessionPort / OrderPort / FollowUpPort；
// 本文件把业务 Service 适配为 Port，在 router 装配期注入。
// ============================================================================

// ----- CustomerPort -----

// CustomerPortAdapter 适配 CustomerService → portcontract.CustomerPort
type CustomerPortAdapter struct {
	svc *CustomerService
}

// NewCustomerPortAdapter 构造
func NewCustomerPortAdapter(svc *CustomerService) *CustomerPortAdapter {
	if svc == nil {
		svc = NewCustomerService()
	}
	return &CustomerPortAdapter{svc: svc}
}

func (a *CustomerPortAdapter) GetCustomerProfile(customerID string) (*portcontract.CustomerProfileView, error) {
	p, err := a.svc.GetCustomerProfile(customerID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	return &portcontract.CustomerProfileView{
		Customer:     p.Customer,
		RecentEvents: p.RecentEvents,
		Tags:         p.Tags,
	}, nil
}

func (a *CustomerPortAdapter) CreateOrUpdate(identity *portcontract.CustomerIdentity) (*model.Customer, error) {
	if identity == nil {
		return nil, ErrInvalidDTO
	}
	return a.svc.CreateOrUpdate(&CustomerDTO{
		Phone:         identity.Phone,
		Email:         identity.Email,
		WechatOpenID:  identity.WechatOpenID,
		DouyinOpenID:  identity.DouyinOpenID,
		XiaohongshuID: identity.XiaohongshuID,
	})
}

func (a *CustomerPortAdapter) MergeCustomers(primaryID, secondaryID string) error {
	return a.svc.MergeCustomers(primaryID, secondaryID)
}

func (a *CustomerPortAdapter) AddTags(customerID string, tags []string) error {
	return a.svc.AddTags(customerID, tags)
}

func (a *CustomerPortAdapter) RemoveTags(customerID string, tags []string) error {
	return a.svc.RemoveTags(customerID, tags)
}

// ----- SessionPort -----

// SessionPortAdapter 适配 CustomerSessionService → portcontract.SessionPort
type SessionPortAdapter struct {
	svc *CustomerSessionService
}

// NewSessionPortAdapter 构造
func NewSessionPortAdapter(svc *CustomerSessionService) *SessionPortAdapter {
	if svc == nil {
		svc = NewCustomerSessionService()
	}
	return &SessionPortAdapter{svc: svc}
}

func (a *SessionPortAdapter) CreateSession(ctx context.Context, in *portcontract.CreateSessionInput) (*model.CustomerSession, error) {
	if in == nil {
		return nil, ErrInvalidDTO
	}
	_ = ctx
	return a.svc.CreateSession(&CreateSessionRequest{
		Platform:  model.Platform(in.Platform),
		AccountID: in.AccountID,
		UserID:    in.UserID,
		UserName:  in.UserName,
		UserPhone: in.UserPhone,
		UserEmail: in.UserEmail,
	})
}

func (a *SessionPortAdapter) GetMessages(sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error) {
	return a.svc.GetMessages(sessionID, page, pageSize)
}

func (a *SessionPortAdapter) SendMessage(ctx context.Context, in *portcontract.SendMessageInput) (*model.SessionMessage, error) {
	if in == nil {
		return nil, ErrInvalidDTO
	}
	_ = ctx
	ct := model.MessageType(in.ContentType)
	if ct == "" {
		ct = model.MessageTypeText
	}
	return a.svc.SendMessage(&SendMessageRequest{
		SessionID:   in.SessionID,
		SenderType:  in.SenderType,
		SenderID:    in.SenderID,
		Content:     in.Content,
		ContentType: ct,
	})
}

// ----- OrderPort -----

// OrderPortAdapter 适配 OrderService → portcontract.OrderPort
type OrderPortAdapter struct {
	svc *OrderService
}

// NewOrderPortAdapter 构造
func NewOrderPortAdapter(svc *OrderService) *OrderPortAdapter {
	if svc == nil {
		svc = NewOrderService()
	}
	return &OrderPortAdapter{svc: svc}
}

func (a *OrderPortAdapter) CreateOrderFromRequest(order *model.Order) (*model.Order, error) {
	return a.svc.CreateOrderFromRequest(order)
}

func (a *OrderPortAdapter) GetOrderByID(orderID string) (*model.Order, error) {
	return a.svc.GetOrderByID(orderID)
}

func (a *OrderPortAdapter) GetOrderList(page, pageSize int) ([]*model.Order, int64, error) {
	return a.svc.GetOrderList(page, pageSize)
}

func (a *OrderPortAdapter) CreatePayAndReturn(accountID string, price float64, tgID int64) (string, string, error) {
	return a.svc.CreatePayAndReturn(accountID, decimal.NewFromFloat(price), tgID)
}

// ----- FollowUpPort -----

// FollowUpPortAdapter 适配 FollowUpService → portcontract.FollowUpPort
type FollowUpPortAdapter struct {
	svc *FollowUpService
}

// NewFollowUpPortAdapter 构造
func NewFollowUpPortAdapter(svc *FollowUpService) *FollowUpPortAdapter {
	if svc == nil {
		svc = NewFollowUpService(NewCustomerJourneyService())
	}
	return &FollowUpPortAdapter{svc: svc}
}

func (a *FollowUpPortAdapter) Schedule(ctx context.Context, customerID, ownerID, reminderType string, dueIn time.Duration, opts *portcontract.FollowUpScheduleOptions) (any, error) {
	rType := ReminderType(reminderType)
	var so *ScheduleOptions
	if opts != nil {
		so = &ScheduleOptions{
			Title:       opts.Title,
			Description: opts.Note,
			Priority:    parseReminderPriority(opts.Priority),
		}
	}
	return a.svc.Schedule(ctx, customerID, ownerID, rType, dueIn, so)
}

func parseReminderPriority(p string) ReminderPriority {
	switch p {
	case "low":
		return PriorityLow
	case "high":
		return PriorityHigh
	case "urgent":
		return PriorityUrgent
	default:
		return PriorityNormal
	}
}

func (a *FollowUpPortAdapter) CompleteWithResult(reminderID string, result, note string) error {
	return a.svc.CompleteWithResult(reminderID, FollowUpResult(result), note)
}

func (a *FollowUpPortAdapter) Cancel(reminderID string) error {
	return a.svc.Cancel(reminderID)
}

func (a *FollowUpPortAdapter) ResultInfo(result string) (stage string, ok bool) {
	info, ok := FollowUpResultInfo[FollowUpResult(result)]
	if !ok {
		return "", false
	}
	return string(info.TargetStage), true
}
