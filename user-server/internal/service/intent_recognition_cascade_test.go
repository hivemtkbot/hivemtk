package service

import (
	"context"
	"math"
	"strings"
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

func cosVec(sim float64, dim int) []float32 {
	q := make([]float32, dim)
	q[0] = float32(sim)
	if dim > 1 {
		q[1] = float32(math.Sqrt(1 - sim*sim))
	}
	return q
}

func unitE(dim int) []float32 {
	v := make([]float32, dim)
	v[0] = 1
	return v
}

var priceExamples = []string{"这个多少钱？", "你们价格怎么样？", "能不能便宜点？"}
var objectionPriceExamples = []string{"这个东西太贵了", "价格有点高啊", "比别家贵好多"}

func newCascadeFixture(t *testing.T, objectionCos float64) (*IntentRecognizer, *stubEmbedder) {
	t.Helper()
	rec, _ := newIntentRecognizerNoDB(t)
	dim := 4
	s := math.Sqrt(1 - objectionCos*objectionCos)
	objection := []float32{float32(objectionCos), float32(s), 0, 0}
	stub := &stubEmbedder{dim: dim, vecs: map[string][]float32{}}
	setAllVecs(stub.vecs, priceExamples, unitE(dim))
	setAllVecs(stub.vecs, objectionPriceExamples, objection)
	rec.SetEmbeddingService(stub)
	<-awaitAnchors(rec)
	return rec, stub
}

func setAllVecs(vecs map[string][]float32, texts []string, v []float32) {
	for _, t := range texts {
		vecs[t] = v
	}
}

func newTestExampleRepo(db *gorm.DB) *repository.IntentExampleRepository {
	r := repository.NewIntentExampleRepository()
	r.SetDB(context.Background(), db)
	return r
}

func TestCascade_RuleLayerWins(t *testing.T) {
	rec, _ := newCascadeFixture(t, 0.94)
	r, err := rec.Recognize(context.Background(), "s", "", "这个多少钱？")
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "rule" {
		t.Fatalf("规则命中应短路返回 rule 层，实际 %s", r.Method)
	}
	if r.IntentType != IntentPriceInquiry {
		t.Fatalf("intent 期望 price_inquiry，实际 %s", r.IntentType)
	}
}

func TestCascade_EmbeddingLayerSecond(t *testing.T) {
	rec, stub := newCascadeFixture(t, 0.94)

	stub.vecs["随便问问价位"] = unitE(4)
	r, err := rec.Recognize(context.Background(), "s", "", "随便问问价位")
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "embedding" {
		t.Fatalf("期望 embedding 层采信，实际 %s (%s)", r.Method, r.IntentType)
	}
	if r.IntentType != IntentPriceInquiry {
		t.Fatalf("intent 期望 price_inquiry，实际 %s", r.IntentType)
	}
}

func TestCascade_LLMFallbackWhenEmbeddingUnavailable(t *testing.T) {

	rec, _ := newIntentRecognizerNoDB(t)
	stub := &stubEmbedder{dim: 4, failN: 1000}
	rec.SetEmbeddingService(stub)
	<-awaitAnchors(rec)
	if r := rec.recognizeByEmbedding(context.Background(), "任意文本"); r != nil {
		t.Fatalf("熔断后 embedding 层应返回 nil 下沉，实际 %+v", r)
	}
	res, err := rec.Recognize(context.Background(), "s", "", "asdfqwer zzzy")
	if err != nil {
		t.Fatal(err)
	}
	if res.IntentType != IntentUnknown || res.Method != "rule" {
		t.Fatalf("全层未命中应兜底 unknown，实际 %+v", res)
	}
}

func TestEmbeddingThreshold_JustBelow075NotTrusted(t *testing.T) {
	rec, stub := newCascadeFixture(t, 0.0)
	stub.vecs["边界查询"] = cosVec(intentEmbeddingTop1-0.0001, 4)
	if r := rec.recognizeByEmbedding(context.Background(), "边界查询"); r != nil {
		t.Fatalf("top1<%.2f 应返回 nil 下沉 LLM，实际强选 %s", intentEmbeddingTop1, r.IntentType)
	}
}

func TestEmbeddingThreshold_JustAbove075Trusted(t *testing.T) {
	rec, stub := newCascadeFixture(t, 0.0)
	stub.vecs["边界查询"] = cosVec(intentEmbeddingTop1+0.0001, 4)
	r := rec.recognizeByEmbedding(context.Background(), "边界查询")
	if r == nil {
		t.Fatalf("top1≥%.2f 应采信直接分类", intentEmbeddingTop1)
	}
	if r.IntentType != IntentPriceInquiry || r.Confidence < intentEmbeddingTop1 {
		t.Fatalf("采信结果异常: %+v", r)
	}
}

func TestClarify_GapBelow005BothAboveThreshold(t *testing.T) {

	rec, _ := newCascadeFixture(t, 0.99)
	r := rec.recognizeByEmbedding(context.Background(), "这个多少钱？")
	if r == nil || r.IntentType != IntentClarify {
		t.Fatalf("gap<0.05 且均过阈应返回 clarify，实际 %+v", r)
	}
	if r.Entities["top1_intent"] != IntentPriceInquiry || r.Entities["top2_intent"] != IntentObjectionPrice {
		t.Fatalf("clarify 候选错误: %v", r.Entities)
	}
	reply := BuildClarifyReply(r)
	if reply == "" {
		t.Fatalf("澄清话术生成失败")
	}
}

func TestNoClarify_GapAtOrAbove005(t *testing.T) {

	rec, _ := newCascadeFixture(t, 0.90)
	r := rec.recognizeByEmbedding(context.Background(), "这个多少钱？")
	if r == nil || r.IntentType == IntentClarify {
		t.Fatalf("gap≥0.05 应直接分类不发澄清，实际 %+v", r)
	}
	if r.IntentType != IntentPriceInquiry {
		t.Fatalf("应采信 top1 price_inquiry，实际 %s", r.IntentType)
	}
}

func TestNoClarify_Top2BelowThresholdEvenIfGapSmall(t *testing.T) {

	gamma := math.Acos(0.76)
	phi := gamma + math.Acos(0.72)
	rec, _ := newCascadeFixture(t, math.Cos(phi))
	r := rec.recognizeByEmbedding(context.Background(), "这个多少钱？")
	if r == nil || r.IntentType == IntentClarify {
		t.Fatalf("top2=%.2f 未过阈不应 clarify，实际 %+v", 0.72, r)
	}
	if r.IntentType != IntentPriceInquiry {
		t.Fatalf("应采信 top1 price_inquiry，实际 %s", r.IntentType)
	}
}

func TestBuildClarifyReply_Fallbacks(t *testing.T) {
	generic := BuildClarifyReply(&dto.RecognizeResult{
		IntentType: IntentClarify,
		Entities:   map[string]any{},
	})
	if generic == "" {
		t.Fatalf("候选缺失时应退化为通用澄清话术")
	}
	if BuildClarifyReply(nil) != "" {
		t.Fatalf("nil 结果应返回空串")
	}
	notClarify := &dto.RecognizeResult{IntentType: IntentUnknown, Entities: map[string]any{
		"top1_intent": IntentPriceInquiry, "top2_intent": IntentObjectionPrice,
	}}
	if BuildClarifyReply(notClarify) != "" {
		t.Fatalf("非 clarify 结果应返回空串")
	}
	named := BuildClarifyReply(&dto.RecognizeResult{
		IntentType: IntentClarify,
		Entities: map[string]any{
			"top1_intent": IntentPriceInquiry,
			"top2_intent": IntentObjectionPrice,
		},
	})
	for _, want := range []string{"价格咨询", "价格异议"} {
		if !strings.Contains(named, want) {
			t.Fatalf("澄清话术应含候选意图名 %q: %q", want, named)
		}
	}
}

func TestUnknownContract_ConstantsAndLowConfDowngrade(t *testing.T) {
	if IntentUnknown != "unknown" {
		t.Fatalf("unknown 常量漂移: %s", IntentUnknown)
	}
	if IntentClarify != "clarify" {
		t.Fatalf("clarify 常量漂移: %s", IntentClarify)
	}

	typ, conf := IntentPurchase, 0.69
	if typ != IntentUnknown && conf < 0.7 {
		typ = IntentUnknown
	}
	if typ != IntentUnknown {
		t.Fatalf("conf<0.7 必须降级 unknown")
	}
}

func TestRefineMinorByKeywords(t *testing.T) {
	cases := []struct {
		major, text, want string
	}{
		{IntentMajorObjection, "你们靠谱吗不会是骗子吧", IntentMinorObjectionTrustIssue},
		{IntentMajorObjection, "太贵了不值", IntentMinorObjectionPriceHigh},
		{IntentMajorAfterSale, "我要退货退款", IntentMinorAfterSaleRefund},
		{IntentMajorPriceInquiry, "请给我报价", IntentMinorPriceBudgetCheck},
		{"unknown", "任意文本", ""},
	}
	for _, c := range cases {
		if got := refineMinorByKeywords(c.major, c.text); got != c.want {
			t.Errorf("refineMinor(%s,%q)=%s want %s", c.major, c.text, got, c.want)
		}
	}
}

func TestEnsureIntentExamplesIndexed_Idempotent(t *testing.T) {
	db := testutil.NewTestDB(t, &model.IntentExample{})
	ctx := context.Background()

	rec, _ := newIntentRecognizerNoDB(t)
	rec.exampleRepo = newTestExampleRepo(db)

	rec.embedSvc = &stubEmbedder{dim: 1024, vecs: map[string][]float32{
		"这个多少钱？":  unitVec(1024, []int{0, 1}),
		"这个东西太贵了": unitVec(1024, []int{2, 3}),
	}}

	imported, err := rec.EnsureIntentExamplesIndexed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if imported <= 0 {
		t.Fatalf("首次导入应 >0 条，实际 %d", imported)
	}
	var count int64
	if err := db.Model(&model.IntentExample{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(imported) {
		t.Fatalf("表行数 %d 与导入数 %d 不一致", count, imported)
	}

	imported2, err := rec.EnsureIntentExamplesIndexed(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if imported2 != 0 {
		t.Fatalf("二次导入应为 0（幂等），实际 %d", imported2)
	}
}

// TestLoadAnchorsFromDB pgvector 持久化锚点加载路径（I-1 核心价值：重启免重算）
func TestLoadAnchorsFromDB(t *testing.T) {
	db := testutil.NewTestDB(t, &model.IntentExample{})
	ctx := context.Background()

	rec, _ := newIntentRecognizerNoDB(t)
	rec.exampleRepo = newTestExampleRepo(db)
	rec.embedSvc = &stubEmbedder{dim: 1024, vecs: map[string][]float32{
		"这个多少钱？": unitVec(1024, []int{0, 1}),
	}}
	if _, err := rec.EnsureIntentExamplesIndexed(ctx); err != nil {
		t.Fatal(err)
	}

	rec.anchorMu.Lock()
	rec.anchorVecs = nil
	rec.anchorMu.Unlock()
	if !rec.loadAnchorsFromDB(ctx) {
		t.Fatalf("从 intent_examples 表加载锚点失败")
	}
	rec.anchorMu.RLock()
	n := len(rec.anchorVecs[IntentPriceInquiry])
	rec.anchorMu.RUnlock()
	if n == 0 {
		t.Fatalf("加载后 price_inquiry 锚点不应为空")
	}

	emptyDB := testutil.NewTestDB(t, &model.IntentExample{})
	rec2, _ := newIntentRecognizerNoDB(t)
	rec2.exampleRepo = newTestExampleRepo(emptyDB)
	rec2.embedSvc = &stubEmbedder{dim: 1024}
	if rec2.loadAnchorsFromDB(ctx) {
		t.Fatalf("空表应返回 false 回退实时计算")
	}
}
