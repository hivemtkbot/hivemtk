package router

import (
	"context"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
)


// operationLogSink 审计日志落库适配器：middleware.AuditEntry → model.OperationLog → repository
type operationLogSink struct{}

func (operationLogSink) Save(ctx context.Context, entry *middleware.AuditEntry) error {
	return repository.NewOperationLogRepository().Create(ctx, &model.OperationLog{
		UserID:     entry.UserID,
		Username:   entry.Username,
		Action:     entry.Action,
		Module:     entry.Module,
		Resource:   entry.Resource,
		ResourceID: entry.ResourceID,
		Detail:     entry.Detail,
		IP:         entry.IP,
		UserAgent:  entry.UserAgent,
	})
}

// chatChannelResolver 渠道解析适配器：service.ChatChannelService → middleware.ChatChannelResolver
type chatChannelResolver struct {
	svc *service.ChatChannelService
}

func (r chatChannelResolver) ResolveByAppKey(ctx context.Context, appKey string) (*middleware.ChatChannelView, error) {
	channel, err := r.svc.GetByAppKey(ctx, appKey)
	if err != nil {
		return nil, err
	}
	return toChatChannelView(channel), nil
}

func (r chatChannelResolver) ResolveByChannelID(ctx context.Context, channelID string) (*middleware.ChatChannelView, error) {
	channel, err := r.svc.GetByChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return toChatChannelView(channel), nil
}

// toChatChannelView model.ChatChannel → middleware.ChatChannelView
func toChatChannelView(c *model.ChatChannel) *middleware.ChatChannelView {
	return &middleware.ChatChannelView{
		ChannelID:      c.ChannelID,
		ChannelName:    c.ChannelName,
		Status:         string(c.Status),
		Active:         service.ChatChannelIsActive(c),
		AllowedOrigins: service.ChatChannelAllowedOriginsList(c),
	}
}

// injectMiddlewarePorts 注入 middleware 窄接口实现（须在路由注册前调用）
func injectMiddlewarePorts() {
	middleware.SetPermChecker(service.NewPermissionService())
	middleware.SetAuditSink(operationLogSink{})
}

