package service

import (
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// FromCustomerRFMModel RFM 实体 → 响应
// 转换属业务层职责（原位于 dto 包，P0-7 下沉至 service，dto 保持纯数据结构）
func FromCustomerRFMModel(rfm *model.CustomerRFM) *dto.CustomerRFMResponse {
	if rfm == nil {
		return nil
	}
	resp := &dto.CustomerRFMResponse{
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

