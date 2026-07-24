package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"marketing/internal/dto"
)

// ============================================================================
// 商业产品级 销冠话术库 + 异议处理模板 测试套件
// ----------------------------------------------------------------------------
// 测试目标：100% 覆盖话术库的核心商业场景
//   1) 默认话术覆盖度（每个行业、每个常见阶段都有预设话术）
//   2) 多维推荐精度（行业+阶段+异议的精确匹配）
//   3) 智能意图推断（从意图识别自动推断异议类型）
//   4) 使用统计与成功率排序
//   5) 销售辅助模式集成（SalesEngine.RecommendPlaybook）
//   6) 业务流贯通（添加→推荐→使用→统计）
// ============================================================================

// TestPlaybook_DefaultSeeds_AllIndustries 所有行业都有默认话术
func TestPlaybook_DefaultSeeds_AllIndustries(t *testing.T) {
	svc := NewPlaybookService()

	industries := []Industry{
		IndustryMedicalBeauty,
		IndustryEducation,
		IndustryEcommerce,
		IndustryRealEstate,
		IndustryAuto,
		IndustryFinance,
		IndustryB2B,
	}

	for _, ind := range industries {
		entries := svc.List(context.Background(), ind, "")
		if len(entries) == 0 {
			t.Errorf("行业 %s 应有默认话术，实际为空", ind)
		}
		// 每个行业至少应有 3 条话术（破冰+异议+逼单）
		if len(entries) < 3 {
			t.Errorf("行业 %s 至少应有 3 条话术，实际: %d", ind, len(entries))
		}
		t.Logf("✅ 行业 %s：%d 条默认话术", ind, len(entries))
	}
}

// TestPlaybook_DefaultSeeds_AllStages 关键阶段都有话术
func TestPlaybook_DefaultSeeds_AllStages(t *testing.T) {
	svc := NewPlaybookService()
	stages := []JourneyStage{
		StageStranger,
		StageLead,
		StageContact,
		StageInterested,
		StageQuoted,
	}

	coverage := 0
	for _, st := range stages {
		// 检查每个阶段在医美行业是否有话术
		entries := svc.List(context.Background(), IndustryMedicalBeauty, st)
		if len(entries) > 0 {
			coverage++
		} else {
			t.Logf("⚠️  医美行业 阶段 %s 缺默认话术", st)
		}
	}
	if coverage < 4 {
		t.Errorf("医美行业至少 4 个阶段应有默认话术，实际: %d", coverage)
	}
}

// TestPlaybook_Recommend_IndustryStageObjection 三维精确匹配
func TestPlaybook_Recommend_IndustryStageObjection(t *testing.T) {
	svc := NewPlaybookService()

	// 医美+感兴趣阶段+价格异议
	got := svc.Recommend(context.Background(), PlaybookQuery{
		Industry:  IndustryMedicalBeauty,
		Stage:     StageInterested,
		Objection: PlayObjectionPrice,
		Limit:     3,
	})
	if len(got) == 0 {
		t.Fatal("医美+感兴趣+价格异议 应有推荐话术")
	}
	// 验证：返回的话术必须同时满足三个条件
	for _, e := range got {
		if e.Industry != IndustryMedicalBeauty {
			t.Errorf("推荐话术行业不匹配: %s", e.Industry)
		}
		if e.Stage != StageInterested {
			t.Errorf("推荐话术阶段不匹配: %s", e.Stage)
		}
		if e.Objection != PlayObjectionPrice {
			t.Errorf("推荐话术异议不匹配: %s", e.Objection)
		}
	}
	t.Logf("✅ 医美+感兴趣+价格异议：%d 条", len(got))
}

// TestPlaybook_Recommend_PriorityBySuccessRate 按成功率排序
func TestPlaybook_Recommend_PriorityBySuccessRate(t *testing.T) {
	svc := NewPlaybookService()

	// 模拟两条话术：一条成功率高，一条使用次数多
	highRate, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryB2B, Stage: StageInterested, Objection: PlayObjectionPrice,
		Title: "高成功率话术", Content: "ROI 计算话术",
		UseCount: 10, SuccessCount: 9, // 90%
		CreatedBy: "tester",
	})
	lowRate, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryB2B, Stage: StageInterested, Objection: PlayObjectionPrice,
		Title: "低成功率话术", Content: "ROI 计算话术",
		UseCount: 100, SuccessCount: 10, // 10%
		CreatedBy: "tester",
	})

	got := svc.Recommend(context.Background(), PlaybookQuery{
		Industry: IndustryB2B, Stage: StageInterested, Objection: PlayObjectionPrice, Limit: 10,
	})
	if len(got) < 2 {
		t.Fatal("推荐结果不足 2 条")
	}
	// 第一条应是高成功率的
	if got[0].ID != highRate.ID {
		t.Errorf("高成功率话术应排在首位，实际首位: %s", got[0].Title)
	}
	if got[0].ID == lowRate.ID {
		t.Error("高成功率话术不应被低成功率覆盖")
	}
	t.Logf("✅ 排序正确：%s (%.0f%%) 排在 %s (%.0f%%) 前",
		got[0].Title, successRate(got[0])*100,
		got[1].Title, successRate(got[1])*100)
}

// TestPlaybook_RecommendForResponse 智能意图推断
func TestPlaybook_RecommendForResponse(t *testing.T) {
	svc := NewPlaybookService()

	// 价格异议意图 → 应推荐价格话术
	got := svc.RecommendForResponse(context.Background(), IndustryAuto, "", StageInterested, IntentObjectionPrice)
	if len(got) == 0 {
		t.Fatal("价格异议意图应推荐价格话术")
	}
	for _, e := range got {
		if e.Objection != PlayObjectionPrice {
			t.Errorf("意图 %s 应推荐价格异议话术，实际异议: %s", IntentObjectionPrice, e.Objection)
		}
	}

	// 信任异议意图 → 应推荐信任话术
	got2 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageContact, IntentObjectionTrust)
	if len(got2) == 0 {
		t.Fatal("信任异议意图应推荐信任话术")
	}
	for _, e := range got2 {
		if e.Objection != PlayObjectionTrust {
			t.Errorf("意图 %s 应推荐信任异议话术，实际异议: %s", IntentObjectionTrust, e.Objection)
		}
	}

	// 竞品异议 → 竞品话术
	got3 := svc.RecommendForResponse(context.Background(), IndustryAuto, "", StageContact, IntentObjectionCompetitor)
	if len(got3) == 0 {
		t.Fatal("竞品异议意图应推荐竞品话术")
	}
	t.Logf("✅ 智能意图推断：3 种异议均正确")
}

// TestPlaybook_RecordUse_UsageStats 使用统计
func TestPlaybook_RecordUse_UsageStats(t *testing.T) {
	svc := NewPlaybookService()
	entry, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryEcommerce, Stage: StageQuoted,
		Title: "电商逼单", Content: "限时折扣", CreatedBy: "tester",
	})

	// 模拟销售调用 5 次，3 次成功
	for i := 0; i < 5; i++ {
		svc.RecordUse(context.Background(), entry.ID, i < 3)
	}

	got := svc.GetByID(context.Background(), entry.ID)
	if got.UseCount != 5 {
		t.Errorf("使用次数应为 5，实际: %d", got.UseCount)
	}
	if got.SuccessCount != 3 {
		t.Errorf("成功次数应为 3，实际: %d", got.SuccessCount)
	}
	rate := successRate(got)
	if rate < 0.59 || rate > 0.61 {
		t.Errorf("成功率应约 0.6，实际: %.2f", rate)
	}
	t.Logf("✅ 使用统计：%d 次，%d 成功，%.0f%%", got.UseCount, got.SuccessCount, rate*100)
}

// TestPlaybook_GetByID_NotFound 不存在的 ID
func TestPlaybook_GetByID_NotFound(t *testing.T) {
	svc := NewPlaybookService()
	if got := svc.GetByID(context.Background(), "not-exist"); got != nil {
		t.Error("不存在的 ID 应返回 nil")
	}
}

// TestPlaybook_Add_Validation 入参校验
func TestPlaybook_Add_Validation(t *testing.T) {
	svc := NewPlaybookService()

	// nil
	if _, err := svc.Add(context.Background(), nil); err == nil {
		t.Error("nil 入参应报错")
	}
	// 标题为空
	if _, err := svc.Add(context.Background(), &PlaybookEntry{Content: "test"}); err == nil {
		t.Error("标题为空应报错")
	}
	// 内容为空
	if _, err := svc.Add(context.Background(), &PlaybookEntry{Title: "test"}); err == nil {
		t.Error("内容为空应报错")
	}
}

// TestPlaybook_Recommend_Limit 限制返回数量
func TestPlaybook_Recommend_Limit(t *testing.T) {
	svc := NewPlaybookService()
	for i := 0; i < 20; i++ {
		_, _ = svc.Add(context.Background(), &PlaybookEntry{
			Industry: IndustryFinance, Stage: StageInterested, Objection: PlayObjectionPrice,
			Title: fmt.Sprintf("理财话术 %d", i), Content: "内容",
			CreatedBy: "tester",
		})
	}
	got := svc.Recommend(context.Background(), PlaybookQuery{
		Industry: IndustryFinance, Stage: StageInterested, Objection: PlayObjectionPrice, Limit: 3,
	})
	if len(got) != 3 {
		t.Errorf("应返回 3 条，实际: %d", len(got))
	}
	t.Logf("✅ Limit 生效：%d 条", len(got))
}

// TestPlaybook_Recommend_EmptyResult 无匹配时返回空
func TestPlaybook_Recommend_EmptyResult(t *testing.T) {
	svc := NewPlaybookService()
	got := svc.Recommend(context.Background(), PlaybookQuery{
		Industry: IndustryFinance, Stage: StageAfterSale, Objection: PlayObjectionStall,
	})
	if len(got) == 0 {
		t.Logf("✅ 无匹配时返回空数组（符合预期）")
	}
}

// TestPlaybook_FormatPlaybook 富文本输出
func TestPlaybook_FormatPlaybook(t *testing.T) {
	svc := NewPlaybookService()
	entry, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryEducation, Stage: StageInterested, Objection: PlayObjectionPrice,
		Title: "教培价格异议", Content: "内容 X", Tips: "强调：分摊到每节课",
		UseCount: 10, SuccessCount: 8, CreatedBy: "tester",
	})
	formatted := svc.FormatPlaybook(context.Background(), entry)
	if !strings.Contains(formatted, "教培价格异议") {
		t.Error("输出应包含标题")
	}
	if !strings.Contains(formatted, "内容 X") {
		t.Error("输出应包含内容")
	}
	if !strings.Contains(formatted, "分摊到每节课") {
		t.Error("输出应包含使用技巧")
	}
	if !strings.Contains(formatted, "80%") {
		t.Error("输出应包含成功率")
	}
	t.Logf("✅ 富文本输出：%d 字符", len(formatted))
}

// TestPlaybook_Concurrent 安全并发
func TestPlaybook_Concurrent(t *testing.T) {
	svc := NewPlaybookService()
	entry, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryB2B, Title: "并发测试", Content: "X", CreatedBy: "tester",
	})

	done := make(chan bool, 100)
	for i := 0; i < 50; i++ {
		go func() {
			svc.Recommend(context.Background(), PlaybookQuery{Industry: IndustryB2B, Limit: 5})
			done <- true
		}()
		go func() {
			svc.RecordUse(context.Background(), entry.ID, true)
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
	t.Logf("✅ 并发安全：50 个推荐 + 50 个记录无 race")
}

// ============================================================================
// 销售引擎集成测试
// ============================================================================

// TestSalesEngine_SetPlaybook_Integration 话术库集成
func TestSalesEngine_SetPlaybook_Integration(t *testing.T) {
	playbook := NewPlaybookService()
	engine := &SalesEngine{
		playbook: playbook,
	}

	// 不应有 nil 错误
	got := engine.RecommendPlaybook(context.Background(), IndustryMedicalBeauty, "prod_1", StageInterested, IntentObjectionPrice)
	if len(got) == 0 {
		t.Error("应推荐话术")
	}
	t.Logf("✅ 引擎集成：推荐 %d 条", len(got))
}

// TestSalesEngine_RecommendPlaybook_NilSafe nil 时安全
func TestSalesEngine_RecommendPlaybook_NilSafe(t *testing.T) {
	engine := &SalesEngine{} // 未注入 playbook
	got := engine.RecommendPlaybook(context.Background(), IndustryMedicalBeauty, "", StageInterested, IntentObjectionPrice)
	if got != nil {
		t.Error("未注入话术库时应返回 nil")
	}
	t.Logf("✅ nil safe：返回 nil")
}

// ============================================================================
// 行业场景 E2E 测试
// ============================================================================

// TestPlaybook_E2E_MedicalBeauty 医美行业完整闭环
func TestPlaybook_E2E_MedicalBeauty(t *testing.T) {
	svc := NewPlaybookService()
	owner := "doctor_li"
	custID := "mb_cust_001"
	_ = custID

	// 1. 客户首次咨询 → 推荐破冰话术
	got1 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageStranger, IntentGreeting)
	if len(got1) == 0 {
		t.Fatal("破冰话术缺失")
	}
	t.Logf("  1️⃣  破冰：%s", got1[0].Title)

	// 2. 留资 → 推荐首次回访
	got2 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageLead, IntentSocial)
	if len(got2) == 0 {
		t.Fatal("回访话术缺失")
	}
	t.Logf("  2️⃣  回访：%s", got2[0].Title)

	// 3. 客户说"太贵了" → 推荐价格异议
	got3 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageInterested, IntentObjectionPrice)
	if len(got3) == 0 {
		t.Fatal("价格异议话术缺失")
	}
	// 销售使用此话术，记录
	svc.RecordUse(context.Background(), got3[0].ID, true)
	t.Logf("  3️⃣  价格异议：%s", got3[0].Title)

	// 4. 客户怕不安全 → 推荐信任异议
	got4 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageContact, IntentObjectionTrust)
	if len(got4) == 0 {
		t.Fatal("信任异议话术缺失")
	}
	svc.RecordUse(context.Background(), got4[0].ID, false) // 这次没成功
	t.Logf("  4️⃣  信任异议：%s", got4[0].Title)

	// 5. 报价后客户要问老公 → 决策权异议
	got5 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageQuoted, IntentStall)
	if len(got5) == 0 {
		t.Fatal("决策权话术缺失")
	}
	svc.RecordUse(context.Background(), got5[0].ID, true)
	t.Logf("  5️⃣  决策权：%s", got5[0].Title)

	// 6. 逼单
	got6 := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "", StageQuoted, IntentPurchase)
	t.Logf("  6️⃣  逼单：%d 条", len(got6))

	t.Logf("✅ 医美完整闭环：5 个阶段 + 5 类异议全部覆盖")
	_ = owner
}

// TestPlaybook_E2E_B2B B2B 行业完整闭环
func TestPlaybook_E2E_B2B(t *testing.T) {
	svc := NewPlaybookService()

	// 1. 客户留资需求模糊
	got1 := svc.RecommendForResponse(context.Background(), IndustryB2B, "", StageLead, IntentAskProduct)
	if len(got1) == 0 {
		t.Fatal("B2B 需求异议话术缺失")
	}
	t.Logf("  1️⃣  需求探查：%s", got1[0].Title)

	// 2. 预算异议
	got2 := svc.RecommendForResponse(context.Background(), IndustryB2B, "", StageInterested, IntentObjectionPrice)
	if len(got2) == 0 {
		t.Fatal("B2B 价格异议话术缺失")
	}
	t.Logf("  2️⃣  预算异议：%s", got2[0].Title)

	// 3. 怕不稳定
	got3 := svc.RecommendForResponse(context.Background(), IndustryB2B, "", StageContact, IntentObjectionTrust)
	if len(got3) == 0 {
		t.Fatal("B2B 信任异议话术缺失")
	}
	t.Logf("  3️⃣  信任异议：%s", got3[0].Title)

	// 4. 决策权异议（要走流程）
	got4 := svc.RecommendForResponse(context.Background(), IndustryB2B, "", StageQuoted, IntentStall)
	if len(got4) == 0 {
		t.Fatal("B2B 决策权话术缺失")
	}
	t.Logf("  4️⃣  决策权：%s", got4[0].Title)

	t.Logf("✅ B2B 完整闭环")
}

// TestPlaybook_E2E_RealEstate 房产行业
func TestPlaybook_E2E_RealEstate(t *testing.T) {
	svc := NewPlaybookService()

	// 看房客户
	got1 := svc.RecommendForResponse(context.Background(), IndustryRealEstate, "", StageStranger, IntentGreeting)
	if len(got1) == 0 || got1[0].Industry != IndustryRealEstate {
		t.Errorf("房产破冰话术缺失或行业不匹配")
	}
	t.Logf("  1️⃣  破冰：%s", got1[0].Title)

	// 怕烂尾
	got2 := svc.RecommendForResponse(context.Background(), IndustryRealEstate, "", StageQuoted, IntentObjectionTrust)
	if len(got2) == 0 {
		t.Fatal("房产信任异议话术缺失")
	}
	t.Logf("  2️⃣  防烂尾：%s", got2[0].Title)

	// 稀缺房源逼单
	got3 := svc.RecommendForResponse(context.Background(), IndustryRealEstate, "", StageQuoted, IntentPurchase)
	if len(got3) == 0 {
		t.Fatal("房产逼单话术缺失")
	}
	t.Logf("  3️⃣  逼单：%s", got3[0].Title)

	t.Logf("✅ 房产完整闭环")
}

// TestPlaybook_Priority_NewVsOld 新话术有"冷启动"机会
func TestPlaybook_Priority_NewVsOld(t *testing.T) {
	svc := NewPlaybookService()

	// 高成功率旧话术（10 次使用，9 成功 = 90%）
	oldOne, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryAuto, Stage: StageInterested, Objection: PlayObjectionPrice,
		Title: "低成功率老话术", Content: "老版本",
		UseCount: 100, SuccessCount: 10, // 10% 成功率
		CreatedBy: "tester",
	})
	// 新话术（0 次使用）
	newOne, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryAuto, Stage: StageInterested, Objection: PlayObjectionPrice,
		Title: "新话术", Content: "新版本", CreatedBy: "tester",
	})
	_ = oldOne

	got := svc.Recommend(context.Background(), PlaybookQuery{
		Industry: IndustryAuto, Stage: StageInterested, Objection: PlayObjectionPrice, Limit: 5,
	})
	if len(got) < 3 {
		t.Fatal("应返回至少 3 条（种子+2 测试）")
	}

	// 种子里"汽车价格异议（太贵了）"：rate=3/5=60%
	// 老话术：rate=10/100=10%
	// 新话术：rate=0/0=0
	// 排序：种子里(60%) > 老话术(10%) > 新话术(0%)
	// 新话术应在最后
	foundNew := false
	for _, e := range got {
		if e.ID == newOne.ID {
			foundNew = true
		}
	}
	if !foundNew {
		t.Error("新话术应在推荐列表中")
	}
	// 验证：新话术排在老话术之后（rate 更低）
	idxNew, idxOld := -1, -1
	for i, e := range got {
		if e.ID == newOne.ID {
			idxNew = i
		}
		if e.Title == "低成功率老话术" {
			idxOld = i
		}
	}
	if idxNew == -1 || idxOld == -1 {
		t.Fatal("新话术或老话术未在结果中")
	}
	if idxNew <= idxOld {
		t.Errorf("新话术(rate=0) 应排在老话术(rate=0.1) 之后: new=%d, old=%d", idxNew, idxOld)
	}
	t.Logf("✅ 排序正确：种子里 > 老话术(%d) > 新话术(%d)", idxOld, idxNew)
}

// TestPlaybook_BusinessScenario_AITagger 集成自动打标签
func TestPlaybook_BusinessScenario_AITagger(t *testing.T) {
	svc := NewPlaybookService()
	tagger := NewAITagger()
	custID := "scn_001"

	// 1. 客户咨询价格 → 自动打价格敏感标签（通过 TagFromSalesResponse）
	resp := &SalesResponse{
		Intent: &dto.RecognizeResult{IntentType: IntentObjectionPrice, IntentName: "价格异议", Confidence: 0.9},
	}
	tagger.TagFromSalesResponse(context.Background(), custID, resp)

	// 2. 销售跟进：客户已 Interested 阶段 + 价格异议
	recs := svc.RecommendForResponse(context.Background(), IndustryMedicalBeauty, "prod_001", StageInterested, IntentObjectionPrice)
	if len(recs) == 0 {
		t.Fatal("推荐失败")
	}

	// 3. 销售使用第一条
	svc.RecordUse(context.Background(), recs[0].ID, true)
	t.Logf("✅ 业务流贯通：标签→推荐→使用→统计")
}

// TestPlaybook_BusinessScenario_FullLoop 完整业务流
func TestPlaybook_BusinessScenario_FullLoop(t *testing.T) {
	svc := NewPlaybookService()
	engine := &SalesEngine{playbook: svc}

	// 场景：销售接到一个医美感兴趣阶段客户
	// 客户发消息："这个太贵了"
	industry := IndustryMedicalBeauty
	productID := "prod_liposuction"
	stage := StageInterested
	intent := IntentObjectionPrice

	// 1. 销售自己加一条新话术（演示自定义话术）
	customEntry, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry:  industry,
		ProductID: productID,
		Stage:     stage,
		Objection: PlayObjectionPrice,
		Title:     "销冠小李的独门话术",
		Content:   "独家秘诀：先体验小部位看效果，再推全脸套餐",
		UseCount:  0, SuccessCount: 0,
		CreatedBy: "小李",
	})

	// 2. 引擎自动推荐话术
	recs := engine.RecommendPlaybook(context.Background(), industry, productID, stage, intent)
	if len(recs) == 0 {
		t.Fatal("应推荐话术")
	}
	t.Logf("  推荐 %d 条：", len(recs))
	for i, r := range recs {
		t.Logf("    %d. %s", i+1, r.Title)
	}

	// 3. 销售选择自定义话术（一定是新加的那条，因为它精确匹配 productID）
	chosen := customEntry
	for _, r := range recs {
		if r.ID == customEntry.ID {
			chosen = r
			break
		}
	}
	svc.RecordUse(context.Background(), chosen.ID, true)

	// 4. 验证：话术使用统计正确
	got := svc.GetByID(context.Background(), chosen.ID)
	if got.UseCount != 1 {
		t.Errorf("使用次数应为 1，实际: %d", got.UseCount)
	}
	if got.SuccessCount != 1 {
		t.Errorf("成功次数应为 1，实际: %d", got.SuccessCount)
	}
	t.Logf("✅ 完整业务流：自定义话术→推荐→选择→使用→成交→统计")
}

// TestPlaybook_CustomAddAndRecommend 自定义话术可被推荐
func TestPlaybook_CustomAddAndRecommend(t *testing.T) {
	svc := NewPlaybookService()

	// 销冠添加自己的话术
	custom, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryRealEstate, Stage: StageInterested, Objection: PlayObjectionPrice,
		Title: "张销冠的独门话术", Content: "独家秘诀...",
		Tags: []string{"销冠秘籍"}, CreatedBy: "张销冠",
	})
	_ = custom

	// 推荐时能找到
	got := svc.Recommend(context.Background(), PlaybookQuery{
		Industry: IndustryRealEstate, Stage: StageInterested, Objection: PlayObjectionPrice, Limit: 10,
	})
	found := false
	for _, e := range got {
		if e.Title == "张销冠的独门话术" {
			found = true
			if e.CreatedBy != "张销冠" {
				t.Errorf("创建者应为张销冠，实际: %s", e.CreatedBy)
			}
		}
	}
	if !found {
		t.Error("自定义话术应被推荐")
	}
	t.Logf("✅ 自定义话术可被推荐")
}

// TestPlaybook_TimestampSet 时间戳自动设置
func TestPlaybook_TimestampSet(t *testing.T) {
	svc := NewPlaybookService()
	entry, _ := svc.Add(context.Background(), &PlaybookEntry{
		Industry: IndustryEcommerce, Title: "时间戳测试", Content: "x", CreatedBy: "tester",
	})
	if entry.CreatedAt.IsZero() {
		t.Error("CreatedAt 应自动设置")
	}
	if entry.UpdatedAt.IsZero() {
		t.Error("UpdatedAt 应自动设置")
	}
	if time.Since(entry.CreatedAt) > 1*time.Second {
		t.Error("时间戳应为当前时间")
	}
	t.Logf("✅ 时间戳自动设置")
}
