package translation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hivemtk-user/internal/model"
	i18npkg "hivemtk-user/internal/pkg/i18n"
)

// mockChannelReader 可配置返回结果或错误。
type mockChannelReader struct {
	ch  *model.ChatChannel
	err error
}

func (m *mockChannelReader) GetByChannelID(_ context.Context, _ string) (*model.ChatChannel, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ch, nil
}

// mockAgentReader 可配置返回结果或错误。
type mockAgentReader struct {
	ag  *model.AIAgent
	err error
}

func (m *mockAgentReader) GetByID(_ context.Context, _ uint) (*model.AIAgent, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.ag, nil
}

// errSentinel 测试用哨兵错误。
var errSentinel = errors.New("mock repo error")

// TestResolve_ChannelPriority 渠道优先级最高
// channel.target_language=en, agent.target_language=ja, agent.internal=zh
// 预期：internal=zh, target=en, src=channel
func TestResolve_ChannelPriority(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "en"}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "zh",
		TargetLanguage:   "ja",
	}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, err := r.Resolve(context.Background(), "ch-1", 1)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "agent", res.InternalSrc)
	assert.Equal(t, "en", res.TargetLang)
	assert.Equal(t, "channel", res.TargetSrc)
	assert.True(t, res.CrossLingual, "zh != en 应跨语言")
	assert.Equal(t, "ch-1", res.ChannelID)
	assert.Equal(t, uint(1), res.AgentID)
}

// TestResolve_AgentWhenChannelMissing 渠道未配置 → 走智能体
// channel.TargetLanguage="" → 退化到 agent.TargetLanguage=en
func TestResolve_AgentWhenChannelMissing(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: ""}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "zh",
		TargetLanguage:   "en",
	}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "en", res.TargetLang)
	assert.Equal(t, "agent", res.TargetSrc)
	assert.True(t, res.CrossLingual)
}

// TestResolve_AgentWhenChannelEmpty channelID 空串 → 走智能体
func TestResolve_AgentWhenChannelEmpty(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "en"}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "zh",
		TargetLanguage:   "ja",
	}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, _ := r.Resolve(context.Background(), "", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "ja", res.TargetLang)
	assert.Equal(t, "agent", res.TargetSrc)
	assert.True(t, res.CrossLingual)
}

// TestResolve_DegradeToInternal channel/agent 都未配置 target → 退化=internal
func TestResolve_DegradeToInternal(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: ""}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "ja",
		TargetLanguage:   "",
	}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "ja", res.InternalLang)
	assert.Equal(t, "ja", res.TargetLang, "未配置 target 时退化=internal")
	assert.Equal(t, "internal", res.TargetSrc)
	assert.False(t, res.CrossLingual, "同语种不应跨语言")
}

// TestResolve_AllMissing agentID=0 + channelID="" → 全部缺失兜底 zh
func TestResolve_AllMissing(t *testing.T) {
	r := NewLangConfigResolver(&mockChannelReader{}, &mockAgentReader{})

	res, err := r.Resolve(context.Background(), "", 0)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "default", res.InternalSrc)
	assert.Equal(t, "zh", res.TargetLang)
	assert.Equal(t, "internal", res.TargetSrc)
	assert.False(t, res.CrossLingual)
}

// TestResolve_CrossLingualFlag internal=zh, target=en → cross_lingual=true
func TestResolve_CrossLingualFlag(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "en"}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{InternalLanguage: "zh"}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "en", res.TargetLang)
	assert.True(t, res.CrossLingual)
}

// TestResolve_SameLang internal=zh, target=zh → cross_lingual=false
func TestResolve_SameLang(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "zh"}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{InternalLanguage: "zh"}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "zh", res.TargetLang)
	assert.False(t, res.CrossLingual)
}

// TestResolve_AgentRepoError agentRepo 报错 → 兜底 zh
func TestResolve_AgentRepoError(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "en"}}
	agRepo := &mockAgentReader{err: errSentinel}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, err := r.Resolve(context.Background(), "ch-1", 1)
	require.NoError(t, err, "Resolve 永不报错（保证返回有效语言）")
	assert.Equal(t, "zh", res.InternalLang, "agent 报错时 internal 兜底 zh")
	assert.Equal(t, "default", res.InternalSrc)
	assert.Equal(t, "en", res.TargetLang)
	assert.Equal(t, "channel", res.TargetSrc)
	assert.True(t, res.CrossLingual)
}

// TestResolve_ChannelRepoError channelRepo 报错 → 跳过渠道，退化到 agent
func TestResolve_ChannelRepoError(t *testing.T) {
	chRepo := &mockChannelReader{err: errSentinel}
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "zh",
		TargetLanguage:   "ja",
	}}
	r := NewLangConfigResolver(chRepo, agRepo)

	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "ja", res.TargetLang)
	assert.Equal(t, "agent", res.TargetSrc, "channel 报错应跳过，走 agent")
	assert.True(t, res.CrossLingual)
}

// TestResolve_NilRepos 不注入 repos 也不应 panic
func TestResolve_NilRepos(t *testing.T) {
	r := NewLangConfigResolver(nil, nil)
	res, err := r.Resolve(context.Background(), "ch-1", 1)
	require.NoError(t, err)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "zh", res.TargetLang)
	assert.Equal(t, "internal", res.TargetSrc)
}

// TestResolve_AgentReturnsNil agent 返回 nil → 走 default
func TestResolve_AgentReturnsNil(t *testing.T) {
	agRepo := &mockAgentReader{ag: nil}
	r := NewLangConfigResolver(nil, agRepo)
	res, _ := r.Resolve(context.Background(), "", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "default", res.InternalSrc)
}

// TestResolve_ChannelReturnsNil channel 返回 nil → 跳过渠道
func TestResolve_ChannelReturnsNil(t *testing.T) {
	chRepo := &mockChannelReader{ch: nil}
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "zh",
		TargetLanguage:   "en",
	}}
	r := NewLangConfigResolver(chRepo, agRepo)
	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "en", res.TargetLang)
	assert.Equal(t, "agent", res.TargetSrc)
}

// TestResolve_AgentIDZero_NoAgentLookup agentID=0 不应调用 agentRepo
func TestResolve_AgentIDZero_NoAgentLookup(t *testing.T) {
	agRepo := &mockAgentReader{ag: &model.AIAgent{
		InternalLanguage: "ja",
		TargetLanguage:   "ja",
	}}
	r := NewLangConfigResolver(nil, agRepo)
	res, _ := r.Resolve(context.Background(), "", 0)
	assert.Equal(t, "zh", res.InternalLang, "agentID=0 时不应查 agentRepo")
	assert.Equal(t, "default", res.InternalSrc)
}

// TestResolve_NormalizesLangCodes 原始配置含 region/大写应被归一化
func TestResolve_NormalizesLangCodes(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "EN-US"}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{InternalLanguage: "ZH-Hans"}}
	r := NewLangConfigResolver(chRepo, agRepo)
	res, _ := r.Resolve(context.Background(), "ch-1", 1)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "en", res.TargetLang)
}

func TestInjectToCtx(t *testing.T) {
	r := NewLangConfigResolver(nil, nil)
	result := &LangResolveResult{
		InternalLang: "zh",
		TargetLang:   "en",
		CrossLingual: true,
	}
	ctx := r.InjectToCtx(context.Background(), result)
	assert.Equal(t, "zh", i18npkg.GetInternalLang(ctx))
	assert.Equal(t, "en", i18npkg.GetTargetLang(ctx))
	assert.True(t, i18npkg.GetCrossLingual(ctx))
}

func TestInjectToCtx_NilResult_NoOp(t *testing.T) {
	r := NewLangConfigResolver(nil, nil)
	ctx := r.InjectToCtx(context.Background(), nil)
	assert.Equal(t, "zh", i18npkg.GetInternalLang(ctx))
	assert.Equal(t, "zh", i18npkg.GetTargetLang(ctx))
	assert.False(t, i18npkg.GetCrossLingual(ctx))
}

func TestResolveAndInject(t *testing.T) {
	chRepo := &mockChannelReader{ch: &model.ChatChannel{TargetLanguage: "en"}}
	agRepo := &mockAgentReader{ag: &model.AIAgent{InternalLanguage: "zh"}}
	r := NewLangConfigResolver(chRepo, agRepo)

	ctx, res := r.ResolveAndInject(context.Background(), "ch-1", 1)
	require.NotNil(t, res)
	assert.Equal(t, "zh", res.InternalLang)
	assert.Equal(t, "en", res.TargetLang)
	assert.Equal(t, "zh", i18npkg.GetInternalLang(ctx))
	assert.Equal(t, "en", i18npkg.GetTargetLang(ctx))
	assert.True(t, i18npkg.GetCrossLingual(ctx))
}
