package browser

import "sync"

// AutoReplyInfra 自动回复基础设施单例
// 提供 RateLimiter 和 SliderSolver 的全局单例，避免每次创建 controller 时重复初始化
type AutoReplyInfra struct {
	RateLimiter  *RateLimiter
	SliderSolver *SliderSolver
}

var infraOnce sync.Once
var infraInstance *AutoReplyInfra

// GetAutoReplyInfra 获取基础设施单例
func GetAutoReplyInfra() *AutoReplyInfra {
	infraOnce.Do(func() {
		infraInstance = &AutoReplyInfra{
			RateLimiter:  NewRateLimiter(DefaultRateLimitConfig),
			SliderSolver: NewSliderSolver(DefaultSliderConfig),
		}
	})
	return infraInstance
}
