package confidence


import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// Calibrator 置信度校准器
type Calibrator struct {
	scaler          *TemperatureScaler
	searcher        *GoldenSectionSearcher
	calibrationRepo *repository.ConfidenceCalibrationRepository
}

// NewCalibrator 创建校准器
//
// repo 可为 nil（用于无 DB 场景，仅做内存校准）
func NewCalibrator(repo *repository.ConfidenceCalibrationRepository) *Calibrator {
	return &Calibrator{
		scaler:          NewTemperatureScaler(1.0), 
		searcher:        NewGoldenSectionSearcher(1e-4, 100),
		calibrationRepo: repo,
	}
}

// FitOnDataset 在标注集上拟合温度参数
//
// samples: (logits[], correct_idx) 列表
// 返回拟合后的 T*、校准前后 ECE、NLL
// 持久化到 confidence_calibrations 表
func (c *Calibrator) FitOnDataset(ctx context.Context, samples []CalibrationSample) (*CalibrationResult, error) {
	if len(samples) == 0 {
		return nil, ErrEmptyCalibrationSet
	}

	eceBefore, nllBefore := c.evaluate(samples, 1.0)

	nllFn := func(t float64) float64 {
		_, nll := c.evaluate(samples, t)
		return nll
	}
	tStar := c.searcher.Minimize(nllFn, 0.05, 5.0)

	eceAfter, nllAfter := c.evaluate(samples, tStar)

	c.scaler.SetTemperature(tStar)

	result := &CalibrationResult{
		Temperature: tStar,
		ECEBefore:   eceBefore,
		ECEAfter:    eceAfter,
		NLLBefore:   nllBefore,
		NLLAfter:    nllAfter,
		SampleSize:  len(samples),
	}

	if c.calibrationRepo != nil && ctx != nil {
		now := time.Now()
		record := &model.ConfidenceCalibration{
			CalibrationID: uuid.New().String(),
			SignalType:    "intent_conf",
			Method:        "temperature_scaling",
			Temperature:   tStar,
			ECEBefore:     eceBefore,
			ECEAfter:      eceAfter,
			NLLBefore:     nllBefore,
			NLLAfter:      nllAfter,
			SampleSize:    len(samples),
			FitStartedAt:  now.Add(-time.Second),
			FitFinishedAt: now,
			IsActive:      true,
		}
		_ = c.calibrationRepo.SaveActive(ctx, record)
	}

	return result, nil
}

// Calibrate 在线校准：对原始 logits 应用当前温度
//
// 返回 top-1 概率（即校准后的置信度）
func (c *Calibrator) Calibrate(logits []float64) float64 {
	return c.scaler.ScaleTop1(logits)
}

// CurrentTemperature 返回当前生效温度
func (c *Calibrator) CurrentTemperature() float64 {
	return c.scaler.Temperature()
}

// SetTemperature 手动设置温度（热重载，用于运营后台调参）
func (c *Calibrator) SetTemperature(t float64) {
	c.scaler.SetTemperature(t)
}

// LoadActiveFromDB 从 DB 加载 active 温度参数
//
// 启动时调用，恢复上次拟合结果
func (c *Calibrator) LoadActiveFromDB(ctx context.Context) error {
	if c.calibrationRepo == nil {
		return nil
	}
	record, err := c.calibrationRepo.GetActive(ctx, "intent_conf")
	if err != nil {
		return err
	}
	if record == nil {
		return nil
	}
	c.scaler.SetTemperature(record.Temperature)
	return nil
}

// evaluate 计算给定 T 下的 ECE 和 NLL
//
// ECE = Σ (|B_m|/N) * |acc(B_m) - conf(B_m)|   （15 个分桶）
// NLL = -(1/N) Σ log(p_{y_i})
func (c *Calibrator) evaluate(samples []CalibrationSample, t float64) (ece, nll float64) {
	bins := 15
	binConf := make([]float64, bins)
	binAcc := make([]float64, bins)
	binCount := make([]int, bins)
	tempScaler := NewTemperatureScaler(t)

	nll = 0
	for _, s := range samples {
		probs := tempScaler.Scale(s.Logits)
		if len(probs) == 0 {
			continue
		}
		topIdx := 0
		topProb := probs[0]
		for i, p := range probs {
			if p > topProb {
				topProb = p
				topIdx = i
			}
		}
		if s.CorrectIdx >= 0 && s.CorrectIdx < len(probs) && probs[s.CorrectIdx] > 0 {
			nll -= math.Log(probs[s.CorrectIdx])
		} else if topProb > 0 {
			nll -= math.Log(topProb)
		}
		binIdx := int(topProb * float64(bins))
		if binIdx >= bins {
			binIdx = bins - 1
		}
		if binIdx < 0 {
			binIdx = 0
		}
		binConf[binIdx] += topProb
		if topIdx == s.CorrectIdx {
			binAcc[binIdx] += 1
		}
		binCount[binIdx]++
	}

	n := float64(len(samples))
	if n == 0 {
		return 0, 0
	}
	nll /= n
	ece = 0
	for i := 0; i < bins; i++ {
		if binCount[i] == 0 {
			continue
		}
		avgConf := binConf[i] / float64(binCount[i])
		avgAcc := binAcc[i] / float64(binCount[i])
		ece += (float64(binCount[i]) / n) * math.Abs(avgAcc-avgConf)
	}
	return ece, nll
}

// CalibrationSample 校准样本
type CalibrationSample struct {
	Logits     []float64 
	CorrectIdx int       
}

// CalibrationResult 校准结果
type CalibrationResult struct {
	Temperature float64
	ECEBefore   float64
	ECEAfter    float64
	NLLBefore   float64
	NLLAfter    float64
	SampleSize  int
}

// ErrEmptyCalibrationSet 标注集为空
var ErrEmptyCalibrationSet = errors.New("calibration sample set is empty")

