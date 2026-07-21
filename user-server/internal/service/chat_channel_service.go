package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"marketing/internal/model"
)

// ChatChannelService 客服 Web Widget 渠道管理服务
//
// 职责：
//   - 渠道（channel）CRUD
//   - AppKey 签发（生成、轮换、吊销）
//   - AppSecret 哈希校验
//   - 渠道使用统计（visitor_count / session_count）
//
// 五层架构：Service 层，依赖 Repository/GORM，被 Controller 调用。
type ChatChannelService struct {
	db *gorm.DB
}

// NewChatChannelService 构造 ChatChannelService
//
// 严格模式：db 为 nil 时返回 error（不再 panic），由调用方决定降级或 fail-fast。
// 兼容：保留 panic 行为作为最后兜底，避免任何调用方意外解包 nil。
func NewChatChannelService(db *gorm.DB) (*ChatChannelService, error) {
	if db == nil {
		return nil, errors.New("ChatChannelService: db is nil")
	}
	return &ChatChannelService{db: db}, nil
}

// MustNewChatChannelService 兼容性构造（db 为 nil 时 panic）
//
// 适用于 router/启动阶段（已确保 db 已初始化），保留旧调用形态
// 避免对所有调用方做侵入式修改。
func MustNewChatChannelService(db *gorm.DB) *ChatChannelService {
	svc, err := NewChatChannelService(db)
	if err != nil {
		panic(err.Error())
	}
	return svc
}

// CreateChannelRequest 创建渠道请求
type CreateChannelRequest struct {
	ChannelName    string   `json:"channel_name" binding:"required,min=1,max=100"`
	AllowedOrigins []string `json:"allowed_origins" binding:"required,min=1"`
	// 私域部署修复：RagProduct.ID 是 string（UUID），改为字符串接收，
	// 服务层再做 hash → uint 转换写入 DB，保证外部 API 友好（前端可直接传产品 UUID）。
	DefaultRAGProductID string   `json:"default_rag_product_id"`
	WelcomeMessage      string   `json:"welcome_message"`
	WidgetColor         string   `json:"widget_color"`
	WidgetPosition      string   `json:"widget_position"`
	WidgetTitle         string   `json:"widget_title"`
	AutoAssign          *bool    `json:"auto_assign"`
	ConfidenceThreshold *float64 `json:"confidence_threshold"`
}

// UpdateChannelRequest 更新渠道请求
type UpdateChannelRequest struct {
	ChannelName         *string   `json:"channel_name"`
	AllowedOrigins      *[]string `json:"allowed_origins"`
	DefaultRAGProductID *string   `json:"default_rag_product_id"`
	WelcomeMessage      *string   `json:"welcome_message"`
	WidgetColor         *string   `json:"widget_color"`
	WidgetPosition      *string   `json:"widget_position"`
	WidgetTitle         *string   `json:"widget_title"`
	Status              *string   `json:"status"`
	AutoAssign          *bool     `json:"auto_assign"`
	ConfidenceThreshold *float64  `json:"confidence_threshold"`
}

// ChannelCreateResult 渠道创建结果（含一次性返回的明文凭证）
type ChannelCreateResult struct {
	Channel   *model.ChatChannel `json:"channel"`
	AppKey    string             `json:"app_key"`
	AppSecret string             `json:"app_secret"` // 仅创建时返回一次
}

// Create 创建渠道（生成 AppKey + AppSecret）
func (s *ChatChannelService) Create(req *CreateChannelRequest, createdBy uint) (*ChannelCreateResult, error) {
	if req == nil {
		return nil, errors.New("请求不能为空")
	}
	if strings.TrimSpace(req.ChannelName) == "" {
		return nil, errors.New("渠道名称不能为空")
	}
	if len(req.AllowedOrigins) == 0 {
		return nil, errors.New("至少配置一个允许的 origin")
	}

	appKey, err := generateAppKey()
	if err != nil {
		return nil, fmt.Errorf("生成 AppKey 失败: %w", err)
	}
	appSecret, err := generateAppSecret()
	if err != nil {
		return nil, fmt.Errorf("生成 AppSecret 失败: %w", err)
	}

	channel := &model.ChatChannel{
		ChannelID:           uuid.NewString(),
		ChannelName:         req.ChannelName,
		AppKey:              appKey,
		AppSecretHash:       hashAppSecret(appSecret),
		AllowedOrigins:      strings.Join(req.AllowedOrigins, ","),
		DefaultRAGProductID: ragProductIDStringToUint(req.DefaultRAGProductID),
		WelcomeMessage:      req.WelcomeMessage,
		WidgetColor:         defaultIfEmpty(req.WidgetColor, "#1989fa"),
		WidgetPosition:      defaultIfEmpty(req.WidgetPosition, "bottom-right"),
		WidgetTitle:         defaultIfEmpty(req.WidgetTitle, "在线客服"),
		Status:              model.ChatChannelStatusActive,
		AutoAssign:          true,
		ConfidenceThreshold: 0.70,
		CreatedBy:           createdBy,
	}
	if req.AutoAssign != nil {
		channel.AutoAssign = *req.AutoAssign
	}
	if req.ConfidenceThreshold != nil {
		channel.ConfidenceThreshold = *req.ConfidenceThreshold
	}

	if err := s.db.Create(channel).Error; err != nil {
		return nil, fmt.Errorf("保存渠道失败: %w", err)
	}

	// 累计访客/会话数初始为 0
	return &ChannelCreateResult{
		Channel:   channel,
		AppKey:    appKey,
		AppSecret: appSecret,
	}, nil
}

// Update 更新渠道（不重置 AppKey/AppSecret）
func (s *ChatChannelService) Update(channelID string, req *UpdateChannelRequest) (*model.ChatChannel, error) {
	channel, err := s.GetByChannelID(channelID)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{}
	if req.ChannelName != nil {
		updates["channel_name"] = *req.ChannelName
	}
	if req.AllowedOrigins != nil {
		updates["allowed_origins"] = strings.Join(*req.AllowedOrigins, ",")
	}
	if req.DefaultRAGProductID != nil {
		updates["default_rag_product_id"] = ragProductIDStringToUint(*req.DefaultRAGProductID)
	}
	if req.WelcomeMessage != nil {
		updates["welcome_message"] = *req.WelcomeMessage
	}
	if req.WidgetColor != nil {
		updates["widget_color"] = *req.WidgetColor
	}
	if req.WidgetPosition != nil {
		updates["widget_position"] = *req.WidgetPosition
	}
	if req.WidgetTitle != nil {
		updates["widget_title"] = *req.WidgetTitle
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.AutoAssign != nil {
		updates["auto_assign"] = *req.AutoAssign
	}
	if req.ConfidenceThreshold != nil {
		updates["confidence_threshold"] = *req.ConfidenceThreshold
	}

	if len(updates) == 0 {
		return channel, nil
	}

	if err := s.db.Model(channel).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("更新渠道失败: %w", err)
	}
	return s.GetByChannelID(channelID)
}

// Delete 删除渠道（软删除：实际改为 disabled）
func (s *ChatChannelService) Delete(channelID string) error {
	channel, err := s.GetByChannelID(channelID)
	if err != nil {
		return err
	}
	return s.db.Model(channel).Update("status", model.ChatChannelStatusDisabled).Error
}

// HardDelete 硬删除（管理后台使用）
func (s *ChatChannelService) HardDelete(channelID string) error {
	return s.db.Where("channel_id = ?", channelID).Delete(&model.ChatChannel{}).Error
}

// GetByChannelID 根据 channel_id 查询
func (s *ChatChannelService) GetByChannelID(channelID string) (*model.ChatChannel, error) {
	if strings.TrimSpace(channelID) == "" {
		return nil, errors.New("channel_id 不能为空")
	}
	var channel model.ChatChannel
	// 兼容查询：前端列表返回的是数字主键 id，这里既支持按字符串 channel_id 查询，
	// 也支持按数字 id 查询（避免详情/编辑/删除接口因标识符不一致而 404）。
	if id, err := strconv.ParseUint(channelID, 10, 64); err == nil {
		if err2 := s.db.Where("id = ?", uint(id)).First(&channel).Error; err2 == nil {
			return &channel, nil
		}
	}
	if err := s.db.Where("channel_id = ?", channelID).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("渠道不存在: %s", channelID)
		}
		return nil, err
	}
	return &channel, nil
}

// GetByAppKey 根据 AppKey 查询
func (s *ChatChannelService) GetByAppKey(appKey string) (*model.ChatChannel, error) {
	if strings.TrimSpace(appKey) == "" {
		return nil, errors.New("app_key 不能为空")
	}
	var channel model.ChatChannel
	if err := s.db.Where("app_key = ?", appKey).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("AppKey 无效: %s", appKey)
		}
		return nil, err
	}
	return &channel, nil
}

// GetOrCreateDefaultChannel 获取或自动创建默认渠道（私域部署用）
//
// 私域部署模式（2026-07-17）：用户首次嵌入客服 widget 时，如果 DB 没有 default 渠道，
// 自动创建一个（无需任何配置），保证访客首次接入即可工作。
// 后续用户可以在管理后台修改 default 渠道的配置。
func (s *ChatChannelService) GetOrCreateDefaultChannel() (*model.ChatChannel, error) {
	const defaultID = "default"
	channel, err := s.GetByChannelID(defaultID)
	if err == nil {
		return channel, nil
	}
	// 不存在则自动创建
	channel = &model.ChatChannel{
		ChannelID:           defaultID,
		ChannelName:         "默认渠道",
		AppKey:              "",
		AppSecretHash:       "",
		AllowedOrigins:      "*",
		WelcomeMessage:      "您好，欢迎咨询！请描述您的问题，AI 助手将为您服务。",
		WidgetColor:         "#1989fa",
		WidgetPosition:      "bottom-right",
		WidgetTitle:         "在线客服",
		Status:              model.ChatChannelStatusActive,
		AutoAssign:          true,
		ConfidenceThreshold: 0.70,
	}
	if err := s.db.Create(channel).Error; err != nil {
		return nil, fmt.Errorf("创建默认渠道失败: %w", err)
	}
	return channel, nil
}

// cardChannelMeta 卡片渠道元数据（4 平台统一）
type cardChannelMeta struct {
	PlatformLabel string // 抖音 / 快手 / 小红书 / 咸鱼
	ThemeColor    string // 品牌主题色
}

var cardChannelMetas = map[string]cardChannelMeta{
	"douyin":      {PlatformLabel: "抖音", ThemeColor: "#000000"},
	"kuaishou":    {PlatformLabel: "快手", ThemeColor: "#ff5000"},
	"xiaohongshu": {PlatformLabel: "小红书", ThemeColor: "#ff2442"},
	"xianyu":      {PlatformLabel: "咸鱼", ThemeColor: "#ff4400"},
}

// IsCardChannelRef 判断 channel_ref 是否为卡片渠道标识（{platform}_card）
// platform ∈ {douyin, kuaishou, xiaohongshu, xianyu}
func IsCardChannelRef(ref string) (string, bool) {
	for platform := range cardChannelMetas {
		if ref == platform+"_card" {
			return platform, true
		}
	}
	return "", false
}

// GetOrCreateCardChannel 获取或自动创建卡片渠道（4 平台统一）
//
// 私域部署模式（2026-07-21）：抖音/快手/小红书/咸鱼 4 个平台的卡片分享页
// 通过此渠道接入网页客服。首次访问时自动创建，无需管理员手动配置。
// 后续可在管理后台修改欢迎语、主题色等。
//
// platform: douyin / kuaishou / xiaohongshu / xianyu
func (s *ChatChannelService) GetOrCreateCardChannel(platform string) (*model.ChatChannel, error) {
	meta, ok := cardChannelMetas[platform]
	if !ok {
		return nil, fmt.Errorf("不支持的卡片平台: %s", platform)
	}
	channelID := platform + "_card"
	channel, err := s.GetByChannelID(channelID)
	if err == nil {
		return channel, nil
	}
	// 自动创建
	// AppKey 必须唯一（DB uniqueIndex），为 4 个平台分别设置确定性 AppKey
	channel = &model.ChatChannel{
		ChannelID:           channelID,
		ChannelName:         meta.PlatformLabel + "卡片客服",
		AppKey:              platform + "_card_app_key",
		AppSecretHash:       "",
		AllowedOrigins:      "*",
		WelcomeMessage:      "您好，欢迎咨询！请描述您的问题，AI 助手将为您服务。",
		WidgetColor:         meta.ThemeColor,
		WidgetPosition:      "bottom-right",
		WidgetTitle:         meta.PlatformLabel + " · 在线客服",
		Status:              model.ChatChannelStatusActive,
		AutoAssign:          true,
		ConfidenceThreshold: 0.70,
	}
	if err := s.db.Create(channel).Error; err != nil {
		return nil, fmt.Errorf("创建卡片渠道失败: %w", err)
	}
	return channel, nil
}

// List 列出渠道
func (s *ChatChannelService) List(keyword string, status string, page, pageSize int) ([]model.ChatChannel, int64, error) {
	query := s.db.Model(&model.ChatChannel{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("channel_name LIKE ? OR app_key LIKE ?", like, like)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var channels []model.ChatChannel
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&channels).Error; err != nil {
		return nil, 0, err
	}
	return channels, total, nil
}

// RotateAppKey 轮换 AppKey（保留 AppSecret 不变）
func (s *ChatChannelService) RotateAppKey(channelID string) (string, error) {
	channel, err := s.GetByChannelID(channelID)
	if err != nil {
		return "", err
	}
	newKey, err := generateAppKey()
	if err != nil {
		return "", fmt.Errorf("生成 AppKey 失败: %w", err)
	}
	if err := s.db.Model(channel).Update("app_key", newKey).Error; err != nil {
		return "", err
	}
	return newKey, nil
}

// ResetAppSecret 重置 AppSecret（返回明文，仅返回一次）
func (s *ChatChannelService) ResetAppSecret(channelID string) (string, error) {
	channel, err := s.GetByChannelID(channelID)
	if err != nil {
		return "", err
	}
	newSecret, err := generateAppSecret()
	if err != nil {
		return "", fmt.Errorf("生成 AppSecret 失败: %w", err)
	}
	if err := s.db.Model(channel).Update("app_secret_hash", hashAppSecret(newSecret)).Error; err != nil {
		return "", err
	}
	return newSecret, nil
}

// IncrementVisitorCount 增加访客计数
func (s *ChatChannelService) IncrementVisitorCount(channelID string) error {
	return s.db.Model(&model.ChatChannel{}).
		Where("channel_id = ?", channelID).
		UpdateColumn("visitor_count", gorm.Expr("visitor_count + 1")).Error
}

// IncrementSessionCount 增加会话计数
func (s *ChatChannelService) IncrementSessionCount(channelID string) error {
	return s.db.Model(&model.ChatChannel{}).
		Where("channel_id = ?", channelID).
		UpdateColumn("session_count", gorm.Expr("session_count + 1")).Error
}

// ============================================================================
// 凭证生成与校验
// ============================================================================

// appKeyAlphabet AppKey 字符表（去歧义字符：0/O/1/l/I）
const appKeyAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

func generateAppKey() (string, error) {
	const length = 32
	result := make([]byte, length)
	alphabetLen := big.NewInt(int64(len(appKeyAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		result[i] = appKeyAlphabet[n.Int64()]
	}
	// 前缀便于识别：ak_live_
	return "ak_" + string(result[:24]), nil
}

func generateAppSecret() (string, error) {
	const length = 48
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk_" + hex.EncodeToString(buf), nil
}

// hashAppSecret 计算 AppSecret 哈希
func hashAppSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// ragProductIDStringToUint 把 RAG 产品 ID 字符串(UUID 等)稳定映射到 uint
//
// 实现：FNV-1a 64-bit hash 截断到 uint。空字符串返回 0。
// 设计动机：chat_channel 表的 default_rag_product_id 是 uint 字段（历史 schema 约束），
// 而 RagProduct.ID 是 string（UUID）。为了既保留历史 schema 兼容又支持 UUID 入参，
// 这里采用稳定 hash 方案。优点：相同输入始终映射到相同 uint，便于跨重启关联；
// 缺点：极端碰撞概率存在（可忽略）。
func ragProductIDStringToUint(s string) uint {
	if s == "" {
		return 0
	}
	const (
		offset64 = 1469598103934665603
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return uint(h & 0x7FFFFFFFFFFFFFFF)
}

// VerifyAppSecret 校验 AppSecret
func (s *ChatChannelService) VerifyAppSecret(channel *model.ChatChannel, secret string) bool {
	if channel == nil || secret == "" {
		return false
	}
	return channel.AppSecretHash == hashAppSecret(secret)
}

func defaultIfEmpty(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// 等待时间以避免 Go vet 警告
var _ = time.Second
