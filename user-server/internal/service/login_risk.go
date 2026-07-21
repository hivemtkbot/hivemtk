package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// 异常登录阈值
const (
	// abnormalHourStart / abnormalHourEnd: 凌晨异常时段（2-5 点）
	abnormalHourStart = 2
	abnormalHourEnd   = 5

	// frequentFailureThreshold: 1 小时内失败次数阈值
	frequentFailureThreshold = 5

	// frequentFailureWindow: 失败计数窗口
	frequentFailureWindow = 1 * time.Hour

	// locationDistanceThreshold: 异地登录距离阈值（千米）
	locationDistanceThreshold = 1000.0

	// deviceFingerprintChangeWindow: 设备指纹变更检查窗口（7 天）
	deviceFingerprintChangeWindow = 7 * 24 * time.Hour
)

// LoginRiskContext 登录风险检测上下文
// 由 controller 在登录入口收集，传给 LoginRiskService.Evaluate
type LoginRiskContext struct {
	UserID            uint
	Username          string
	IP                string
	UserAgent         string
	DeviceFingerprint string
	Success           bool
	LoginAt           time.Time
	Reason            string // 失败原因
}

// LoginRiskResult 风险评估结果
type LoginRiskResult struct {
	RiskLevel        model.RiskLevel
	ShouldAlert      bool   // 是否需要触发告警
	ShouldForceMFA   bool   // 是否需要强制二次验证
	AlertType        string // 告警类型（ShouldAlert=true 时有效）
	AlertTitle       string
	AlertDescription string
	Reasons          []string // 触发风险的原因列表
	Location         string   // IP 地理位置（暂用 IP 段模拟）
	LoginEventID     uint     // 写入的 LoginEvent.ID
	SecurityAlertID  uint     // 写入的 SecurityAlert.ID（如有）
}

// LoginRiskService 登录风险评估服务
type LoginRiskService struct{}

// NewLoginRiskService 创建登录风险服务
func NewLoginRiskService() *LoginRiskService {
	return &LoginRiskService{}
}

// Evaluate 评估登录风险并写入事件 / 告警
//
// 评估维度：
//  1. 异常时段（凌晨 2-5 点）
//  2. 频次异常（1 小时内 ≥5 次失败）
//  3. 异地登录（IP 与上次登录地距离 > 1000km）
//  4. 设备指纹变更（7 天内首次出现的新指纹）
//
// 输出：risk_level ∈ {low, medium, high, critical}
//   - low: 无异常
//   - medium: 单一中等风险信号（异常时段 / 设备指纹变更）
//   - high: 异地登录 / 频繁失败
//   - critical: 多维度同时触发（如异地 + 频繁失败）
func (s *LoginRiskService) Evaluate(ctx *LoginRiskContext) (*LoginRiskResult, error) {
	if ctx.LoginAt.IsZero() {
		ctx.LoginAt = time.Now()
	}

	result := &LoginRiskResult{
		RiskLevel: model.RiskLevelLow,
	}

	// 解析 IP 地理位置（私域部署简化版：基于 IP 前缀模拟，生产环境对接 IP 库）
	result.Location = s.resolveLocation(ctx.IP)

	database := db.GetDB()

	// 1. 异常时段检查（凌晨 2-5 点）
	if s.isAbnormalHour(ctx.LoginAt) {
		result.Reasons = append(result.Reasons, fmt.Sprintf("异常时段登录（%02d:00-%02d:00）", abnormalHourStart, abnormalHourEnd))
		if result.RiskLevel == model.RiskLevelLow {
			result.RiskLevel = model.RiskLevelMedium
		}
	}

	// 2. 频次异常检查（1 小时内 ≥5 次失败）
	failureCount, err := s.countRecentFailures(database, ctx.UserID, ctx.Username, ctx.LoginAt)
	if err != nil {
		logger.Errorf("查询最近失败次数失败: %v", err)
	}
	if failureCount >= frequentFailureThreshold {
		result.Reasons = append(result.Reasons, fmt.Sprintf("1 小时内失败 %d 次（阈值 %d）", failureCount, frequentFailureThreshold))
		// 频繁失败属于高风险信号
		result.RiskLevel = s.upgradeRisk(result.RiskLevel, model.RiskLevelHigh)
		result.AlertType = model.AlertTypeFrequentFailure
	}

	// 3. 异地登录检查（与上次成功登录 IP 距离 > 1000km）
	prevLocation, prevIP, hasPrev := s.getPreviousLoginLocation(database, ctx.UserID)
	if hasPrev {
		distance := s.calculateDistance(result.Location, prevLocation)
		if distance > locationDistanceThreshold {
			result.Reasons = append(result.Reasons, fmt.Sprintf("异地登录：本次=%s, 上次=%s(%s), 距离≈%.0fkm",
				result.Location, prevLocation, prevIP, distance))
			result.RiskLevel = s.upgradeRisk(result.RiskLevel, model.RiskLevelHigh)
			if result.AlertType == "" {
				result.AlertType = model.AlertTypeAbnormalLogin
			}
		}
	}

	// 4. 设备指纹变更检查（7 天内首次出现的新指纹）
	if ctx.DeviceFingerprint != "" {
		if s.isDeviceFingerprintChanged(database, ctx.UserID, ctx.DeviceFingerprint, ctx.LoginAt) {
			result.Reasons = append(result.Reasons, "设备指纹变更（7 天内首次出现）")
			if result.RiskLevel == model.RiskLevelLow {
				result.RiskLevel = model.RiskLevelMedium
			}
			if result.AlertType == "" {
				result.AlertType = model.AlertTypeDeviceChange
			}
		}
	}

	// 5. 严重程度判定：两个以上高风险信号 → critical
	if len(result.Reasons) >= 2 && result.RiskLevel == model.RiskLevelHigh {
		result.RiskLevel = model.RiskLevelCritical
	}

	// high/critical 触发告警 + 强制二次验证
	if result.RiskLevel == model.RiskLevelHigh || result.RiskLevel == model.RiskLevelCritical {
		result.ShouldAlert = true
		result.ShouldForceMFA = true
		if result.AlertType == "" {
			result.AlertType = model.AlertTypeAbnormalLogin
		}
		result.AlertTitle = s.buildAlertTitle(ctx, result.RiskLevel)
		result.AlertDescription = s.buildAlertDescription(ctx, result)
	}

	// 写入 login_events
	loginEvent := &model.LoginEvent{
		UserID:            ctx.UserID,
		Username:          ctx.Username,
		IP:                ctx.IP,
		UserAgent:         ctx.UserAgent,
		DeviceFingerprint: ctx.DeviceFingerprint,
		LoginAt:           ctx.LoginAt,
		Success:           ctx.Success,
		RiskLevel:         result.RiskLevel,
		Location:          result.Location,
		Reason:            strings.Join(result.Reasons, "; "),
	}
	if err := database.Create(loginEvent).Error; err != nil {
		logger.Errorf("写入 login_events 失败: %v", err)
		return result, fmt.Errorf("写入登录事件失败: %w", err)
	}
	result.LoginEventID = loginEvent.ID

	// high/critical 写入 security_alerts
	if result.ShouldAlert {
		alert := &model.SecurityAlert{
			UserID:       ctx.UserID,
			Username:     ctx.Username,
			AlertType:    result.AlertType,
			RiskLevel:    result.RiskLevel,
			Title:        result.AlertTitle,
			Description:  result.AlertDescription,
			IP:           ctx.IP,
			Location:     result.Location,
			LoginEventID: loginEvent.ID,
			Status:       model.SecurityAlertStatusOpen,
		}
		if err := database.Create(alert).Error; err != nil {
			logger.Errorf("写入 security_alerts 失败: %v", err)
		} else {
			result.SecurityAlertID = alert.ID
			// 推送站内通知
			s.pushNotification(alert)
		}
	}

	return result, nil
}

// isAbnormalHour 判断是否在凌晨异常时段
func (s *LoginRiskService) isAbnormalHour(t time.Time) bool {
	hour := t.Hour()
	return hour >= abnormalHourStart && hour < abnormalHourEnd
}

// countRecentFailures 统计最近 1 小时内同一用户名（或 user_id）的失败次数
func (s *LoginRiskService) countRecentFailures(database *gorm.DB, userID uint, username string, now time.Time) (int64, error) {
	if database == nil {
		return 0, errors.New("db 为空")
	}
	since := now.Add(-frequentFailureWindow)
	query := database.Model(&model.LoginEvent{}).
		Where("success = ? AND login_at >= ?", false, since)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	} else if username != "" {
		query = query.Where("username = ?", username)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// getPreviousLoginLocation 获取上次成功登录的地点和 IP
func (s *LoginRiskService) getPreviousLoginLocation(database *gorm.DB, userID uint) (location, ip string, hasPrev bool) {
	if database == nil || userID == 0 {
		return "", "", false
	}
	var prev model.LoginEvent
	err := database.Where("user_id = ? AND success = ?", userID, true).
		Order("login_at DESC").
		First(&prev).Error
	if err != nil {
		return "", "", false
	}
	return prev.Location, prev.IP, true
}

// isDeviceFingerprintChanged 检查设备指纹是否为 7 天内首次出现
// 如果是首次出现，则返回 true
func (s *LoginRiskService) isDeviceFingerprintChanged(database *gorm.DB, userID uint, fingerprint string, now time.Time) bool {
	if database == nil || userID == 0 || fingerprint == "" {
		return false
	}
	since := now.Add(-deviceFingerprintChangeWindow)
	var count int64
	err := database.Model(&model.LoginEvent{}).
		Where("user_id = ? AND device_fingerprint = ? AND login_at >= ?", userID, fingerprint, since).
		Count(&count).Error
	if err != nil {
		return false
	}
	// 之前没人用此指纹登录过 → 新设备
	return count == 0
}

// resolveLocation 根据 IP 解析地理位置
// 私域部署简化版：根据 IP 前缀做粗略地理分类
// 生产环境应接入专业 IP 库（如阿里云 IP 库 / MaxMind GeoIP）
func (s *LoginRiskService) resolveLocation(ip string) string {
	if ip == "" {
		return "unknown"
	}
	// 简化版：本地回环 / 私网 / 公网
	if ip == "127.0.0.1" || ip == "::1" {
		return "localhost"
	}
	if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") {
		return "private-network"
	}
	// 真实场景应通过 IP 库查询国家/省/市
	// 此处使用 IP 哈希模拟稳定的地理位置（同一 IP 总是返回相同 location，便于异地检测）
	h := sha256.Sum256([]byte(ip))
	hashStr := hex.EncodeToString(h[:])
	// 从哈希中取 4 字节作为虚拟地理坐标
	lat := float64(int(hashStr[0])<<8|int(hashStr[1]))/65535.0*180.0 - 90.0
	lon := float64(int(hashStr[2])<<8|int(hashStr[3]))/65535.0*360.0 - 180.0
	return fmt.Sprintf("geo(%.4f,%.4f)", lat, lon)
}

// calculateDistance 计算两个 location 字符串之间的距离（千米）
// 简化版：使用 Haversine 公式
func (s *LoginRiskService) calculateDistance(loc1, loc2 string) float64 {
	lat1, lon1, ok1 := s.parseGeo(loc1)
	lat2, lon2, ok2 := s.parseGeo(loc2)
	if !ok1 || !ok2 {
		return 0
	}

	const earthRadiusKm = 6371.0

	dLat := toRadians(lat2 - lat1)
	dLon := toRadians(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRadians(lat1))*math.Cos(toRadians(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// parseGeo 解析 "geo(lat,lon)" 格式
func (s *LoginRiskService) parseGeo(s_ string) (float64, float64, bool) {
	var lat, lon float64
	if _, err := fmt.Sscanf(s_, "geo(%f,%f)", &lat, &lon); err != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// toRadians 角度转弧度
func toRadians(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// upgradeRisk 升级风险等级（只升不降）
func (s *LoginRiskService) upgradeRisk(current, target model.RiskLevel) model.RiskLevel {
	order := map[model.RiskLevel]int{
		model.RiskLevelLow:      0,
		model.RiskLevelMedium:   1,
		model.RiskLevelHigh:     2,
		model.RiskLevelCritical: 3,
	}
	if order[target] > order[current] {
		return target
	}
	return current
}

// buildAlertTitle 构建告警标题
func (s *LoginRiskService) buildAlertTitle(ctx *LoginRiskContext, level model.RiskLevel) string {
	username := ctx.Username
	if username == "" {
		username = fmt.Sprintf("user#%d", ctx.UserID)
	}
	return fmt.Sprintf("[%s] 异常登录告警: %s", strings.ToUpper(string(level)), username)
}

// buildAlertDescription 构建告警描述
func (s *LoginRiskService) buildAlertDescription(ctx *LoginRiskContext, result *LoginRiskResult) string {
	parts := []string{
		fmt.Sprintf("用户: %s", ctx.Username),
		fmt.Sprintf("IP: %s", ctx.IP),
		fmt.Sprintf("地点: %s", result.Location),
		fmt.Sprintf("时间: %s", ctx.LoginAt.Format(time.RFC3339)),
	}
	if len(result.Reasons) > 0 {
		parts = append(parts, "风险原因: "+strings.Join(result.Reasons, "; "))
	}
	return strings.Join(parts, "\n")
}

// pushNotification 推送站内通知
// 私域部署简化版：写入 notifications 表（用户 ID 为 0 表示管理员广播）
func (s *LoginRiskService) pushNotification(alert *model.SecurityAlert) {
	if alert == nil {
		return
	}
	database := db.GetDB()
	notif := &model.Notification{
		UserID:  alert.UserID,
		Type:    model.NotificationTypeWarning,
		Title:   alert.Title,
		Content: alert.Description,
	}
	if err := database.Create(notif).Error; err != nil {
		logger.Errorf("推送安全告警通知失败: %v", err)
	}

	// 标记告警已通知
	if err := database.Model(&model.SecurityAlert{}).
		Where("id = ?", alert.ID).
		Update("notified", true).Error; err != nil {
		logger.Errorf("更新告警 notified 状态失败: %v", err)
	}
}

// ComputeDeviceFingerprint 从 User-Agent + IP 计算设备指纹
// 简化版：SHA-256(UA + "|" + IP 前 3 段)
// 真实场景应加入更多维度：屏幕分辨率、时区、语言、Canvas 指纹等
func ComputeDeviceFingerprint(userAgent, ip string) string {
	// 取 IP 前 3 段（/24 网段），避免同一内网不同 IP 被判为新设备
	ipPrefix := ip
	parts := strings.Split(ip, ".")
	if len(parts) >= 3 {
		ipPrefix = strings.Join(parts[:3], ".")
	}

	data := userAgent + "|" + ipPrefix
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])[:32]
}

// ListLoginEvents 查询用户登录事件列表（分页）
func (s *LoginRiskService) ListLoginEvents(userID uint, page, pageSize int) ([]*model.LoginEvent, int64, error) {
	database := db.GetDB()
	if database == nil {
		return nil, 0, errors.New("db 未初始化")
	}

	var total int64
	query := database.Model(&model.LoginEvent{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var events []*model.LoginEvent
	if err := query.Order("login_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

// ListSecurityAlerts 查询安全告警列表
func (s *LoginRiskService) ListSecurityAlerts(userID uint, status string, page, pageSize int) ([]*model.SecurityAlert, int64, error) {
	database := db.GetDB()
	if database == nil {
		return nil, 0, errors.New("db 未初始化")
	}

	var total int64
	query := database.Model(&model.SecurityAlert{})
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var alerts []*model.SecurityAlert
	if err := query.Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&alerts).Error; err != nil {
		return nil, 0, err
	}
	return alerts, total, nil
}

// ResolveSecurityAlert 处理安全告警
func (s *LoginRiskService) ResolveSecurityAlert(alertID, resolverUserID uint, note string) error {
	database := db.GetDB()
	if database == nil {
		return errors.New("db 未初始化")
	}
	now := time.Now()
	return database.Model(&model.SecurityAlert{}).
		Where("id = ?", alertID).
		Updates(map[string]any{
			"status":       model.SecurityAlertStatusResolved,
			"resolved_at":  now,
			"resolved_by":  resolverUserID,
			"resolve_note": note,
		}).Error
}

// IgnoreSecurityAlert 忽略告警
func (s *LoginRiskService) IgnoreSecurityAlert(alertID, resolverUserID uint, note string) error {
	database := db.GetDB()
	if database == nil {
		return errors.New("db 未初始化")
	}
	now := time.Now()
	return database.Model(&model.SecurityAlert{}).
		Where("id = ?", alertID).
		Updates(map[string]any{
			"status":       model.SecurityAlertStatusIgnored,
			"resolved_at":  now,
			"resolved_by":  resolverUserID,
			"resolve_note": note,
		}).Error
}
