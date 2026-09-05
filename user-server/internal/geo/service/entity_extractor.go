package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// EntityExtractorService 实体抽取服务（G7.2）
//
// 从 GeoKnowledgeDocument 中通过 LLM 批量抽取实体（产品/人物/组织/地点/概念）
// 和实体关系（is_a/used_for/competitor_of/part_of）。写入 geo_entities /
// geo_entity_relations。
//
// 依赖注入：EntityRepository（实体持久化，repository 包已用 type alias 适配）、
// GeoKnowledgeDocumentRepository（文档读取）、LLM Dispatcher（模型调用）。
// 全部接口化，便于测试。
type EntityExtractorService struct {
	entityRepo    repository.EntityRepository
	kbRepo        repository.GeoKnowledgeDocumentRepository
	llmDispatcher *llm.Dispatcher
	promptManager *PromptManager
}

// NewEntityExtractorService 创建实体抽取服务
func NewEntityExtractorService(
	entityRepo repository.EntityRepository,
	kbRepo repository.GeoKnowledgeDocumentRepository,
	llmDispatcher *llm.Dispatcher,
) *EntityExtractorService {
	return &EntityExtractorService{
		entityRepo:    entityRepo,
		kbRepo:        kbRepo,
		llmDispatcher: llmDispatcher,
		promptManager: NewPromptManager(),
	}
}

type extractedEntity struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
	Confidence  float64  `json:"confidence"`
}

type extractedRelation struct {
	EntityA  string `json:"entity_a"`
	EntityB  string `json:"entity_b"`
	Relation string `json:"relation"`
}

type extractResponse struct {
	Entities  []extractedEntity   `json:"entities"`
	Relations []extractedRelation `json:"relations"`
}

// ExtractFromDocument 从指定知识库文档抽取实体和关系
//
// 1. 拉取文档内容；
// 2. 调用 LLM 做结构化抽取（JSON mode）；
// 3. 将实体批量写入 geo_entities；
// 4. 按实体名→ID 回查，写入 geo_entity_relations。
func (s *EntityExtractorService) ExtractFromDocument(ctx context.Context, docID string) error {
	doc, err := s.kbRepo.GetByID(docID)
	if err != nil {
		return fmt.Errorf("加载知识库文档失败: %w", err)
	}
	if strings.TrimSpace(doc.Content) == "" {
		return fmt.Errorf("文档内容为空，跳过抽取")
	}

	prompt := s.promptManager.EntityExtractPrompt(doc.Title, doc.Content)

	req := llm.DispatchRequest{
		Scenario:    llm.ScenarioHighQuality,
		Prompt:      prompt,
		MaxTokens:   4000,
		JSONMode:    true,
		Temperature: 0.1,
	}
	resp, err := s.llmDispatcher.Dispatch(ctx, req)
	if err != nil {
		return fmt.Errorf("LLM 抽取失败: %w", err)
	}

	var parsed extractResponse
	preview := resp.Content
	if len(preview) > 500 {
		preview = preview[:500]
	}
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return fmt.Errorf("解析 LLM 返回 JSON 失败: %w, raw=%s", err, preview)
	}

	entities := make([]*model.GeoEntity, 0, len(parsed.Entities))
	for _, e := range parsed.Entities {
		if e.Confidence <= 0 || e.Confidence > 1 {
			e.Confidence = 0.8
		}
		ent := &model.GeoEntity{
			Name:        strings.TrimSpace(e.Name),
			Type:        e.Type,
			Aliases:     mustJSON(e.Aliases),
			Description: e.Description,
			SourceDocID: 0,
			Confidence:  e.Confidence,
		}
		if ent.Name == "" {
			continue
		}
		entities = append(entities, ent)
	}

	if len(entities) > 0 {
		if err := s.entityRepo.BatchCreate(ctx, entities); err != nil {
			return fmt.Errorf("批量写入实体失败: %w", err)
		}
	}

	nameToID := make(map[string]uint, len(entities))
	list, _, err := s.entityRepo.List(ctx, "", "", 1, len(entities)+10)
	if err != nil {

	} else {
		for _, e := range list {
			nameToID[e.Name] = e.ID
		}
	}

	for _, r := range parsed.Relations {
		aID, okA := nameToID[r.EntityA]
		bID, okB := nameToID[r.EntityB]
		if !okA || !okB || aID == 0 || bID == 0 {
			continue
		}
		rel := &model.GeoEntityRelation{
			EntityAID: aID,
			EntityBID: bID,
			Relation:  r.Relation,
		}
		if err := s.entityRepo.CreateRelation(ctx, rel); err != nil {

			continue
		}
	}

	return nil
}

func mustJSON(v any) []byte {
	if v == nil {
		return []byte("null")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}
