package fingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// GenerateFingerprint 生成设备指纹
func GenerateFingerprint() (string, error) {
	// 收集设备信息
	info := collectDeviceInfo()

	// 生成指纹
	hash := md5.Sum([]byte(info))
	return hex.EncodeToString(hash[:]), nil
}

// collectDeviceInfo 收集设备信息
func collectDeviceInfo() string {
	var info strings.Builder

	// 操作系统信息
	info.WriteString(fmt.Sprintf("os:%s,%s\n", runtime.GOOS, runtime.GOARCH))

	// 主机名
	hostname, err := os.Hostname()
	if err == nil {
		info.WriteString(fmt.Sprintf("hostname:%s\n", hostname))
	}

	// 环境变量（安全的）
	if os.Getenv("USER") != "" {
		info.WriteString(fmt.Sprintf("user:%s\n", os.Getenv("USER")))
	}

	// 当前工作目录
	cwd, err := os.Getwd()
	if err == nil {
		info.WriteString(fmt.Sprintf("cwd:%s\n", cwd))
	}

	return info.String()
}
