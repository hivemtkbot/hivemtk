package service

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/model"
)

// =============================================================================
// DetectTelegramIntent：中英双语意向打分（纯函数）
// =============================================================================

func TestDetectTelegramIntent(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		minScore int
		wantOpp  bool
		wantSig  string // 期望命中的关键信号（可空）
	}{
		{name: "空消息", text: "", minScore: 0, wantOpp: false},
		{name: "普通寒暄", text: "大家好呀", minScore: 8, wantOpp: false},
		{name: "高意向-中文", text: "请问这个产品价格多少？可以批发吗", minScore: 60, wantOpp: true, wantSig: "价格"},
		{name: "中意向", text: "我想了解一下你们的产品", minScore: 20, wantOpp: false, wantSig: "了解"},
		{name: "优惠咨询", text: "有什么优惠活动吗", minScore: 20, wantOpp: false, wantSig: "优惠"},
		{name: "联系方式信号", text: "加我微信 vx:abc123 聊聊", minScore: 40, wantOpp: true, wantSig: "联系方式"},
		{name: "邮箱联系方式", text: "please contact me at sales@example.com", minScore: 40, wantOpp: true, wantSig: "联系方式"},
		{name: "高意向-英文", text: "Hi, how much is the price? I want to buy wholesale", minScore: 90, wantOpp: true, wantSig: "how much"},
		{name: "命令不算意向", text: "/start", minScore: 8, wantOpp: false},
		{name: "纯表情", text: "😀😀😀", minScore: 8, wantOpp: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			score, signals, isOpp := DetectTelegramIntent(c.text)
			if score < c.minScore {
				t.Fatalf("text=%q score=%d 期望 >= %d", c.text, score, c.minScore)
			}
			if isOpp != c.wantOpp {
				t.Fatalf("text=%q isOpp=%v 期望 %v (score=%d signals=%v)", c.text, isOpp, c.wantOpp, score, signals)
			}
			if c.wantSig != "" {
				hit := false
				for _, s := range signals {
					if strings.Contains(s, c.wantSig) {
						hit = true
						break
					}
				}
				if !hit {
					t.Fatalf("text=%q 期望命中信号含 %q，实际 signals=%v", c.text, c.wantSig, signals)
				}
			}
		})
	}
}

func TestDetectTelegramIntent_ScoreCap(t *testing.T) {
	// 多条高意向词叠加应封顶 100
	score, _, _ := DetectTelegramIntent("how much price buy wholesale distributor reseller procurement")
	if score != 100 {
		t.Fatalf("期望封顶 100，实际 %d", score)
	}
}

// =============================================================================
// format / parse 意向分往返
// =============================================================================

func TestFormatAndParseTelegramLeadScore(t *testing.T) {
	desc := formatTelegramLeadDesc("销售交流群", "我想买", 82, []string{"价格", "合作"}, true)
	if !strings.Contains(desc, "[意向分:82]") {
		t.Fatalf("desc 未内嵌意向分: %s", desc)
	}
	if !strings.Contains(desc, "群发言商机") {
		t.Fatalf("商机应标记为「群发言商机」: %s", desc)
	}
	if got := formatTelegramLeadDesc("群", "hi", 8, nil, false); !strings.Contains(got, "群发言线索") {
		t.Fatalf("普通线索应标记为「群发言线索」: %s", got)
	}
}

// =============================================================================
// telegramLeadAccountKey：去重键
// =============================================================================

func TestTelegramLeadAccountKey(t *testing.T) {
	if k := telegramLeadAccountKey("alice", "123"); k != "@alice" {
		t.Fatalf("username 优先，期望 @alice 实际 %q", k)
	}
	if k := telegramLeadAccountKey("@bob", "456"); k != "@bob" {
		t.Fatalf("username 带 @ 应原样保留，实际 %q", k)
	}
	if k := telegramLeadAccountKey("", "789"); k != "tg:789" {
		t.Fatalf("无 username 回退 tg:<id>，实际 %q", k)
	}
	if k := telegramLeadAccountKey("", "0"); k != "" {
		t.Fatalf("id 为 0 应返回空（不建线索），实际 %q", k)
	}
	if k := telegramLeadAccountKey("", ""); k != "" {
		t.Fatalf("全空应返回空，实际 %q", k)
	}
}

// =============================================================================
// mineTelegramGroupLead：去重 / 意向分增量 / 噪声过滤
// =============================================================================

func TestMineTelegramGroupLead_CreateDedupUpgrade(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}
	account := "@alice"
	hub := &model.MessageHub{MsgID: "m-1", ConversationID: "conv-alice", SenderID: "123"}

	// 首次：寒暄 意向分 8 → 新建（非商机，newOpportunity=false）
	newOpp := svc.mineTelegramGroupLead(hub, "-1001", "群A", "123", "alice", "Alice", "大家好呀")
	if newOpp {
		t.Fatalf("寒暄不应标记为新晋商机")
	}
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, account)
	if err != nil || got == nil {
		t.Fatalf("首次挖掘应创建线索: err=%v got=%v", err, got)
	}
	if got.Account != account || got.Type != ClueTypeTelegram {
		t.Fatalf("线索字段异常: %+v", got)
	}
	if !strings.Contains(got.Desc, "[意向分:8]") {
		t.Fatalf("首条 desc 应含意向分8: %s", got.Desc)
	}
	// 结构化意向分 + 会话关联（不再依赖解析 Desc 文本）
	if got.IntentScore != 8 || got.IsOpportunity != 0 {
		t.Fatalf("首条意向分/商机标记异常: score=%d opp=%d", got.IntentScore, got.IsOpportunity)
	}
	if got.MessageID != "m-1" || got.ConversationID != "conv-alice" || got.OneID != "123" {
		t.Fatalf("会话关联字段异常: msg=%q conv=%q one=%q", got.MessageID, got.ConversationID, got.OneID)
	}
	if n := countClues(t, db, account); n != 1 {
		t.Fatalf("同一账号应仅 1 条线索，实际 %d", n)
	}

	// 再次：高意向（请问12+价格25+多少12+批发25+base8=82）→ 增量更新到 82，仍 1 条，且新晋为商机
	newOpp = svc.mineTelegramGroupLead(hub, "-1001", "群A", "123", "alice", "Alice", "请问价格多少批发")
	if !newOpp {
		t.Fatalf("跨过阈值应为新晋商机")
	}
	got, _ = svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, account)
	if !strings.Contains(got.Desc, "[意向分:82]") {
		t.Fatalf("应升级到意向分82: %s", got.Desc)
	}
	if got.IntentScore != 82 || got.IsOpportunity != 1 {
		t.Fatalf("应升级到意向分82且标为商机: score=%d opp=%d", got.IntentScore, got.IsOpportunity)
	}
	if got.MessageID != "m-1" || got.ConversationID != "conv-alice" || got.OneID != "123" {
		t.Fatalf("升级后会话关联应保持: msg=%q conv=%q one=%q", got.MessageID, got.ConversationID, got.OneID)
	}
	if n := countClues(t, db, account); n != 1 {
		t.Fatalf("升级不应新增线索，实际 %d", n)
	}

	// 再次：低意向 8 → 不降级（保留 82）；已为商机，newOpportunity=false
	newOpp = svc.mineTelegramGroupLead(hub, "-1001", "群A", "123", "alice", "Alice", "你好啊")
	if newOpp {
		t.Fatalf("非跨阈值升级不应再标记新晋商机")
	}
	got, _ = svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, account)
	if !strings.Contains(got.Desc, "[意向分:82]") {
		t.Fatalf("低意向不应覆盖高分: %s", got.Desc)
	}
	if got.IntentScore != 82 || got.IsOpportunity != 1 {
		t.Fatalf("低意向不应覆盖高分: score=%d opp=%d", got.IntentScore, got.IsOpportunity)
	}
	if n := countClues(t, db, account); n != 1 {
		t.Fatalf("去重后仍为 1 条，实际 %d", n)
	}
}

func TestMineTelegramGroupLead_NoiseSkipped(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}
	account := "@bob"
	hub := &model.MessageHub{MsgID: "m-b", ConversationID: "conv-bob", SenderID: "999"}

	svc.mineTelegramGroupLead(hub, "-1001", "群A", "999", "bob", "Bob", "/start") // 命令
	svc.mineTelegramGroupLead(hub, "-1001", "群A", "999", "bob", "Bob", "😀😀😀")    // 纯表情
	svc.mineTelegramGroupLead(hub, "-1001", "群A", "999", "bob", "Bob", "   ")    // 空白

	if got, _ := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, account); got != nil {
		t.Fatalf("噪声消息不应建线索，实际: %+v", got)
	}
	if n := countClues(t, db, account); n != 0 {
		t.Fatalf("噪声不应产生线索，实际 %d", n)
	}
}

func TestMineTelegramGroupLead_FallbackID(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}
	hub := &model.MessageHub{MsgID: "m-c", ConversationID: "conv-555", SenderID: "555"}
	// 无 username → 用 tg:<id> 作去重键
	svc.mineTelegramGroupLead(hub, "-1001", "群A", "555", "", "Unknown", "我想采购一批货")
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "tg:555")
	if err != nil || got == nil {
		t.Fatalf("无 username 应回退 tg:<id> 建线索: err=%v got=%v", err, got)
	}
	if got.OneID != "555" || got.ConversationID != "conv-555" {
		t.Fatalf("无 username 会话关联异常: one=%q conv=%q", got.OneID, got.ConversationID)
	}
}

// =============================================================================
// dispatchTelegram 集成：真人发言建线索 / 机器人自身不建
// =============================================================================

func TestDispatchTelegram_MinesHumanGroupMessage(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}

	payload := []byte(`{
		"update_id": 2001,
		"message": {
			"message_id": 6001,
			"from": {"id": 7777, "first_name": "Carol", "username": "carol77", "is_bot": false},
			"chat": {"id": -1001234567890, "type": "supergroup", "title": "销售交流群"},
			"date": 1700000000,
			"text": "这个怎么买代理"
		}
	}`)
	_, extra, err := svc.dispatchTelegram("1", &ParsedPayload{EventID: "tg_mine_1", EventType: "message"}, payload)
	if err != nil {
		t.Fatalf("dispatchTelegram failed: %v", err)
	}
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "@carol77")
	if err != nil || got == nil {
		t.Fatalf("真人群发言应建线索: err=%v got=%v", err, got)
	}
	// 高意向群发言应标记为「新晋商机」（触发群内主动触达），且因未配置 username 而 Mentioned=false
	if extra == nil {
		t.Fatalf("dispatchTelegram 应返回 tgDispatchExtra")
	}
	if !extra.NewOpportunity {
		t.Fatalf("高意向群发言应标记 NewOpportunity=true，实际 false")
	}
	if extra.Mentioned {
		t.Fatalf("未配置 username 时 Mentioned 应为 false")
	}
}

func TestDispatchTelegram_DoesNotMineBotItself(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}

	payload := []byte(`{
		"update_id": 2002,
		"message": {
			"message_id": 6002,
			"from": {"id": 8888, "first_name": "MyBot", "username": "mybot", "is_bot": true},
			"chat": {"id": -1001234567890, "type": "supergroup", "title": "销售交流群"},
			"date": 1700000000,
			"text": "大家好，我是销售助手"
		}
	}`)
	if _, _, err := svc.dispatchTelegram("1", &ParsedPayload{EventID: "tg_mine_2", EventType: "message"}, payload); err != nil {
		t.Fatalf("dispatchTelegram failed: %v", err)
	}
	if got, _ := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "@mybot"); got != nil {
		t.Fatalf("机器人自身发言不应建线索: %+v", got)
	}
}

func TestDispatchTelegram_MinesPrivateHumanDM(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}

	payload := []byte(`{
		"update_id": 2003,
		"message": {
			"message_id": 6003,
			"from": {"id": 9999, "first_name": "Dave", "username": "dave99", "is_bot": false},
			"chat": {"id": 9999, "type": "private"},
			"date": 1700000000,
			"text": "请问怎么合作代理"
		}
	}`)
	if _, _, err := svc.dispatchTelegram("1", &ParsedPayload{EventID: "tg_mine_3", EventType: "message"}, payload); err != nil {
		t.Fatalf("dispatchTelegram failed: %v", err)
	}
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "@dave99")
	if err != nil || got == nil {
		t.Fatalf("私聊真人应建线索: err=%v got=%v", err, got)
	}
}

// =============================================================================
// isTelegramBotMentioned：群内 @机器人 提及识别（纯函数）
// =============================================================================

func TestIsTelegramBotMentioned(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		botUsername string
		want        bool
	}{
		{name: "空 username 不识别", text: "hello @mybot", botUsername: "", want: false},
		{name: "精确 @username", text: "请问 @mybot 怎么买", botUsername: "mybot", want: true},
		{name: "大小写不敏感", text: "hi @MyBot 在吗", botUsername: "mybot", want: true},
		{name: "无 @提及", text: "请问价格多少", botUsername: "mybot", want: false},
		{name: "提及别人", text: "找 @otherbot 吧", botUsername: "mybot", want: false},
		{name: "用户名带前缀 @ 不匹配", text: "hi @mybotX", botUsername: "mybot", want: false}, // 子串不算，必须精确 @username
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTelegramBotMentioned(c.text, c.botUsername); got != c.want {
				t.Fatalf("isTelegramBotMentioned(%q,%q)=%v 期望 %v", c.text, c.botUsername, got, c.want)
			}
		})
	}
}

// =============================================================================
// tgLeadOutreachAllowed：群内「发现线索主动触达」冷却
// =============================================================================

func TestTgLeadOutreachAllowed(t *testing.T) {
	svc := &WebhookService{tgOutreachLast: map[string]time.Time{}}

	key1 := "acc1:-100:u1"
	key2 := "acc1:-100:u2"

	// 首次均允许
	if !svc.tgLeadOutreachAllowed("acc1", "-100", "u1") {
		t.Fatalf("首次应允许触达")
	}
	// 同一发言者冷却期内拒绝
	if svc.tgLeadOutreachAllowed("acc1", "-100", "u1") {
		t.Fatalf("冷却期内不应重复触达同一发言者")
	}
	// 不同发言者不受影响
	if !svc.tgLeadOutreachAllowed("acc1", "-100", "u2") {
		t.Fatalf("不同发言者不受前者冷却影响")
	}
	// key1/key2 都已记录
	if _, ok := svc.tgOutreachLast[key1]; !ok {
		t.Fatalf("key1 未记录冷却")
	}
	if _, ok := svc.tgOutreachLast[key2]; !ok {
		t.Fatalf("key2 未记录冷却")
	}

	// 直接修改记录时间以模拟冷却到期，应再次允许
	svc.tgOutreachMu.Lock()
	svc.tgOutreachLast[key1] = time.Now().Add(-(tgLeadOutreachCooldown + time.Minute))
	svc.tgOutreachMu.Unlock()
	if !svc.tgLeadOutreachAllowed("acc1", "-100", "u1") {
		t.Fatalf("冷却到期后应再次允许触达")
	}
}

// countClues 统计指定账号的 TG 线索条数（验证去重）
func countClues(t *testing.T, db *gorm.DB, account string) int {
	var list []model.Clue
	if err := db.Where("type = ? AND account = ?", ClueTypeTelegram, account).Find(&list).Error; err != nil {
		t.Fatalf("count clues failed: %v", err)
	}
	return len(list)
}
