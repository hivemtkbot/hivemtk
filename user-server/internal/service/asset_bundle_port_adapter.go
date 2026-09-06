package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/aiagent/agent/portcontract"
	"hivemtk-user/internal/model"
)

// AssetBundleWeavePortAdapter 适配 AssetBundleService → portcontract.AssetBundleWeavePort
type AssetBundleWeavePortAdapter struct {
	svc *AssetBundleService
}

// NewAssetBundleWeavePortAdapter 构造
func NewAssetBundleWeavePortAdapter(svc *AssetBundleService) *AssetBundleWeavePortAdapter {
	if svc == nil {
		return &AssetBundleWeavePortAdapter{svc: nil}
	}
	return &AssetBundleWeavePortAdapter{svc: svc}
}

// WeaveForRequest 实现 portcontract.AssetBundleWeavePort
//
// 把 portcontract.WeaveRequestPort 转换为 service.WeaveInput，
// 调用 svc.WeaveForRequest 完成织布
func (a *AssetBundleWeavePortAdapter) WeaveForRequest(ctx context.Context, in portcontract.WeaveRequestPort) ([]model.AssetBundleMessage, error) {
	if a == nil || a.svc == nil {
		return nil, errors.New("asset bundle weave port adapter not configured")
	}
	ragDocs := make([]RAGDocument, 0, len(in.RAGDocs))
	for _, d := range in.RAGDocs {
		ragDocs = append(ragDocs, RAGDocument{
			ID:      d.ID,
			Title:   d.Title,
			Content: d.Content,
			Score:   d.Score,
			Source:  d.Source,
		})
	}
	weaveIn := WeaveInput{
		UserQuery:    in.UserQuery,
		RAGDocs:      ragDocs,
		ChatHistory:  in.ChatHistory,
		MerchantVars: in.MerchantVars,
		Options: WeaveOptions{
			RAGPosition:         RAGInsertPosition(in.Options.RAGPosition),
			MaxHistoryMessages:  in.Options.MaxHistoryMessages,
			StripFewShotJSON:    in.Options.StripFewShotJSON,
			IncludeMerchantVars: in.Options.IncludeMerchantVars,
		},
	}
	msgs, err := a.svc.WeaveForRequest(ctx, in.AssetID, in.UserQuery, &weaveIn)
	if err != nil {
		if errors.Is(err, ErrBundleNotHotEnabled) {
			return nil, portcontract.ErrAssetBundleNotEnabled
		}
		return nil, err
	}
	return msgs, nil
}

// IsBundleEnabled 实现 portcontract.AssetBundleWeavePort
// IsBundleEnabled 判断资产是否启用
//
// 工具层 Port 接口签名约束为单参;此处为 Port 模式薄包装,使用 background ctx
// 隔离与上游请求的取消传播（资产启用判断不应被业务请求取消影响）。
func (a *AssetBundleWeavePortAdapter) IsBundleEnabled(assetID string) bool {
	if a == nil || a.svc == nil {
		return false
	}
	return a.svc.IsBundleEnabled(context.Background(), assetID)
}

var _ portcontract.AssetBundleWeavePort = (*AssetBundleWeavePortAdapter)(nil)

// KnowledgeSearchPortAdapter 适配 KnowledgeBaseService → portcontract.KnowledgeSearchPort
type KnowledgeSearchPortAdapter struct {
	svc KnowledgeSearcher
}

// KnowledgeSearcher 抽象 KnowledgeBaseService 的 Search 方法（避免硬依赖）
type KnowledgeSearcher interface {
	Search(ctx context.Context, query string, topK int) ([]portcontract.RAGDocumentPort, error)
}

// NewKnowledgeSearchPortAdapter 构造
func NewKnowledgeSearchPortAdapter(svc KnowledgeSearcher) *KnowledgeSearchPortAdapter {
	return &KnowledgeSearchPortAdapter{svc: svc}
}

// Search 实现 portcontract.KnowledgeSearchPort
func (a *KnowledgeSearchPortAdapter) Search(ctx context.Context, query string, topK int) ([]portcontract.RAGDocumentPort, error) {
	if a == nil || a.svc == nil {
		return nil, errors.New("knowledge search port adapter not configured")
	}
	return a.svc.Search(ctx, query, topK)
}

var _ portcontract.KnowledgeSearchPort = (*KnowledgeSearchPortAdapter)(nil)
