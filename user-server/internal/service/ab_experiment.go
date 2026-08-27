package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/utils/logger"
)

// ===== T-7 A/B 实验框架 =====
//
// 决策依据 M17 T-7：
//   - FNV-1a 稳定分桶：hash("expID:customerID")%100 <50 → control，否则 treatment（纯函数）
//   - 曝光记录 fire-and-forget：buffered chan 异步落库，满则丢弃计数，绝不阻塞业务
//   - lazy ensureSchema：仿 InitComplianceAuditLogger 先例，db 为 nil（纯逻辑模式）时跳过 AutoMigrate 安全降级

// 变体常量
const (
	AbVariantControl   = "control"
	AbVariantTreatment = "treatment"
)

// AbExposureBuffer 曝光异步落库缓冲容量
const AbExposureBuffer = 1024

// AbExposure 曝光/转化记录表
type AbExposure struct {
	ID           uint       `gorm:"primarykey" json:"id"`
	ExperimentID string     `gorm:"size:64;index:idx_ab_exp_customer,priority:1" json:"experiment_id"`
	CustomerID   string     `gorm:"size:64;index:idx_ab_exp_customer,priority:2" json:"customer_id"`
	Variant      string     `gorm:"size:16" json:"variant"`
	SessionID    string     `gorm:"size:64" json:"session_id"`
	ExposedAt    time.Time  `json:"exposed_at"`
	ConvertedAt  *time.Time `json:"converted_at"`
}

// fnv1a32 FNV-1a 32 位哈希
func fnv1a32(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// Assign 稳定分桶：同 (expID, customerID) 恒定返回同一变体
func Assign(expID, customerID string) string {
	if fnv1a32(expID+":"+customerID)%100 < 50 {
		return AbVariantControl
	}
	return AbVariantTreatment
}

// AbVariantSummary 变体汇总
type AbVariantSummary struct {
	Variant        string  `json:"variant"`
	Exposed        int64   `json:"exposed"`
	Converted      int64   `json:"converted"`
	ConversionRate float64 `json:"conversion_rate"`
}

// ABExperiment A/B 实验框架（曝光落库 + 转化回填 + 汇总）
type ABExperiment struct {
	db           *gorm.DB
	ch           chan AbExposure
	done         chan struct{}
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	stopOnce     sync.Once
	DroppedCount atomic.Int64
}

var abSchemaOnce sync.Once

// abEnsureSchema lazy ensureSchema：db 为 nil 时跳过（纯逻辑模式）
func abEnsureSchema(db *gorm.DB) {
	abSchemaOnce.Do(func() {
		if db == nil {
			return
		}
		if err := db.AutoMigrate(&AbExposure{}); err != nil {
			logger.Errorf("[T-7] AbExposure AutoMigrate 失败: %v", err)
		}
	})
}

// NewABExperiment 构造并启动异步落库 worker（db 为 nil 时纯内存模式）
func NewABExperiment(db *gorm.DB, bufferSize int) *ABExperiment {
	if bufferSize <= 0 {
		bufferSize = AbExposureBuffer
	}
	a := &ABExperiment{
		db:   db,
		ch:   make(chan AbExposure, bufferSize),
		done: make(chan struct{}),
	}
	abEnsureSchema(db)
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-ctx.Done():
				close(a.done)
				return
			case e := <-a.ch:
				a.insert(e)
			}
		}
	}()
	return a
}

// insert 单条曝光落库（nil db 跳过）
func (a *ABExperiment) insert(e AbExposure) {
	if a.db == nil {
		return
	}
	if err := a.db.Create(&e).Error; err != nil {
		logger.Errorf("[T-7] 曝光落库失败 exp=%s cust=%s err=%v", e.ExperimentID, e.CustomerID, err)
	}
}

// LogExposure 异步记录曝光（fire-and-forget：缓冲满丢弃并计数，绝不阻塞）
func (a *ABExperiment) LogExposure(expID, variant, customerID, sessionID string) {
	if a == nil {
		return
	}
	select {
	case a.ch <- AbExposure{
		ExperimentID: expID,
		Variant:      variant,
		CustomerID:   customerID,
		SessionID:    sessionID,
		ExposedAt:    time.Now(),
	}:
	default:
		a.DroppedCount.Add(1)
	}
}

// MarkConversion 回填转化时间（首次转化生效；nil db 安全跳过）
func (a *ABExperiment) MarkConversion(expID, customerID string) {
	if a == nil || a.db == nil {
		return
	}
	if err := a.db.Model(&AbExposure{}).
		Where("experiment_id = ? AND customer_id = ? AND converted_at IS NULL", expID, customerID).
		Update("converted_at", time.Now()).Error; err != nil {
		logger.Errorf("[T-7] 转化回填失败 exp=%s cust=%s err=%v", expID, customerID, err)
	}
}

// Summaries 汇总窗口内各变体曝光数/转化数/转化率
func (a *ABExperiment) Summaries(expID string, window time.Duration) map[string]AbVariantSummary {
	res := map[string]AbVariantSummary{
		AbVariantControl:   {Variant: AbVariantControl},
		AbVariantTreatment: {Variant: AbVariantTreatment},
	}
	if a == nil || a.db == nil {
		return res
	}
	since := time.Now().Add(-window)
	var rows []AbExposure
	if err := a.db.Where("experiment_id = ? AND exposed_at >= ?", expID, since).Find(&rows).Error; err != nil {
		logger.Errorf("[T-7] 汇总查询失败 exp=%s err=%v", expID, err)
		return res
	}
	for _, r := range rows {
		s, ok := res[r.Variant]
		if !ok {
			s = AbVariantSummary{Variant: r.Variant}
		}
		s.Exposed++
		if r.ConvertedAt != nil {
			s.Converted++
		}
		res[r.Variant] = s
	}
	for k, s := range res {
		if s.Exposed > 0 {
			s.ConversionRate = float64(s.Converted) / float64(s.Exposed)
		}
		res[k] = s
	}
	return res
}

// Stop 幂等停止：取消 worker 并等待退出（缓冲中未落库曝光不保证写入）
func (a *ABExperiment) Stop() {
	a.stopOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
	})
	a.wg.Wait()
}

var (
	abExperimentOnce sync.Once
	abExperiment     *ABExperiment
)

// InitABExperiment 全局初始化（main 装配阶段调用一次）
func InitABExperiment(db *gorm.DB) *ABExperiment {
	abExperimentOnce.Do(func() {
		abExperiment = NewABExperiment(db, AbExposureBuffer)
	})
	return abExperiment
}

// GetABExperiment 获取全局实例（未初始化返回 nil）
func GetABExperiment() *ABExperiment { return abExperiment }
