package router

import (
	"context"

	knowledgectrl "hivemtk-user/internal/aiagent/knowledge/controller"
	knowledgemodel "hivemtk-user/internal/aiagent/knowledge/model"
	"hivemtk-user/internal/service"
)

// openAPISourceAdapter P2-3：service.OpenAPIService → knowledgectrl.OpenAPISourcePort 适配器。
//
// 装配期在 router 注入，使 aiagent/knowledge/controller 不再 import service。
type openAPISourceAdapter struct {
	svc *service.OpenAPIService
}

func (a *openAPISourceAdapter) ListSources(ctx context.Context, productID string) ([]knowledgemodel.KnowledgeOpenAPISource, error) {
	return a.svc.ListSources(ctx, productID)
}

func (a *openAPISourceAdapter) CreateSource(ctx context.Context, src *knowledgemodel.KnowledgeOpenAPISource) error {
	return a.svc.CreateSource(ctx, src)
}

func (a *openAPISourceAdapter) GetSource(ctx context.Context, productID string, id int64) (*knowledgemodel.KnowledgeOpenAPISource, error) {
	return a.svc.GetSource(ctx, productID, id)
}

func (a *openAPISourceAdapter) UpdateSource(ctx context.Context, src *knowledgemodel.KnowledgeOpenAPISource) error {
	return a.svc.UpdateSource(ctx, src)
}

func (a *openAPISourceAdapter) DeleteSource(ctx context.Context, productID string, id int64) error {
	return a.svc.DeleteSource(ctx, productID, id)
}

func (a *openAPISourceAdapter) ToggleEnabled(ctx context.Context, productID string, id int64, enabled bool) error {
	return a.svc.ToggleEnabled(ctx, productID, id, enabled)
}

func (a *openAPISourceAdapter) SyncSource(ctx context.Context, productID string, sourceID int64) (*knowledgectrl.OpenAPISyncResult, error) {
	r, err := a.svc.SyncSource(ctx, productID, sourceID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return &knowledgectrl.OpenAPISyncResult{
		SourceID:    r.SourceID,
		TotalItems:  r.TotalItems,
		ImportedNum: r.ImportedNum,
		SkippedNum:  r.SkippedNum,
		FailedNum:   r.FailedNum,
		DurationMs:  r.DurationMs,
		Status:      r.Status,
		ErrorMsg:    r.ErrorMsg,
	}, nil
}

func (a *openAPISourceAdapter) TestConnection(ctx context.Context, src *knowledgemodel.KnowledgeOpenAPISource) (map[string]any, error) {
	return a.svc.TestConnection(ctx, src)
}
