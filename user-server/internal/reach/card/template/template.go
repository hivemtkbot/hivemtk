package template

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"path/filepath"
	"strings"
)

// CardTemplateData 卡片模板数据
type CardTemplateData struct {
	Title       string
	Description string
	ImageURL    string
	RedirectURL string
}

// CardChatTemplateData 卡片聊天页模板数据（四平台统一）
type CardChatTemplateData struct {
	Title         string
	Description   string
	ImageURL      string
	Tags          string
	TagList       []string
	Platform      string 
	PlatformLabel string 
	ThemeColor    string 
	ChatURL       string 
}

// LiveCodeTemplateData 活码模板数据
type LiveCodeTemplateData struct {
	ID          string
	Title       string
	Description string
	ImageURL    string
	EntryURL    string
	LandingURL  string
	ShowStats   bool
	ShowQR      bool
	TotalClicks int
	TodayClicks int
	QRCount     int
	QRImageURL  string
}

// TemplateService 模板服务
type TemplateService struct {
	templateDir string
}

// NewTemplateService 创建模板服务
func NewTemplateService(templateDir string) *TemplateService {
	return &TemplateService{
		templateDir: templateDir,
	}
}

// GenerateDouyinCardPage 生成抖音卡片页面
func (s *TemplateService) GenerateDouyinCardPage(data *CardTemplateData) (string, error) {
	tmplPath := filepath.Join(s.templateDir, "douyin_card.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateLiveCodePage 生成活码页面
func (s *TemplateService) GenerateLiveCodePage(data *LiveCodeTemplateData) (string, error) {
	tmplPath := filepath.Join(s.templateDir, "live_code.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateXiaohongshuCardPage 生成小红书卡片页面
func (s *TemplateService) GenerateXiaohongshuCardPage(data *CardTemplateData) (string, error) {
	tmplPath := filepath.Join(s.templateDir, "xiaohongshu_card.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateKuaishouCardPage 生成快手卡片页面
func (s *TemplateService) GenerateKuaishouCardPage(data *CardTemplateData) (string, error) {
	tmplPath := filepath.Join(s.templateDir, "kuaishou_card.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RenderXianyuCard 渲染闲鱼卡片
func (s *TemplateService) RenderXianyuCard(card any) (string, error) {
	tmplPath := filepath.Join(s.templateDir, "xianyu_card.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, card); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// PlatformLabel 平台中文名称
func PlatformLabel(platform string) string {
	switch platform {
	case "douyin":
		return "抖音"
	case "kuaishou":
		return "快手"
	case "xiaohongshu":
		return "小红书"
	case "xianyu":
		return "闲鱼"
	default:
		return platform
	}
}

// PlatformThemeColor 平台品牌主题色
func PlatformThemeColor(platform string) string {
	switch platform {
	case "douyin":
		return "#000000"
	case "kuaishou":
		return "#ff5000"
	case "xiaohongshu":
		return "#ff2442"
	case "xianyu":
		return "#ff4400"
	default:
		return "#1989fa"
	}
}

// BuildChatURL 构造跳转到 embed chat 的 URL
// platform: douyin / kuaishou / xiaohongshu / xianyu
// cardID: 卡片 ID（作为 query 参数 source / card_id 传递，用于追踪；channel_ref 用平台级 {platform}_card）
// baseURL: 站点根地址（如 https://chat.example.com）；为空时使用相对路径 /chat/embed/...
func BuildChatURL(platform string, cardID uint, baseURL, cardTitle string) string {
	channelRef := fmt.Sprintf("%s_card", platform)
	q := url.Values{}
	if cardTitle != "" {
		q.Set("title", cardTitle)
	}
	q.Set("color", PlatformThemeColor(platform))
	q.Set("source", platform)
	q.Set("card_id", fmt.Sprintf("%d", cardID))
	queryStr := q.Encode()
	if baseURL == "" {
		return fmt.Sprintf("/chat/embed/%s?%s", channelRef, queryStr)
	}
	base := strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/chat/embed/%s?%s", base, channelRef, queryStr)
}

// ParseTagList 将逗号分隔的标签字符串拆分为列表
func ParseTagList(tags string) []string {
	if tags == "" {
		return nil
	}
	parts := strings.Split(tags, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// GenerateCardChatPage 生成卡片聊天页（抖音 / 快手 / 小红书 / 闲鱼 四平台统一模板）
func (s *TemplateService) GenerateCardChatPage(data *CardChatTemplateData) (string, error) {
	tmplPath := filepath.Join(s.templateDir, "card_chat.html")
	tmpl, err := template.New("card_chat.html").Funcs(template.FuncMap{
	}).ParseFiles(tmplPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderCardChatPage 从基础卡片字段直接生成聊天页（四平台统一入口）
// platform: douyin / kuaishou / xiaohongshu / xianyu
// cardID: 卡片主键
// title, description, imageURL, tags: 卡片基础字段
// baseURL: 站点根地址；为空时使用相对路径
func (s *TemplateService) RenderCardChatPage(platform string, cardID uint, title, description, imageURL, tags, baseURL string) (string, error) {
	data := &CardChatTemplateData{
		Title:         title,
		Description:   description,
		ImageURL:      imageURL,
		Tags:          tags,
		TagList:       ParseTagList(tags),
		Platform:      platform,
		PlatformLabel: PlatformLabel(platform),
		ThemeColor:    PlatformThemeColor(platform),
		ChatURL:       BuildChatURL(platform, cardID, baseURL, title),
	}
	return s.GenerateCardChatPage(data)
}

