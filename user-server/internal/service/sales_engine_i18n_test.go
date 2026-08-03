package service

import (
	"context"
	"strings"
	"testing"

	i18npkg "marketing/internal/pkg/i18n"
)

// stubGlossary / stubCalibrator 用于验证 SalesEngine 语言链路接线（依赖倒置接口）。
type stubGlossary struct{ block string }

func (s *stubGlossary) Render(ctx context.Context, lang string) string { return s.block }

type stubCalibrator struct{ out string }

func (s *stubCalibrator) Calibrate(ctx context.Context, text, targetLang string) (string, error) {
	return s.out, nil
}

// TestSalesEngineResolveTargetLang 验证目标语种解析：配置优先 + 客户消息自动检测 + 拉丁保守回退。
func TestSalesEngineResolveTargetLang(t *testing.T) {
	e := &SalesEngine{}

	// 内部=zh，未配置 target（==内部），客户日语 → 自动检测 ja
	ctx := i18npkg.WithInternalLang(context.Background(), "zh")
	ctx = i18npkg.WithTargetLang(ctx, "zh")
	if got := e.resolveTargetLang(ctx, "こんにちは、商品について教えて"); got != "ja" {
		t.Fatalf("auto-detect want ja, got %q", got)
	}

	// 配置了 target=en（!=内部zh）→ 严格遵循配置，不被客户中文覆盖
	ctx2 := i18npkg.WithInternalLang(context.Background(), "zh")
	ctx2 = i18npkg.WithTargetLang(ctx2, "en")
	if got := e.resolveTargetLang(ctx2, "你好"); got != "en" {
		t.Fatalf("config priority want en, got %q", got)
	}

	// 纯拉丁（无目标配置）→ 回退内部 zh，避免中文坐席场景误判
	ctx3 := i18npkg.WithInternalLang(context.Background(), "zh")
	ctx3 = i18npkg.WithTargetLang(ctx3, "zh")
	if got := e.resolveTargetLang(ctx3, "hello there"); got != "zh" {
		t.Fatalf("latin fallback want zh, got %q", got)
	}
}

// TestSalesEnginePersonaWithLang 验证跨语言时追加语种指令 + 术语表块，同语种零开销。
func TestSalesEnginePersonaWithLang(t *testing.T) {
	e := &SalesEngine{glossary: &stubGlossary{block: "GLOSSARY"}}
	ctx := i18npkg.WithInternalLang(context.Background(), "zh")
	ctx = i18npkg.WithTargetLang(ctx, "ja")

	out := e.personaWithLang(ctx, "你是客服", "ja")
	if out == "你是客服" {
		t.Fatal("expected language block appended for cross-lingual")
	}
	if !strings.Contains(out, "LANGUAGE REQUIREMENT") || !strings.Contains(out, "GLOSSARY") {
		t.Fatalf("missing instruction/glossary: %q", out)
	}

	// 同语种 → 原样返回，不附加任何指令
	ctxZ := i18npkg.WithInternalLang(context.Background(), "zh")
	ctxZ = i18npkg.WithTargetLang(ctxZ, "zh")
	if out := e.personaWithLang(ctxZ, "你是客服", "zh"); out != "你是客服" {
		t.Fatalf("same-lang should return unchanged, got %q", out)
	}
}

// TestSalesEngineCalibrate 验证输出后置校准：注入校准器时生效，未注入时原样返回。
func TestSalesEngineCalibrate(t *testing.T) {
	e := &SalesEngine{calibrator: &stubCalibrator{out: "CALIBRATED"}}
	if got := e.calibrate(context.Background(), "raw", "en"); got != "CALIBRATED" {
		t.Fatalf("want CALIBRATED, got %q", got)
	}

	e2 := &SalesEngine{}
	if got := e2.calibrate(context.Background(), "raw", "en"); got != "raw" {
		t.Fatalf("no calibrator should return unchanged, got %q", got)
	}
}
