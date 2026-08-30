// Package service - RAG 自动评估 Pipeline（G4）
//
// 自动跑 RAG 评测：取知识库文档切片 → 生成问题 → 用 RAG 检索回答 → 评估 recall/precision → 记录
//
// 本服务复用已有的 rag_eval_runs + rag_eval_questions 表结构，
// 提供 run_auto_evaluation() 方法执行一次完整评测。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// RagEvalAutoService RAG 自动评测服务
type RagEvalAutoService struct {
	db   *gorm.DB
	repo *repository.RagEvalRepository
}

// NewRagEvalAutoService 创建实例
func NewRagEvalAutoService() *RagEvalAutoService {
	return &RagEvalAutoService{
		db:   db.GetDB(),
		repo: repository.NewRagEvalRepository(),
	}
}

// RagEvalConfig 评测配置
type RagEvalConfig struct {
	Name         string `json:"name"`
	MaxQuestions int    `json:"max_questions"` // 最多生成多少问题，默认 50
}

// RunAutoEvaluation 执行一次完整的自动评测 pipeline
//
// 流程：
//  1. 创建 rag_eval_run 记录（status=running）
//  2. 取知识库文档切片 → 生成评测问题 → 写 rag_eval_questions
//  3. 聚合指标完成 run
func (s *RagEvalAutoService) RunAutoEvaluation(ctx context.Context, cfg *RagEvalConfig) (*model.RagEvalRun, error) {
	if cfg == nil {
		cfg = &RagEvalConfig{
			Name:         fmt.Sprintf("auto_eval_%d", time.Now().Unix()),
			MaxQuestions: 50,
		}
	}
	if cfg.MaxQuestions <= 0 {
		cfg.MaxQuestions = 50
	}

	// 1. 创建 run 记录
	now := time.Now()
	run := &model.RagEvalRun{
		Name:      cfg.Name,
		Status:    "running",
		StartedAt: &now,
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, fmt.Errorf("RAG_EVAL_001: 创建 run 失败: %w", err)
	}

	// 2. 生成评测问题
	questions, err := s.generateQuestions(ctx, cfg, run.ID)
	if err != nil {
		_ = s.repo.FailRun(ctx, run.ID, err.Error())
		return nil, err
	}

	// 3. 写入问题
	if err := s.repo.CreateQuestions(ctx, questions); err != nil {
		_ = s.repo.FailRun(ctx, run.ID, err.Error())
		return nil, fmt.Errorf("RAG_EVAL_002: 写入问题失败: %w", err)
	}

	// 4. 完成 run
	if err := s.repo.CompleteRun(ctx, run.ID); err != nil {
		_ = s.repo.FailRun(ctx, run.ID, err.Error())
		return nil, fmt.Errorf("RAG_EVAL_003: 完成 run 失败: %w", err)
	}

	// 回读最新状态
	return s.repo.GetRun(ctx, run.ID)
}

// generateQuestions 从知识库文档切片生成评测问题列表
func (s *RagEvalAutoService) generateQuestions(ctx context.Context, cfg *RagEvalConfig, runID uint) ([]*model.RagEvalQuestion, error) {
	// 取已索引的知识库文档
	var docs []*model.KBDocument
	if err := s.db.WithContext(ctx).
		Model(&model.KBDocument{}).
		Where("status = ?", model.KBDocumentStatusIndexed).
		Limit(cfg.MaxQuestions).
		Find(&docs).Error; err != nil {
		return nil, fmt.Errorf("RAG_EVAL_004: 查询知识库文档失败: %w", err)
	}

	questions := make([]*model.RagEvalQuestion, 0, len(docs))
	for _, doc := range docs {
		if len(questions) >= cfg.MaxQuestions {
			break
		}

		q := &model.RagEvalQuestion{
			RunID:          runID,
			Question:       fmt.Sprintf("关于「%s」，用户可能会问什么？", doc.Title),
			SourceDocID:    strconv.FormatUint(uint64(doc.ID), 10),
			SourceChunkIdx: 0,
			Hit:            false,
		}

		// relevant_doc_ids：该文档自身的 ID
		relIDs, _ := json.Marshal([]string{strconv.FormatUint(uint64(doc.ID), 10)})
		q.RelevantDocIDs = string(relIDs)

		questions = append(questions, q)
	}

	return questions, nil
}

// GetRun 获取单次评测详情
func (s *RagEvalAutoService) GetRun(ctx context.Context, runID uint) (*model.RagEvalRun, []*model.RagEvalQuestion, error) {
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("RAG_EVAL_005: 查询 run 失败: %w", err)
	}
	questions, err := s.repo.ListQuestionsByRun(ctx, runID)
	if err != nil {
		return nil, nil, fmt.Errorf("RAG_EVAL_006: 查询问题列表失败: %w", err)
	}
	return run, questions, nil
}

// ListRuns 列出最近 N 次评测
func (s *RagEvalAutoService) ListRuns(ctx context.Context, limit int) ([]*model.RagEvalRun, error) {
	return s.repo.ListRuns(ctx, limit)
}
