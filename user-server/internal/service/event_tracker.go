package service

import (
	"context"
	"encoding/json"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"time"
)

// EventTracker 事件追踪服务
type EventTracker struct {
	repo         repository.CustomerEventRepository
	customerRepo repository.CustomerRepository
	autoTagger   *AutoTagger
	disableAsync bool // 用于测试禁用异步处理
}

// NewEventTracker 创建事件追踪服务实例
func NewEventTracker(customerService *CustomerService) *EventTracker {
	return &EventTracker{
		repo:         repository.NewCustomerEventRepository(),
		customerRepo: repository.NewCustomerRepository(),
		autoTagger:   NewAutoTagger(),
	}
}

// DisableAsync 禁用异步处理（用于测试）
func (s *EventTracker) DisableAsync(ctx context.Context)  {
	s.disableAsync = true
}

// EventDTO 事件数据传输对象
type EventDTO struct {
	CustomerID  string            `json:"customer_id"`
	EventType   model.EventType   `json:"event_type"`
	EventSource model.EventSource `json:"event_source"`
	EventData   map[string]any    `json:"event_data"`
}

// Track 追踪通用事件
func (s *EventTracker) Track(ctx context.Context, dto *EventDTO) error {
	if dto == nil || dto.CustomerID == "" {
		return ErrInvalidDTO
	}

	// 创建事件记录
	event := &model.CustomerEvent{
		CustomerID:  dto.CustomerID,
		EventType:   dto.EventType,
		EventSource: dto.EventSource,
		OccurredAt:  time.Now(),
	}

	// 设置事件数据
	if dto.EventData != nil {
		if err := SetCustomerEventData(event, dto.EventData); err != nil {
			return err
		}
	}

	// 记录事件
	if err := s.repo.Record(ctx, event); err != nil {
		return err
	}

	// 异步触发自动标签处理
	if !s.disableAsync {
		// R6 修复：原 goroutine 无 recover，panic 会杀进程。添加 recover 保护。
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("event_tracker: AutoTagger.ProcessEvent recovered from panic: %v", r)
				}
			}()
			if err := s.autoTagger.ProcessEvent(ctx, event); err != nil {
				logger.Errorf("AutoTagger.ProcessEvent error: %v", err)
			}
		}()
	}

	return nil
}

// TrackPageView 追踪页面浏览
func (s *EventTracker) TrackPageView(ctx context.Context, customerID, url, title string) error {
	return s.Track(ctx, &EventDTO{
		CustomerID:  customerID,
		EventType:   model.EventTypePageView,
		EventSource: model.EventSourceWebsite,
		EventData: map[string]any{
			"url":   url,
			"title": title,
		},
	})
}

// TrackClick 追踪点击事件
func (s *EventTracker) TrackClick(ctx context.Context, customerID, element, target string) error {
	return s.Track(ctx, &EventDTO{
		CustomerID:  customerID,
		EventType:   model.EventTypeClick,
		EventSource: model.EventSourceWebsite,
		EventData: map[string]any{
			"element": element,
			"target":  target,
		},
	})
}

// TrackPurchase 追踪购买事件
func (s *EventTracker) TrackPurchase(ctx context.Context, customerID string, amount float64, items []string) error {
	return s.Track(ctx, &EventDTO{
		CustomerID:  customerID,
		EventType:   model.EventTypePurchase,
		EventSource: model.EventSourceWebsite,
		EventData: map[string]any{
			"amount": amount,
			"items":  items,
		},
	})
}

// GetEventHistory 获取客户事件历史
func (s *EventTracker) GetEventHistory(ctx context.Context, customerID string, limit int)  ([]*model.CustomerEvent, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	return s.repo.GetByCustomerID(ctx, customerID, limit)
}

// GetStats 获取事件统计
func (s *EventTracker) GetStats(ctx context.Context, start, end string)  (*repository.EventStats, error) {
	// 解析时间范围
	startTime, err := time.Parse("2006-01-02", start)
	if err != nil {
		startTime = time.Now().AddDate(0, -1, 0) // 默认过去 30 天
	}

	endTime, err := time.Parse("2006-01-02", end)
	if err != nil {
		endTime = time.Now()
	}

	return s.repo.GetStats(ctx, startTime, endTime)
}

// TrackSignup 追踪注册事件
func (s *EventTracker) TrackSignup(ctx context.Context, customerID, signupMethod string) error {
	return s.Track(ctx, &EventDTO{
		CustomerID:  customerID,
		EventType:   model.EventTypeSignup,
		EventSource: model.EventSourceWebsite,
		EventData: map[string]any{
			"signup_method": signupMethod,
		},
	})
}

// TrackLogin 追踪登录事件
func (s *EventTracker) TrackLogin(ctx context.Context, customerID, loginMethod string) error {
	return s.Track(ctx, &EventDTO{
		CustomerID:  customerID,
		EventType:   model.EventTypeLogin,
		EventSource: model.EventSourceWebsite,
		EventData: map[string]any{
			"login_method": loginMethod,
		},
	})
}

// TrackAddToCart 追踪加购事件
func (s *EventTracker) TrackAddToCart(ctx context.Context, customerID, productID, productName string, price float64, quantity int) error {
	return s.Track(ctx, &EventDTO{
		CustomerID:  customerID,
		EventType:   model.EventTypeAddToCart,
		EventSource: model.EventSourceWebsite,
		EventData: map[string]any{
			"product_id":   productID,
			"product_name": productName,
			"price":        price,
			"quantity":     quantity,
		},
	})
}

// TrackWithEventData 追踪带自定义数据的事件
func (s *EventTracker) TrackWithEventData(ctx context.Context, customerID, eventType, eventSource string, eventData map[string]any) error {
	dto := &EventDTO{
		CustomerID: customerID,
		EventType:  model.EventType(eventType),
	}

	if eventSource != "" {
		dto.EventSource = model.EventSource(eventSource)
	}

	if eventData != nil {
		dto.EventData = eventData
	}

	return s.Track(ctx, dto)
}

// GetEventCount 获取客户事件数量
func (s *EventTracker) GetEventCount(ctx context.Context, customerID string) (int64, error) {
	events, err := s.repo.GetByCustomerID(ctx, customerID, -1)
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}

// DeleteByCustomerID 删除指定客户的所有事件，返回删除条数
func (s *EventTracker) DeleteByCustomerID(ctx context.Context, customerID string)  (int64, error) {
	return s.repo.DeleteByCustomerID(ctx, customerID)
}

// SerializeEventData 序列化事件数据为 JSON 字符串
func SerializeEventData(data map[string]any) (string, error) {
	if data == nil {
		return "{}", nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}
