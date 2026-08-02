package humanize

// tfidf_phrase_test.go TF-IDF 短语提取器单元测试
//
// 覆盖：
//  1. 基本提取流程（多文档）
//  2. TF-IDF 计算
//  3. 短语分类（action/empathy/professional/persuasion/general）
//  4. 停用词过滤
//  5. 标点过滤
//  6. 空输入与边界用例
//  7. top-N 截断
//  8. 滑窗大小（2-4 字）

import (
	"testing"
)

// ============================================================================
// 基本提取测试
// ============================================================================

// TestTFIDF_Extract_Basic 基本提取
func TestTFIDF_Extract_Basic(t *testing.T) {
	messages := []ChampionMessage{
		{Content: "这款产品的成分是烟酰胺，保湿效果好。"},
		{Content: "现在下单立享优惠，包邮活动。"},
		{Content: "理解您的心情，抱歉给您带来困扰。"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 10)
	if len(phrases) == 0 {
		t.Fatal("应提取出短语")
	}
	// 验证 TF-IDF 降序
	for i := 1; i < len(phrases); i++ {
		if phrases[i-1].TFIDFScore < phrases[i].TFIDFScore {
			t.Errorf("短语应按 TF-IDF 降序: phrases[%d]=%v < phrases[%d]=%v",
				i-1, phrases[i-1].TFIDFScore, i, phrases[i].TFIDFScore)
		}
	}
	// 验证 rank 从 1 开始
	for i, p := range phrases {
		if p.Rank != i+1 {
			t.Errorf("phrases[%d].Rank=%d want %d", i, p.Rank, i+1)
		}
	}
}

// TestTFIDF_Extract_EmptyMessages 空消息
func TestTFIDF_Extract_EmptyMessages(t *testing.T) {
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(nil, 10)
	if phrases != nil {
		t.Errorf("空消息应返回 nil, got %v", phrases)
	}
	phrases = extractor.Extract([]ChampionMessage{}, 10)
	if phrases != nil {
		t.Errorf("空消息应返回 nil, got %v", phrases)
	}
}

// TestTFIDF_Extract_TopNZero topN=0
func TestTFIDF_Extract_TopNZero(t *testing.T) {
	messages := []ChampionMessage{{Content: "测试内容"}}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 0)
	if phrases != nil {
		t.Errorf("topN=0 应返回 nil, got %v", phrases)
	}
}

// TestTFIDF_Extract_TopNTruncation top-N 截断
func TestTFIDF_Extract_TopNTruncation(t *testing.T) {
	messages := []ChampionMessage{
		{Content: "成分烟酰胺保湿肤质"},
		{Content: "续航性能处理器内存屏幕"},
		{Content: "面料版型尺码剪裁"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 5)
	if len(phrases) > 5 {
		t.Errorf("topN=5 应最多返回 5 个, got %d", len(phrases))
	}
}

// ============================================================================
// TF-IDF 计算测试
// ============================================================================

// TestTFIDF_TFIDFScoreCalculation TF-IDF 计算正确性
func TestTFIDF_TFIDFScoreCalculation(t *testing.T) {
	// 3 个文档，"成分" 出现在 1 个文档中
	// TF=1, DF=1, N=3, IDF=log(3/1)≈1.0986
	// TF-IDF = 1 * 1.0986 ≈ 1.0986
	messages := []ChampionMessage{
		{Content: "成分是关键"},
		{Content: "测试文档二"},
		{Content: "测试文档三"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 50)
	// 找出 "成分" 这个短语
	var found *TFIDFPhrase
	for i := range phrases {
		if phrases[i].Phrase == "成分" {
			found = &phrases[i]
			break
		}
	}
	if found == nil {
		t.Fatal("未提取出 '成分' 短语")
	}
	if found.TF != 1 {
		t.Errorf("'成分' TF=%d want 1", found.TF)
	}
	if found.DF != 1 {
		t.Errorf("'成分' DF=%d want 1", found.DF)
	}
	// TF-IDF ≈ 1.0986
	if !approxEqualTol(found.TFIDFScore, 1.0986, 1e-3) {
		t.Errorf("'成分' TF-IDF=%v want ≈ 1.0986", found.TFIDFScore)
	}
}

// TestTFIDF_DocumentFrequency DF 计算
func TestTFIDF_DocumentFrequency(t *testing.T) {
	// "测试" 出现在 3 个文档中，总文档数 4 → IDF = log(4/3) ≈ 0.288
	messages := []ChampionMessage{
		{Content: "测试一"},
		{Content: "测试二"},
		{Content: "测试三"},
		{Content: "其他内容"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 50)
	var found *TFIDFPhrase
	for i := range phrases {
		if phrases[i].Phrase == "测试" {
			found = &phrases[i]
			break
		}
	}
	if found == nil {
		t.Fatal("未提取出 '测试' 短语")
	}
	if found.DF != 3 {
		t.Errorf("'测试' DF=%d want 3", found.DF)
	}
	// IDF = log(4/3) ≈ 0.288，TF-IDF > 0 应保留
	if found.TFIDFScore <= 0 {
		t.Errorf("'测试' TF-IDF=%v 应 > 0", found.TFIDFScore)
	}
	t.Logf("'测试' TF=%d DF=%d TF-IDF=%v", found.TF, found.DF, found.TFIDFScore)
}

// TestTFIDF_DocumentFrequency_AllDocuments 出现在所有文档中的短语应被过滤（IDF=0）
func TestTFIDF_DocumentFrequency_AllDocuments(t *testing.T) {
	messages := []ChampionMessage{
		{Content: "测试一"},
		{Content: "测试二"},
		{Content: "测试三"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 50)
	for _, p := range phrases {
		if p.Phrase == "测试" {
			t.Errorf("'测试' 出现在所有 3 个文档中 IDF=0，应被过滤，但被保留: TF-IDF=%v", p.TFIDFScore)
		}
	}
}

// ============================================================================
// 短语分类测试
// ============================================================================

// TestTFIDF_ClassifyPhrase 短语分类
func TestTFIDF_ClassifyPhrase(t *testing.T) {
	extractor := NewTFIDFPhraseExtractor()
	tests := []struct {
		phrase string
		want   PhraseType
	}{
		{"下单试试", PhraseTypeAction},
		{"立即咨询", PhraseTypeAction},
		{"联系客服", PhraseTypeAction},
		{"理解抱歉", PhraseTypeEmpathy},
		{"放心明白", PhraseTypeEmpathy},
		{"成分肤质", PhraseTypeProfessional},
		{"续航参数", PhraseTypeProfessional},
		{"面料版型", PhraseTypeProfessional},
		{"优惠折扣", PhraseTypePersuasion},
		{"限时包邮", PhraseTypePersuasion},
		{"产品特点", PhraseTypeGeneral},
		{"使用方法", PhraseTypeGeneral},
	}
	for _, tt := range tests {
		got := extractor.classifyPhrase(tt.phrase)
		if got != tt.want {
			t.Errorf("classifyPhrase(%q)=%q want %q", tt.phrase, got, tt.want)
		}
	}
}

// TestTFIDF_Extract_PhraseTypeInResult 结果中包含分类
func TestTFIDF_Extract_PhraseTypeInResult(t *testing.T) {
	messages := []ChampionMessage{
		{Content: "现在下单立享优惠"},
		{Content: "成分是烟酰胺"},
		{Content: "理解您的心情"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 30)
	typeCount := make(map[PhraseType]int)
	for _, p := range phrases {
		typeCount[p.PhraseType]++
	}
	// 至少有一种分类
	if len(typeCount) == 0 {
		t.Error("应至少有一种短语分类")
	}
}

// ============================================================================
// 停用词与标点过滤测试
// ============================================================================

// TestTFIDF_StopWordFilter 停用词过滤
func TestTFIDF_StopWordFilter(t *testing.T) {
	// "的" 是停用词，开头的短语应被过滤
	messages := []ChampionMessage{
		{Content: "的产品"},
		{Content: "的产品"},
		{Content: "的产品"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 50)
	for _, p := range phrases {
		runes := []rune(p.Phrase)
		if len(runes) > 0 && extractor.stopWords[string(runes[0])] {
			t.Errorf("短语 %q 以停用词开头，应被过滤", p.Phrase)
		}
		if len(runes) > 0 && extractor.stopWords[string(runes[len(runes)-1])] {
			t.Errorf("短语 %q 以停用词结尾，应被过滤", p.Phrase)
		}
	}
}

// TestContainsPunct 标点检测
func TestContainsPunct(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"hello", false},
		{"你好", false},
		{"hel.lo", true},
		{"你好。", true},
		{"好！", true},
		{"怎么样？", true},
		{"a,b", true},
		{"a，b", true},
		{"a；b", true},
		{"a:b", true},
		{"a b", true}, // 空格也算
	}
	for _, tt := range tests {
		got := containsPunct(tt.input)
		if got != tt.want {
			t.Errorf("containsPunct(%q)=%v want %v", tt.input, got, tt.want)
		}
	}
}

// TestContainsAny 包含任一词
func TestContainsAny(t *testing.T) {
	tests := []struct {
		s     string
		words []string
		want  bool
	}{
		{"下单试试", []string{"下单", "拍下"}, true},
		{"拍下", []string{"下单", "拍下"}, true},
		{"试试", []string{"下单", "拍下"}, false},
		{"", []string{"下单"}, false},
		{"下单", []string{}, false},
	}
	for _, tt := range tests {
		got := containsAny(tt.s, tt.words)
		if got != tt.want {
			t.Errorf("containsAny(%q, %v)=%v want %v", tt.s, tt.words, got, tt.want)
		}
	}
}

// TestContainsSubstring 子串包含
func TestContainsSubstring(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "o w", true},
		{"hello", "hello world", false}, // sub 比 s 长
		{"hello", "", false},
		{"你好世界", "好世", true},
		{"你好世界", "世界", true},
		{"你好", "你好", true},
		{"你好", "再见", false},
	}
	for _, tt := range tests {
		got := containsSubstring(tt.s, tt.sub)
		if got != tt.want {
			t.Errorf("containsSubstring(%q, %q)=%v want %v", tt.s, tt.sub, got, tt.want)
		}
	}
}

// ============================================================================
// 滑窗大小测试
// ============================================================================

// TestTFIDF_SlidingWindowSize 2-4 字滑窗
func TestTFIDF_SlidingWindowSize(t *testing.T) {
	// 单个长文档
	messages := []ChampionMessage{
		{Content: "成分是烟酰胺保湿提亮肤色"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 100)
	// 所有短语长度应在 [2, 4]
	for _, p := range phrases {
		runeCount := len([]rune(p.Phrase))
		if runeCount < 2 || runeCount > 4 {
			t.Errorf("短语 %q 长度=%d 应在 [2, 4]", p.Phrase, runeCount)
		}
	}
}

// TestTFIDF_Extract_ShortText 短文本（< 2 字）
func TestTFIDF_Extract_ShortText(t *testing.T) {
	messages := []ChampionMessage{
		{Content: "a"}, // 单字符
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 10)
	// 单字符无法形成 2-4 字短语
	if len(phrases) != 0 {
		t.Errorf("单字符应无短语, got %d", len(phrases))
	}
}

// ============================================================================
// 多样化文档测试
// ============================================================================

// TestTFIDF_Extract_RealisticScenario 真实销冠对话场景
func TestTFIDF_Extract_RealisticScenario(t *testing.T) {
	messages := []ChampionMessage{
		{Content: "亲，这款精华液含烟酰胺，能深层保湿提亮肤色哦。现在下单有优惠呢！"},
		{Content: "理解您的顾虑，这款产品成分安全，肤质适应性广。"},
		{Content: "现在入手立享 8 折包邮活动，限时优惠！"},
		{Content: "这款面霜的成分含玻尿酸，保湿效果一流。"},
		{Content: "抱歉让您久等了，您的订单已发货，敬请放心。"},
	}
	extractor := NewTFIDFPhraseExtractor()
	phrases := extractor.Extract(messages, 30)
	if len(phrases) == 0 {
		t.Fatal("应提取出短语")
	}
	// top 短语应有较高的 TF-IDF
	if phrases[0].TFIDFScore <= 0 {
		t.Errorf("top-1 短语 TF-IDF 应 > 0, got %v", phrases[0].TFIDFScore)
	}
	t.Logf("top-5 短语: ")
	for i := 0; i < 5 && i < len(phrases); i++ {
		t.Logf("  %d. %s (TF=%d DF=%d TFIDF=%v type=%s)",
			phrases[i].Rank, phrases[i].Phrase, phrases[i].TF, phrases[i].DF, phrases[i].TFIDFScore, phrases[i].PhraseType)
	}
}
