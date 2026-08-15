package feedbackloop


import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// ChampionDialogueAnalyzer 销冠对话分析器
type ChampionDialogueAnalyzer struct {
	repo       *repository.FeedbackLoopRepository
	embedder   Embedder
	dispatcher LLMDispatcher
	config     ChampionAnalyzerConfig
}

// NewChampionDialogueAnalyzer 构造分析器
//
// 参数 db 仅为构造签名兼容保留，内部用 db 构造 repository，不存到 struct
func NewChampionDialogueAnalyzer(
	db *gorm.DB,
	embedder Embedder,
	dispatcher LLMDispatcher,
	cfg ChampionAnalyzerConfig,
) *ChampionDialogueAnalyzer {
	if cfg.MinReward == 0 {
		cfg.MinReward = 1.0
	}
	if cfg.ClusterSimThreshold == 0 {
		cfg.ClusterSimThreshold = 0.85
	}
	if cfg.MinClusterSize == 0 {
		cfg.MinClusterSize = 3
	}
	if cfg.TopKPerCluster == 0 {
		cfg.TopKPerCluster = 3
	}
	if cfg.MaxDialoguesPerRun == 0 {
		cfg.MaxDialoguesPerRun = 500
	}
	return &ChampionDialogueAnalyzer{
		repo:       repository.NewFeedbackLoopRepositoryWithDB(db),
		embedder:   embedder,
		dispatcher: dispatcher,
		config:     cfg,
	}
}

// AnalyzePipeline 执行完整分析管道
//
// 返回 AnalysisReport（包含候选数/簇数/提取话术列表/错误）
func (a *ChampionDialogueAnalyzer) AnalyzePipeline(ctx context.Context, since time.Time) (*dto.ChampionAnalysisReport, error) {
	report := &dto.ChampionAnalysisReport{
		RunAt:            time.Now(),
		Since:            since,
		ExtractedScripts: make([]dto.ExtractedScriptDTO, 0),
		Errors:           make([]string, 0),
	}

	candidates, err := a.fetchCandidates(ctx, since)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("fetch candidates: %v", err))
		return report, fmt.Errorf("fetch candidates: %w", err)
	}
	report.CandidateCount = len(candidates)
	if len(candidates) == 0 {
		return report, nil
	}

	clusters, err := a.clusterDialogues(ctx, candidates)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("cluster dialogues: %v", err))
		return report, fmt.Errorf("cluster dialogues: %w", err)
	}
	report.ClusterCount = len(clusters)

	for clusterID, dialogues := range clusters {
		if len(dialogues) < a.config.MinClusterSize {
			continue
		}
		topK := a.takeTopK(dialogues, a.config.TopKPerCluster)
		for _, d := range topK {
			if err := a.persistDialogue(ctx, d, clusterID); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("persist dialogue (cluster=%d): %v", clusterID, err))
				continue
			}
			report.PersistedCount++
		}
		scripts, err := a.extractScriptsWithLLM(ctx, topK)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("extract scripts (cluster=%d): %v", clusterID, err))
			continue
		}
		for i := range scripts {
			scripts[i].ClusterID = clusterID
		}
		report.ExtractedScripts = append(report.ExtractedScripts, scripts...)
		a.saveScriptsToTemplate(ctx, scripts, clusterID)
	}

	return report, nil
}


// fetchCandidates 从 feedback_signals 拉取高价值候选对话
//
// SQL：聚合 reward + 关联最近一条 feedback_events 取 customer_msg / ai_reply 快照
func (a *ChampionDialogueAnalyzer) fetchCandidates(ctx context.Context, since time.Time) ([]repository.ChampionDialogueRow, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("repo is nil")
	}
	rows, err := a.repo.FetchChampionDialogueCandidates(ctx, model.ChampionScenarioGeneral, a.config.MinReward, since, a.config.MaxDialoguesPerRun)
	if err != nil {
		return nil, err
	}
	return rows, nil
}


// championDialogueWithEmb 候选对话 + 向量
type championDialogueWithEmb struct {
	repository.ChampionDialogueRow
	Embedding []float32
}

// clusterDialogues pgvector 余弦相似度聚类（简化 DBSCAN）
//
// 算法：
//  1. 向量化每条对话（customer_msg + " || " + ai_reply）
//  2. BFS 扩展：任取未访问样本，找到所有余弦相似度 ≥ ClusterSimThreshold 的样本形成一簇
//  3. 簇大小 < MinClusterSize 视为噪声丢弃
func (a *ChampionDialogueAnalyzer) clusterDialogues(ctx context.Context, rows []repository.ChampionDialogueRow) (map[uint][]championDialogueWithEmb, error) {
	if a.embedder == nil {
		return nil, ErrEmbedderNotConfig
	}
	candidates := make([]championDialogueWithEmb, len(rows))
	for i, r := range rows {
		text := r.CustomerMsg + " || " + r.AIReply
		candidates[i] = championDialogueWithEmb{
			ChampionDialogueRow: r,
			Embedding:           a.embedder.Embed(text),
		}
	}

	visited := make([]bool, len(candidates))
	clusterID := uint(0)
	clusters := make(map[uint][]championDialogueWithEmb)
	simThreshold := float32(a.config.ClusterSimThreshold) 

	for i := 0; i < len(candidates); i++ {
		if visited[i] {
			continue
		}
		visited[i] = true
		cluster := []championDialogueWithEmb{candidates[i]}
		queue := []int{i}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for j := 0; j < len(candidates); j++ {
				if visited[j] {
					continue
				}
				if cosineSimilarity(candidates[cur].Embedding, candidates[j].Embedding) >= simThreshold {
					visited[j] = true
					cluster = append(cluster, candidates[j])
					queue = append(queue, j)
				}
			}
		}
		if len(cluster) >= a.config.MinClusterSize {
			clusters[clusterID] = cluster
			clusterID++
		}
	}
	return clusters, nil
}

// cosineSimilarity 余弦相似度（1 - 余弦距离）
func cosineSimilarity(v1, v2 []float32) float32 {
	if len(v1) != len(v2) || len(v1) == 0 {
		return 0
	}
	var dot, norm1, norm2 float32
	for i := range v1 {
		dot += v1[i] * v2[i]
		norm1 += v1[i] * v1[i]
		norm2 += v2[i] * v2[i]
	}
	if norm1 == 0 || norm2 == 0 {
		return 0
	}
	return dot / (sqrtF32(norm1) * sqrtF32(norm2))
}

// takeTopK 每簇按 reward 排序取 Top-K
func (a *ChampionDialogueAnalyzer) takeTopK(dialogues []championDialogueWithEmb, k int) []championDialogueWithEmb {
	sort.Slice(dialogues, func(i, j int) bool {
		return dialogues[i].Reward > dialogues[j].Reward
	})
	if k > len(dialogues) {
		k = len(dialogues)
	}
	return dialogues[:k]
}


// extractScriptsWithLLM LLM 提取话术
//
// Prompt 模板：注入 llm.Dispatcher 的 ScenarioHighQuality 场景
// 输出格式：JSON 数组 [{title, content, scenario, trigger_keywords, journey_stage, effectiveness_score}]
func (a *ChampionDialogueAnalyzer) extractScriptsWithLLM(ctx context.Context, dialogues []championDialogueWithEmb) ([]dto.ExtractedScriptDTO, error) {
	if a.dispatcher == nil {
		return nil, ErrDispatcherNotConfig
	}
	var samples []string
	for _, d := range dialogues {
		samples = append(samples, fmt.Sprintf("【客户】%s\n【销冠】%s", d.CustomerMsg, d.AIReply))
	}
	prompt := fmt.Sprintf(`你是销冠话术分析师。以下是从真实销冠对话中聚类得到的代表样本：

%s

请提取 1-3 条可复用话术，输出 JSON 数组（仅输出 JSON，不要其他文字）：
[
  {
    "title": "话术标题（≤20字）",
    "content": "话术正文（含变量占位符 {{product}}/{{customer_name}}）",
    "scenario": "objection/closing/followup/nurture/repurchase",
    "trigger_keywords": ["关键词1"],
    "journey_stage": "lead/contact/consider/decide/retain",
    "effectiveness_score": 0.0-1.0
  }
]`, strings.Join(samples, "\n\n"))

	content, _, err := a.dispatcher.Dispatch(ctx, "high_quality", prompt, "你是销冠话术分析师，严格输出 JSON 数组。", true, 1500)
	if err != nil {
		return nil, fmt.Errorf("dispatch extract: %w", err)
	}
	jsonStr := extractJSON(content)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON content in LLM response: %s", content)
	}
	var scripts []dto.ExtractedScriptDTO
	if err := json.Unmarshal([]byte(jsonStr), &scripts); err != nil {
		return nil, fmt.Errorf("parse scripts json: %w", err)
	}
	return scripts, nil
}

// extractJSON 从可能包含 markdown 围栏的字符串中提取 JSON 数组/对象子串
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.IndexAny(s, "[{")
	if start < 0 {
		return ""
	}
	end := strings.LastIndexAny(s, "]}")
	if end < start {
		return ""
	}
	return s[start : end+1]
}


// persistDialogue 持久化销冠对话到 champion_dialogues
//
// 注意：Embedding 字段用 pgvector 字符串字面量 '[v1,v2,...]' 通过原生 SQL 写入
// GORM 不直接支持 vector 类型，Create 会失败
func (a *ChampionDialogueAnalyzer) persistDialogue(ctx context.Context, d championDialogueWithEmb, clusterID uint) error {
	if a.repo == nil {
		return fmt.Errorf("repo is nil")
	}
	fingerprint := d.SessionID + "_" + d.Scenario
	embStr := formatEmbeddingForPgVector(d.Embedding)
	conversionAchieved := d.Reward >= 2.0 

	return a.repo.PersistChampionDialogue(ctx, repository.ChampionDialoguePersist{
		Fingerprint:        fingerprint,
		SessionID:          d.SessionID,
		CustomerID:         d.CustomerID,
		Scenario:           d.Scenario,
		CustomerMsg:        d.CustomerMsg,
		ChampionReply:      d.AIReply,
		EmbeddingLiteral:   embStr,
		ClusterID:          clusterID,
		Reward:             d.Reward,
		ConversionAchieved: conversionAchieved,
	})
}

// formatEmbeddingForPgVector 格式化为 pgvector 字符串字面量 '[v1,v2,...]'
func formatEmbeddingForPgVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%.6f", f))
	}
	b.WriteByte(']')
	return b.String()
}

// saveScriptsToTemplate 写入话术库（script_templates）
//
// 写入时：
//   - Source = 'champion_extract'
//   - ChampionDialogueID = 最新 champion_dialogue.id（按 cluster_id 反查）
//   - EffectivenessScore = LLM 返回的评分
//   - TriggerKeywords = 逗号分隔的关键词
//   - JourneyStage = 旅程阶段
func (a *ChampionDialogueAnalyzer) saveScriptsToTemplate(ctx context.Context, scripts []dto.ExtractedScriptDTO, clusterID uint) {
	if a.repo == nil || len(scripts) == 0 {
		return
	}
	dialogueID, _ := a.repo.GetChampionDialogueIDByCluster(ctx, clusterID)

	for _, s := range scripts {
		tags := strings.Join(s.TriggerKeywords, ",")
		effectiveScore := s.EffectivenessScore
		if effectiveScore > 1 {
			effectiveScore = 1
		} else if effectiveScore < 0 {
			effectiveScore = 0
		}
		if err := a.repo.InsertScriptTemplate(ctx, s.Scenario, s.Title, s.Content, tags, effectiveScore, s.JourneyStage, dialogueID); err != nil {
			continue
		}
	}
}

// absFloat32 绝对值（保留供未来扩展使用）
func absFloat32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

// _ 确保 math 包被引用（未来 sqrtF32 等函数可能扩展）
var _ = math.Pi

