package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"hivemtk-user/internal/model"
)

// P1h SOP 节点级转化热力图（竞品吸收：小冠AI，见 AI_CORE_COMPETITIVE_ANALYSIS.md）
// 聚合维度：每个 SOP 的每个节点统计 entered / completed / drop_rate / avg_duration
// 数据源：SOPAgent.SOPGraph(nodes) + SOPExecution.ExecutedNodes(CompensationRecord)

// SopAgentGetter SOP 智能体读取端口（支持接口注入，便于测试 mock）
type SopAgentGetter interface {
	GetByID(ctx context.Context, id uint) (*model.SOPAgent, error)
}

// SopExecLister SOP 执行记录列表端口
type SopExecLister interface {
	ListBySOPID(ctx context.Context, sopID uint, limit int) ([]model.SOPExecution, error)
}

// SopHeatmapService SOP 热力图服务
type SopHeatmapService struct {
	agentRepo SopAgentGetter
	execRepo  SopExecLister
}

// NewSopHeatmapService 构造热力图服务
func NewSopHeatmapService(agentRepo SopAgentGetter, execRepo SopExecLister) *SopHeatmapService {
	return &SopHeatmapService{
		agentRepo: agentRepo,
		execRepo:  execRepo,
	}
}

// SopHeatmapReport 单个 SOP 的节点级热力图报告
type SopHeatmapReport struct {
	SOPID       uint               `json:"sop_id"`
	SOPName     string             `json:"sop_name"`
	Variant     string             `json:"variant"`
	TotalExec   int                `json:"total_executions"`
	Nodes       []NodeHeatmapEntry `json:"nodes"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// NodeHeatmapEntry 单节点热力图条目
type NodeHeatmapEntry struct {
	NodeID        string  `json:"node_id"`
	NodeType      string  `json:"node_type"`
	Entered       int     `json:"entered"`         // 到达该节点的执行数
	Completed     int     `json:"completed"`       // 该节点成功完成数
	DropRate      float64 `json:"drop_rate"`       // 1 - completed/entered
	AvgDurationMs float64 `json:"avg_duration_ms"` // 平均耗时(ms)，仅基于已完成
	StatusDist    map[string]int `json:"status_dist,omitempty"` // status 计数：completed/failed/skipped...
}

// SopNodeMeta SOP 图中的节点元数据
type SopNodeMeta struct {
	NodeID  string `json:"node_id"`
	NodeType string `json:"node_type"`
}

// GenerateHeatmapForSOP 生成指定 SOP 的节点级转化热力图
func (s *SopHeatmapService) GenerateHeatmapForSOP(ctx context.Context, sopID uint, variant string, limit int) (*SopHeatmapReport, error) {
	if s.agentRepo == nil || s.execRepo == nil {
		return nil, errors.New("heatmap service repos not initialized")
	}

	agent, err := s.agentRepo.GetByID(ctx, sopID)
	if err != nil {
		return nil, fmt.Errorf("sop agent not found: %w", err)
	}

	// 1. 从 SOPGraph 提取有序节点列表（作为热力图基准）
	nodeMetas := extractNodeMetas(agent.SOPGraph)
	if len(nodeMetas) == 0 {
		return nil, errors.New("sop graph has no nodes")
	}

	// 2. 拉取该 SOP 的执行记录
	execs, err := s.execRepo.ListBySOPID(ctx, sopID, limit)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	if variant != "" {
		// 仅保留匹配 variant 的执行
		filtered := execs[:0]
		for _, e := range execs {
			if e.Variant == variant {
				filtered = append(filtered, e)
			}
		}
		execs = filtered
	}

	// 3. 聚合
	nodeStats := make(map[string]*NodeHeatmapEntry)
	for _, meta := range nodeMetas {
		nodeStats[meta.NodeID] = &NodeHeatmapEntry{
			NodeID:     meta.NodeID,
			NodeType:   meta.NodeType,
			StatusDist: make(map[string]int),
		}
	}

	for _, exec := range execs {
		processExecutionExecutedNodes(exec, nodeStats)
	}

// 4. 计算派生字段
	nodesOut := make([]NodeHeatmapEntry, 0, len(nodeMetas)+len(nodeStats))
	seen := make(map[string]bool)
	// 先按图顺序输出基准节点
	for _, meta := range nodeMetas {
		if entry, ok := nodeStats[meta.NodeID]; ok {
			if entry.Entered > 0 {
				entry.DropRate = 1.0 - float64(entry.Completed)/float64(entry.Entered)
				if entry.Completed > 0 {
					entry.AvgDurationMs = math.Round(entry.AvgDurationMs*100) / 100
				}
			}
			nodesOut = append(nodesOut, *entry)
			seen[meta.NodeID] = true
		} else {
			// 基准节点但无执行记录
			nodesOut = append(nodesOut, NodeHeatmapEntry{NodeID: meta.NodeID, NodeType: meta.NodeType})
			seen[meta.NodeID] = true
		}
	}
	// 再追加仅在执行中出现的动态节点
	for nodeID, entry := range nodeStats {
		if !seen[nodeID] {
			if entry.Entered > 0 {
				entry.DropRate = 1.0 - float64(entry.Completed)/float64(entry.Entered)
				if entry.Completed > 0 {
					entry.AvgDurationMs = math.Round(entry.AvgDurationMs*100) / 100
				}
			}
			nodesOut = append(nodesOut, *entry)
		}
	}

	return &SopHeatmapReport{
		SOPID:       sopID,
		SOPName:     agent.Name,
		Variant:     variant,
		TotalExec:   len(execs),
		Nodes:       nodesOut,
		GeneratedAt: time.Now(),
	}, nil
}

// extractNodeMetas 从 SOPAgent.SOPGraph 提取节点元数据（复用反馈闭环的提取逻辑）
func extractNodeMetas(graph model.JSONMap) []SopNodeMeta {
	if len(graph) == 0 {
		return nil
	}
	for _, key := range []string{"nodes", "steps", "node_list"} {
		raw, ok := graph[key]
		if !ok {
			continue
		}
		arr, ok := raw.([]any)
		if !ok {
			continue
		}
		out := make([]SopNodeMeta, 0, len(arr))
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := stringOf(m["id"])
			if id == "" {
				continue
			}
			out = append(out, SopNodeMeta{
				NodeID:   id,
				NodeType: stringOf(m["type"]),
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// processExecutionExecutedNodes 解析单条执行的 ExecutedNodes，累加到 nodeStats
func processExecutionExecutedNodes(exec model.SOPExecution, nodeStats map[string]*NodeHeatmapEntry) {
	if len(exec.ExecutedNodes) == 0 {
		return
	}
	// ExecutedNodes 是 []any (GORM JSONB 反序列化)，每项为 map[string]any
	for _, item := range exec.ExecutedNodes {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		nodeID := stringOf(m["node_id"])
		if nodeID == "" {
			continue
		}
		nodeType := stringOf(m["node_type"])
		status := stringOf(m["status"])
		entry, ok := nodeStats[nodeID]
		if !ok {
			// 节点不在基准图中（可能是 variant 图差异），动态创建
			entry = &NodeHeatmapEntry{
				NodeID:     nodeID,
				NodeType:   nodeType,
				StatusDist: make(map[string]int),
			}
			nodeStats[nodeID] = entry
		}
		entry.Entered++
		entry.StatusDist[status]++
		if status == "completed" {
			entry.Completed++
			// 解析 started_at / finished_at (RFC3339 字符串)
			if started, _ := time.Parse(time.RFC3339, stringOf(m["started_at"])); !started.IsZero() {
				if finished, _ := time.Parse(time.RFC3339, stringOf(m["finished_at"])); !finished.IsZero() {
					dur := finished.Sub(started).Milliseconds()
					entry.AvgDurationMs = (entry.AvgDurationMs*float64(entry.Completed-1) + float64(dur)) / float64(entry.Completed)
				}
			}
		}
	}
}