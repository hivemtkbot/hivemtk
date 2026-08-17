package tooluse

import (
	"time"
)

// nowUnixNano 统一的时间戳（便于 mock）
func nowUnixNano() int64 { return time.Now().UnixNano() }

// nowRFC3339 统一的时间格式
func nowRFC3339() string { return time.Now().Format(time.RFC3339) }
