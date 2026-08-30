package controller

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CustomerServiceController 客服队列与容量管理控制器
// G9: 竞品标配功能 - 实时队列长度、坐席容量、坐席状态列表
type CustomerServiceController struct {
	db *gorm.DB
}

// NewCustomerServiceController 创建客服队列控制器
func NewCustomerServiceController() *CustomerServiceController {
	return &CustomerServiceController{db: db.GetDB()}
}

// NewCustomerServiceControllerWithDB 注入 DB（测试用）
func NewCustomerServiceControllerWithDB(d *gorm.DB) *CustomerServiceController {
	return &CustomerServiceController{db: d}
}

// QueueSnapshot 队列快照
type QueueSnapshot struct {
	WaitingCount     int64  `json:"waiting_count"`      // 等待中的会话数（unassigned + status=pending）
	LongestWaitSec    int64  `json:"longest_wait_sec"`   // 最长等待秒数
	EstimatedWaitSec  int64  `json:"estimated_wait_sec"` // 预计平均等待秒数
}

// GetQueue 获取客服队列长度（等待中的会话数）
// GET /api/customer-service/queue
func (c *CustomerServiceController) GetQueue(ctx *gin.Context) {
	var waitingCount int64
	if c.db != nil {
		// 客户会话表中 status 为 pending / waiting 且未分配坐席的会话计数
		c.db.Model(&model.CustomerSession{}).
			Where("status IN ? AND assigned_agent_id IS NULL", []string{"pending", "waiting"}).
			Count(&waitingCount)
	}

	// 估计等待时长 = 等待数 / 可用坐席数 * 平均处理时长（简化公式）
	var availableAgents int64
	if c.db != nil {
		c.db.Model(&model.AgentStatus{}).
			Where("status = ?", "available").
			Count(&availableAgents)
	}

	estimated := int64(0)
	longestWait := int64(0)
	if availableAgents > 0 {
		estimated = waitingCount / availableAgents * 60 // 假设每会话平均 60 秒
	}

	snapshot := QueueSnapshot{
		WaitingCount:    waitingCount,
		LongestWaitSec:  longestWait,
		EstimatedWaitSec: estimated,
	}
	response.Success(ctx, snapshot, "获取成功")
}

// CapacitySnapshot 坐席容量快照
type CapacitySnapshot struct {
	TotalAgents       int64   `json:"total_agents"`        // 坐席总数
	OnlineAgents      int64   `json:"online_agents"`       // 在线坐席数（status != offline）
	AvailableAgents   int64   `json:"available_agents"`    // 可接待坐席数（status=available）
	BusyAgents        int64   `json:"busy_agents"`         // 忙碌坐席数
	OfflineAgents     int64   `json:"offline_agents"`      // 离线坐席数
	TotalMaxCapacity  int64   `json:"total_max_capacity"`  // 全员最大可承载会话数（Sum(MaxSessions)）
	TotalActiveLoad   int64   `json:"total_active_load"`   // 当前活跃会话总数（Sum(ActiveSessions)）
	AvgLoadRatio      float64 `json:"avg_load_ratio"`      // 平均负载率 0~1
}

// GetCapacity 获取坐席容量聚合视图
// GET /api/customer-service/capacity
func (c *CustomerServiceController) GetCapacity(ctx *gin.Context) {
	result := CapacitySnapshot{}
	if c.db == nil {
		response.Success(ctx, result, "无数据")
		return
	}

	var agents []model.AgentStatus
	if err := c.db.Find(&agents).Error; err != nil {
		response.ErrorFromDB(ctx, err, "查询坐席状态失败")
		return
	}

	result.TotalAgents = int64(len(agents))
	for _, a := range agents {
		switch a.Status {
		case "offline":
			result.OfflineAgents++
		case "busy":
			result.BusyAgents++
			result.OnlineAgents++
		case "available", "idle":
			result.AvailableAgents++
			result.OnlineAgents++
		default:
			result.OnlineAgents++
		}
		result.TotalMaxCapacity += int64(a.MaxSessions)
		result.TotalActiveLoad += int64(a.ActiveSessions)
	}

	if result.TotalMaxCapacity > 0 {
		result.AvgLoadRatio = float64(result.TotalActiveLoad) / float64(result.TotalMaxCapacity)
	}

	response.Success(ctx, result, "获取成功")
}

// AgentStatusItem 简化的坐席状态项（不含敏感字段）
type AgentStatusItem struct {
	AgentID        uint   `json:"agent_id"`
	AgentName      string `json:"agent_name"`
	Status         string `json:"status"`
	MaxSessions    int    `json:"max_sessions"`
	ActiveSessions int    `json:"active_sessions"`
	LoadRatio      float64 `json:"load_ratio"`
}

// GetAgents 获取所有坐席的实时状态列表
// GET /api/customer-service/agents
func (c *CustomerServiceController) GetAgents(ctx *gin.Context) {
	var agents []model.AgentStatus
	if c.db != nil {
		if err := c.db.Find(&agents).Error; err != nil {
			response.ErrorFromDB(ctx, err, "查询坐席列表失败")
			return
		}
	}
	items := make([]AgentStatusItem, 0, len(agents))
	for _, a := range agents {
		ratio := 0.0
		if a.MaxSessions > 0 {
			ratio = float64(a.ActiveSessions) / float64(a.MaxSessions)
		}
		items = append(items, AgentStatusItem{
			AgentID:        a.AgentID,
			AgentName:      a.AgentName,
			Status:         a.Status,
			MaxSessions:    a.MaxSessions,
			ActiveSessions: a.ActiveSessions,
			LoadRatio:      ratio,
		})
	}
	response.SuccessWithList(ctx, items, int64(len(items)))
}
