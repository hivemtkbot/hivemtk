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
	info := collectDeviceInfo()

	hash := md5.Sum([]byte(info))
	return hex.EncodeToString(hash[:]), nil
}

func collectDeviceInfo() string {
	var info strings.Builder

	info.WriteString(fmt.Sprintf("os:%s,%s\n", runtime.GOOS, runtime.GOARCH))

	hostname, err := os.Hostname()
	if err == nil {
		info.WriteString(fmt.Sprintf("hostname:%s\n", hostname))
	}

	if os.Getenv("USER") != "" {
		info.WriteString(fmt.Sprintf("user:%s\n", os.Getenv("USER")))
	}

	cwd, err := os.Getwd()
	if err == nil {
		info.WriteString(fmt.Sprintf("cwd:%s\n", cwd))
	}

	return info.String()
}
