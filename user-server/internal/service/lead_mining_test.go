package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// ---------------- 内存 Fake 实现（无 DB 依赖） ----------------

type fakeCustRepo struct {
	mu    sync.Mutex
	byID  map[string]*model.Customer
	byUID map[string]*model.Customer
	seq   int
}

func newFakeCustRepo() *fakeCustRepo {
	return &fakeCustRepo{byID: map[string]*model.Customer{}, byUID: map[string]*model.Customer{}}
}

func (f *fakeCustRepo) Create(_ context.Context, c *model.Customer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	c.ID = "cust-" + lmItoa(f.seq)
	f.byID[c.ID] = c
	if c.UnifiedID != "" {
		f.byUID[c.UnifiedID] = c
	}
	return nil
}
func (f *fakeCustRepo) GetByID(_ context.Context, id string) (*model.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id], nil
}
func (f *fakeCustRepo) GetByUnifiedID(_ context.Context, uid string) (*model.Customer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byUID[uid], nil
}
func (f *fakeCustRepo) Update(_ context.Context, c *model.Customer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[c.ID] = c
	if c.UnifiedID != "" {
		f.byUID[c.UnifiedID] = c
	}
	return nil
}
func (f *fakeCustRepo) GetByPhone(context.Context, string) (*model.Customer, error) { return nil, nil }
func (f *fakeCustRepo) GetByEmail(context.Context, string) (*model.Customer, error) { return nil, nil }
func (f *fakeCustRepo) GetByWechatOpenID(context.Context, string) (*model.Customer, error) {
	return nil, nil
}
func (f *fakeCustRepo) GetByDouyinOpenID(context.Context, string) (*model.Customer, error) {
	return nil, nil
}
func (f *fakeCustRepo) GetByXiaohongshuID(context.Context, string) (*model.Customer, error) {
	return nil, nil
}
func (f *fakeCustRepo) Delete(context.Context, string) error { return nil }
func (f *fakeCustRepo) List(context.Context, int, int, string) ([]*model.Customer, int64, error) {
	return nil, 0, nil
}
func (f *fakeCustRepo) FindByIdentity(context.Context, string, string, string, string, string) (*model.Customer, error) {
	return nil, nil
}
func (f *fakeCustRepo) FindByIdentityAll(context.Context, string, string, string, string, string) ([]*model.Customer, error) {
	return nil, nil
}
func (f *fakeCustRepo) ReassignSessionOneID(context.Context, string, string) error {
	return nil
}
func (f *fakeCustRepo) CountNotEmpty(context.Context, string) (int64, error) { return 0, nil }
func (f *fakeCustRepo) CountMultiIdentity(context.Context) (int64, error)    { return 0, nil }
func (f *fakeCustRepo) ListByIDs(context.Context, []string) (map[string]*model.Customer, error) {
	return nil, nil
}
func (f *fakeCustRepo) SearchByFilter(context.Context, repository.CustomerSearchFilter) ([]*model.Customer, int64, error) {
	return nil, 0, nil
}

type fakeClueRepo struct {
	mu         sync.Mutex
	byID       map[string]*model.Clue
	byTypeAcct map[string]*model.Clue
	seq        int
	createCnt  int
	updateCnt  int
}

func newFakeClueRepo() *fakeClueRepo {
	return &fakeClueRepo{byID: map[string]*model.Clue{}, byTypeAcct: map[string]*model.Clue{}}
}

func (f *fakeClueRepo) Create(_ context.Context, c *model.Clue) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := lmItoa(int(c.Type)) + ":" + c.Account
	if _, ok := f.byTypeAcct[key]; ok {
		return lmErrDuplicate
	}
	f.seq++
	c.ID = "clue-" + lmItoa(f.seq)
	f.byID[c.ID] = c
	f.byTypeAcct[key] = c
	f.createCnt++
	return nil
}
func (f *fakeClueRepo) GetByID(_ context.Context, id uint) (*model.Clue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[lmItoa(int(id))], nil
}
func (f *fakeClueRepo) FindByTypeAndAccount(_ context.Context, t int64, acct string) (*model.Clue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byTypeAcct[lmItoa(int(t))+":"+acct], nil
}
func (f *fakeClueRepo) UpdateByID(_ context.Context, id string, _ map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[id]; !ok {
		return lmErrNotFound
	}
	f.updateCnt++
	return nil
}
func (f *fakeClueRepo) ExistsByTypeAndAccount(_ context.Context, t int64, acct string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.byTypeAcct[lmItoa(int(t))+":"+acct]
	return ok, nil
}
func (f *fakeClueRepo) GetClueList(context.Context, int, int) ([]*model.Clue, int64, error) {
	return nil, 0, nil
}
func (f *fakeClueRepo) ListByAccount(context.Context, string, int, int) ([]*model.Clue, int64, error) {
	return nil, 0, nil
}
func (f *fakeClueRepo) Delete(context.Context, string) error { return nil }
func (f *fakeClueRepo) GetRecentClueList(context.Context) ([]*model.Clue, error) {
	return nil, nil
}
func (f *fakeClueRepo) GetClueStatistics(context.Context) ([]map[string]any, error) {
	return nil, nil
}
func (f *fakeClueRepo) GetClueAllList(context.Context, int64) ([]*model.Clue, int64, error) {
	return nil, 0, nil
}
func (f *fakeClueRepo) GetDistinctTypes(context.Context) ([]int64, error) { return nil, nil }
func (f *fakeClueRepo) ListByAccounts(context.Context, []string) ([]*model.Clue, error) {
	return nil, nil
}
func (f *fakeClueRepo) BatchUpdateInTx(context.Context, []string, map[string]any) (int, error) {
	return 0, nil
}

type fakeCfgRepo struct {
	cfg *model.LeadMiningConfig
}

func (f *fakeCfgRepo) GetSingleton(context.Context) (*model.LeadMiningConfig, error) {
	if f.cfg == nil {
		return &model.LeadMiningConfig{ID: 1, MinIntentScore: 50}, nil
	}
	return f.cfg, nil
}
func (f *fakeCfgRepo) Save(context.Context, *model.LeadMiningConfig) error { return nil }

type fakeJudge struct {
	calls int
	resp  *LeadJudgement
}

func (f *fakeJudge) Judge(context.Context, *model.LeadMiningConfig, []llm.ChatMessage) (*LeadJudgement, error) {
	f.calls++
	return f.resp, nil
}

// ---------------- 辅助 ----------------

func lmItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var (
	lmErrDuplicate = &lmRepoError{"重复数据"}
	lmErrNotFound  = &lmRepoError{"not found"}
)

type lmRepoError struct{ msg string }

func (e *lmRepoError) Error() string { return e.msg }

func cannedHistory() func(context.Context, *model.MessageHub) []llm.ChatMessage {
	return func(_ context.Context, _ *model.MessageHub) []llm.ChatMessage {
		return []llm.ChatMessage{
			{Role: "user", Content: "你好，在吗？"},
			{Role: "user", Content: "我想了解一下你们的产品和报价"},
		}
	}
}

func newTestService(cfg *model.LeadMiningConfig, j *fakeJudge) *Service {
	return &Service{
		judge:          j,
		historyFetcher: cannedHistory(),
		custRepo:       newFakeCustRepo(),
		clueRepo:       newFakeClueRepo(),
		cfgRepo:        &fakeCfgRepo{cfg: cfg},
		lastJudge:      map[string]time.Time{},
	}
}

func sampleHub(platform, senderID, content string) *model.MessageHub {
	return &model.MessageHub{
		ID:             1,
		Platform:       platform,
		SenderID:       senderID,
		SenderName:     "张三",
		Direction:      "inbound",
		Content:        content,
		ConversationID: "conv-" + senderID,
	}
}

// ---------------- 纯函数测试 ----------------

func TestMergeTags(t *testing.T) {
	got := mergeTags([]string{"A", "B"}, []string{"b", "C", ""})
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("mergeTags=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mergeTags[%d]=%s want %s", i, got[i], want[i])
		}
	}
}

func TestChannelEnabled(t *testing.T) {
	cfg := &model.LeadMiningConfig{} // 空=全部
	if !channelEnabled(cfg, "douyin") {
		t.Fatal("空渠道配置应放行全部渠道")
	}
	cfg2 := &model.LeadMiningConfig{Channels: []string{"telegram", "douyin"}}
	if !channelEnabled(cfg2, "telegram") {
		t.Fatal("telegram 应启用")
	}
	if channelEnabled(cfg2, "wechat") {
		t.Fatal("wechat 未启用应拦截")
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	cfg := &model.LeadMiningConfig{
		Keywords:    []string{"购买", "代理"},
		Tags:        []string{"高意向"},
		Requirement: "客户明确表达购买意向",
	}
	p := buildSystemPrompt(cfg)
	for _, need := range []string{"购买", "代理", "高意向", "客户明确表达购买意向"} {
		if !strings.Contains(p, need) {
			t.Fatalf("系统提示词缺少 %q", need)
		}
	}
}

// ---------------- 端到端逻辑测试 ----------------

func TestProcess_LeadDetected(t *testing.T) {
	cfg := &model.LeadMiningConfig{
		Enabled:        true,
		Keywords:       []string{"购买"},
		Tags:           []string{"高意向"},
		MinIntentScore: 50,
	}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 80, MatchedTags: []string{"AI兴趣"}}}
	s := newTestService(cfg, j)
	fcr := s.custRepo.(*fakeCustRepo)
	fkr := s.clueRepo.(*fakeClueRepo)

	s.process(context.Background(), sampleHub("telegram", "u1", "我想购买"))

	if j.calls != 1 {
		t.Fatalf("期望判定 1 次，实际 %d", j.calls)
	}
	// 客户被打标签
	c := fcr.byUID["lm:telegram:u1"]
	if c == nil {
		t.Fatal("未创建/解析客户")
	}
	var tags []string
	_ = json.Unmarshal([]byte(c.Tags), &tags)
	if !lmContains(tags, "高意向") || !lmContains(tags, "AI兴趣") {
		t.Fatalf("客户标签不正确: %v", tags)
	}
	// 线索写入库存
	clue := fkr.byTypeAcct[lmItoa(int(ClueTypeLeadMining))+":telegram:u1"]
	if clue == nil {
		t.Fatal("未写入线索库存")
	}
	if clue.IntentScore != 80 || clue.SourceID != "lead_mining" {
		t.Fatalf("线索字段异常: %+v", clue)
	}
	if fkr.createCnt != 1 {
		t.Fatalf("期望 Create 1 次，实际 %d", fkr.createCnt)
	}
}

func TestProcess_BelowThreshold(t *testing.T) {
	cfg := &model.LeadMiningConfig{Enabled: true, MinIntentScore: 90}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 50}}
	s := newTestService(cfg, j)
	fkr := s.clueRepo.(*fakeClueRepo)

	s.process(context.Background(), sampleHub("telegram", "u2", "随便聊聊"))

	if fkr.createCnt != 0 {
		t.Fatalf("低于阈值不应写线索，实际 Create %d", fkr.createCnt)
	}
}

func TestProcess_NotLead(t *testing.T) {
	cfg := &model.LeadMiningConfig{Enabled: true, MinIntentScore: 50}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: false, IntentScore: 10}}
	s := newTestService(cfg, j)
	fkr := s.clueRepo.(*fakeClueRepo)

	s.process(context.Background(), sampleHub("telegram", "u3", "你好"))

	if fkr.createCnt != 0 {
		t.Fatalf("非线索不应写线索，实际 Create %d", fkr.createCnt)
	}
}

func TestProcess_ChannelDisabled(t *testing.T) {
	cfg := &model.LeadMiningConfig{Enabled: true, Channels: []string{"telegram"}, MinIntentScore: 50}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 90}}
	s := newTestService(cfg, j)

	s.process(context.Background(), sampleHub("douyin", "u4", "我想购买"))

	if j.calls != 0 {
		t.Fatalf("渠道未启用不应触发判定，实际判定 %d 次", j.calls)
	}
}

func TestProcess_Disabled(t *testing.T) {
	cfg := &model.LeadMiningConfig{Enabled: false, MinIntentScore: 50}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 90}}
	s := newTestService(cfg, j)

	s.process(context.Background(), sampleHub("telegram", "u5", "我想购买"))

	if j.calls != 0 {
		t.Fatalf("未启用不应触发判定，实际 %d", j.calls)
	}
}

func TestProcess_Debounce(t *testing.T) {
	cfg := &model.LeadMiningConfig{Enabled: true, MinIntentScore: 50}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 80}}
	s := newTestService(cfg, j)
	fkr := s.clueRepo.(*fakeClueRepo)

	// 同客户连续两条消息（60s 窗口内）→ 只判定/写入一次
	s.process(context.Background(), sampleHub("telegram", "u6", "在吗"))
	s.process(context.Background(), sampleHub("telegram", "u6", "我想购买"))

	if j.calls != 1 {
		t.Fatalf("去抖后应只判定 1 次，实际 %d", j.calls)
	}
	if fkr.createCnt != 1 {
		t.Fatalf("去抖后应只写 1 条线索，实际 %d", fkr.createCnt)
	}
}

func TestProcess_DedupAcrossTime(t *testing.T) {
	cfg := &model.LeadMiningConfig{Enabled: true, MinIntentScore: 50}
	j := &fakeJudge{resp: &LeadJudgement{IsLead: true, IntentScore: 80}}
	s := newTestService(cfg, j)
	fkr := s.clueRepo.(*fakeClueRepo)

	account := "telegram:u7"
	s.process(context.Background(), sampleHub("telegram", "u7", "第一次咨询"))
	// 模拟时间窗口过去：清除去抖标记，再次触发判定
	delete(s.lastJudge, account)
	s.process(context.Background(), sampleHub("telegram", "u7", "第二次咨询"))

	if j.calls != 2 {
		t.Fatalf("跨窗口应判定 2 次，实际 %d", j.calls)
	}
	if fkr.createCnt != 1 {
		t.Fatalf("同账号线索应只 Create 1 次（其余 Update），实际 %d", fkr.createCnt)
	}
	if fkr.updateCnt != 1 {
		t.Fatalf("第二次应走 UpdateByID，实际 %d", fkr.updateCnt)
	}
}

func lmContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ---------------- 抖音私聊触达契约测试 ----------------

// TestDouyinLeadOutreachUsesOperatorAccount 守护「发现线索立即私聊」的路由键契约：
// 私信 outbox 按运营账号(AccountID)拉取，hook 必须收到 hub.AccountID，而非客户键(account=platform:sender)。
// 若回归客户键，桥接扩展按运营账号轮询时永远取不到 → 私聊静默丢失。
func TestDouyinLeadOutreachUsesOperatorAccount(t *testing.T) {
	if err := os.Setenv("DOUYIN_LEAD_DM_ENABLED", "1"); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv("DOUYIN_LEAD_DM_ENABLED")

	svc := &Service{
		clueRepo:  newFakeClueRepo(),
		custRepo:  newFakeCustRepo(),
		cfgRepo:   &fakeCfgRepo{cfg: &model.LeadMiningConfig{MinIntentScore: 10}},
		lastJudge: map[string]time.Time{},
	}

	got := make(chan [3]string, 1)
	orig := DouyinLeadOutreachHook
	defer func() { DouyinLeadOutreachHook = orig }()
	DouyinLeadOutreachHook = func(_ context.Context, accountID, memberOpenID, groupConvID, _, _ string) {
		got <- [3]string{accountID, memberOpenID, groupConvID}
	}

	hub := &model.MessageHub{
		Platform:       "douyin",
		AccountID:      "op-acct-9", // 运营账号（桥接轮询账号）
		SenderID:       "member-1",  // 客户（私聊目标）
		SenderName:     "张三",
		ConversationID: "group-g1",
		MsgID:          "m1",
		Content:        "怎么买",
		Direction:      "inbound",
		IsGroup:        true,
	}
	svc.persistLead(context.Background(), &model.LeadMiningConfig{MinIntentScore: 10}, hub, "douyin:member-1", &LeadJudgement{IsLead: true, IntentScore: 80, Reason: "高意向"})

	select {
	case args := <-got:
		if args[0] != "op-acct-9" {
			t.Fatalf("抖音发现线索私聊必须传运营账号 hub.AccountID, got=%q", args[0])
		}
		if args[1] != "member-1" {
			t.Fatalf("私聊目标应为 hub.SenderID, got=%q", args[1])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("抖音线索私聊 hook 未被触发（可能阈值/开关未命中）")
	}
}

// TestFirstSeenDouyinGroupMember 守护入群近似触达的去重缓存语义：
// 同一(运营账号,群,成员) 24h 内只触达一次；跨运营账号/跨成员独立计数。
func TestFirstSeenDouyinGroupMember(t *testing.T) {
	douyinJoinSeen = map[string]time.Time{} // 隔离全局去重缓存
	if !firstSeenDouyinGroupMember("op1", "g1", "m1") {
		t.Fatal("首次发言应返回 true")
	}
	if firstSeenDouyinGroupMember("op1", "g1", "m1") {
		t.Fatal("24h 内重复发言同一成员应返回 false")
	}
	if !firstSeenDouyinGroupMember("op1", "g1", "m2") {
		t.Fatal("不同成员应返回 true")
	}
	if !firstSeenDouyinGroupMember("op2", "g1", "m1") {
		t.Fatal("不同运营账号应返回 true")
	}
}
