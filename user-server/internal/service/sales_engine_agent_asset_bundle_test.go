package service

import (
	"context"
	"errors"
	"testing"
)

type stubAssetBundleResolver struct {
	prompt string
	err    error
}

func (s *stubAssetBundleResolver) ResolveSystemPrompt(ctx context.Context, assetID string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.prompt, nil
}

// TestResolveAssetBundlePersona 验证「智能体→资产包」绑定解析逻辑：
// 绑定资产包时返回资产包 system prompt；未绑定 / 解析失败 / resolver 为空时安全降级为空（沿用原人设）。
func TestResolveAssetBundlePersona(t *testing.T) {
	cases := []struct {
		name     string
		agentCtx *AgentContext
		resolver AssetBundleResolver
		want     string
	}{
		{
			name:     "绑定资产包-返回资产包 system prompt",
			agentCtx: &AgentContext{AssetBundleID: "bundle_x", Persona: "原始人设"},
			resolver: &stubAssetBundleResolver{prompt: "资产包人设话术"},
			want:     "资产包人设话术",
		},
		{
			name:     "未绑定资产包-返回空(沿用原人设)",
			agentCtx: &AgentContext{AssetBundleID: "", Persona: "原始人设"},
			resolver: &stubAssetBundleResolver{prompt: "资产包人设话术"},
			want:     "",
		},
		{
			name:     "解析失败-返回空(降级沿用原人设)",
			agentCtx: &AgentContext{AssetBundleID: "bundle_x"},
			resolver: &stubAssetBundleResolver{err: errors.New("not found")},
			want:     "",
		},
		{
			name:     "resolver 为 nil-返回空",
			agentCtx: &AgentContext{AssetBundleID: "bundle_x"},
			resolver: nil,
			want:     "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveAssetBundlePersona(context.Background(), c.agentCtx, c.resolver)
			if got != c.want {
				t.Errorf("resolveAssetBundlePersona = %q, want %q", got, c.want)
			}
		})
	}
}

// TestSetAssetBundleResolver 验证运行期解析器可被注入（渠道→智能体→资产包 接线点）
func TestSetAssetBundleResolver(t *testing.T) {
	defer func() { assetBundleResolverForSalesEngine = nil }()
	r := &stubAssetBundleResolver{prompt: "p"}
	SetAssetBundleResolver(r)
	if assetBundleResolverForSalesEngine == nil {
		t.Fatal("SetAssetBundleResolver 未注入解析器")
	}
	if got := resolveAssetBundlePersona(context.Background(),
		&AgentContext{AssetBundleID: "b"}, assetBundleResolverForSalesEngine); got != "p" {
		t.Errorf("注入后解析失败，got=%q", got)
	}
}
