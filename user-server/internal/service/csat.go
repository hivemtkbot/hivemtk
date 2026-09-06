// csat.go CSAT 满意度服务（五层 L3）
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// CSATService 满意度调查服务
type CSATService struct {
	repo    *repository.CSATSurveyRepository
	session *repository.CustomerSessionRepository
	kv      repository.SystemConfigKVRepository
	now     func() time.Time
}

// NewCSATService 构造
func NewCSATService() *CSATService {
	return &CSATService{
		repo:    repository.NewCSATSurveyRepository(),
		session: repository.NewCustomerSessionRepository(),
		kv:      repository.NewSystemConfigKVRepository(),
		now:     time.Now,
	}
}

// Trigger 手动/自动触发调查（一会话一调查幂等；状态 pending→sent）
//
// 修复 CSAT Trigger P0 断点：落库后同步向访客所在渠道推送评分邀请消息。
// WebWidget 等 bridge 渠道通过 DeliverBridgeOutbound 下发，前端会话关闭后即可收到；
// 非 bridge 渠道暂不落渠道消息，仅落库 csat_surveys(state=sent) 并记录日志，
// 后续前端/Webhook 可根据 sent 状态轮询展示评分 UI。
func (s *CSATService) Trigger(ctx context.Context, sessionID, triggeredBy string) (*model.CSATSurvey, error) {
	if triggeredBy == "" {
		triggeredBy = "manual"
	}
	sess, err := s.session.GetBySessionID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("会话不存在: %w", err)
	}
	survey, err := s.repo.UpsertBySession(ctx, &model.CSATSurvey{
		SessionID:   sessionID,
		OneID:       sess.OneID,
		Status:      model.CSATStatusSent,
		TriggeredBy: triggeredBy,
	})
	if err != nil {
		return nil, err
	}
	if err := s.repo.MarkSent(ctx, sessionID); err != nil {
		return nil, err
	}
	survey.Status = model.CSATStatusSent
	now := s.now()
	survey.SentAt = &now

	platform := string(sess.Platform)
	accountID := sess.AccountID
	if accountID == "" {
		logger.Ctx(ctx).Warn().
			Str("module", "csat").
			Str("session_id", sessionID).
			Msg("skip csat outbound: session has empty account_id")
		return survey, nil
	}

	ratingMsg := "本次会话已结束，请为我们的服务打分 ⭐ 1-5 分（回复数字即可）"

	if isBridgeChannel(platform) {
		if err := DeliverBridgeOutbound(ctx, platform, accountID, sessionID, "text", ratingMsg, "csat-trigger"); err != nil {
			logger.Ctx(ctx).Error().Err(err).
				Str("module", "csat").
				Str("channel", platform).
				Str("account_id", accountID).
				Str("session_id", sessionID).
				Msg("csat bridge outbound failed (non-fatal; survey already persisted)")
		} else {
			logger.Ctx(ctx).Info().
				Str("module", "csat").
				Str("channel", platform).
				Str("account_id", accountID).
				Str("session_id", sessionID).
				Msg("csat rating invite pushed via bridge outbound")
		}
	} else {

		logger.Ctx(ctx).Info().
			Str("module", "csat").
			Str("channel", platform).
			Str("account_id", accountID).
			Str("session_id", sessionID).
			Uint("survey_id", survey.ID).
			Msg("CSAT survey created (state=sent, non-bridge channel; awaiting frontend/webhook to pick up)")
	}

	return survey, nil
}

// Submit 提交评分（公开端点：客户提交后回写统计）
func (s *CSATService) Submit(ctx context.Context, sessionID string, score int, comment string) (*model.CSATSurvey, error) {
	if score < 1 || score > 5 {
		return nil, fmt.Errorf("score 必须在 1-5")
	}
	return s.repo.SubmitResponse(ctx, sessionID, score, comment)
}

// Stats 统计
func (s *CSATService) Stats(ctx context.Context) (map[string]any, error) {
	return s.repo.Stats(ctx)
}

// Trend 趋势
func (s *CSATService) Trend(ctx context.Context, days int) ([]map[string]any, error) {
	return s.repo.Trend(ctx, days)
}

// Negative 差评列表（阈值取模板 low_threshold，默认 3）
func (s *CSATService) Negative(ctx context.Context, limit int) ([]*model.CSATSurvey, int, error) {
	tpl := s.GetTemplate(ctx)
	threshold := 3
	if tpl["low_threshold"] != nil {
		if v, ok := tpl["low_threshold"].(float64); ok && v > 0 {
			threshold = int(v)
		}
	}
	list, err := s.repo.ListNegative(ctx, threshold, limit)
	return list, threshold, err
}

// GetTemplate 模板配置（未配置回退默认）
func (s *CSATService) GetTemplate(ctx context.Context) map[string]any {
	out := map[string]any{}
	raw, err := s.kv.Get(ctx, model.CSATTemplateKey)
	if err == nil && raw != "" {
		if err := json.Unmarshal([]byte(raw), &out); err == nil && len(out) > 0 {
			return out
		}
	}
	_ = json.Unmarshal([]byte(model.CSATDefaultTemplate), &out)
	return out
}

// SaveTemplate 保存模板配置
func (s *CSATService) SaveTemplate(ctx context.Context, tpl map[string]any) error {
	raw, err := json.Marshal(tpl)
	if err != nil {
		return err
	}
	_, err = s.kv.Upsert(ctx, model.CSATTemplateKey, string(raw))
	return err
}
