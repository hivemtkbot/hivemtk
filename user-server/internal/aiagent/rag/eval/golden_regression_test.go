package eval

import (
	"testing"

	"hivemtk-user/internal/pkg/testutil"
)

// D18: golden_set.json 可加载、schema 完整、指标不塌方——作为变更回归门的数据完整性检查。
// 完整回归（检索行为变化 → 分数变化）由 rag_eval_cron 每日真实检索链路承担；
// 本测试锁"评测集本身不被污染/删减"。
func TestD18_GoldenSetIntegrity(t *testing.T) {
	db := testutil.NewTestDB(t)
	_ = db
	cases, err := LoadGoldenSet("golden_set.json")
	if err != nil {
		t.Fatalf("golden_set.json 加载失败: %v", err)
	}
	if len(cases) < 3 {
		t.Fatalf("golden set 不应少于 3 条（防误删）, got %d", len(cases))
	}
	for i, c := range cases {
		if c.Question == "" || c.Answer == "" || c.GroundTruth == "" || len(c.Contexts) == 0 {
			t.Errorf("case[%d] schema 不完整: q/a/gt/contexts 必填", i)
		}
	}

	report := RunEval(cases)
	if report.AvgFaithfulness < 0.5 {
		t.Errorf("Faithfulness 塌方: %.3f < 0.5", report.AvgFaithfulness)
	}
	if report.AvgContextRecall < 0.5 {
		t.Errorf("ContextRecall 塌方: %.3f < 0.5", report.AvgContextRecall)
	}
	if report.AvgAnswerRelevance < 0.5 {
		t.Errorf("AnswerRelevance 塌方: %.3f < 0.5", report.AvgAnswerRelevance)
	}
}
