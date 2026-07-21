package dto

import (
	"time"

	"marketing/internal/model"
)

// CustomerRFMResponse 客户 RFM 响应
type CustomerRFMResponse struct {
	CustomerID     string `json:"customer_id"`
	UnifiedID      string `json:"unified_id"`
	RecencyDays    int    `json:"recency_days"`
	Frequency      int    `json:"frequency"`
	MonetaryTotal  int64  `json:"monetary_total"`
	AvgOrderValue  int64  `json:"avg_order_value"`
	RScore         int    `json:"r_score"`
	FScore         int    `json:"f_score"`
	MScore         int    `json:"m_score"`
	CompositeScore int    `json:"composite_score"`
	Segment        string `json:"segment"`
	SegmentDesc    string `json:"segment_desc"`
	ChurnRiskLevel string `json:"churn_risk_level"`
	ChurnScore     int    `json:"churn_score"`
	LastActiveAt   string `json:"last_active_at"`
	ComputedAt     string `json:"computed_at"`
}

// FromRFM RFM 实体 → 响应
func (r *CustomerRFMResponse) FromRFM(rfm *model.CustomerRFM) *CustomerRFMResponse {
	if rfm == nil {
		return nil
	}
	resp := &CustomerRFMResponse{
		CustomerID:     rfm.CustomerID,
		UnifiedID:      rfm.UnifiedID,
		RecencyDays:    rfm.RecencyDays,
		Frequency:      rfm.Frequency,
		MonetaryTotal:  rfm.MonetaryTotal,
		AvgOrderValue:  rfm.AvgOrderValue,
		RScore:         rfm.RScore,
		FScore:         rfm.FScore,
		MScore:         rfm.MScore,
		CompositeScore: rfm.CompositeScore,
		Segment:        rfm.Segment,
		SegmentDesc:    model.RFMSegmentDescriptions[rfm.Segment],
		ChurnRiskLevel: rfm.ChurnRiskLevel,
		ChurnScore:     rfm.ChurnScore,
		ComputedAt:     rfm.ComputedAt.UTC().Format(time.RFC3339),
	}
	if rfm.LastActiveAt != nil {
		resp.LastActiveAt = rfm.LastActiveAt.UTC().Format(time.RFC3339)
	}
	return resp
}

// CustomerRFMListResponse 列表响应
type CustomerRFMListResponse struct {
	List     []*CustomerRFMResponse `json:"list"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

// RFMComputeRequest 触发计算请求
type RFMComputeRequest struct {
	CustomerID string `json:"customer_id"`
}

// RFMComputeAllResponse 批量计算响应
type RFMComputeAllResponse struct {
	Computed int `json:"computed"`
	Limit    int `json:"limit"`
}

// RFMComputeAllRequest 批量计算请求
type RFMComputeAllRequest struct {
	Limit int `form:"limit" json:"limit"`
}

// RFMDistributionResponse 分层分布
type RFMDistributionResponse struct {
	Distribution map[string]int64 `json:"distribution"`
	Total        int64            `json:"total"`
}
