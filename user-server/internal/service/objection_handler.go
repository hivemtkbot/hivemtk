package service

import (
	"context"
	"hash/fnv"
	"log/slog"
	"math/rand"
	"sort"
	"strings"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// ScriptLibraryRepo 话术库仓储接口（供测试注入 mock）
type ScriptLibraryRepo interface {
	ListObjectionTemplates(ctx context.Context, objectionCategory string, limit int) ([]model.ScriptLibrary, error)
	IncrementUsageStats(ctx context.Context, templateID uint, success bool) error
}

// ObjectionHandlerService 异议处理服务
type ObjectionHandlerService struct {
	scriptRepo ScriptLibraryRepo
}

// NewObjectionHandlerService 创建服务
func NewObjectionHandlerService() *ObjectionHandlerService {
	return &ObjectionHandlerService{scriptRepo: repository.NewScriptLibraryRepository(repository.GetDB())}
}

// ObjectionCategory 异议类别
type ObjectionCategory string

const (
	ObjectionPrice     ObjectionCategory = "price"
	ObjectionNeed      ObjectionCategory = "need"
	ObjectionTrust     ObjectionCategory = "trust"
	ObjectionTiming    ObjectionCategory = "timing"
	ObjectionStatusQuo ObjectionCategory = "status_quo"
	ObjectionCompare   ObjectionCategory = "compare"
	ObjectionFeature   ObjectionCategory = "feature"
	ObjectionOther     ObjectionCategory = "other"
)

// ObjectionTemplate 异议处理模板
type ObjectionTemplate struct {
	ID          uint              `json:"id"`
	Category    ObjectionCategory `json:"category"`
	Keywords    []string          `json:"keywords"`
	Title       string            `json:"title"`
	Content     string            `json:"content"`
	UsageCount  int               `json:"usage_count"`
	SuccessRate float64           `json:"success_rate"`
}

// objectionRule 关键词 → 类别映射
//
// 规则按顺序求值、首个命中即返回，因此高优先级类别必须排在前面：
// status_quo（T-3）须先于 need/timing —— "不需要了"含 need 的"不需要"、
// "再想想"原属 timing，均应优先归入维持现状类（竞品数据：status quo 单占丢单 38%）。
var objectionRules = []struct {
	Keywords []string
	Category ObjectionCategory
	Name     string
}{
	{Keywords: []string{"再想想", "暂时不用", "目前挺好", "挺好的", "不需要了", "用不着", "维持现状", "就用现在的"}, Category: ObjectionStatusQuo, Name: "维持现状异议"},
	{Keywords: []string{"贵", "太贵", "便宜点", "折扣", "降价", "优惠", "price", "expensive"}, Category: ObjectionPrice, Name: "价格异议"},
	{Keywords: []string{"不需要", "没需求", "已经有了", "用不上", "don't need"}, Category: ObjectionNeed, Name: "需求异议"},
	{Keywords: []string{"骗子", "假的", "骗人", "不靠谱", "安全", "trust", "骗"}, Category: ObjectionTrust, Name: "信任异议"},
	{Keywords: []string{"考虑一下", "下次", "以后", "再看看", "later", "think"}, Category: ObjectionTiming, Name: "时机异议"},
	{Keywords: []string{"其他家", "别家", "对比", "其他品牌", "competitor", "compare"}, Category: ObjectionCompare, Name: "比较异议"},
	{Keywords: []string{"功能", "不支持", "做不到", "没有这个", "feature", "can't"}, Category: ObjectionFeature, Name: "特性异议"},
}

// 异议分类置信度分级（按命中该类别的关键词个数）
const (
	confidenceMultiHit  = 0.90
	confidenceSingleHit = 0.70
	confidenceFallback  = 0.40
)

// Classify 异议分类
func (s *ObjectionHandlerService) Classify(ctx context.Context, text string) (ObjectionCategory, string) {
	category, name, _ := s.classifyWithConfidence(ctx, text)
	return category, name
}

// classifyWithConfidence 异议分类并按命中关键词数给出置信度：
// 该类别命中关键词数 >= 2 → 0.90；恰好 1 个 → 0.70；未命中任何规则（other 兜底）→ 0.40。
func (s *ObjectionHandlerService) classifyWithConfidence(ctx context.Context, text string) (ObjectionCategory, string, float64) {
	t := strings.ToLower(text)
	for _, rule := range objectionRules {
		hits := 0
		for _, kw := range rule.Keywords {
			if strings.Contains(t, strings.ToLower(kw)) {
				hits++
			}
		}
		if hits > 0 {
			conf := confidenceSingleHit
			if hits >= 2 {
				conf = confidenceMultiHit
			}
			return rule.Category, rule.Name, conf
		}
	}
	return ObjectionOther, "其他异议", confidenceFallback
}

// HandleRequest 异议处理请求
type HandleRequest struct {
	Text     string `json:"text"`
	Category string `json:"category"`
	UseLLM   bool   `json:"use_llm"`
}

// HandleResponse 异议处理响应
type HandleResponse struct {
	Category     ObjectionCategory   `json:"category"`
	CategoryName string              `json:"category_name"`
	Confidence   float64             `json:"confidence"`
	Template     *ObjectionTemplate  `json:"template,omitempty"`
	Templates    []ObjectionTemplate `json:"templates"`
	Suggestion   string              `json:"suggestion"`
	Acknowledge  string              `json:"acknowledge,omitempty"`
	Clarify      string              `json:"clarify_question,omitempty"`
}

// Handle 处理异议
func (s *ObjectionHandlerService) Handle(ctx context.Context, req HandleRequest) (*HandleResponse, error) {
	category, name, confidence := s.classifyWithConfidence(ctx, req.Text)
	resp := &HandleResponse{
		Category:     category,
		CategoryName: name,
		Confidence:   confidence,
		Templates:    make([]ObjectionTemplate, 0),
	}

	if req.Category == "" {
		req.Category = string(category)
	}

	var scripts []model.ScriptLibrary
	if s.scriptRepo != nil {
		scripts, _ = s.scriptRepo.ListObjectionTemplates(ctx, req.Category, 5)
	}

	for _, sc := range scripts {
		ot := ObjectionTemplate{
			ID:          sc.ID,
			Category:    ObjectionCategory(sc.Category),
			Title:       sc.Title,
			Content:     sc.Content,
			UsageCount:  sc.UsageCount,
			SuccessRate: sc.ConversionRate,
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
		// T-2 归因闭环：推荐话术后异步记录 usage（异议→话术→转化率结构化数据）
		// 失败仅告警，不影响主响应
		s.recordUsageAsync(ctx, resp.Template.ID)
	}

	if len(resp.Templates) == 0 {
		resp.Suggestion = s.defaultSuggestion(ctx, category, req.Text)
	}

	// T-4 LAER 编排：Acknowledge（镜像复述前缀，按模板 ID 稳定伪随机 3 选 1）
	// + Explore（price/trust 高价值异议先发澄清问题再答）
	resp.Acknowledge = pickAcknowledge(category, ackSeedID(resp.Template), req.Text)
	if q, ok := exploreClarifyQuestions[category]; ok {
		resp.Clarify = q
	}

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
	case ObjectionStatusQuo:
		return "不要急于说服，先认同现状的合理性，再挖掘现状中的隐性成本与不满点。提供低门槛试用或小步替换方案，避免正面否定客户当前选择。"
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
			"category": string(r.Category),
			"name":     r.Name,
		})
	}
	return out
}

// RecordUsage 记录使用（学习闭环）
func (s *ObjectionHandlerService) RecordUsage(ctx context.Context, templateID uint, success bool) error {
	if s.scriptRepo == nil {
		return nil
	}
	return s.scriptRepo.IncrementUsageStats(ctx, templateID, success)
}

// recordUsageAsync 异步记录模板推荐 usage（T-2 归因闭环）
//
// 使用脱离取消信号的 context（context.WithoutCancel）确保请求返回后记录仍能落库；
// 记录失败仅打日志，绝不影响主响应。
func (s *ObjectionHandlerService) recordUsageAsync(ctx context.Context, templateID uint) {
	if s == nil || s.scriptRepo == nil || templateID == 0 {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		if err := s.scriptRepo.IncrementUsageStats(detached, templateID, false); err != nil {
			slog.Warn("objection handler auto record usage failed", "template_id", templateID, "error", err)
		}
	}()
}

// acknowledgeTemplates T-4 LAER-Acknowledge 镜像复述前缀模板层。
// 业界依据：多数异议实现跳过 Acknowledge 层直接 Respond，机器味重；
// 先镜像复述客户顾虑再作答可显著降低对抗感。每类别 3 选 1，
// 以话术模板 ID 为种子做稳定伪随机（同模板同选择，避免同会话内漂移）。
var acknowledgeTemplates = map[ObjectionCategory][]string{
	ObjectionPrice: {
		"价格确实是大家都会关心的，您有这个顾虑很正常。",
		"理解您对成本的谨慎，这钱要花得值才好。",
		"嗯，预算方面多考虑是对的，我说明白您再判断。",
	},
	ObjectionNeed: {
		"明白您的意思，觉得现在用不上也是实情。",
		"理解，需求这东西确实因人而异。",
		"您说暂时没这方面需要，我先把情况讲清楚。",
	},
	ObjectionTrust: {
		"信任需要慢慢建立，您谨慎是对的。",
		"您的担心可以理解，合作前多了解是应该的。",
		"换作是我也会先确认靠不靠谱。",
	},
	ObjectionTiming: {
		"好的，不着急，时机确实要自己把握。",
		"理解您想再等等，这个节奏没问题。",
		"行，先放一放也合理，我先把关键信息给您留底。",
	},
	ObjectionStatusQuo: {
		"现在的方式用得挺好就不想折腾，这很正常。",
		"理解，稳定跑着的东西谁都不想轻易换。",
		"嗯，现状能凑合就先不动，是很务实的想法。",
	},
	ObjectionCompare: {
		"多方对比再做决定是对的，您应该多看看。",
		"了解，货比三家不吃亏。",
		"您去比较很正常，我把我们的特点说清楚供您参考。",
	},
	ObjectionFeature: {
		"您提到的这点确实关键，功能匹配度最重要。",
		"明白，具体能不能做到您关心的事，我来确认下。",
		"这个顾虑实际，功能不合用买回来也是摆设。",
	},
}

// exploreClarifyQuestions T-4 LAER-Explore：高价值异议（price/trust）先澄清再答，
// 避免答非所问——价格异议需区分"超预算"与"对比后觉得偏高"，信任异议需定位具体担忧面。
var exploreClarifyQuestions = map[ObjectionCategory]string{
	ObjectionPrice: "方便问一下，您是觉得超出预算了，还是和同类产品对比后觉得偏高呢？",
	ObjectionTrust: "方便说说主要担心哪方面吗？比如效果、售后还是服务保障？",
}

// ackSeedID 确定 Acknowledge 选择的种子：有推荐模板时用模板 ID（同模板输出稳定），
// 无模板时回退按文本哈希。种子为 0 视为未初始化。
func ackSeedID(tpl *ObjectionTemplate) uint {
	if tpl != nil && tpl.ID > 0 {
		return tpl.ID
	}
	return 0
}

// pickAcknowledge 按类别从 3 个镜像复述模板中稳定选取 1 条：
// 种子取模板 ID（优先）或文本 FNV-1a 哈希（兜底），保证同一输入选择恒定。
// other 兜底类别无镜像复述（未知顾虑复述易答非所问），返回空串。
func pickAcknowledge(cat ObjectionCategory, seedID uint, text string) string {
	tpls, ok := acknowledgeTemplates[cat]
	if !ok || len(tpls) == 0 {
		return ""
	}
	var seed int64
	if seedID > 0 {
		seed = int64(seedID)
	} else {
		h := fnv.New32a()
		_, _ = h.Write([]byte(text))
		seed = int64(h.Sum32())
	}
	idx := int(rand.New(rand.NewSource(seed)).Int63()) % len(tpls)
	return tpls[idx]
}
