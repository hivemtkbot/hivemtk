package cron

import (
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
	"time"
)

func AutoReplyCleanupCron() {
	logger.Info("开始执行自动回复日志清理任务...")
	cutoff := time.Now().Add(-72 * time.Hour)
	db.GetDB().Exec("DELETE FROM auto_reply_logs WHERE created_at < ?", cutoff)
	logger.Info("自动回复日志清理完成")
}
