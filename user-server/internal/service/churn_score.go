package service

// 分档常量
const (
	ChurnBandHealthy  = "healthy"
	ChurnBandWatch    = "watch"
	ChurnBandHighRisk = "high_risk"
)

// ChurnInput 流失评分输入
type ChurnInput struct {
	// RFMScore RFM 归一化分（0-1）
	RFMScore float64
	// MsgSlope30d 近30天/前30天消息量比值（>1 增长，<1 衰退）
	MsgSlope30d float64
	// NegSentRatio 负面情绪消息占比（0-1）
	NegSentRatio float64
	// ReplyDelayProlong 回复延迟延长程度（0-1）
	ReplyDelayProlong float64
}

func churnClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ChurnScore 流失评分 = 40*RFM + 25*clamp(slope,0,1) + 20*NegSentRatio + 15*ReplyDelayProlong
func ChurnScore(in ChurnInput) float64 {
	return 40*churnClamp01(in.RFMScore) +
		25*churnClamp01(in.MsgSlope30d) +
		20*churnClamp01(in.NegSentRatio) +
		15*churnClamp01(in.ReplyDelayProlong)
}

// ChurnBand 三档分级：<40 healthy / 40-70 watch / >70 high_risk
func ChurnBand(score float64) string {
	if score > 70 {
		return ChurnBandHighRisk
	}
	if score >= 40 {
		return ChurnBandWatch
	}
	return ChurnBandHealthy
}
