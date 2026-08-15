package service

import (
	"encoding/json"
	"errors"
	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/ops/repository"
	"math"
	"sync"
)

// ABExperimentService A/B 测试服务
type ABExperimentService struct {
	experimentRepo *repository.ABExperimentRepository
	variantRepo    *repository.ABVariantRepository
	conversionRepo *repository.ABConversionEventRepository
	resultRepo     *repository.ABExperimentResultRepository
	variantCache   map[uint]*model.ABVariant 
	cacheMutex     sync.RWMutex
}

// NewABExperimentService 创建 A/B 测试服务实例
func NewABExperimentService() *ABExperimentService {
	return &ABExperimentService{
		experimentRepo: repository.NewABExperimentRepository(),
		variantRepo:    repository.NewABVariantRepository(),
		conversionRepo: repository.NewABConversionEventRepository(),
		resultRepo:     repository.NewABExperimentResultRepository(),
		variantCache:   make(map[uint]*model.ABVariant),
	}
}

// CreateExperimentRequest 创建实验请求
type CreateExperimentRequest struct {
	Name         string          `json:"name" binding:"required"`
	Description  string          `json:"description"`
	SourceType   string          `json:"source_type" binding:"required"`
	SourceID     string          `json:"source_id" binding:"required"`
	TrafficSplit int             `json:"traffic_split"`
	StartDate    string          `json:"start_date"`
	EndDate      string          `json:"end_date"`
	Variants     []VariantConfig `json:"variants"`
}

// VariantConfig 变体配置
type VariantConfig struct {
	Name      string         `json:"name" binding:"required"`
	IsControl bool           `json:"is_control"`
	Weight    int            `json:"weight"`
	Config    map[string]any `json:"config"`
}

// CreateExperiment 创建实验
func (s *ABExperimentService) CreateExperiment(req *CreateExperimentRequest) (*model.ABExperiment, error) {
	experiment := &model.ABExperiment{
		Name:         req.Name,
		Description:  req.Description,
		SourceType:   req.SourceType,
		SourceID:     req.SourceID,
		TrafficSplit: req.TrafficSplit,
		Status:       "draft",
	}

	if err := s.experimentRepo.Create(experiment); err != nil {
		return nil, err
	}

	for i, v := range req.Variants {
		configJSON, _ := json.Marshal(v.Config)
		variant := &model.ABVariant{
			ExperimentID: experiment.ID,
			Name:         v.Name,
			IsControl:    v.IsControl,
			Weight:       v.Weight,
			Config:       string(configJSON),
		}
		if i == 0 {
			variant.IsControl = true 
		}
		if err := s.variantRepo.Create(variant); err != nil {
			return nil, err
		}
	}

	return experiment, nil
}

// GetExperiment 获取实验详情
func (s *ABExperimentService) GetExperiment(id uint) (*model.ABExperiment, error) {
	return s.experimentRepo.GetByID(id)
}

// GetExperimentList 获取实验列表
func (s *ABExperimentService) GetExperimentList(page, pageSize int) ([]*model.ABExperiment, int64, error) {
	return s.experimentRepo.GetAll(page, pageSize)
}

// UpdateExperiment 更新实验
func (s *ABExperimentService) UpdateExperiment(id uint, req *CreateExperimentRequest) (*model.ABExperiment, error) {
	experiment, err := s.experimentRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	experiment.Name = req.Name
	experiment.Description = req.Description
	experiment.SourceType = req.SourceType
	experiment.SourceID = req.SourceID
	experiment.TrafficSplit = req.TrafficSplit

	if err := s.experimentRepo.Update(experiment); err != nil {
		return nil, err
	}

	s.variantRepo.DeleteByExperiment(id)
	for i, v := range req.Variants {
		configJSON, _ := json.Marshal(v.Config)
		variant := &model.ABVariant{
			ExperimentID: id,
			Name:         v.Name,
			IsControl:    v.IsControl,
			Weight:       v.Weight,
			Config:       string(configJSON),
		}
		if i == 0 {
			variant.IsControl = true
		}
		if err := s.variantRepo.Create(variant); err != nil {
			return nil, err
		}
	}

	return experiment, nil
}

// DeleteExperiment 删除实验
func (s *ABExperimentService) DeleteExperiment(id uint) error {
	s.variantRepo.DeleteByExperiment(id)
	return s.experimentRepo.Delete(id)
}

// StartExperiment 启动实验
func (s *ABExperimentService) StartExperiment(id uint) error {
	return s.experimentRepo.UpdateStatus(id, "running")
}

// PauseExperiment 暂停实验
func (s *ABExperimentService) PauseExperiment(id uint) error {
	return s.experimentRepo.UpdateStatus(id, "paused")
}

// StopExperiment 停止实验
func (s *ABExperimentService) StopExperiment(id uint) error {
	if err := s.experimentRepo.UpdateStatus(id, "completed"); err != nil {
		return err
	}
	return s.CalculateResults(id)
}

// GetVariant 获取变体（用于流量分配）
func (s *ABExperimentService) GetVariant(sourceID string) (*model.ABVariant, error) {
	s.cacheMutex.RLock()
	variant, ok := s.variantCache[s.hashSourceID(sourceID)]
	s.cacheMutex.RUnlock()

	if ok {
		return variant, nil
	}

	return s.getVariantFromDB(sourceID)
}

// hashSourceID 简单哈希
func (s *ABExperimentService) hashSourceID(sourceID string) uint {
	hash := uint(0)
	for i := 0; i < len(sourceID); i++ {
		hash = hash*31 + uint(sourceID[i])
	}
	return hash % 1000000
}

// getVariantFromDB 从数据库获取变体
// 根据 sourceID 查找运行中的实验，按权重分配变体
func (s *ABExperimentService) getVariantFromDB(sourceID string) (*model.ABVariant, error) {
	experiment, err := s.experimentRepo.GetRunningBySourceID(sourceID)
	if err != nil {
		return nil, errors.New("no running experiment for this source")
	}

	variants, err := s.variantRepo.GetByExperiment(experiment.ID)
	if err != nil || len(variants) == 0 {
		return nil, errors.New("no variants found for experiment")
	}

	totalWeight := 0
	for _, v := range variants {
		if v.Weight <= 0 {
			totalWeight++
		} else {
			totalWeight += v.Weight
		}
	}

	if totalWeight <= 0 {
		return variants[0], nil
	}

	seed := s.hashSourceID(sourceID)
	pick := int(seed) % totalWeight

	cumulative := 0
	for _, v := range variants {
		w := v.Weight
		if w <= 0 {
			w = 1
		}
		cumulative += w
		if pick < cumulative {
			s.variantRepo.IncrementTraffic(v.ID)

			s.cacheMutex.Lock()
			s.variantCache[s.hashSourceID(sourceID)] = v
			s.cacheMutex.Unlock()

			return v, nil
		}
	}

	return variants[0], nil
}

// RecordConversion 记录转化事件
// eventValue 单位：分（int64），如购买金额 99.99 元应传 9999
func (s *ABExperimentService) RecordConversion(experimentID, variantID uint, eventName, eventType string, eventValue int64, userID string, metadata map[string]any) error {
	metadataJSON, _ := json.Marshal(metadata)
	event := &model.ABConversionEvent{
		ExperimentID: experimentID,
		EventName:    eventName,
		EventType:    eventType,
		EventValue:   eventValue,
		UserID:       userID,
		VariantID:    variantID,
		Metadata:     string(metadataJSON),
	}
	if err := s.conversionRepo.Create(event); err != nil {
		return err
	}

	return s.variantRepo.IncrementConversion(variantID)
}

// CalculateResults 计算实验结果
func (s *ABExperimentService) CalculateResults(experimentID uint) error {
	variants, err := s.variantRepo.GetByExperiment(experimentID)
	if err != nil {
		return err
	}

	for _, v := range variants {
		trafficCount := v.TrafficCount
		conversionCount := v.ConversionCount

		conversionRate := 0.0
		if trafficCount > 0 {
			conversionRate = float64(conversionCount) / float64(trafficCount) * 100
		}

		result := &model.ABExperimentResult{
			ExperimentID:    experimentID,
			VariantID:       v.ID,
			VariantName:     v.Name,
			IsControl:       v.IsControl,
			TrafficCount:    trafficCount,
			ConversionCount: conversionCount,
			ConversionRate:  conversionRate,
		}

		if err := s.resultRepo.Upsert(result); err != nil {
			return err
		}
	}

	return s.calculateWinner(experimentID)
}

// calculateWinner 计算获胜者
func (s *ABExperimentService) calculateWinner(experimentID uint) error {
	results, err := s.resultRepo.GetByExperiment(experimentID)
	if err != nil {
		return err
	}

	if len(results) < 2 {
		return nil
	}

	// 找到转化率最高的变体
	var winner *model.ABExperimentResult
	maxRate := 0.0

	for _, r := range results {
		if r.ConversionRate > maxRate {
			maxRate = r.ConversionRate
			winner = r
		}
		r.ConfidenceLevel = s.calculateConfidence(r)
		s.resultRepo.Upsert(r)
	}

	if winner != nil {
		return s.resultRepo.UpdateWinner(experimentID, winner.VariantID)
	}

	return nil
}

// calculateConfidence 计算置信度（简化版本，使用正态分布近似）
func (s *ABExperimentService) calculateConfidence(result *model.ABExperimentResult) float64 {
	if result.TrafficCount == 0 {
		return 0
	}

	p := result.ConversionRate / 100
	n := float64(result.TrafficCount)

	se := math.Sqrt(p * (1 - p) / n)

	if se == 0 {
		return 0
	}

	z := (p - 0.5) / se

	confidence := 0.5 * (1 + math.Erf(z/math.Sqrt2))

	return confidence
}

// GetExperimentResults 获取实验结果
func (s *ABExperimentService) GetExperimentResults(experimentID uint) ([]*model.ABExperimentResult, error) {
	return s.resultRepo.GetByExperiment(experimentID)
}

// GetConversionEvents 获取转化事件列表
func (s *ABExperimentService) GetConversionEvents(experimentID uint, page, pageSize int) ([]*model.ABConversionEvent, int64, error) {
	return s.conversionRepo.GetByExperiment(experimentID, page, pageSize)
}

