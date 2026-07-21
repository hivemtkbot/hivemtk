package ragcustomerservice

import (
	"context"
	"math"
	"strings"
	"time"
)

// QualityAssessorImpl 质量评估器实现
type QualityAssessorImpl struct {
	config *QualityAssessmentConfig
}

// QualityAssessmentConfig 质量评估配置
type QualityAssessmentConfig struct {
	RelevanceWeight   float64 `json:"relevance_weight"`   // 相关性权重
	AccuracyWeight    float64 `json:"accuracy_weight"`    // 准确性权重
	CoherenceWeight   float64 `json:"coherence_weight"`   // 连贯性权重
	HelpfulnessWeight float64 `json:"helpfulness_weight"` // 有用性权重
	MinimumScore      float64 `json:"minimum_score"`      // 最低分数阈值
}

// NewQualityAssessorImpl 创建新的质量评估器
func NewQualityAssessorImpl(config *QualityAssessmentConfig) *QualityAssessorImpl {
	if config == nil {
		config = &QualityAssessmentConfig{
			RelevanceWeight:   0.3,
			AccuracyWeight:    0.3,
			CoherenceWeight:   0.2,
			HelpfulnessWeight: 0.2,
			MinimumScore:      0.5,
		}
	}

	return &QualityAssessorImpl{
		config: config,
	}
}

// EvaluateResponse 评估回复质量
func (qa *QualityAssessorImpl) EvaluateResponse(ctx context.Context, responseContent, query string, searchResults []any) (float64, error) {
	if responseContent == "" {
		return 0.0, nil
	}

	// 计算各项指标
	relevance, err := qa.EvaluateRelevance(ctx, responseContent, query)
	if err != nil {
		return 0.0, err
	}

	accuracy, err := qa.EvaluateAccuracy(ctx, responseContent, extractReferenceSources(searchResults))
	if err != nil {
		return 0.0, err
	}

	coherence, err := qa.EvaluateCoherence(ctx, responseContent)
	if err != nil {
		return 0.0, err
	}

	helpfulness := qa.evaluateHelpfulness(responseContent, query)

	// 计算综合分数
	compositeScore := (relevance * qa.config.RelevanceWeight) +
		(accuracy * qa.config.AccuracyWeight) +
		(coherence * qa.config.CoherenceWeight) +
		(helpfulness * qa.config.HelpfulnessWeight)

	return compositeScore, nil
}

// EvaluateRelevance 评估相关性
func (qa *QualityAssessorImpl) EvaluateRelevance(ctx context.Context, response, query string) (float64, error) {
	if response == "" || query == "" {
		return 0.0, nil
	}

	// 计算查询与回复的语义相似度
	similarity := calculateSemanticSimilarity(query, response)

	// 检查是否包含查询中的关键词
	queryKeywords := extractKeywords(query)
	responseLower := strings.ToLower(response)

	keywordMatches := 0
	for _, keyword := range queryKeywords {
		if strings.Contains(responseLower, strings.ToLower(keyword)) {
			keywordMatches++
		}
	}

	keywordScore := 0.0
	if len(queryKeywords) > 0 {
		keywordScore = float64(keywordMatches) / float64(len(queryKeywords))
	}

	// 综合相似度和关键词匹配分数
	finalScore := (similarity*0.7 + keywordScore*0.3)

	// 确保分数在0-1范围内
	if finalScore < 0 {
		finalScore = 0
	}
	if finalScore > 1 {
		finalScore = 1
	}

	return finalScore, nil
}

// EvaluateAccuracy 评估准确性
func (qa *QualityAssessorImpl) EvaluateAccuracy(ctx context.Context, response string, referenceSources []string) (float64, error) {
	if response == "" {
		return 0.0, nil
	}

	if len(referenceSources) == 0 {
		// 如果没有参考源，检查回复是否包含不确定性表述
		uncertaintyIndicators := []string{"可能", "也许", "大概", "不确定", "据我所知"}
		uncertaintyCount := 0
		responseLower := strings.ToLower(response)

		for _, indicator := range uncertaintyIndicators {
			if strings.Contains(responseLower, strings.ToLower(indicator)) {
				uncertaintyCount++
			}
		}

		// 如果有很多不确定性表述，准确性分数较低
		if uncertaintyCount > 0 {
			return 0.3, nil
		}

		// 默认情况下，如果没有参考源，假设准确性较低
		return 0.4, nil
	}

	// 在实际实现中，这里应该检查回复内容是否与参考源一致
	// 现在返回一个模拟分数
	consistencyScore := calculateConsistencyScore(response, referenceSources)

	return consistencyScore, nil
}

// EvaluateCoherence 评估连贯性
func (qa *QualityAssessorImpl) EvaluateCoherence(ctx context.Context, response string) (float64, error) {
	if response == "" {
		return 0.0, nil
	}

	// 检查回复的语法连贯性
	coherenceScore := calculateCoherenceScore(response)

	return coherenceScore, nil
}

// GetQualityMetrics 获取质量指标
func (qa *QualityAssessorImpl) GetQualityMetrics(ctx context.Context, sessionID string) (QualityMetrics, error) {
	// 在实际实现中，这里应该从存储中获取会话的质量指标
	// 现在返回默认值
	metrics := QualityMetrics{
		SessionID:      sessionID,
		AvgRelevance:   0.0,
		AvgAccuracy:    0.0,
		AvgCoherence:   0.0,
		ResolutionRate: 0.0,
		UpdateTime:     time.Now(),
	}

	return metrics, nil
}

// calculateSemanticSimilarity 计算语义相似度
func calculateSemanticSimilarity(text1, text2 string) float64 {
	// 使用Jaccard相似度作为简单的语义相似度计算
	set1 := createWordSet(text1)
	set2 := createWordSet(text2)

	if len(set1) == 0 && len(set2) == 0 {
		return 1.0
	}

	if len(set1) == 0 || len(set2) == 0 {
		return 0.0
	}

	intersection := 0
	union := len(set2) // 从set2开始计算并集

	// 计算交集和更新并集
	for word := range set1 {
		if set2[word] {
			intersection++
		} else {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// createWordSet 创建词汇集合
func createWordSet(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	wordSet := make(map[string]bool)

	for _, word := range words {
		// 移除标点符号
		cleanWord := strings.Trim(word, ".,!?;:\"'()[]{}")
		if cleanWord != "" {
			wordSet[cleanWord] = true
		}
	}

	return wordSet
}

// extractKeywords 提取关键词
func extractKeywords(text string) []string {
	words := strings.Fields(text)
	var keywords []string

	// 简单的关键词提取：排除停用词
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "我": true,
		"有": true, "和": true, "就": true, "不": true, "人": true,
		"都": true, "一": true, "一个": true, "上": true, "也": true,
		"很": true, "到": true, "说": true, "要": true, "去": true,
		"你": true, "会": true, "着": true, "没有": true, "看": true,
		"好": true, "自己": true, "这": true,
	}

	for _, word := range words {
		cleanWord := strings.Trim(word, ".,!?;:\"'()[]{}")
		if cleanWord != "" && !stopWords[cleanWord] {
			keywords = append(keywords, cleanWord)
		}
	}

	return keywords
}

// extractReferenceSources 从搜索结果中提取参考源
func extractReferenceSources(results []any) []string {
	var sources []string

	for _, result := range results {
		if resultMap, ok := result.(map[string]any); ok {
			if docID, exists := resultMap["document_id"]; exists {
				if docIDStr, ok := docID.(string); ok {
					sources = append(sources, docIDStr)
				}
			}
		}
	}

	return sources
}

// calculateConsistencyScore 计算一致性分数
func calculateConsistencyScore(response string, referenceSources []string) float64 {
	// 简单的一致性计算：检查回复是否与参考源相关
	if len(referenceSources) == 0 {
		return 0.5 // 没有参考源时的默认分数
	}

	// 在实际实现中，这里应该使用更复杂的算法来比较回复与参考源的内容
	// 现在返回一个固定的分数
	return 0.8
}

// calculateCoherenceScore 计算连贯性分数
func calculateCoherenceScore(response string) float64 {
	if response == "" {
		return 0.0
	}

	// 检查句子结构和逻辑连贯性
	sentences := splitIntoSentences(response)

	if len(sentences) == 0 {
		return 0.0
	}

	// 检查句子之间的连接和主题一致性
	totalScore := 0.0
	for _, sentence := range sentences {
		sentenceScore := evaluateSentenceCoherence(sentence)
		totalScore += sentenceScore
	}

	averageScore := totalScore / float64(len(sentences))

	// 检查整体结构连贯性
	structureScore := evaluateStructureCoherence(sentences)

	// 综合同词句分数和结构分数
	finalScore := (averageScore*0.6 + structureScore*0.4)

	return finalScore
}

// splitIntoSentences 分割句子
func splitIntoSentences(text string) []string {
	// 简单的句子分割
	separators := []string{".", "!", "?", "。", "！", "？"}

	// 替换分隔符为统一标记
	markedText := text
	for _, sep := range separators {
		markedText = strings.ReplaceAll(markedText, sep, "|SENTENCE_END|")
	}

	// 分割并清理
	sentences := strings.Split(markedText, "|SENTENCE_END|")
	var cleanedSentences []string

	for _, sentence := range sentences {
		trimmed := strings.TrimSpace(sentence)
		if trimmed != "" {
			cleanedSentences = append(cleanedSentences, trimmed)
		}
	}

	return cleanedSentences
}

// evaluateSentenceCoherence 评估单个句子的连贯性
func evaluateSentenceCoherence(sentence string) float64 {
	words := strings.Fields(sentence)

	if len(words) == 0 {
		return 0.0
	}

	// 检查句子是否包含有意义的词汇
	meaningfulWords := 0
	for _, word := range words {
		if len([]rune(word)) > 1 { // 至少2个字符
			meaningfulWords++
		}
	}

	// 基于有意义词汇的比例评估连贯性
	wordRatio := float64(meaningfulWords) / float64(len(words))

	// 检查句子长度是否合理
	lengthScore := calculateLengthScore(len(words))

	// 综合评估
	finalScore := (wordRatio*0.6 + lengthScore*0.4)

	return finalScore
}

// calculateLengthScore 计算长度分数
func calculateLengthScore(length int) float64 {
	if length < 2 {
		return 0.1 // 太短
	}
	if length > 50 {
		return 0.3 // 太长
	}
	if length >= 5 && length <= 20 {
		return 1.0 // 理想长度
	}

	// 根据距离理想长度的程度计算分数
	distance := math.Abs(float64(length) - 12.5) // 12.5是5到20的中心值
	maxDistance := 37.5                          // 从12.5到50的距离
	normalizedDistance := distance / maxDistance

	return 1.0 - normalizedDistance
}

// evaluateStructureCoherence 评估结构连贯性
func evaluateStructureCoherence(sentences []string) float64 {
	if len(sentences) == 0 {
		return 0.0
	}

	if len(sentences) == 1 {
		return 0.8 // 单句默认较高分数
	}

	// 检查句子之间的主题连续性
	connectedSentences := 0

	for i := 1; i < len(sentences); i++ {
		prevSentence := sentences[i-1]
		currSentence := sentences[i]

		// 检查是否有连接词或共同主题
		if hasConnection(prevSentence, currSentence) {
			connectedSentences++
		}
	}

	connectionRatio := float64(connectedSentences) / float64(len(sentences)-1)

	return connectionRatio
}

// hasConnection 检查两个句子之间是否有连接
func hasConnection(prev, curr string) bool {
	// 检查是否共享关键词
	prevWords := createWordSet(prev)
	currWords := createWordSet(curr)

	sharedCount := 0
	for word := range prevWords {
		if currWords[word] {
			sharedCount++
		}
	}

	// 如果共享至少一个关键词，则认为有连接
	return sharedCount > 0
}

// evaluateHelpfulness 评估有用性
func (qa *QualityAssessorImpl) evaluateHelpfulness(response, query string) float64 {
	if response == "" || query == "" {
		return 0.0
	}

	// 检查回复是否解决了查询问题
	responseLower := strings.ToLower(response)
	queryLower := strings.ToLower(query)

	// 检查是否直接回答了问题
	answered := qa.directlyAnswersQuestion(responseLower, queryLower)

	// 检查是否提供了有用的额外信息
	informative := qa.containsUsefulInformation(responseLower)

	// 检查回复长度是否足够详细
	detailed := qa.isDetailedEnough(response)

	// 综合评估
	var helpfulness float64
	if answered {
		helpfulness += 0.5
	}
	if informative {
		helpfulness += 0.3
	}
	if detailed {
		helpfulness += 0.2
	}

	return helpfulness
}

// directlyAnswersQuestion 检查是否直接回答问题
func (qa *QualityAssessorImpl) directlyAnswersQuestion(response, query string) bool {
	// 简单的检查：回复是否包含了查询中的关键信息
	queryWords := strings.Fields(query)
	responseWords := createWordSet(response)

	matchedKeywords := 0
	for _, word := range queryWords {
		cleanWord := strings.Trim(word, ".,!?;:\"'()[]{}")
		if cleanWord != "" && responseWords[strings.ToLower(cleanWord)] {
			matchedKeywords++
		}
	}

	// 如果匹配了大部分关键词，则认为直接回答了问题
	return float64(matchedKeywords)/float64(len(queryWords)) > 0.5
}

// containsUsefulInformation 检查是否包含有用信息
func (qa *QualityAssessorImpl) containsUsefulInformation(response string) bool {
	// 检查是否包含数字、时间、联系方式等有用信息
	usefulPatterns := []string{
		"电话", "微信", "地址", "时间", "价格", "费用", "数量", "日期", "号码",
		"小时", "分钟", "秒", "年", "月", "日", "点", "分", "元", "万", "千",
	}

	for _, pattern := range usefulPatterns {
		if strings.Contains(response, pattern) {
			return true
		}
	}

	// 检查是否包含具体的指导步骤
	stepIndicators := []string{"第一步", "第二步", "首先", "然后", "最后", "步骤", "方法"}
	for _, indicator := range stepIndicators {
		if strings.Contains(response, indicator) {
			return true
		}
	}

	return false
}

// isDetailedEnough 检查回复是否足够详细
func (qa *QualityAssessorImpl) isDetailedEnough(response string) bool {
	// 检查回复长度和复杂度
	words := strings.Fields(response)

	// 至少需要一定数量的词才算详细
	minWords := 10

	return len(words) >= minWords
}
