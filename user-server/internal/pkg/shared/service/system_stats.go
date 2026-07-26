package service

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
)

// SystemStatsService 系统统计服务
type SystemStatsService struct{}

// NewSystemStatsService 创建系统统计服务
func NewSystemStatsService() *SystemStatsService {
	return &SystemStatsService{}
}

// SystemInfo 系统信息
type SystemInfo struct {
	CPUUsage     float64 `json:"cpu_usage"`     // CPU 使用率 (%)
	MemoryUsage  float64 `json:"memory_usage"`  // 内存使用率 (%)
	DiskUsage    float64 `json:"disk_usage"`    // 磁盘使用率 (%)
	Uptime       int64   `json:"uptime"`        // 运行时间 (秒)
	ServerTime   string  `json:"server_time"`   // 服务器时间
	Hostname     string  `json:"hostname"`      // 主机名
	GoVersion    string  `json:"go_version"`    // Go 版本
	NumCPU       int     `json:"num_cpu"`       // CPU 核心数
	NumGoroutine int     `json:"num_goroutine"` // Goroutine 数量
	AllocMemory  uint64  `json:"alloc_memory"`  // 已分配内存 (字节)
	SysMemory    uint64  `json:"sys_memory"`    // 系统内存 (字节)
}

// GetSystemInfo 获取系统信息
func (s *SystemStatsService) GetSystemInfo() (*SystemInfo, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取 CPU 使用率（部分平台首次调用可能返回 not implemented，降级为 0 不报错）
	cpuPercent, err := cpu.Percent(0, false)
	cpuUsage := 0.0
	if err == nil && len(cpuPercent) > 0 {
		cpuUsage = cpuPercent[0]
	}

	// 获取磁盘使用率
	diskUsage, err := disk.Usage("/")
	diskPct := 0.0
	if err == nil {
		diskPct = diskUsage.UsedPercent
	}

	// 获取主机名
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 获取运行时间
	uptime, err := host.Uptime()
	if err != nil {
		uptime = 0
	}

	// 计算内存使用率（防止除零）
	memPct := 0.0
	if m.Sys > 0 {
		memPct = float64(m.Alloc) / float64(m.Sys) * 100
	}

	return &SystemInfo{
		CPUUsage:     cpuUsage,
		MemoryUsage:  memPct,
		DiskUsage:    diskPct,
		Uptime:       int64(uptime),
		ServerTime:   time.Now().Format("2006-01-02 15:04:05"),
		Hostname:     hostname,
		GoVersion:    runtime.Version(),
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
		AllocMemory:  m.Alloc,
		SysMemory:    m.Sys,
	}, nil
}
