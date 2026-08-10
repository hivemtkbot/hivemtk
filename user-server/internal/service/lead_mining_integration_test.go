package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// 以下集成测试构造真实数据（lead_mining_config / message_hub / customers / clues），
// 在真实 PostgreSQL 中验证「配置落库→多轮提取→LLM判定→打标签→写线索库存→去重」全链路。
// 不依赖真实 LLM：注入 fakeJudge，但 custRepo/clueRepo/cfgRepo/historyFetcher 全部走真实实现。

// 构造一条带唯一 MsgID 的入站消息
func seedHub(platform, sender, name, content string, idx int) model.MessageHub {
	return model.MessageHub{
		MsgID:          fmt.Sprintf("seed-%s-%d", sender, idx),
		Platform:       platform,
		SenderID:       sender,
		SenderName:     name,
		Direction:      "inbound",
		MsgType:        "text",
		Content:        content,
		ConversationID: "conv-" + sender,
		SentAt:         time.Now().Add(-time.Duration(10-idx) * time.Minute),
	}
}

// 真实管道构造：配置落库 + 多轮消息落库 + 真实 fetchHistoryDB + 真实写库
func TestLeadMining_Integration_FullPipeline(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.LeadMiningConfig{},
		&model.MessageHub{},
		&model.Customer{},
		&model.Clue{},
	)
	db.SetTestDB(database)

	ctx := context.Background()
	cfgRepo := repository.NewLeadMiningConfigRepository()

	// 1) 构造配置（真实落库）
	cfg := &model.LeadMiningConfig{
		Enabled:        true,
		Keywords:       model.JSONStrings{"购买", "代理", "报价", "合作"},
		Tags:           model.JSONStrings{"高意向", "待跟进"},
		Channels:       model.JSONStrings{"telegram", "douyin"},
		Requirement:    "客户明确表达购买意向或咨询代理/报价，即视为线索",
		MinIntentScore: 50,
	}
	if err := cfgRepo.Save(ctx, cfg); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}
	// 校验 GetSingleton 能从 DB 读回
	got, err := cfgRepo.GetSingleton(ctx)
	if err != nil || got == nil || !got.Enabled {
		t.Fatalf("配置未正确落库: err=%v got=%+v", err, got)
	}
	if len(got.Keywords) != 4 {
		t.Fatalf("配置关键词未持久化: %v", got.Keywords)
	}

	// 2) 构造真实多轮入站消息（同一 telegram 客户，4 条入站 + 1 条出站）
	sender := "integration_lead_001"
	cleanupLeadMiningData(t, database, sender)
	msgs := []model.MessageHub{
		seedHub("telegram", sender, "王经理", "你好，在吗", 1),
		seedHub("telegram", sender, "王经理", "我想了解一下你们的产品报价", 2),
		seedHub("telegram", sender, "王经理", "如果合适我想做区域代理", 3),
		seedHub("telegram", sender, "王经理", "大概预算 5 万左右", 4),
	}
	for i := range msgs {
		if err := database.Create(&msgs[i]).Error; err != nil {
			t.Fatalf("构造消息失败: %v", err)
		}
	}
	// 出站消息不应进入多轮历史
	out := seedHub("telegram", sender, "王经理", "好的，我给您发资料", 5)
	out.Direction = "outbound"
	if err := database.Create(&out).Error; err != nil {
		t.Fatalf("构造出站消息失败: %v", err)
	}

	// 3) 真实管道：注入 fakeJudge，但其余全部走真实实现
	judge := &fakeJudge{resp: &LeadJudgement{
		IsLead:          true,
		IntentScore:     85,
		MatchedKeywords: []string{"报价", "代理"},
		MatchedTags:     []string{"AI兴趣"},
		Summary:         "王经理咨询产品报价并表达区域代理意向，预算约5万",
		Confidence:      0.95,
	}}
	s := &Service{
		judge:     judge,
		custRepo:  repository.NewCustomerRepository(),
		clueRepo:  repository.NewClueRepository(),
		cfgRepo:   cfgRepo,
		lastJudge: map[string]time.Time{},
	}

	// 触发判定（取最后一条入站消息作为钩子入口）
	last := msgs[len(msgs)-1]
	s.process(ctx, &last)

	// 4) 断言：线索写入真实 clues 表（type=8）
	account := "telegram:" + sender
	var clue model.Clue
	if err := database.Where("type = ? AND account = ?", ClueTypeLeadMining, account).First(&clue).Error; err != nil {
		t.Fatalf("未写入线索库存: %v", err)
	}
	if clue.IntentScore != 85 {
		t.Fatalf("线索意向分异常: %d", clue.IntentScore)
	}
	if clue.SourceID != "lead_mining" {
		t.Fatalf("线索来源异常: %s", clue.SourceID)
	}
	if clue.IsOpportunity != 1 {
		t.Fatalf("高意向(>=70)应标记商机，实际 %d", clue.IsOpportunity)
	}
	if clue.OneID != "lm:"+account {
		t.Fatalf("线索 OneID 应关联客户稳定键，实际 %s", clue.OneID)
	}

	// 5) 断言：客户被打标签（真实 customers 表）
	var cust model.Customer
	if err := database.Where("unified_id = ?", "lm:"+account).First(&cust).Error; err != nil {
		t.Fatalf("未创建客户: %v", err)
	}
	var tags []string
	_ = json.Unmarshal([]byte(cust.Tags), &tags)
	if !containsTag(tags, "高意向") || !containsTag(tags, "待跟进") || !containsTag(tags, "AI兴趣") {
		t.Fatalf("客户标签缺失，实际: %v", tags)
	}

	// 6) 去重：跨时间窗再次判定，应走 UpdateByID 而非新建
	delete(s.lastJudge, account)
	s.process(ctx, &last)
	var cnt int64
	database.Model(&model.Clue{}).Where("type = ? AND account = ?", ClueTypeLeadMining, account).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("去重失败：同账号线索应只有 1 条，实际 %d", cnt)
	}
}

// 非线索场景：构造闲聊消息，判定为 false，不应写库
func TestLeadMining_Integration_NotLead(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.LeadMiningConfig{},
		&model.MessageHub{},
		&model.Customer{},
		&model.Clue{},
	)
	db.SetTestDB(database)

	ctx := context.Background()
	cfgRepo := repository.NewLeadMiningConfigRepository()
	cfg := &model.LeadMiningConfig{Enabled: true, MinIntentScore: 50}
	if err := cfgRepo.Save(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	sender := "integration_chat_002"
	cleanupLeadMiningData(t, database, sender)
	msg := seedHub("douyin", sender, "小李", "今天天气真不错", 1)
	if err := database.Create(&msg).Error; err != nil {
		t.Fatal(err)
	}

	judge := &fakeJudge{resp: &LeadJudgement{IsLead: false, IntentScore: 10}}
	s := &Service{
		judge:     judge,
		custRepo:  repository.NewCustomerRepository(),
		clueRepo:  repository.NewClueRepository(),
		cfgRepo:   cfgRepo,
		lastJudge: map[string]time.Time{},
	}
	s.process(ctx, &msg)

	var cnt int64
	database.Model(&model.Clue{}).Where("type = ?", ClueTypeLeadMining).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("非线索不应写库，实际 %d 条", cnt)
	}
}

// 异步非阻塞路径：Enqueue 后由 worker 在真实 DB 中落地线索
func TestLeadMining_Integration_AsyncEnqueue(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.LeadMiningConfig{},
		&model.MessageHub{},
		&model.Customer{},
		&model.Clue{},
	)
	db.SetTestDB(database)

	ctx := context.Background()
	cfgRepo := repository.NewLeadMiningConfigRepository()
	cfg := &model.LeadMiningConfig{Enabled: true, Tags: model.JSONStrings{"高意向"}, MinIntentScore: 50}
	if err := cfgRepo.Save(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	sender := "integration_async_003"
	cleanupLeadMiningData(t, database, sender)
	msg := seedHub("telegram", sender, "赵总", "我想购买你们的企业版", 1)
	if err := database.Create(&msg).Error; err != nil {
		t.Fatal(err)
	}

	judge := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 75, MatchedTags: []string{"AI兴趣"}}}
	ctxCancel, cancel := context.WithCancel(ctx)
	s := &Service{
		queue:     make(chan *model.MessageHub, 16),
		workers:   1,
		judge:     judge,
		custRepo:  repository.NewCustomerRepository(),
		clueRepo:  repository.NewClueRepository(),
		cfgRepo:   cfgRepo,
		cancel:    cancel,
		lastJudge: map[string]time.Time{},
	}
	s.wg.Add(1)
	go s.worker(ctxCancel)
	defer s.Stop()

	// 非阻塞入队（模拟 persistMessage 中的唯一钩子）
	s.Enqueue(&msg)

	// 轮询真实 DB，等待 worker 异步落地
	account := "telegram:" + sender
	deadline := time.Now().Add(3 * time.Second)
	for {
		var cnt int64
		database.Model(&model.Clue{}).Where("type = ? AND account = ?", ClueTypeLeadMining, account).Count(&cnt)
		if cnt >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("异步 worker 未在 3s 内将线索落地到真实 DB")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if judge.calls != 1 {
		t.Fatalf("异步路径应判定 1 次，实际 %d", judge.calls)
	}
}

// 去抖：同客户窗口内多条消息只判定一次（真实管道）
func TestLeadMining_Integration_Debounce(t *testing.T) {
	database := testutil.NewTestDB(t,
		&model.LeadMiningConfig{},
		&model.MessageHub{},
		&model.Customer{},
		&model.Clue{},
	)
	db.SetTestDB(database)

	ctx := context.Background()
	cfgRepo := repository.NewLeadMiningConfigRepository()
	cfg := &model.LeadMiningConfig{Enabled: true, MinIntentScore: 50}
	if err := cfgRepo.Save(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	sender := "integration_deb_004"
	cleanupLeadMiningData(t, database, sender)
	for i := 1; i <= 3; i++ {
		m := seedHub("telegram", sender, "钱总", fmt.Sprintf("消息%d：我想咨询代理", i), i)
		if err := database.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}

	judge := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 80}}
	s := &Service{
		judge:     judge,
		custRepo:  repository.NewCustomerRepository(),
		clueRepo:  repository.NewClueRepository(),
		cfgRepo:   cfgRepo,
		lastJudge: map[string]time.Time{},
	}
	// 模拟同一客户短时间内连续多条消息进入
	for i := 1; i <= 3; i++ {
		var m model.MessageHub
		database.Where("msg_id = ?", fmt.Sprintf("seed-%s-%d", sender, i)).First(&m)
		s.process(ctx, &m)
	}
	if judge.calls != 1 {
		t.Fatalf("同客户窗口内应只判定 1 次，实际 %d", judge.calls)
	}
	var cnt int64
	database.Model(&model.Clue{}).Where("type = ?", ClueTypeLeadMining).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("去抖后应只写 1 条线索，实际 %d", cnt)
	}
}

func containsTag(tags []string, v string) bool {
	for _, x := range tags {
		if strings.EqualFold(strings.TrimSpace(x), strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

// cleanupLeadMiningData 测试结束后清理本测试写入共享测试库的数据，
// 避免污染同进程后续测试（NewTestDB 设计上同进程共享库、AutoMigrate 累加，
// 不主动清数据会导致后续顺序相关断言失败）。仅清理本测试使用的 sender 维度数据。
func cleanupLeadMiningData(t *testing.T, database *gorm.DB, senders ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, s := range senders {
			account := "telegram:" + s
			database.Where("type = ? AND account = ?", ClueTypeLeadMining, account).Delete(&model.Clue{})
			database.Where("unified_id = ?", "lm:"+account).Delete(&model.Customer{})
			database.Where("sender_id = ?", s).Delete(&model.MessageHub{})
			// douyin 渠道场景
			accountD := "douyin:" + s
			database.Where("type = ? AND account = ?", ClueTypeLeadMining, accountD).Delete(&model.Clue{})
			database.Where("unified_id = ?", "lm:"+accountD).Delete(&model.Customer{})
		}
		// 单例配置表：清理本测试写入的配置，避免污染后续测试的 GetSingleton 读取
		database.Delete(&model.LeadMiningConfig{})
	})
}
