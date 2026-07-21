package service

import (
	"encoding/json"
	"errors"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
)

// PaymentConfigService 支付配置服务
type PaymentConfigService struct {
	repo *repository.PaymentConfigRepository
}

// NewPaymentConfigService 创建支付配置服务实例
func NewPaymentConfigService() *PaymentConfigService {
	return &PaymentConfigService{repo: repository.NewPaymentConfigRepository()}
}

// PaymentConfigRequest 前端提交配置请求
type PaymentConfigRequest struct {
	DefaultMethod   string         `json:"defaultMethod"`
	Timeout         int            `json:"timeout"`
	AutoConfirm     bool           `json:"autoConfirm"`
	AutoConfirmDays int            `json:"autoConfirmDays"`
	RefundAudit     bool           `json:"refundAudit"`
	Alipay          map[string]any `json:"alipay"`
	Wechat          map[string]any `json:"wechat"`
	Unionpay        map[string]any `json:"unionpay"`
}

// PaymentConfigResponse 前端响应的配置
type PaymentConfigResponse struct {
	ID              uint           `json:"id"`
	DefaultMethod   string         `json:"defaultMethod"`
	Timeout         int            `json:"timeout"`
	AutoConfirm     bool           `json:"autoConfirm"`
	AutoConfirmDays int            `json:"autoConfirmDays"`
	RefundAudit     bool           `json:"refundAudit"`
	Alipay          map[string]any `json:"alipay"`
	Wechat          map[string]any `json:"wechat"`
	Unionpay        map[string]any `json:"unionpay"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
}

// GetConfig 获取支付配置(单租户模式)
func (s *PaymentConfigService) GetConfig() (*PaymentConfigResponse, error) {
	config, err := s.repo.GetConfig()
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return defaultPaymentConfig(), nil
		}
		return nil, err
	}
	if config == nil {
		// 返回默认配置
		return defaultPaymentConfig(), nil
	}
	return toPaymentConfigResponse(config), nil
}

// SaveConfig 保存支付配置(单租户模式)
func (s *PaymentConfigService) SaveConfig(req *PaymentConfigRequest) (*PaymentConfigResponse, error) {
	if req == nil {
		return nil, errors.New("配置数据不能为空")
	}

	// 校验
	if req.DefaultMethod != "" && req.DefaultMethod != "alipay" && req.DefaultMethod != "wechat" && req.DefaultMethod != "unionpay" {
		return nil, errors.New("defaultMethod 必须是 alipay/wechat/unionpay 之一")
	}
	if req.Timeout < 0 {
		req.Timeout = 30
	}
	if req.Timeout > 1440 {
		req.Timeout = 1440
	}
	if req.AutoConfirmDays < 1 {
		req.AutoConfirmDays = 7
	}
	if req.AutoConfirmDays > 30 {
		req.AutoConfirmDays = 30
	}

	alipayJSON, _ := json.Marshal(req.Alipay)
	wechatJSON, _ := json.Marshal(req.Wechat)
	unionpayJSON, _ := json.Marshal(req.Unionpay)

	config := &model.PaymentConfig{
		DefaultMethod:   req.DefaultMethod,
		Timeout:         req.Timeout,
		AutoConfirm:     req.AutoConfirm,
		AutoConfirmDays: req.AutoConfirmDays,
		RefundAudit:     req.RefundAudit,
		AlipayConfig:    string(alipayJSON),
		WechatConfig:    string(wechatJSON),
		UnionpayConfig:  string(unionpayJSON),
	}

	if err := s.repo.Upsert(config); err != nil {
		return nil, err
	}

	// 重新读取以返回最新数据
	updated, _ := s.repo.GetConfig()
	if updated == nil {
		return toPaymentConfigResponse(config), nil
	}
	return toPaymentConfigResponse(updated), nil
}

// defaultPaymentConfig 返回默认配置
func defaultPaymentConfig() *PaymentConfigResponse {
	return &PaymentConfigResponse{
		DefaultMethod:   "alipay",
		Timeout:         30,
		AutoConfirm:     true,
		AutoConfirmDays: 7,
		RefundAudit:     false,
		Alipay:          map[string]any{"appId": "", "privateKey": "", "publicKey": ""},
		Wechat:          map[string]any{"appId": "", "mchId": "", "apiKey": ""},
		Unionpay:        map[string]any{"merId": "", "certPath": ""},
	}
}

func toPaymentConfigResponse(config *model.PaymentConfig) *PaymentConfigResponse {
	resp := &PaymentConfigResponse{
		ID:              config.ID,
		DefaultMethod:   config.DefaultMethod,
		Timeout:         config.Timeout,
		AutoConfirm:     config.AutoConfirm,
		AutoConfirmDays: config.AutoConfirmDays,
		RefundAudit:     config.RefundAudit,
		Alipay:          safeParseJSON(config.AlipayConfig),
		Wechat:          safeParseJSON(config.WechatConfig),
		Unionpay:        safeParseJSON(config.UnionpayConfig),
	}
	if !config.CreatedAt.IsZero() {
		resp.CreatedAt = config.CreatedAt.Format(time.RFC3339)
	}
	if !config.UpdatedAt.IsZero() {
		resp.UpdatedAt = config.UpdatedAt.Format(time.RFC3339)
	}
	return resp
}

func safeParseJSON(s2 string) map[string]any {
	if s2 == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s2), &m); err != nil {
		return map[string]any{}
	}
	return m
}
