package service

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"context"
	"hivemtk-user/internal/repository"
)

// processStartTime 记录进程启动时间，用于计算系统运行时长
var processStartTime = time.Now()

// formatUptime 将运行时长格式化为 "Xh Ym Zs" 形式
func formatUptime(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	m := int(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := int(d / time.Second)
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}

// getCPUUsage 通过 syscall.Getrusage 采样进程 CPU 使用率（占 CPU 总容量的百分比）
// 跨平台实现：在 macOS/Linux 上均使用 getrusage 采样进程用户态+内核态 CPU 时间，
// 结合墙钟时间与 CPU 核心数计算使用率，返回真实测量值。
func getCPUUsage() float64 {
	var r1, r2 syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r1); err != nil {
		return 0
	}
	t1 := time.Now()
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r2); err != nil {
		return 0
	}
	t2 := time.Now()

	cpu1 := time.Duration(r1.Utime.Nano()) + time.Duration(r1.Stime.Nano())
	cpu2 := time.Duration(r2.Utime.Nano()) + time.Duration(r2.Stime.Nano())
	cpuDelta := cpu2 - cpu1
	wallDelta := t2.Sub(t1)
	if wallDelta <= 0 {
		return 0
	}
	numCPU := runtime.NumCPU()
	if numCPU < 1 {
		numCPU = 1
	}
	usage := float64(cpuDelta) / float64(wallDelta) * 100 / float64(numCPU)
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage
}

// getDiskUsage 通过 syscall.Statfs 获取当前工作目录所在磁盘的使用率（百分比）
func getDiskUsage() float64 {
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(cwd, &stat); err != nil {
		return 0
	}
	total := uint64(stat.Blocks) * uint64(stat.Bsize)
	if total == 0 {
		return 0
	}
	free := uint64(stat.Bfree) * uint64(stat.Bsize)
	used := total - free
	usage := float64(used) / float64(total) * 100
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}
	return usage
}

// SystemMonitorService 系统监控服务
//
// 通过依赖注入 statsRepo 避免在 service 层直接访问数据库
type SystemMonitorService struct {
	statsRepo repository.SystemStatsRepository
}

// NewSystemMonitorService 创建系统监控服务实例
func NewSystemMonitorService() *SystemMonitorService {
	return &SystemMonitorService{
		statsRepo: repository.NewSystemStatsRepository(),
	}
}

// NewSystemMonitorServiceWithRepo 通过 repo 创建(用于测试)
func NewSystemMonitorServiceWithRepo(repo repository.SystemStatsRepository) *SystemMonitorService {
	return &SystemMonitorService{statsRepo: repo}
}

// GetSystemStats 获取系统统计信息
func (s *SystemMonitorService) GetSystemStats(ctx context.Context) (map[string]any, error) {
	// 全部通过 repository 访问数据库,避免 service 层直接调 db
	totalUsers, _ := s.statsRepo.CountSystemUsers(ctx)
	totalOrders, _ := s.statsRepo.CountOrders(ctx)
	totalCards, _ := s.statsRepo.CountCards(ctx)
	totalShortLinks, _ := s.statsRepo.CountShortLinks(ctx)

	// 获取今天的访问量（从访问日志表）
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())
	todayVisits, _ := s.statsRepo.CountTodayVisits(ctx, todayStart.Unix())

	// 获取系统运行时间（基于进程启动时间计算）
	uptime := formatUptime(time.Since(processStartTime))

	// 获取系统资源使用情况
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取CPU和内存使用率
	cpuUsage := getCPUUsage()
	memUsage := float64(m.Alloc) / float64(m.Sys) * 100

	// 获取磁盘使用情况（当前工作目录所在分区）
	diskUsage := getDiskUsage()

	stats := map[string]any{
		"total_users":       totalUsers,
		"total_orders":      totalOrders,
		"total_cards":       totalCards,
		"total_short_links": totalShortLinks,
		"today_visits":      todayVisits,
		"system_uptime":     uptime,
		"cpu_usage":         cpuUsage,
		"memory_usage":      memUsage,
		"disk_usage":        diskUsage,
		"timestamp":         time.Now(),
	}

	return stats, nil
}

// GetDetailedSystemStats 获取详细的系统统计信息
func (s *SystemMonitorService) GetDetailedSystemStats(ctx context.Context) (map[string]any, error) {
	// 全部通过 repository 访问数据库,避免 service 层直接调 db
	todayStart := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Now().Location())
	activeUsers, _ := s.statsRepo.CountActiveSystemUsers(ctx, todayStart.Unix())

	// 商户数(开源版：固定 1，因为不再有 License 维度)
	totalMerchants := int64(1)
	// 开源版：移除 License 计数（License 模型已删除）

	// 获取邮件相关统计
	totalEmailLists, _ := s.statsRepo.CountEmailLists(ctx)
	totalEmailJobs, _ := s.statsRepo.CountEmailJobs(ctx)

	// 获取素材库统计
	totalMaterials, _ := s.statsRepo.CountMaterials(ctx)

	// 重新计算基本统计数据
	totalUsers, _ := s.statsRepo.CountSystemUsers(ctx)
	totalOrders, _ := s.statsRepo.CountOrders(ctx)
	totalCards, _ := s.statsRepo.CountCards(ctx)
	totalShortLinks, _ := s.statsRepo.CountShortLinks(ctx)

	// 获取今天的访问量（从访问日志表）
	todayVisits, _ := s.statsRepo.CountTodayVisits(ctx, todayStart.Unix())

	// 获取系统指标
	systemMetrics, _ := s.statsRepo.ListRecentSystemMetrics(ctx, 10)

	detailedStats := map[string]any{
		"basic_stats": map[string]any{
			"total_users":        totalUsers,
			"total_orders":       totalOrders,
			"total_cards":        totalCards,
			"total_short_links":  totalShortLinks,
			"today_visits":       todayVisits,
			"active_users_today": activeUsers,
			"total_merchants":    totalMerchants,
			// 开源版：移除 total_licenses 字段
		},
		"business_stats": map[string]any{
			"total_email_lists":         totalEmailLists,
			"total_email_jobs":          totalEmailJobs,
			"total_materials":           totalMaterials,
		},
		"system_metrics": systemMetrics,
		"timestamp":      time.Now(),
	}

	return detailedStats, nil
}
