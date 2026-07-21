package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	sysmodel "marketing/internal/model"
	"marketing/internal/ops/model"
	opsrepo "marketing/internal/ops/repository"
	sysrepo "marketing/internal/repository"
	"time"
)

// DashboardScreenService 数据大屏服务
type DashboardScreenService struct {
	screenRepo *opsrepo.DashboardScreenRepository
	widgetRepo *opsrepo.DashboardWidgetRepository
	db         any
}

// NewDashboardScreenService 创建数据大屏服务实例
func NewDashboardScreenService() *DashboardScreenService {
	return &DashboardScreenService{
		screenRepo: opsrepo.NewDashboardScreenRepository(),
		widgetRepo: opsrepo.NewDashboardWidgetRepository(),
	}
}

// CreateScreenRequest 创建大屏请求
type CreateScreenRequest struct {
	Name     string         `json:"name" binding:"required"`
	Layout   map[string]any `json:"layout"`
	Theme    string         `json:"theme"`
	IsPublic bool           `json:"is_public"`
}

// CreateScreen 创建大屏
func (s *DashboardScreenService) CreateScreen(createdBy uint, req *CreateScreenRequest) (*model.DashboardScreen, error) {
	layout, _ := json.Marshal(req.Layout)

	screen := &model.DashboardScreen{
		Name:      req.Name,
		Code:      generateScreenCode(),
		Layout:    string(layout),
		Theme:     req.Theme,
		IsPublic:  req.IsPublic,
		CreatedBy: createdBy,
	}

	if screen.Theme == "" {
		screen.Theme = "dark"
	}

	if err := s.screenRepo.Create(screen); err != nil {
		return nil, err
	}

	return screen, nil
}

// generateScreenCode 生成大屏访问码
func generateScreenCode() string {
	// 生成唯一码：时间戳 + 4 字节随机 hex，确保并发下不重复
	timestamp := time.Now().Format("20060102150405")
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		// 极端兜底：使用纳秒时间戳的十六进制后 8 位
		return fmt.Sprintf("screen_%s_%08x", timestamp, time.Now().UnixNano()&0xFFFFFFFF)
	}
	return "screen_" + timestamp + "_" + hex.EncodeToString(randomBytes)
}

// GetScreenList 获取大屏列表
func (s *DashboardScreenService) GetScreenList(page, pageSize int) ([]*model.DashboardScreen, int64, error) {
	return s.screenRepo.GetAll(page, pageSize)
}

// GetScreenByID 获取大屏详情
func (s *DashboardScreenService) GetScreenByID(id uint) (*model.DashboardScreen, error) {
	screen, err := s.screenRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return screen, nil
}

// UpdateScreenRequest 更新大屏请求
type UpdateScreenRequest struct {
	Name     string         `json:"name"`
	Layout   map[string]any `json:"layout"`
	Widgets  []WidgetConfig `json:"widgets"`
	Theme    string         `json:"theme"`
	IsPublic bool           `json:"is_public"`
}

// WidgetConfig Widget 配置
type WidgetConfig struct {
	WidgetType string         `json:"widget_type"`
	Title      string         `json:"title"`
	Config     map[string]any `json:"config"`
	DataSource string         `json:"data_source"`
	X          int            `json:"x"`
	Y          int            `json:"y"`
	Width      int            `json:"width"`
	Height     int            `json:"height"`
}

// UpdateScreen 更新大屏
func (s *DashboardScreenService) UpdateScreen(id uint, req *UpdateScreenRequest) (*model.DashboardScreen, error) {
	screen, err := s.screenRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		screen.Name = req.Name
	}
	if req.Layout != nil {
		layout, _ := json.Marshal(req.Layout)
		screen.Layout = string(layout)
	}
	if req.Theme != "" {
		screen.Theme = req.Theme
	}
	screen.IsPublic = req.IsPublic

	if err := s.screenRepo.Update(screen); err != nil {
		return nil, err
	}

	// 更新 widgets
	if req.Widgets != nil {
		s.updateWidgets(screen.ID, req.Widgets)
	}

	return screen, nil
}

// updateWidgets 更新 widgets
func (s *DashboardScreenService) updateWidgets(screenID uint, widgets []WidgetConfig) error {
	// 删除原有 widgets
	s.widgetRepo.DeleteByScreenID(screenID)

	// 创建新 widgets
	for i, w := range widgets {
		config, _ := json.Marshal(w.Config)
		widget := &model.DashboardWidget{
			ScreenID:   screenID,
			WidgetType: w.WidgetType,
			Title:      w.Title,
			Config:     string(config),
			DataSource: w.DataSource,
			X:          w.X,
			Y:          w.Y,
			Width:      w.Width,
			Height:     w.Height,
			SortOrder:  i,
		}
		s.widgetRepo.Create(widget)
	}

	return nil
}

// DeleteScreen 删除大屏
func (s *DashboardScreenService) DeleteScreen(id uint) error {
	screen, err := s.screenRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = screen

	// 删除 widgets
	s.widgetRepo.DeleteByScreenID(id)

	return s.screenRepo.Delete(id)
}

// GetPublicScreen 公开访问大屏
func (s *DashboardScreenService) GetPublicScreen(code string) (*model.DashboardScreen, error) {
	screen, err := s.screenRepo.GetByCode(code)
	if err != nil {
		return nil, err
	}

	// 增加访问次数
	s.screenRepo.IncrementViewCount(screen.ID)

	return screen, nil
}

// GetScreenWidgets 获取大屏 widgets
func (s *DashboardScreenService) GetScreenWidgets(screenID uint) ([]*model.DashboardWidget, error) {
	return s.widgetRepo.GetByScreenID(screenID)
}

// DashboardKPI 数据大屏 KPI 数据（从 controller 提取的 Service 层方法）
type DashboardKPI struct {
	TotalClues      int64 `json:"total_clues"`
	TodayClues      int64 `json:"today_clues"`
	YesterdayClues  int64 `json:"yesterday_clues"`
	TotalCustomers  int64 `json:"total_customers"`
	TotalOrders     int64 `json:"total_orders"`
	VerifiedClues   int64 `json:"verified_clues"`
	TrendToday      int   `json:"trend_today"`
	TrendYesterday  int   `json:"trend_yesterday"`
	VerifiedRate    int   `json:"verified_rate"`
	ClueGrowthRate  int   `json:"clue_growth_rate"`
	OrderGrowthRate int   `json:"order_growth_rate"`
}

// RealtimeActivity 实时活动项
type RealtimeActivity struct {
	Type      string    `json:"type"`      // clue/order/message/customer
	Title     string    `json:"title"`     // 活动标题
	UserName  string    `json:"user_name"` // 用户名
	CreatedAt time.Time `json:"created_at"`
}

// AggregateDashboardData 聚合大屏数据（Service 层）
// 私域部署：单租户，查询所有数据
func (s *DashboardScreenService) AggregateDashboardData() (*DashboardKPI, error) {
	gormDB := sysrepo.GetDB() // 通过 repository 访问 DB
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	yesterdayStart := todayStart - 86400
	thirtyDaysAgo := now.AddDate(0, 0, -30).Unix()

	kpi := &DashboardKPI{}

	// 总线索数
	gormDB.Model(&sysmodel.Clue{}).Count(&kpi.TotalClues)

	// 今日新增线索
	gormDB.Model(&sysmodel.Clue{}).Where("create_time >= ?", todayStart).Count(&kpi.TodayClues)

	// 昨日新增线索
	gormDB.Model(&sysmodel.Clue{}).Where("create_time >= ? AND create_time < ?", yesterdayStart, todayStart).Count(&kpi.YesterdayClues)

	// 总客户数
	if gormDB.Migrator().HasTable(&sysmodel.Customer{}) {
		gormDB.Model(&sysmodel.Customer{}).Count(&kpi.TotalCustomers)
	}

	// 总订单数
	if gormDB.Migrator().HasTable(&sysmodel.Order{}) {
		gormDB.Model(&sysmodel.Order{}).Count(&kpi.TotalOrders)
	}

	// 已验证线索数
	gormDB.Model(&sysmodel.Clue{}).Where("is_verify = ?", 1).Count(&kpi.VerifiedClues)

	// 趋势计算
	if kpi.YesterdayClues > 0 {
		kpi.TrendToday = int(float64(kpi.TodayClues-kpi.YesterdayClues) / float64(kpi.YesterdayClues) * 100)
	} else if kpi.TodayClues > 0 {
		kpi.TrendToday = 100
	}

	// 30 天前总线索（用于增长率）
	var thirtyDaysAgoClues int64
	gormDB.Model(&sysmodel.Clue{}).Where("create_time >= ?", thirtyDaysAgo).Count(&thirtyDaysAgoClues)
	if thirtyDaysAgoClues > 0 {
		kpi.ClueGrowthRate = int(float64(kpi.TotalClues-thirtyDaysAgoClues) / float64(thirtyDaysAgoClues) * 100)
	}

	// 验证率
	if kpi.TotalClues > 0 {
		kpi.VerifiedRate = int(float64(kpi.VerifiedClues) / float64(kpi.TotalClues) * 100)
	}

	return kpi, nil
}

// FetchRealtimeActivities 获取实时活动（Service 层）
func (s *DashboardScreenService) FetchRealtimeActivities(limit int) ([]RealtimeActivity, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	gormDB := sysrepo.GetDB()
	activities := make([]RealtimeActivity, 0)

	// 最新线索
	var clues []sysmodel.Clue
	if err := gormDB.Order("create_time DESC").Limit(limit).Find(&clues).Error; err == nil {
		for _, c := range clues {
			activities = append(activities, RealtimeActivity{
				Type:      "clue",
				Title:     fmt.Sprintf("新线索：%s", c.Name),
				UserName:  c.Account,
				CreatedAt: time.Unix(c.CreateTime, 0),
			})
		}
	}

	// 截断
	if len(activities) > limit {
		activities = activities[:limit]
	}

	// 按时间倒序
	for i, j := 0, len(activities)-1; i < j; i, j = i+1, j-1 {
		activities[i], activities[j] = activities[j], activities[i]
	}

	return activities, nil
}
