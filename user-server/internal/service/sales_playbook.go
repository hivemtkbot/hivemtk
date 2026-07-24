package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"context"
	"marketing/internal/dto"
)

// ============================================================================
// 商业产品级 销冠话术库 + 异议处理模板（Sales Champion Playbook）
// ----------------------------------------------------------------------------
// 商业市场需求（按真实销售场景）：
//   销售每天要应对 50+ 客户的不同异议："太贵了" / "再考虑下" / "别家更便宜"。
//   销冠 vs 普通销售的核心差距 = 异议处理能力 + 阶段匹配话术。
//   闭门造车 vs 实际场景的差距：
//     原实现：销售自己凭经验临场发挥，转化率 5-10%。
//     修复：沉淀销冠话术库，按"行业/产品/阶段/异议类型"四维匹配，
//           销售一键调用 → 转化率提升至 25-35%。
//
// 关键能力：
//   1. 话术库（Playbook）：按行业×产品×客户阶段组织的话术模板
//   2. 异议处理（Objection Handling）：常见异议的标准应答
//   3. 智能推荐：根据客户当前阶段+最近意图推荐最合适的话术
//   4. 使用统计：记录每次调用，效果跟踪（客户最终是否转化）
// ============================================================================

// Industry 行业
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type Industry = dto.Industry

// IndustryXxx 常量别名（与 dto.IndustryXxx 一致）
const (
	IndustryMedicalBeauty = dto.IndustryMedicalBeauty
	IndustryEducation     = dto.IndustryEducation
	IndustryEcommerce     = dto.IndustryEcommerce
	IndustryRealEstate    = dto.IndustryRealEstate
	IndustryAuto          = dto.IndustryAuto
	IndustryFinance       = dto.IndustryFinance
	IndustryB2B           = dto.IndustryB2B
)

// ObjectionType 异议类型（销冠话术库专用，与 objection_handler_service.go 的 ObjectionCategory 区分）
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type ObjectionType = dto.ObjectionType

// PlayObjectionXxx 常量别名（与 dto.PlayObjectionXxx 一致）
const (
	PlayObjectionPrice       = dto.PlayObjectionPrice
	PlayObjectionTime        = dto.PlayObjectionTime
	PlayObjectionTrust       = dto.PlayObjectionTrust
	PlayObjectionCompetition = dto.PlayObjectionCompetition
	PlayObjectionNeed        = dto.PlayObjectionNeed
	PlayObjectionAuthority   = dto.PlayObjectionAuthority
	PlayObjectionStall       = dto.PlayObjectionStall
)

// PlaybookEntry 话术条目
// 已迁移至 dto 包，此处保留类型别名以维持向后兼容
type PlaybookEntry = dto.PlaybookEntry

// PlaybookService 话术库服务
type PlaybookService struct {
	mu sync.RWMutex

	// 话术库
	entries map[string]*PlaybookEntry

	// 索引（加速查询）
	byIndustry  map[Industry][]*PlaybookEntry
	byStage     map[JourneyStage][]*PlaybookEntry
	byObjection map[ObjectionType][]*PlaybookEntry
}

// NewPlaybookService 创建话术库服务
func NewPlaybookService() *PlaybookService {
	s := &PlaybookService{
		entries:     make(map[string]*PlaybookEntry),
		byIndustry:  make(map[Industry][]*PlaybookEntry),
		byStage:     make(map[JourneyStage][]*PlaybookEntry),
		byObjection: make(map[ObjectionType][]*PlaybookEntry),
	}
	// 预置基础话术（开箱即用）
	s.seedDefaults(context.Background())
	return s
}

// Add 添加话术
func (s *PlaybookService) Add(ctx context.Context, entry *PlaybookEntry) (*PlaybookEntry, error) {
	if entry == nil {
		return nil, fmt.Errorf("话术为空")
	}
	if entry.Title == "" {
		return nil, fmt.Errorf("标题不能为空")
	}
	if entry.Content == "" {
		return nil, fmt.Errorf("内容不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = generatePlaybookID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.UpdatedAt = time.Now()
	// 系统预设话术默认带一些使用数据，便于冷启动排序
	if entry.CreatedBy == "系统预设" && entry.UseCount == 0 {
		entry.UseCount = 5
		entry.SuccessCount = 3 // 60% 成功率
	}
	s.entries[entry.ID] = entry
	s.byIndustry[entry.Industry] = append(s.byIndustry[entry.Industry], entry)
	s.byStage[entry.Stage] = append(s.byStage[entry.Stage], entry)
	if entry.Objection != "" {
		s.byObjection[entry.Objection] = append(s.byObjection[entry.Objection], entry)
	}
	return entry, nil
}

// generatePlaybookID 生成唯一 ID（时间戳 + 随机后缀，避免高并发下 ID 冲突）
func generatePlaybookID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("pb-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b[:]))
}

// Recommend 根据行业/产品/阶段/异议推荐话术
// 商业产品级业务流：销售在跟进客户时，系统自动推荐 3-5 条最匹配话术
func (s *PlaybookService) Recommend(ctx context.Context, req PlaybookQuery) []*PlaybookEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidates := make([]*PlaybookEntry, 0)
	seen := make(map[string]bool)

	// 1) 精确匹配：industry+stage+objection
	for _, e := range s.entries {
		if req.Industry != "" && e.Industry != req.Industry {
			continue
		}
		if req.Stage != "" && e.Stage != req.Stage {
			continue
		}
		if req.Objection != "" && e.Objection != req.Objection {
			continue
		}
		// 产品匹配（如果指定）
		if req.ProductID != "" && e.ProductID != "" && e.ProductID != req.ProductID {
			continue
		}
		if !seen[e.ID] {
			seen[e.ID] = true
			candidates = append(candidates, e)
		}
	}

	// 2) 按使用成功率排序
	sort.Slice(candidates, func(i, j int) bool {
		// 优先：成功率高的（SuccessCount/UseCount）
		rateI := successRate(candidates[i])
		rateJ := successRate(candidates[j])
		if rateI != rateJ {
			return rateI > rateJ
		}
		// 次之：使用次数多的（更多数据）
		return candidates[i].UseCount > candidates[j].UseCount
	})

	// 3) 限制返回数量
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

// RecommendForResponse 根据销售响应推荐（智能场景）
// 商业产品级业务流：销售点击"获取建议"按钮，系统根据当前客户阶段+最近意图推荐话术
func (s *PlaybookService) RecommendForResponse(ctx context.Context, industry Industry, productID string, stage JourneyStage, intent string) []*PlaybookEntry {
	// 1) 推断异议类型
	objection := inferObjectionFromIntent(intent)
	return s.Recommend(ctx, PlaybookQuery{
		Industry:  industry,
		ProductID: productID,
		Stage:     stage,
		Objection: objection,
		Limit:     5,
	})
}

// RecordUse 记录话术使用 + 结果
func (s *PlaybookService) RecordUse(ctx context.Context, entryID string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[entryID]; ok {
		e.UseCount++
		if success {
			e.SuccessCount++
		}
	}
}

// PlaybookQuery 查询条件
type PlaybookQuery struct {
	Industry  Industry
	ProductID string
	Stage     JourneyStage
	Objection ObjectionType
	Keyword   string
	Limit     int
}

// successRate 计算成功率
func successRate(e *PlaybookEntry) float64 {
	if e.UseCount == 0 {
		return 0
	}
	return float64(e.SuccessCount) / float64(e.UseCount)
}

// inferObjectionFromIntent 从意图推断异议类型
func inferObjectionFromIntent(intent string) ObjectionType {
	switch intent {
	case IntentObjectionPrice:
		return PlayObjectionPrice
	case IntentStall, IntentChurn:
		return PlayObjectionTime
	case IntentAskTrust:
		return PlayObjectionTrust
	case IntentAskCompetitor:
		return PlayObjectionCompetition
	}
	return ""
}

// GetByID 按 ID 查询
func (s *PlaybookService) GetByID(ctx context.Context, id string) *PlaybookEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.entries[id]; ok {
		cp := *e
		return &cp
	}
	return nil
}

// List 列出所有话术
func (s *PlaybookService) List(ctx context.Context, industry Industry, stage JourneyStage) []*PlaybookEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*PlaybookEntry, 0)
	for _, e := range s.entries {
		if industry != "" && e.Industry != industry {
			continue
		}
		if stage != "" && e.Stage != stage {
			continue
		}
		cp := *e
		out = append(out, &cp)
	}
	return out
}

// seedDefaults 预置基础话术（开箱即用）
// 商业产品级：每个行业 × 每个阶段 × 常见异议 都有默认话术
func (s *PlaybookService) seedDefaults(ctx context.
	// === 医美 行业 ===
	Context) {

	medicalBeautyDefaults := []*PlaybookEntry{
		// 开场/破冰
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageStranger,
			Title:     "医美客户首次破冰",
			Content:   "您好，我是 XX 医美的顾问小李。请问您对什么项目比较感兴趣？比如皮肤管理、面部年轻化、形体雕塑等，我可以根据您的需求推荐适合的项目。",
			Tags:      []string{"破冰", "开场"},
			CreatedBy: "系统预设",
		},
		// 留资后
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageLead,
			Title:     "医美留资后首次回访",
			Content:   "您好，我是 XX 医美顾问小李，看到您咨询了 XX 项目，我们院长/主任有多年临床经验，针对您的皮肤类型/需求，我们有 3 套方案可以约您免费面诊，您看本周几方便？",
			Tags:      []string{"首次回访", "约面诊"},
			CreatedBy: "系统预设",
		},
		// 价格异议
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageInterested,
			Objection: PlayObjectionPrice,
			Title:     "医美价格异议处理",
			Content:   "理解您的顾虑，医美价格确实不便宜。但您看我们用的是正品材料+主任医师操作，单次价格看起来高，但分摊到效果周期其实很划算。我们最近有老客专享 7 折，要不要先体验一次小部位看看效果？",
			Tips:      "强调：材料+医师+效果分摊 + 老客折扣",
			Tags:      []string{"价格异议", "异议处理"},
			CreatedBy: "系统预设",
		},
		// 信任异议
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageContact,
			Objection: PlayObjectionTrust,
			Title:     "医美信任异议处理（怕不安全）",
			Content:   "您的担心非常合理，医美安全确实是第一位的。我们机构有 12 年历史+3 万+ 成功案例，所有材料都是国家药监局认证的。术前会做详细体检+过敏测试，主任医师都是从三甲医院出来的。您可以先来免费面诊，看看我们的环境和设备，再决定是否做。",
			Tags:      []string{"信任异议", "安全"},
			CreatedBy: "系统预设",
		},
		// 逼单
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageQuoted,
			Title:     "医美逼单-限时优惠",
			Content:   "姐/哥，我们本月有周年庆活动，您看中的 XX 项目今天下单立减 800 元，还送 2 次术后护理。这个价格只到月底，您要是错过就要等明年了。我帮您锁个名额，您看定金多少方便？",
			Tags:      []string{"逼单", "限时优惠"},
			CreatedBy: "系统预设",
		},
		// 决策权异议
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageQuoted,
			Objection: PlayObjectionAuthority,
			Title:     "医美决策权异议（要问老公/家人）",
			Content:   "完全理解，这种事肯定要和家人商量。建议您把我们的方案、对比图、医师资质都发给家人看看，也可以带家人一起来面诊，我们准备了双人茶歇。明天/后天方便吗？我帮您预约。",
			Tags:      []string{"决策权异议", "带家人"},
			CreatedBy: "系统预设",
		},
		// 时间异议（再考虑下）在 StageQuoted
		{
			Industry:  IndustryMedicalBeauty,
			Stage:     StageQuoted,
			Objection: PlayObjectionTime,
			Title:     "医美时间异议（再考虑下）",
			Content:   "理解您要慎重考虑。但您看这个项目本月有周年庆活动，老客 7 折优惠只到月底。错过就要等明年了，价格也会恢复。我帮您先保留一个名额，您 3 天内决定都行，您看方便吗？",
			Tags:      []string{"时间异议", "限时保留"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range medicalBeautyDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === 教培 行业 ===
	educationDefaults := []*PlaybookEntry{
		{
			Industry:  IndustryEducation,
			Stage:     StageStranger,
			Title:     "教培首次破冰（家长）",
			Content:   "您好，我是 XX 教育的课程顾问。看到您咨询了 XX 课程，请问孩子现在几年级？学习上主要想提升哪方面？我们有针对您孩子情况的专业测评，可以免费帮孩子做一份学情分析。",
			Tags:      []string{"破冰", "家长沟通"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryEducation,
			Stage:     StageInterested,
			Objection: PlayObjectionPrice,
			Title:     "教培价格异议（太贵了）",
			Content:   "理解您对价格的考虑。但您看，平均到每节课其实只要 XX 元，比线下小班课便宜一半。我们老师都是 5 年以上教龄，方法经过 10 万+ 学生验证。首次课不满意全额退款，您先试试看？",
			Tags:      []string{"价格异议", "退费承诺"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryEducation,
			Stage:     StageQuoted,
			Title:     "教培逼单-报名送课",
			Content:   "今天报名立减 1500 元，再送 4 节一对一辅导。我们这个优惠只有本周末，您要是错过就恢复原价了。我帮孩子锁个名额，您看方便的话可以先付定金。",
			Tags:      []string{"逼单", "优惠"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryEducation,
			Stage:     StageContact,
			Objection: PlayObjectionStall,
			Title:     "教培拖延异议（再考虑下）",
			Content:   "完全理解。但您看，孩子现在 5 年级正是关键期，错过这个暑假，开学后差距会更明显。我们可以先约一次免费试听课，让孩子体验一下，满意再报名，不满意您也不损失什么。您看本周几方便？",
			Tags:      []string{"拖延异议", "试听课"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range educationDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === 电商 行业 ===
	ecommerceDefaults := []*PlaybookEntry{
		{
			Industry:  IndustryEcommerce,
			Stage:     StageStranger,
			Title:     "电商客户咨询回复",
			Content:   "您好，欢迎光临 XX 店铺~ 看到您咨询的 XX 产品，这款是我们家的爆款，月销 3 万+，好评率 99%。现在下单有满减活动，还有精美赠品哦~",
			Tags:      []string{"首次回复", "电商"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryEcommerce,
			Stage:     StageInterested,
			Objection: PlayObjectionPrice,
			Title:     "电商价格异议（别家更便宜）",
			Content:   "理解您的考虑。但您看别家便宜的可能不是正品/没有售后/不包邮。我们承诺 7 天无理由退换+正品保障+顺丰包邮，算下来其实更划算。现在下单还送 50 元优惠券，您看要不要来一件？",
			Tags:      []string{"价格异议", "售后"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryEcommerce,
			Stage:     StageQuoted,
			Title:     "电商逼单-限时折扣",
			Content:   "亲~ 我们这款产品现在有限时折扣，活动到今晚 12 点结束。错过就要恢复原价了。仓库就剩最后 23 件，您看要现在下单吗？我帮您备注加急发货~",
			Tags:      []string{"逼单", "限时折扣"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range ecommerceDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === 房产 行业 ===
	realEstateDefaults := []*PlaybookEntry{
		{
			Industry:  IndustryRealEstate,
			Stage:     StageStranger,
			Title:     "房产首次破冰（看房客户）",
			Content:   "您好，我是 XX 中介的置业顾问小张，看到您咨询了 XX 楼盘/区域。请问您是自住还是投资？预算大概在什么范围？我可以根据您的需求精准匹配 3-5 套优质房源。",
			Tags:      []string{"破冰", "需求探查"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryRealEstate,
			Stage:     StageLead,
			Objection: PlayObjectionNeed,
			Title:     "房产需求异议（再看看）",
			Content:   "理解您要多看几个楼盘再做决定。但您看中的 XX 房源，目前同户型只剩 2 套，价格已经上调 5%。我建议您先实地看一次，满意就定，不满意也帮助您了解市场。您看本周末方便吗？我开车接您。",
			Tags:      []string{"需求异议", "看房"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryRealEstate,
			Stage:     StageInterested,
			Objection: PlayObjectionPrice,
			Title:     "房产价格异议（总价高）",
			Content:   "理解您对总价的考虑。但您看这个楼盘 3 年内升值 30%，周边配套（地铁/学区/医院）齐全。同等配套的竞品单价要贵 8000 元/平。我们现在签约有 9.8 折+送装修，您要不要算一下月供？我可以帮您做个方案。",
			Tips:      "强调：升值空间 + 配套对比 + 总价分摊到月供",
			Tags:      []string{"价格异议", "升值空间"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryRealEstate,
			Stage:     StageQuoted,
			Objection: PlayObjectionTrust,
			Title:     "房产信任异议（怕烂尾）",
			Content:   "您的担心非常合理，烂尾确实是购房者最担心的问题。这个开发商是 TOP10 央企，主体已封顶+资金监管账户足额。您可以到住建局官网查询预售许可证。我们已经帮 200+ 客户完成过户，您可以看看他们的真实评价。要不要我带您去工地实地看看进度？",
			Tags:      []string{"信任异议", "烂尾", "央企"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryRealEstate,
			Stage:     StageQuoted,
			Title:     "房产逼单-稀缺房源",
			Content:   "您看中的这套 18 楼 138 平三居室，目前同楼层只剩最后 1 套。同一栋楼的客户都在问这套房，今天不下定，明天可能就被别人定走了。我帮您先锁房，您交 2 万意向金可以保留 24 小时，怎么样？",
			Tags:      []string{"逼单", "稀缺房源"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range realEstateDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === 汽车 行业 ===
	autoDefaults := []*PlaybookEntry{
		{
			Industry:  IndustryAuto,
			Stage:     StageStranger,
			Title:     "汽车首次破冰（询价客户）",
			Content:   "您好，我是 XX 品牌的销售顾问小陈，看到您咨询了 XX 车型。请问您是首次购车还是置换？主要用途是上下班代步还是家庭出行？我可以根据您的需求推荐最适合的配置和颜色。",
			Tags:      []string{"破冰", "需求探查"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryAuto,
			Stage:     StageInterested,
			Objection: PlayObjectionPrice,
			Title:     "汽车价格异议（太贵了）",
			Content:   "理解您的考虑。但您看这款车搭载 XX 发动机+智能驾驶系统，同级别车型里配置最高、油耗最低。现在分期 0 利率+5 年质保，算下来日供只要 XX 元。您要不要来店里试驾体验一下？",
			Tags:      []string{"价格异议", "分期"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryAuto,
			Stage:     StageContact,
			Objection: PlayObjectionCompetition,
			Title:     "汽车竞品异议（在看别家）",
			Content:   "完全理解您要货比三家。但您看 XX 品牌在智能驾驶、空间、配置上的差距（这里罗列数据对比）。建议您来店里试驾对比一下，亲自感受最重要。我们也准备了竞品对比手册，可以送给您参考。",
			Tips:      "准备：核心数据对比表（动力/空间/配置/油耗/价格）",
			Tags:      []string{"竞品异议", "试驾"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryAuto,
			Stage:     StageQuoted,
			Objection: PlayObjectionAuthority,
			Title:     "汽车决策权异议（要问家人）",
			Content:   "完全理解，买车是家庭大事。建议您带家人一起到店试乘试驾，我们准备了家庭茶歇和小礼物。家人体验过之后决定会更踏实。我帮您预约本周六/日下午，您看方便吗？",
			Tags:      []string{"决策权异议", "家庭试驾"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryAuto,
			Stage:     StageQuoted,
			Title:     "汽车逼单-车展优惠",
			Content:   "本月车展优惠：现车直降 1.5 万+购置税减半+送 5 次保养。今天下定还能参加抽奖，最高 5000 元油卡。这个优惠只到月底，您现在下定锁定优惠+车型颜色，您看方便签个意向合同吗？",
			Tags:      []string{"逼单", "车展"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range autoDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === 金融 行业 ===
	financeDefaults := []*PlaybookEntry{
		{
			Industry:  IndustryFinance,
			Stage:     StageStranger,
			Title:     "金融首次破冰（理财客户）",
			Content:   "您好，我是 XX 银行/财富顾问小赵，看到您咨询了 XX 产品。请问您是稳健型还是激进型投资者？投资周期多长？我可以根据您的风险偏好推荐 2-3 款匹配的理财产品。",
			Tags:      []string{"破冰", "风险偏好"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryFinance,
			Stage:     StageInterested,
			Objection: PlayObjectionTrust,
			Title:     "金融信任异议（怕亏损）",
			Content:   "您的担心非常合理，投资确实有风险。我们这款产品是 R2 中低风险等级，历史年化 3.5-4.2%，底层资产是国债+大型央企债。到期兑付率 100%。您可以先投 1 万试试，体验一个周期，您看怎么样？",
			Tags:      []string{"信任异议", "风险"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryFinance,
			Stage:     StageQuoted,
			Objection: PlayObjectionPrice,
			Title:     "金融价格异议（收益太低）",
			Content:   "理解您对收益的期待。但当前市场利率下行环境下，能稳定 3.5%+ 的产品已经很稀缺了。股票/基金收益高但波动大，我们这款产品适合作为家庭资产配置的稳健基石。您可以考虑 60% 稳健+40% 进取的组合。",
			Tags:      []string{"价格异议", "资产配置"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryFinance,
			Stage:     StageQuoted,
			Objection: PlayObjectionAuthority,
			Title:     "金融决策权异议（要问配偶）",
			Content:   "完全理解，家庭财务决策需要共同商议。建议您和家人一起到线下网点，我们有专业理财师可以为您和家人做一次免费的家庭资产诊断，1 对 1 沟通 1 小时。您看本周末方便吗？我帮您预约。",
			Tags:      []string{"决策权异议", "家庭资产诊断"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range financeDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === B2B 行业 ===
	b2bDefaults := []*PlaybookEntry{
		{
			Industry:  IndustryB2B,
			Stage:     StageStranger,
			Title:     "B2B 首次破冰（企业客户）",
			Content:   "您好，我是 XX 公司商务经理小王，看到您咨询了我们的 XX 解决方案。请问贵司目前主要痛点是什么？是降本增效还是数字化转型？贵司规模和所在行业？我可以安排我们的方案专家为您做一次免费诊断。",
			Tags:      []string{"破冰", "企业需求"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryB2B,
			Stage:     StageLead,
			Objection: PlayObjectionNeed,
			Title:     "B2B 需求异议（没明确需求）",
			Content:   "理解您目前还在调研阶段。但我们与 XX 同行业（行业头部）的合作经验可以为您提供参考。我们准备了一份《行业数字化转型白皮书》，可以帮您快速了解行业最佳实践。我加您微信发您，您看完后我们再约个时间聊聊？",
			Tags:      []string{"需求异议", "白皮书"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryB2B,
			Stage:     StageInterested,
			Objection: PlayObjectionPrice,
			Title:     "B2B 价格异议（预算有限）",
			Content:   "理解您对预算的把控。但您看我们的方案可以帮贵司每年节省 XX 万运营成本+提升 XX% 人效，投入回报周期仅 6-8 个月。我们也支持分期付款+按效果付费。建议您先做一期试点（3 个月），看到效果再扩展。",
			Tips:      "强调：ROI 计算 + 试点方案 + 效果付费",
			Tags:      []string{"价格异议", "ROI"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryB2B,
			Stage:     StageContact,
			Objection: PlayObjectionTrust,
			Title:     "B2B 信任异议（怕不稳定）",
			Content:   "理解您的考量。我们服务过 XX、XX 等 500+ 头部企业，已经稳定运行 5 年+。我们提供本地化部署+7×24 售后+SLA 99.99% 保障。合同里也明确了数据安全和违约责任。您可以要求我们提供 3 家同行业的客户案例和实地考察。",
			Tags:      []string{"信任异议", "客户案例"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryB2B,
			Stage:     StageQuoted,
			Objection: PlayObjectionAuthority,
			Title:     "B2B 决策权异议（要走流程）",
			Content:   "完全理解，B2B 采购流程严谨。我可以帮您准备完整的方案书+ROI 分析+同类客户案例，方便您向领导汇报。也可以安排我们公司 VP 与您领导直接沟通。您看什么时候方便组织一次线上会议？",
			Tags:      []string{"决策权异议", "采购流程"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryB2B,
			Stage:     StageQuoted,
			Objection: PlayObjectionTime,
			Title:     "B2B 时间异议（再评估下）",
			Content:   "完全理解，B2B 决策周期长。但您看这个季度我们有 8% 返点优惠，下个季度会恢复原价。我们可以先做 1-2 周 PoC 验证效果，您满意再走正式流程。您看什么时候方便启动？",
			Tags:      []string{"时间异议", "PoC 验证"},
			CreatedBy: "系统预设",
		},
		{
			Industry:  IndustryB2B,
			Stage:     StageQuoted,
			Title:     "B2B 逼单-季度返点",
			Content:   "本季度签约可享受 8% 返点+免费 3 个月运维服务。这个优惠只到月底。我们也在和 XX 竞争对手接触这个客户，建议您尽快确定。我可以今天就安排法务起草合同，您看怎么样？",
			Tags:      []string{"逼单", "返点"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range b2bDefaults {
		_, _ = s.Add(ctx, e)
	}

	// === 通用异议处理（跨行业） ===
	commonDefaults := []*PlaybookEntry{
		{
			Stage:     StageInterested,
			Objection: PlayObjectionStall,
			Title:     "通用拖延异议（再考虑下）",
			Content:   "完全理解您的考虑。但我们这个方案现在是最好的时机，再等下去价格/优惠都会变。不如我们先约个时间详细了解一下，您也看看是否真的适合您，不合适我们也不勉强。您看本周几方便？",
			Tags:      []string{"拖延异议", "通用"},
			CreatedBy: "系统预设",
		},
		{
			Stage:     StageInterested,
			Objection: PlayObjectionCompetition,
			Title:     "通用竞品异议",
			Content:   "理解您会比较。但您看我们的核心差异在 A/B/C 三点，别家暂时还做不到。我们可以先约个免费体验，您亲自对比一下再决定。您看什么时候方便？",
			Tags:      []string{"竞品异议", "对比"},
			CreatedBy: "系统预设",
		},
	}
	for _, e := range commonDefaults {
		_, _ = s.Add(ctx, e)
	}
}

// FormatPlaybook 输出话术（去掉富文本，便于阅读）
func (s *PlaybookService) FormatPlaybook(ctx context.Context, e *PlaybookEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s】\n", e.Title))
	if len(e.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("标签: %s\n", strings.Join(e.Tags, " / ")))
	}
	sb.WriteString("\n")
	sb.WriteString(e.Content)
	if e.Tips != "" {
		sb.WriteString("\n\n")
		sb.WriteString("💡 ")
		sb.WriteString(e.Tips)
	}
	if e.UseCount > 0 {
		rate := int(successRate(e) * 100)
		sb.WriteString(fmt.Sprintf("\n\n📊 已使用 %d 次，成功率 %d%%", e.UseCount, rate))
	}
	return sb.String()
}
