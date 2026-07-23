package service

import (
	"sort"
	"strings"

	"gorm.io/gorm"
	"marketing/internal/model"
	"marketing/internal/repository"
	"context"
)

// ObjectionHandlerService 异议处理服务
type ObjectionHandlerService struct {
	db *gorm.DB
}

// NewObjectionHandlerService 创建服务
func NewObjectionHandlerService() *ObjectionHandlerService {
	return &ObjectionHandlerService{db: repository.GetDB()}
}

// ObjectionCategory 异议类别
type ObjectionCategory string

const (
	ObjectionPrice		ObjectionCategory	= "price"	// 价格异议
	ObjectionNeed		ObjectionCategory	= "need"	// 需求异议
	ObjectionTrust		ObjectionCategory	= "trust"	// 信任异议
	ObjectionTiming		ObjectionCategory	= "timing"	// 时机异议
	ObjectionCompare	ObjectionCategory	= "compare"	// 比较异议
	ObjectionFeature	ObjectionCategory	= "feature"	// 特性异议
	ObjectionOther		ObjectionCategory	= "other"	// 其他
)

// ObjectionTemplate 异议处理模板
type ObjectionTemplate struct {
	ID		uint			`json:"id"`
	Category	ObjectionCategory	`json:"category"`
	Keywords	[]string		`json:"keywords"`
	Title		string			`json:"title"`
	Content		string			`json:"content"`
	UsageCount	int			`json:"usage_count"`
	SuccessRate	float64			`json:"success_rate"`
}

// objectionRule 关键词 → 类别映射
var objectionRules = []struct {
	Keywords	[]string
	Category	ObjectionCategory
	Name		string
}{
	{Keywords: []string{"贵", "太贵", "便宜点", "折扣", "降价", "优惠", "price", "expensive"}, Category: ObjectionPrice, Name: "价格异议"},
	{Keywords: []string{"不需要", "没需求", "已经有了", "用不上", "不需要", "don't need"}, Category: ObjectionNeed, Name: "需求异议"},
	{Keywords: []string{"骗子", "假的", "骗人", "不靠谱", "安全", "trust", "骗"}, Category: ObjectionTrust, Name: "信任异议"},
	{Keywords: []string{"再想想", "考虑一下", "下次", "以后", "再看看", "later", "think"}, Category: ObjectionTiming, Name: "时机异议"},
	{Keywords: []string{"其他家", "别家", "对比", "其他品牌", "competitor", "compare"}, Category: ObjectionCompare, Name: "比较异议"},
	{Keywords: []string{"功能", "不支持", "做不到", "没有这个", "feature", "can't"}, Category: ObjectionFeature, Name: "特性异议"},
}

// Classify 异议分类
func (s *ObjectionHandlerService) Classify(ctx context.Context, text string) (ObjectionCategory, string) {
	t := strings.ToLower(text)
	for _, rule := range objectionRules {
		for _, kw := range rule.Keywords {
			if strings.Contains(t, strings.ToLower(kw)) {
				return rule.Category, rule.Name
			}
		}
	}
	return ObjectionOther, "其他异议"
}

// HandleRequest 异议处理请求
type HandleRequest struct {
	Text		string	`json:"text"`
	Category	string	`json:"category"`
	UseLLM		bool	`json:"use_llm"`
}

// HandleResponse 异议处理响应
type HandleResponse struct {
	Category	ObjectionCategory	`json:"category"`
	CategoryName	string			`json:"category_name"`
	Confidence	float64			`json:"confidence"`
	Template	*ObjectionTemplate	`json:"template,omitempty"`
	Templates	[]ObjectionTemplate	`json:"templates"`
	Suggestion	string			`json:"suggestion"`
}

// Handle 处理异议
func (s *ObjectionHandlerService) Handle(ctx context.Context, req HandleRequest) (*HandleResponse, error) {
	category, name := s.Classify(ctx, req.Text)
	resp := &HandleResponse{
		Category:	category,
		CategoryName:	name,
		Confidence:	0.85,
		Templates:	make([]ObjectionTemplate, 0),
	}

	// 从 script_library 拉取匹配模板
	if req.Category == "" {
		req.Category = string(category)
	}

	var scripts []model.ScriptLibrary
	s.db.WithContext(ctx).Where("category = ? OR subcategory = ?", "objection", req.Category).
		Order("usage_count DESC").Limit(5).
		Find(&scripts)

	for _, sc := range scripts {
		ot := ObjectionTemplate{
			ID:		sc.ID,
			Category:	ObjectionCategory(sc.Category),
			Title:		sc.Title,
			Content:	sc.Content,
			UsageCount:	sc.UsageCount,
			SuccessRate:	sc.ConversionRate,
		}
		if sc.Tags != nil {
			kws := make([]string, len(sc.Tags))
			for i, t := range sc.Tags {
				if s, ok := t.(string); ok {
					kws[i] = s
				} else {
					kws[i] = ""
				}
			}
			ot.Keywords = kws
		}
		resp.Templates = append(resp.Templates, ot)
	}

	if len(resp.Templates) > 0 {
		resp.Template = &resp.Templates[0]
	}

	// 兜底建议
	if len(resp.Templates) == 0 {
		resp.Suggestion = s.defaultSuggestion(ctx, category, req.Text)
	}

	// 按使用次数排序
	sort.Slice(resp.Templates, func(i, j int) bool {
		return resp.Templates[i].UsageCount > resp.Templates[j].UsageCount
	})

	return resp, nil
}

func (s *ObjectionHandlerService) defaultSuggestion(ctx context.Context, cat ObjectionCategory, text string) string {
	switch cat {
	case ObjectionPrice:
		return "理解客户对价格的关注，先肯定产品价值再谈价格。强调性价比、长期收益、对比其他方案的 TCO。可提供分期方案或试用。"
	case ObjectionNeed:
		return "通过案例和数据说明产品在类似场景的解决效果。引导客户描述痛点，深入了解需求。"
	case ObjectionTrust:
		return "提供资质证书、用户案例、品牌背书、第三方评测。先建立信任再谈产品。"
	case ObjectionTiming:
		return "不要强推，先记录需求。设置 7 天跟进，提供限时优惠引导立即行动。"
	case ObjectionCompare:
		return "了解客户对比的具体产品，从差异化优势切入，不要贬低竞品。"
	case ObjectionFeature:
		return "深入了解客户的实际使用场景。如确实不支持，可推荐相近功能或定制方案。"
	default:
		return "倾听客户异议背后的真实顾虑，不要急于反驳，先共情再解释。"
	}
}

// ListCategories 列出所有类别
func (s *ObjectionHandlerService) ListCategories(ctx context.Context) []map[string]string {
	out := make([]map[string]string, 0, len(objectionRules))
	for _, r := range objectionRules {
		out = append(out, map[string]string{
			"category":	string(r.Category),
			"name":		r.Name,
		})
	}
	return out
}

// RecordUsage 记录使用（学习闭环）
func (s *ObjectionHandlerService) RecordUsage(ctx context.Context, templateID uint, success bool) error {
	updates := map[string]any{
		"usage_count": gorm.Expr("usage_count + 1"),
	}
	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
		updates["conversion_rate"] = gorm.Expr("success_count::float / GREATEST(usage_count, 1)")
	}
	return s.db.WithContext(ctx).Model(&model.ScriptLibrary{}).
		Where("id = ?", templateID).
		Updates(updates).Error
}
