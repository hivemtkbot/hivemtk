package tooluse

import (
	"time"
)

func nowUnixNano() int64 { return time.Now().UnixNano() }

func nowRFC3339() string { return time.Now().Format(time.RFC3339) }
