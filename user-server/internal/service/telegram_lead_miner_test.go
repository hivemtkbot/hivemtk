package service

import (
	"strings"
	"testing"

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
	if got := parseTelegramLeadScore(desc); got != 82 {
		t.Fatalf("解析意向分失败，期望 82 实际 %d", got)
	}
	// 无标记时返回 0
	if got := parseTelegramLeadScore("普通描述无标记"); got != 0 {
		t.Fatalf("无标记应返回 0，实际 %d", got)
	}
	// 普通线索（非商机）标记
	desc2 := formatTelegramLeadDesc("群", "hi", 8, nil, false)
	if !strings.Contains(desc2, "群发言线索") {
		t.Fatalf("普通线索应标记为「群发言线索」: %s", desc2)
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

	// 首次：寒暄 意向分 8 → 新建
	svc.mineTelegramGroupLead("-1001", "群A", "123", "alice", "Alice", "大家好呀")
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
	if n := countClues(t, db, account); n != 1 {
		t.Fatalf("同一账号应仅 1 条线索，实际 %d", n)
	}

	// 再次：高意向（请问12+价格25+多少12+批发25+base8=82）→ 增量更新到 82，仍 1 条
	svc.mineTelegramGroupLead("-1001", "群A", "123", "alice", "Alice", "请问价格多少批发")
	got, _ = svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, account)
	if !strings.Contains(got.Desc, "[意向分:82]") {
		t.Fatalf("应升级到意向分82: %s", got.Desc)
	}
	if n := countClues(t, db, account); n != 1 {
		t.Fatalf("升级不应新增线索，实际 %d", n)
	}

	// 再次：低意向 8 → 不降级（保留 82）
	svc.mineTelegramGroupLead("-1001", "群A", "123", "alice", "Alice", "你好啊")
	got, _ = svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, account)
	if !strings.Contains(got.Desc, "[意向分:82]") {
		t.Fatalf("低意向不应覆盖高分: %s", got.Desc)
	}
	if n := countClues(t, db, account); n != 1 {
		t.Fatalf("去重后仍为 1 条，实际 %d", n)
	}
}

func TestMineTelegramGroupLead_NoiseSkipped(t *testing.T) {
	db := setupTelegramTestDB(t)
	svc := &WebhookService{db: db}
	account := "@bob"

	svc.mineTelegramGroupLead("-1001", "群A", "999", "bob", "Bob", "/start") // 命令
	svc.mineTelegramGroupLead("-1001", "群A", "999", "bob", "Bob", "😀😀😀")  // 纯表情
	svc.mineTelegramGroupLead("-1001", "群A", "999", "bob", "Bob", "   ")    // 空白

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
	// 无 username → 用 tg:<id> 作去重键
	svc.mineTelegramGroupLead("-1001", "群A", "555", "", "Unknown", "我想采购一批货")
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "tg:555")
	if err != nil || got == nil {
		t.Fatalf("无 username 应回退 tg:<id> 建线索: err=%v got=%v", err, got)
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
	if _, err := svc.dispatchTelegram("1", &ParsedPayload{EventID: "tg_mine_1", EventType: "message"}, payload); err != nil {
		t.Fatalf("dispatchTelegram failed: %v", err)
	}
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "@carol77")
	if err != nil || got == nil {
		t.Fatalf("真人群发言应建线索: err=%v got=%v", err, got)
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
	if _, err := svc.dispatchTelegram("1", &ParsedPayload{EventID: "tg_mine_2", EventType: "message"}, payload); err != nil {
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
	if _, err := svc.dispatchTelegram("1", &ParsedPayload{EventID: "tg_mine_3", EventType: "message"}, payload); err != nil {
		t.Fatalf("dispatchTelegram failed: %v", err)
	}
	got, err := svc.clueRepo.FindByTypeAndAccount(ClueTypeTelegram, "@dave99")
	if err != nil || got == nil {
		t.Fatalf("私聊真人应建线索: err=%v got=%v", err, got)
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
