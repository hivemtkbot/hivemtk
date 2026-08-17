package controller

import (
	"net/http"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

func timeNow() time.Time { return time.Now() }

// safeCount 安全计数：DB 异常时记录日志并返回 0，避免对前端展示误导
func (c *ChannelOverviewController) safeCount(label string, query func() (int64, error)) int {
	if c.db == nil {
		return 0
	}
	n, err := query()
	if err != nil {
		logger.Warnf("[ChannelOverview] %s count failed: %v", label, err)
		return 0
	}
	return int(n)
}

// ChannelOverviewController 渠道概览控制器
//
// 2026-08-16 严肃化：13 个真实渠道的统一配置入口。
// 提供 GET /api/channels/overview 列出所有渠道的账号数 / 在线状态 / 配置入口 URL。
// 解决"用户找不到入口"问题。
type ChannelOverviewController struct {
	db *gorm.DB
}

// NewChannelOverviewController 创建概览控制器
func NewChannelOverviewController(db *gorm.DB) *ChannelOverviewController {
	return &ChannelOverviewController{db: db}
}

// ChannelStatus 单渠道状态
type ChannelStatus struct {
	Channel         string   `json:"channel"`
	ChannelName     string   `json:"channel_name"`
	Category        string   `json:"category"`          // official_api / bridge
	AccountCount    int      `json:"account_count"`
	ActiveCount     int      `json:"active_count"`
	OnlineCount     int      `json:"online_count"`
	IntegrationReady bool    `json:"integration_ready"`
	RequiredFields  []string `json:"required_fields"`   // 配置时需要填写
	ConfigURLs      []string `json:"config_urls"`       // 配置入口 URL
	HealthURL       string   `json:"health_url,omitempty"`
}

// ChannelOverview 渠道总览
type ChannelOverview struct {
	Channels     []ChannelStatus `json:"channels"`
	TotalChannels int             `json:"total_channels"`
	RealChannels  int             `json:"real_channels"`     // 真实可用数
	BridgeChannels int            `json:"bridge_channels"`   // Bridge 类
	OfficialChannels int          `json:"official_channels"` // 官方 API 类
}

// Overview 列出所有 13 渠道的当前状态
// GET /api/channels/overview
func (c *ChannelOverviewController) Overview(ctx *gin.Context) {
	channels := []ChannelStatus{
		// ===== 官方 API 渠道 =====
		{
			Channel:         "telegram",
			ChannelName:     "Telegram Bot",
			Category:        "official_api",
			AccountCount:    c.countTelegram(),
			ActiveCount:     c.countTelegramActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"bot_token", "webhook_secret", "agent_id"},
			ConfigURLs:      []string{"/api/telegram/accounts", "/api/telegram/accounts (POST)"},
			HealthURL:       "/api/webhook/telegram/{account_id}",
		},
		{
			Channel:         "whatsapp",
			ChannelName:     "WhatsApp Cloud API",
			Category:        "official_api",
			AccountCount:    c.countWhatsApp(),
			ActiveCount:     c.countWhatsAppActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"phone_id", "access_token", "waba_id"},
			ConfigURLs:      []string{"/api/whatsapp-cloud/accounts", "/api/whatsapp-cloud/accounts (POST)"},
			HealthURL:       "/api/webhook/whatsapp/{account_id}",
		},
		{
			Channel:         "feishu",
			ChannelName:     "飞书机器人",
			Category:        "official_api",
			AccountCount:    c.countFeishu(),
			ActiveCount:     c.countFeishuActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"app_id", "app_secret", "verification_token", "encrypt_key"},
			ConfigURLs:      []string{"/api/feishu/accounts", "/api/feishu/accounts (POST)"},
			HealthURL:       "/api/webhook/feishu/{account_id}",
		},
		{
			Channel:         "wecom",
			ChannelName:     "企业微信",
			Category:        "official_api",
			AccountCount:    c.countWeCom(),
			ActiveCount:     c.countWeComOnline(),
			IntegrationReady: true,
			RequiredFields:  []string{"corp_id", "corp_secret", "agent_id"},
			ConfigURLs:      []string{"/api/wecom/accounts", "/api/wecom/accounts (POST)"},
			HealthURL:       "/api/webhook/wecom/{account_id}",
		},
		{
			Channel:         "dingtalk",
			ChannelName:     "钉钉群机器人",
			Category:        "official_api",
			AccountCount:    c.countDingTalk(),
			ActiveCount:     c.countDingTalkActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"webhook_url", "sign_secret"},
			ConfigURLs:      []string{"/api/dingtalk-app/accounts", "/api/dingtalk-app/accounts (POST)"},
		},
		{
			Channel:         "sms",
			ChannelName:     "阿里云短信",
			Category:        "official_api",
			AccountCount:    c.countSMS(),
			ActiveCount:     c.countSMSActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"access_key_id", "access_key_secret", "sign_name", "template_id"},
			ConfigURLs:      []string{"/api/sms/config", "/api/sms/config (POST)"},
		},
		{
			Channel:         "email",
			ChannelName:     "SMTP 邮件",
			Category:        "official_api",
			AccountCount:    c.countEmail(),
			ActiveCount:     c.countEmailActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"smtp_host", "smtp_port", "username", "password", "from_addr"},
			ConfigURLs:      []string{"POST /api/email/accounts", "GET /api/email/accounts"},
		},

		// ===== Bridge 浏览器扩展渠道 =====
		{
			Channel:         "douyin",
			ChannelName:     "抖音（Bridge）",
			Category:        "bridge",
			AccountCount:    c.countBridge("douyin"),
			OnlineCount:     c.countBridgeOnline("douyin"),
			IntegrationReady: true,
			RequiredFields:  []string{"chrome_extension_installed", "user_login_douyin"},
			ConfigURLs:      []string{"Chrome 扩展: ingestBridge.js", "/api/bridge/account (POST)"},
			HealthURL:       "/api/bridge/outbox?channel=douyin&account_id=...",
		},
		{
			Channel:         "kuaishou",
			ChannelName:     "快手（Bridge）",
			Category:        "bridge",
			AccountCount:    c.countBridge("kuaishou"),
			OnlineCount:     c.countBridgeOnline("kuaishou"),
			IntegrationReady: true,
			RequiredFields:  []string{"chrome_extension_installed", "user_login_kuaishou"},
			ConfigURLs:      []string{"Chrome 扩展: content-kuaishou.js", "/api/bridge/account (POST)"},
			HealthURL:       "/api/bridge/outbox?channel=kuaishou&account_id=...",
		},
		{
			Channel:         "xiaohongshu",
			ChannelName:     "小红书（Bridge）",
			Category:        "bridge",
			AccountCount:    c.countBridge("xiaohongshu"),
			OnlineCount:     c.countBridgeOnline("xiaohongshu"),
			IntegrationReady: true,
			RequiredFields:  []string{"chrome_extension_installed", "user_login_xhs"},
			ConfigURLs:      []string{"Chrome 扩展: ingestBridge.js", "/api/bridge/account (POST)"},
		},
		{
			Channel:         "tiktok",
			ChannelName:     "TikTok（Bridge）",
			Category:        "bridge",
			AccountCount:    c.countBridge("tiktok"),
			OnlineCount:     c.countBridgeOnline("tiktok"),
			IntegrationReady: true,
			RequiredFields:  []string{"chrome_extension_installed", "user_login_tiktok"},
			ConfigURLs:      []string{"Chrome 扩展: ingestBridge.js", "/api/bridge/account (POST)"},
		},
		{
			Channel:         "xianyu",
			ChannelName:     "闲鱼（Bridge）",
			Category:        "bridge",
			AccountCount:    c.countBridge("xianyu"),
			OnlineCount:     c.countBridgeOnline("xianyu"),
			IntegrationReady: true,
			RequiredFields:  []string{"chrome_extension_installed", "user_login_xianyu"},
			ConfigURLs:      []string{"Chrome 扩展: ingestBridge.js", "/api/bridge/account (POST)"},
		},
		{
			Channel:         "wechat",
			ChannelName:     "微信公众号",
			Category:        "official_api",
			AccountCount:    c.countWechat(),
			ActiveCount:     c.countWechatActive(),
			IntegrationReady: true,
			RequiredFields:  []string{"app_id", "app_secret", "原始ID"},
			ConfigURLs:      []string{"/api/wechat/accounts", "/api/wechat/accounts (POST)"},
			HealthURL:       "/api/webhook/wechat/{account_id}",
		},
	}

	overview := ChannelOverview{
		Channels:         channels,
		TotalChannels:    len(channels),
		RealChannels:     13,
		BridgeChannels:   5,
		OfficialChannels: 8,
	}
	response.Success(ctx, overview, "查询成功")
}

// countXxx 各渠道账号数（best-effort，DB 失败返回 0）
func (c *ChannelOverviewController) countTelegram() int {
	return c.safeCount("telegram", func() (int64, error) {
		var n int64
		err := c.db.Table("telegram_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countTelegramActive() int {
	return c.safeCount("telegram_active", func() (int64, error) {
		var n int64
		err := c.db.Table("telegram_accounts").Where("status = ?", "active").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countWhatsApp() int {
	return c.safeCount("whatsapp", func() (int64, error) {
		var n int64
		err := c.db.Table("whatsapp_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countWhatsAppActive() int {
	return c.safeCount("whatsapp_active", func() (int64, error) {
		var n int64
		err := c.db.Table("whatsapp_accounts").Where("status = ?", "active").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countFeishu() int {
	return c.safeCount("feishu", func() (int64, error) {
		var n int64
		err := c.db.Table("feishu_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countFeishuActive() int {
	return c.safeCount("feishu_active", func() (int64, error) {
		var n int64
		err := c.db.Table("feishu_accounts").Where("status = ?", 1).Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countWeCom() int {
	return c.safeCount("wecom", func() (int64, error) {
		var n int64
		err := c.db.Table("wecom_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countWeComOnline() int {
	return c.safeCount("wecom_online", func() (int64, error) {
		var n int64
		err := c.db.Table("wecom_accounts").Where("login_state = ?", "online").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countDingTalk() int {
	return c.safeCount("dingtalk", func() (int64, error) {
		var n int64
		err := c.db.Table("dingtalk_app_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countDingTalkActive() int {
	return c.safeCount("dingtalk_active", func() (int64, error) {
		var n int64
		err := c.db.Table("dingtalk_app_accounts").Where("status = ?", "active").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countSMS() int {
	return c.safeCount("sms", func() (int64, error) {
		var n int64
		err := c.db.Table("sms_configs").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countSMSActive() int {
	return c.safeCount("sms_active", func() (int64, error) {
		var n int64
		err := c.db.Table("sms_configs").Where("provider IS NOT NULL AND provider != ''").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countEmail() int {
	return c.safeCount("email", func() (int64, error) {
		var n int64
		err := c.db.Table("email_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countEmailActive() int {
	return c.safeCount("email_active", func() (int64, error) {
		var n int64
		err := c.db.Table("email_accounts").Where("status = ?", "active").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countBridge(channel string) int {
	return c.safeCount("bridge_"+channel, func() (int64, error) {
		var n int64
		err := c.db.Table("bridge_accounts").Where("channel = ?", channel).Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countWechat() int {
	return c.safeCount("wechat", func() (int64, error) {
		var n int64
		err := c.db.Table("wechat_accounts").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countWechatActive() int {
	return c.safeCount("wechat_active", func() (int64, error) {
		var n int64
		err := c.db.Table("wechat_accounts").Where("status = ?", "active").Count(&n).Error
		return n, err
	})
}

func (c *ChannelOverviewController) countBridgeOnline(channel string) int {
	return c.safeCount("bridge_online_"+channel, func() (int64, error) {
		var n int64
		err := c.db.Table("bridge_accounts").Where("channel = ? AND status = ?", channel, "online").Count(&n).Error
		return n, err
	})
}

// CustomerChannelBinding 客户渠道绑定（POST /api/channels/bind）
type CustomerChannelBinding struct {
	CustomerID   string `json:"customer_id" binding:"required"`
	OneID        string `json:"one_id,omitempty"`
	Channel      string `json:"channel" binding:"required"`
	ChannelUserID string `json:"channel_user_id" binding:"required"`
	ChannelName  string `json:"channel_name,omitempty"`
	AccountID    string `json:"account_id,omitempty"`
	IsPrimary    bool   `json:"is_primary"`
	GroupID      string `json:"group_id,omitempty"`
}

// BindChannel 绑定客户到某渠道（用于主动收集 OneID 信息）
// POST /api/channels/bind
func (c *ChannelOverviewController) BindChannel(ctx *gin.Context) {
	var req CustomerChannelBinding
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if c.db == nil {
		response.Error(ctx, http.StatusInternalServerError, "db not initialized")
		return
	}

	// 通过 customer_id 找到 one_id
	oneID := req.OneID
	if oneID == "" && req.CustomerID != "" {
		var cust struct {
			UnifiedID string
		}
		if err := c.db.Table("customers").Select("unified_id").Where("id = ?", req.CustomerID).Scan(&cust).Error; err != nil || cust.UnifiedID == "" {
			response.Error(ctx, http.StatusNotFound, "customer not found")
			return
		}
		oneID = cust.UnifiedID
	}
	if oneID == "" {
		response.Error(ctx, http.StatusBadRequest, "one_id or customer_id required")
		return
	}

	// Upsert
	cc := model.CustomerChannel{
		OneID:         oneID,
		Channel:       req.Channel,
		ChannelUserID: req.ChannelUserID,
		ChannelName:   req.ChannelName,
		AccountID:     req.AccountID,
	}
	assignVals := map[string]any{
		"channel_user_id": req.ChannelUserID,
		"channel_name":    req.ChannelName,
		"account_id":      req.AccountID,
		"is_primary":      req.IsPrimary,
		"group_id":        req.GroupID,
		"updated_at":      timeNow(),
	}
	if err := c.db.Where("one_id = ? AND channel = ?", oneID, req.Channel).
		Assign(assignVals).
		FirstOrCreate(&cc).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, "upsert failed: "+err.Error())
		return
	}

	// 同步更新 customers 表的主字段
	//
	// 2026-08-17 严肃化：键名是 GORM model 名，值是 DB 实际列名（不一致时需 column tag 区分）
	updateCustomerField := map[string]string{
		"telegram":       "telegram_chat_id",
		"whatsapp":       "whats_app_phone",
		"wechat":         "wechat_open_id",
		"feishu":         "feishu_open_id",
		"wecom":          "we_com_external_id",
		"douyin":         "douyin_open_id",
		"tiktok":         "tik_tok_open_id",
		"kuaishou":       "kuaishou_open_id",
		"xiaohongshu":    "xiaohongshu_id",
		"xianyu":         "xianyu_id",
	}
	if field, ok := updateCustomerField[req.Channel]; ok {
		if err := c.db.Table("customers").Where("unified_id = ?", oneID).Update(field, req.ChannelUserID).Error; err != nil {
			logger.Warnf("[ChannelOverview] update customer %s field %s failed: %v", oneID, field, err)
		}
	}

	response.Success(ctx, gin.H{
		"one_id":   oneID,
		"channel":  req.Channel,
		"identity": req.ChannelUserID,
	}, "绑定成功")
}

// ListCustomerChannels 列出客户的所有渠道绑定
// GET /api/channels/customer/:customer_id
func (c *ChannelOverviewController) ListCustomerChannels(ctx *gin.Context) {
	customerID := ctx.Param("customer_id")
	if customerID == "" {
		response.Error(ctx, http.StatusBadRequest, "customer_id required")
		return
	}
	if c.db == nil {
		response.Error(ctx, http.StatusInternalServerError, "db not initialized")
		return
	}

	var cust struct {
		UnifiedID string
		Phone    string
		Email    string
		Name     string
	}
	// 2026-08-17：先 count 行数确认有没有命中，再用 raw SQL 查
	var n int64
	if err := c.db.Raw(`SELECT COUNT(*) FROM customers WHERE id = ?`, customerID).Scan(&n).Error; err != nil {
		logger.Warnf("[ChannelOverview] count err: %v, customer_id=%s", err, customerID)
	}
	logger.Infof("[ChannelOverview] ListCustomerChannels count=%d customer_id=%s", n, customerID)

	row := c.db.Raw(`SELECT unified_id, phone, email, name FROM customers WHERE id = ?`, customerID).Row()
	if err := row.Scan(&cust.UnifiedID, &cust.Phone, &cust.Email, &cust.Name); err != nil {
		logger.Warnf("[ChannelOverview] ListCustomerChannels scan err: %v, customer_id=%s", err, customerID)
		response.Error(ctx, http.StatusNotFound, "customer not found")
		return
	}
	logger.Infof("[ChannelOverview] ListCustomerChannels customer_id=%s -> unified_id=%s phone=%s email=%s name=%s",
		customerID, cust.UnifiedID, cust.Phone, cust.Email, cust.Name)

	var rows []map[string]any
	c.db.Table("customer_channels").Where("one_id = ?", cust.UnifiedID).Order("is_primary DESC, preferred_rank ASC").Find(&rows)

	response.Success(ctx, gin.H{
		"customer_id":  customerID,
		"one_id":       cust.UnifiedID,
		"name":         cust.Name,
		"phone":        cust.Phone,
		"email":        cust.Email,
		"bindings":     rows,
		"total":        len(rows),
	}, "查询成功")
}
