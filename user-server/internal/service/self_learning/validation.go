package selflearning

// validation.go 自我学习机制的请求校验纯函数
//
// 五层架构归属: L4 能力层
//
// 设计说明：
//   - 历史上校验逻辑放在 dto.Validate() 方法中，违反了五层架构规范
//     （§七：DTO 层禁止含业务逻辑）
//   - 现下沉至 service 层，由 L3 (SelfLearningService) 和 L4 (SwitchService) 共享调用
//   - 错误变量仍保留在 dto 包中（错误定义属于传输层契约）
//   - 本文件仅提供校验函数，不修改 DTO 结构

import (
	"marketing/internal/dto"
	"marketing/internal/model"
)

// ValidateSwitchConfig 校验自我学习开关配置请求
//
// 等价于原 dto.SwitchConfigRequest.Validate() 方法体
//
// 调用方：
//   - L3: SelfLearningService.validateSwitchConfig（门面层主校验）
//   - L4: SwitchService.UpdateSwitch（防御性校验，信任 L3 已校验但仍做兜底）
func ValidateSwitchConfig(req *dto.SwitchConfigRequest) error {
	if req == nil {
		return dto.ErrSelfLearningRequestNil
	}
	switch req.AutonomyLevel {
	case model.AutonomyLevelManual, model.AutonomyLevelSupervised, model.AutonomyLevelAutonomous:
		// ok
	default:
		return dto.ErrInvalidAutonomyLevel
	}
	if req.MaxDailyCorrections < 0 {
		return dto.ErrInvalidMaxDailyCorrections
	}
	if req.MaxDailyPromotions < 0 {
		return dto.ErrInvalidMaxDailyPromotions
	}
	if req.LowQualityThreshold < 0 {
		return dto.ErrInvalidLowQualityThreshold
	}
	if req.ChampionRewardThreshold < 0 {
		return dto.ErrInvalidChampionRewardThreshold
	}
	if req.ABTestMinSamples < 0 {
		return dto.ErrInvalidABTestMinSamples
	}
	if req.CircuitBreakerThreshold < 0 || req.CircuitBreakerThreshold > 1 {
		return dto.ErrInvalidCircuitBreakerThreshold
	}
	if req.CircuitBreakerWindowMin <= 0 {
		req.CircuitBreakerWindowMin = 30
	}
	return nil
}
