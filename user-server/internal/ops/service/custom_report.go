package service

import (
	"context"
	"encoding/json"
	"errors"
	sysmodel "hivemtk-user/internal/model"
	"hivemtk-user/internal/ops/model"
	opsrepo "hivemtk-user/internal/ops/repository"
	_db "hivemtk-user/internal/pkg/db"
	sysrepo "hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// CustomReportService 自定义报表服务
type CustomReportService struct {
	db          *gorm.DB
	reportRepo  *opsrepo.CustomReportRepository
	sessionRepo *sysrepo.CustomerSessionRepository
	clueRepo    sysrepo.ClueRepository
	userRfmRepo *sysrepo.UserRFMRepository
}

// NewCustomReportService 创建自定义报表服务
func NewCustomReportService() *CustomReportService {
	db := _db.GetDB()
	return &CustomReportService{
		db:          db,
		reportRepo:  opsrepo.NewCustomReportRepositoryWithDB(db),
		sessionRepo: sysrepo.NewCustomerSessionRepositoryWithDB(db),
		clueRepo:    sysrepo.NewClueRepositoryWithDB(db),
		userRfmRepo: sysrepo.NewUserRFMRepositoryWithDB(db),
	}
}

// NewCustomReportServiceWithDB 创建指定数据库连接的自定义报表服务实例（用于测试）
func NewCustomReportServiceWithDB(db *gorm.DB) *CustomReportService {
	return &CustomReportService{
		db:          db,
		reportRepo:  opsrepo.NewCustomReportRepositoryWithDB(db),
		sessionRepo: sysrepo.NewCustomerSessionRepositoryWithDB(db),
		clueRepo:    sysrepo.NewClueRepositoryWithDB(db),
		userRfmRepo: sysrepo.NewUserRFMRepositoryWithDB(db),
	}
}

// CreateReportRequest 创建报表请求
type CreateReportRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	DataSource  string                  `json:"data_source"`
	Dimensions  []model.ReportDimension `json:"dimensions"`
	Metrics     []model.ReportMetric    `json:"metrics"`
	Filters     []model.ReportFilter    `json:"filters"`
	ChartType   string                  `json:"chart_type"`
	ChartConfig map[string]any          `json:"chart_config"`
	IsPublic    bool                    `json:"is_public"`
}

// UpdateReportRequest 更新报表请求
type UpdateReportRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	DataSource  string                  `json:"data_source"`
	Dimensions  []model.ReportDimension `json:"dimensions"`
	Metrics     []model.ReportMetric    `json:"metrics"`
	Filters     []model.ReportFilter    `json:"filters"`
	ChartType   string                  `json:"chart_type"`
	ChartConfig map[string]any          `json:"chart_config"`
	IsPublic    bool                    `json:"is_public"`
}

// CreateReport 创建报表
func (s *CustomReportService) CreateReport(createdBy uint, req *CreateReportRequest) (*model.CustomReport, error) {
	if !isValidDataSource(req.DataSource) {
		return nil, errors.New("不支持的数据源类型")
	}

	if !isValidChartType(req.ChartType) {
		return nil, errors.New("不支持的图表类型")
	}

	dimensionsJSON, _ := json.Marshal(req.Dimensions)
	metricsJSON, _ := json.Marshal(req.Metrics)
	filtersJSON, _ := json.Marshal(req.Filters)
	chartConfigJSON, _ := json.Marshal(req.ChartConfig)

	report := &model.CustomReport{
		Name:        req.Name,
		Description: req.Description,
		DataSource:  req.DataSource,
		Dimensions:  string(dimensionsJSON),
		Metrics:     string(metricsJSON),
		Filters:     string(filtersJSON),
		ChartType:   req.ChartType,
		ChartConfig: string(chartConfigJSON),
		IsPublic:    req.IsPublic,
		CreatedBy:   createdBy,
	}

	err := s.reportRepo.Create(report)
	if err != nil {
		return nil, err
	}

	return report, nil
}

// GetReport 获取报表详情
func (s *CustomReportService) GetReport(id uint) (*model.CustomReport, error) {
	report, err := s.reportRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("报表不存在")
	}

	if !report.IsPublic {
		return nil, errors.New("无权限查看")
	}

	return report, nil
}

// GetReportList 获取报表列表
func (s *CustomReportService) GetReportList(page, pageSize int) ([]*model.CustomReport, int64, error) {
	return s.reportRepo.GetAll(page, pageSize)
}

// UpdateReport 更新报表
func (s *CustomReportService) UpdateReport(id uint, req *UpdateReportRequest) (*model.CustomReport, error) {
	report, err := s.reportRepo.GetByID(id)
	if err != nil {
		return nil, errors.New("报表不存在")
	}

	_ = report

	if !isValidDataSource(req.DataSource) {
		return nil, errors.New("不支持的数据源类型")
	}

	if !isValidChartType(req.ChartType) {
		return nil, errors.New("不支持的图表类型")
	}

	dimensionsJSON, _ := json.Marshal(req.Dimensions)
	metricsJSON, _ := json.Marshal(req.Metrics)
	filtersJSON, _ := json.Marshal(req.Filters)
	chartConfigJSON, _ := json.Marshal(req.ChartConfig)

	report.Name = req.Name
	report.Description = req.Description
	report.DataSource = req.DataSource
	report.Dimensions = string(dimensionsJSON)
	report.Metrics = string(metricsJSON)
	report.Filters = string(filtersJSON)
	report.ChartType = req.ChartType
	report.ChartConfig = string(chartConfigJSON)
	report.IsPublic = req.IsPublic

	err = s.reportRepo.Update(report)
	if err != nil {
		return nil, err
	}

	return report, nil
}

// DeleteReport 删除报表
func (s *CustomReportService) DeleteReport(id uint) error {
	report, err := s.reportRepo.GetByID(id)
	if err != nil {
		return errors.New("报表不存在")
	}

	_ = report

	return s.reportRepo.Delete(id)
}

// GetPublicTemplates 获取公开模板
func (s *CustomReportService) GetPublicTemplates() ([]*model.CustomReport, error) {
	return s.reportRepo.GetPublicTemplates()
}

// UseTemplate 使用模板
func (s *CustomReportService) UseTemplate(templateID uint, createdBy uint) (*model.CustomReport, error) {
	return s.reportRepo.UseTemplate(templateID, createdBy)
}

// QueryReportData 查询报表数据
func (s *CustomReportService) QueryReportData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	switch report.DataSource {
	case "sessions":
		return s.querySessionData(ctx, report, params)
	case "messages":
		return s.queryMessageData(ctx, report, params)
	case "clues":
		return s.queryClueData(ctx, report, params)
	case "rfm":
		return s.queryRFMData(ctx, report, params)
	case "users":
		return s.queryUserData(ctx, report, params)
	case "agents":
		return s.queryAgentData(ctx, report, params)
	default:
		return nil, errors.New("不支持的数据源")
	}
}

// isValidDataSource 验证数据源
func isValidDataSource(dataSource string) bool {
	validSources := map[string]bool{
		"sessions": true,
		"messages": true,
		"orders":   true,
		"clues":    true,
		"users":    true,
		"rfm":      true,
		"agents":   true,
	}
	return validSources[dataSource]
}

// isValidChartType 验证图表类型
func isValidChartType(chartType string) bool {
	validTypes := map[string]bool{
		"table": true,
		"line":  true,
		"bar":   true,
		"pie":   true,
		"area":  true,
		"card":  true,
	}
	return validTypes[chartType]
}

// querySessionData 查询会话数据
func (s *CustomReportService) querySessionData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	// 解析维度配置
	var dimensions []model.ReportDimension
	json.Unmarshal([]byte(report.Dimensions), &dimensions)

	// 解析指标配置
	var metrics []model.ReportMetric
	json.Unmarshal([]byte(report.Metrics), &metrics)

	sessions, total, err := s.sessionRepo.GetByMerchant(ctx, sysmodel.SessionStatus(""), 1, 1000)
	if err != nil {
		return nil, err
	}

	data := make([]map[string]any, 0)
	for _, session := range sessions {
		row := make(map[string]any)

		for _, dim := range dimensions {
			if dim.Field == "date" {
				row["date"] = session.CreatedAt.Format("2006-01-02")
			} else if dim.Field == "status" {
				row["status"] = string(session.Status)
			} else if dim.Field == "agent_name" {
				row["agent_name"] = session.AgentName
			}
		}

		for _, metric := range metrics {
			if metric.Field == "session_count" {
				row["session_count"] = 1
			} else if metric.Field == "message_count" {
				row["message_count"] = session.MessageCount
			}
		}

		data = append(data, row)
	}

	dimNames := make([]string, len(dimensions))
	for i, dim := range dimensions {
		dimNames[i] = dim.Label
	}

	metricNames := make([]string, len(metrics))
	for i, metric := range metrics {
		metricNames[i] = metric.Label
	}

	return &model.ReportData{
		Dimensions: dimNames,
		Metrics:    metricNames,
		Data:       data,
		Total:      total,
	}, nil
}

// queryMessageData 查询消息数据
func (s *CustomReportService) queryMessageData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	var dimensions []model.ReportDimension
	json.Unmarshal([]byte(report.Dimensions), &dimensions)

	var metrics []model.ReportMetric
	json.Unmarshal([]byte(report.Metrics), &metrics)

	dimField := "msg_type"
	metricField := "message_count"
	if len(dimensions) > 0 {
		dimField = dimensions[0].Field
	}
	if len(metrics) > 0 {
		metricField = metrics[0].Field
	}

	type msgAgg struct {
		ContentType string
		Count       int64
	}
	var aggs []msgAgg
	if err := s.db.WithContext(ctx).
		Model(&sysmodel.UnifiedMessage{}).
		Select("COALESCE(content_type, 'unknown') AS content_type, COUNT(*) AS count").
		Group("COALESCE(content_type, 'unknown')").
		Scan(&aggs).Error; err != nil {
		return nil, err
	}

	data := make([]map[string]any, 0, len(aggs))
	for _, a := range aggs {
		row := make(map[string]any)
		row[dimField] = a.ContentType
		row[metricField] = a.Count
		data = append(data, row)
	}

	dimNames := make([]string, len(dimensions))
	for i, dim := range dimensions {
		dimNames[i] = dim.Label
	}

	metricNames := make([]string, len(metrics))
	for i, metric := range metrics {
		metricNames[i] = metric.Label
	}

	return &model.ReportData{
		Dimensions: dimNames,
		Metrics:    metricNames,
		Data:       data,
		Total:      int64(len(data)),
	}, nil
}

// queryClueData 查询线索数据
func (s *CustomReportService) queryClueData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	var dimensions []model.ReportDimension
	json.Unmarshal([]byte(report.Dimensions), &dimensions)

	var metrics []model.ReportMetric
	json.Unmarshal([]byte(report.Metrics), &metrics)

	clues, _, err := s.clueRepo.GetClueList(ctx, 1, 1000)
	if err != nil {
		return nil, err
	}

	data := make([]map[string]any, 0)
	for _, clue := range clues {
		row := make(map[string]any)
		for _, dim := range dimensions {
			if dim.Field == "type" {
				row["type"] = clue.Type
			} else if dim.Field == "is_verify" {
				row["is_verify"] = clue.IsVerify
			}
		}
		for _, metric := range metrics {
			if metric.Field == "clue_count" {
				row["clue_count"] = 1
			}
		}
		data = append(data, row)
	}

	dimNames := make([]string, len(dimensions))
	for i, dim := range dimensions {
		dimNames[i] = dim.Label
	}

	metricNames := make([]string, len(metrics))
	for i, metric := range metrics {
		metricNames[i] = metric.Label
	}

	return &model.ReportData{
		Dimensions: dimNames,
		Metrics:    metricNames,
		Data:       data,
		Total:      int64(len(data)),
	}, nil
}

// queryRFMData 查询 RFM 数据
func (s *CustomReportService) queryRFMData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	var dimensions []model.ReportDimension
	json.Unmarshal([]byte(report.Dimensions), &dimensions)

	var metrics []model.ReportMetric
	json.Unmarshal([]byte(report.Metrics), &metrics)

	var layer string
	if v, ok := params["layer"]; ok && v != nil {
		if s, ok := v.(string); ok {
			layer = s
		}
	}
	var rfms []*sysmodel.UserRFM
	var total int64
	var err error

	if layer != "" {
		rfms, total, err = s.userRfmRepo.GetByLayer(ctx, layer, 1, 1000)
		if err != nil {
			return nil, err
		}
	} else {
		rfms, total, err = s.userRfmRepo.GetAll(ctx, 1, 1000)
		if err != nil {
			return nil, err
		}
	}

	data := make([]map[string]any, 0)
	layerCount := make(map[string]int)

	for _, rfm := range rfms {
		layerCount[rfm.Layer]++
	}

	for layerName, count := range layerCount {
		row := make(map[string]any)
		for _, dim := range dimensions {
			if dim.Field == "layer" {
				row["layer"] = layerName
			}
		}
		for _, metric := range metrics {
			if metric.Field == "user_count" {
				row["user_count"] = count
			}
		}
		data = append(data, row)
	}

	dimNames := make([]string, len(dimensions))
	for i, dim := range dimensions {
		dimNames[i] = dim.Label
	}

	metricNames := make([]string, len(metrics))
	for i, metric := range metrics {
		metricNames[i] = metric.Label
	}

	return &model.ReportData{
		Dimensions: dimNames,
		Metrics:    metricNames,
		Data:       data,
		Total:      total,
	}, nil
}

// queryUserData 查询用户数据
func (s *CustomReportService) queryUserData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	var dimensions []model.ReportDimension
	json.Unmarshal([]byte(report.Dimensions), &dimensions)

	var metrics []model.ReportMetric
	json.Unmarshal([]byte(report.Metrics), &metrics)

	dimField := "date"
	if len(dimensions) > 0 {
		dimField = dimensions[0].Field
	}

	groupExpr := "DATE(created_at)"
	dimValueExpr := "TO_CHAR(DATE(created_at), 'YYYY-MM-DD')"
	switch dimField {
	case "user_type", "churn_risk":
		groupExpr = "churn_risk"
		dimValueExpr = "churn_risk::text"
	case "date", "created_at":
		groupExpr = "DATE(created_at)"
		dimValueExpr = "TO_CHAR(DATE(created_at), 'YYYY-MM-DD')"
	default:
		groupExpr = "DATE(created_at)"
		dimValueExpr = "TO_CHAR(DATE(created_at), 'YYYY-MM-DD')"
	}

	type userAgg struct {
		DimValue string
		Count    int64
	}
	var aggs []userAgg
	if err := s.db.WithContext(ctx).
		Table("customers").
		Select(dimValueExpr + " AS dim_value, COUNT(*) AS count").
		Group(groupExpr).
		Order(groupExpr).
		Scan(&aggs).Error; err != nil {
		return nil, err
	}

	data := make([]map[string]any, 0, len(aggs))
	for _, a := range aggs {
		row := make(map[string]any)
		row[dimField] = a.DimValue
		row["count"] = a.Count
		if len(metrics) > 0 {
			row[metrics[0].Field] = a.Count
		}
		data = append(data, row)
	}

	dimNames := make([]string, len(dimensions))
	for i, dim := range dimensions {
		dimNames[i] = dim.Label
	}
	metricNames := make([]string, len(metrics))
	for i, metric := range metrics {
		metricNames[i] = metric.Label
	}

	return &model.ReportData{
		Dimensions: dimNames,
		Metrics:    metricNames,
		Data:       data,
		Total:      int64(len(data)),
	}, nil
}

// queryAgentData 查询客服数据
func (s *CustomReportService) queryAgentData(ctx context.Context, report *model.CustomReport, params map[string]any) (*model.ReportData, error) {
	var dimensions []model.ReportDimension
	json.Unmarshal([]byte(report.Dimensions), &dimensions)

	var metrics []model.ReportMetric
	json.Unmarshal([]byte(report.Metrics), &metrics)

	// 真实聚合：从 customer_sessions 表按客服分组统计会话数与平均响应时长
	type agentAgg struct {
		AgentName        string
		SessionCount     int64
		AvgResponseTime  float64
	}
	var aggs []agentAgg
	if err := s.db.WithContext(ctx).
		Table("customer_sessions").
		Select("agent_name AS agent_name, COUNT(*) AS session_count, COALESCE(AVG(avg_response_time), 0) AS avg_response_time").
		Where("agent_name IS NOT NULL AND agent_name <> ''").
		Group("agent_name").
		Order("session_count DESC").
		Scan(&aggs).Error; err != nil {
		return nil, err
	}

	data := make([]map[string]any, 0, len(aggs))
	for _, a := range aggs {
		row := make(map[string]any)
		row["agent_name"] = a.AgentName
		row["session_count"] = a.SessionCount
		row["avg_response_time"] = a.AvgResponseTime
		data = append(data, row)
	}

	dimNames := make([]string, len(dimensions))
	for i, dim := range dimensions {
		dimNames[i] = dim.Label
	}
	metricNames := make([]string, len(metrics))
	for i, metric := range metrics {
		metricNames[i] = metric.Label
	}

	return &model.ReportData{
		Dimensions: dimNames,
		Metrics:    metricNames,
		Data:       data,
		Total:      int64(len(data)),
	}, nil
}

