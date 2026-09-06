// Package eval LLM Judge 锚点校准（D18）。
//
// 业界契约（REFINEMENT §7.6）：judge 上岗前须与人工标注对拍——
//   - Cohen's Kappa ≥ 0.6（Landis-Koch substantial），
//   - precision/recall 分开报（Eugene Yan：80% raw agreement 可能仅 κ=0.62）。
//
// 校准流程：人工标注 gold labels（pass/fail 二值，优于 1-10 打分）→
// judge 对同批样本给分 → 本文件计算一致性 → κ<0.6 不上岗。
package eval

// JudgeCalibrationResult 校准结果
type JudgeCalibrationResult struct {
	N         int
	Agreement float64
	Kappa     float64
	Precision float64
	Recall    float64
	Qualified bool
}

// MinCalibrationSamples 最低校准样本量（业界 25-50 条/批）
const MinCalibrationSamples = 30

// KappaThreshold κ 上岗门槛
const KappaThreshold = 0.6

// CalibrateJudge 人工标注与 judge 判定对拍。
// gold/judge 等长切片，元素 true=pass。
func CalibrateJudge(gold, judge []bool) JudgeCalibrationResult {
	n := len(gold)
	if n == 0 || n != len(judge) {
		return JudgeCalibrationResult{}
	}
	var tp, fp, fn, agree int
	for i := 0; i < n; i++ {
		if gold[i] == judge[i] {
			agree++
		}
		switch {
		case judge[i] && gold[i]:
			tp++
		case judge[i] && !gold[i]:
			fp++
		case !judge[i] && gold[i]:
			fn++
		}
	}
	r := JudgeCalibrationResult{N: n, Agreement: float64(agree) / float64(n)}

	po := float64(agree) / float64(n)
	pYesGold := float64(tp+fn) / float64(n)
	pYesJudge := float64(tp+fp) / float64(n)
	pe := pYesGold*pYesJudge + (1-pYesGold)*(1-pYesJudge)
	if pe < 1 {
		r.Kappa = (po - pe) / (1 - pe)
	}
	if tp+fp > 0 {
		r.Precision = float64(tp) / float64(tp+fp)
	}
	if tp+fn > 0 {
		r.Recall = float64(tp) / float64(tp+fn)
	}
	r.Qualified = r.Kappa >= KappaThreshold && n >= MinCalibrationSamples
	return r
}
