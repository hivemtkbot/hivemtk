package websocket

// 本文件原为进程内 atomic seq 实现（globalSeq）。
// D15 起序列与纪元逻辑迁移至 seq_redis.go（Redis INCR + epoch + 降级锁定）；
// NextSeq/PeekSeq/CurrentEpoch 由 seq_redis.go 提供，签名兼容。
