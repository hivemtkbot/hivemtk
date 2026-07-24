// Command reset-admin 重置 / 清理 user-server 超管账号
//
// 用途：
//   - 安装向导（InitSetup）中断 / 超管密码遗忘 / seed 误删真实超管等场景，
//     让用户能以命令行的方式安全地"重置"超管账号，然后重新走 InitAdmin 流程。
//   - 私域合规基线：超管密码只能从 InitAdmin 入库，命令行只做清理 / 解锁，
//     不提供"重置为某个固定密码"的能力，杜绝弱口令。
//
// 用法：
//
//	cd user-server && go run ./cmd/reset-admin                       # 列出所有超管账号（不删）
//	cd user-server && go run ./cmd/reset-admin --username=admin      # 删除指定 username 的超管
//	cd user-server && go run ./cmd/reset-admin --all                 # 删除全部 role='admin' 账号
//	cd user-server && go run ./cmd/reset-admin --username=admin --wipe-install-lock
//	                                                                  # 同时清空 install.lock，便于重走安装向导
//
// 安全护栏：
//   - 默认仅按 username 精准匹配，--all 必须显式声明
//   - 删除前打印即将删除的账号列表，要求用户在终端二次确认（stdin 输入 y/N）
//   - 失败时不删 install.lock，确保系统状态可回滚
//   - 私域部署时，可通过环境变量 SKIP_CONFIRM=1 跳过交互（仅推荐 CI / 自动恢复使用）
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/config"
	"marketing/internal/system/install"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

func main() {
	var (
		username        = flag.String("username", "", "要删除的超管 username（精确匹配）。与 --all 互斥")
		deleteAll       = flag.Bool("all", false, "删除全部 role=admin 的账号（慎用；用于「彻底重置」场景）")
		wipeInstallLock = flag.Bool("wipe-install-lock", false, "同时清空 install.lock，便于重走安装向导")
		skipConfirm     = flag.Bool("yes", false, "跳过二次确认（仅 CI / 自动恢复场景）")
	)
	flag.Parse()

	if *username != "" && *deleteAll {
		log.Fatalf("--username 与 --all 互斥，请只指定其中一个")
	}

	database := initDB()

	// 1. 列出当前超管（避免误删）
	var admins []model.SystemUser
	q := database.Where("role = ?", model.SystemUserRoleAdmin)
	if *username != "" {
		q = q.Where("username = ?", *username)
	} else if !*deleteAll {
		// 既没指定 username 也没 --all → 仅查看，不删
		log.Println("[INFO] 既未指定 --username 也未指定 --all，仅列出超管账号，不会执行删除。")
		log.Println("[HINT] 如需删除，请加 --username=xxx 或 --all")
	}
	if err := q.Order("id ASC").Find(&admins).Error; err != nil {
		log.Fatalf("查询超管失败: %v", err)
	}

	if len(admins) == 0 {
		log.Println("[OK] 当前没有匹配的超管账号，无需清理。")
		if *wipeInstallLock {
			wipeLock()
		}
		return
	}

	fmt.Println("即将删除以下超管账号：")
	fmt.Println(strings.Repeat("-", 80))
	for _, u := range admins {
		fmt.Printf("  id=%d  username=%s  real_name=%q  email=%q  phone=%q  last_login=%v\n",
			u.ID, u.Username, u.RealName, u.Email, u.Phone, formatTime(u.LastLogin))
	}
	fmt.Println(strings.Repeat("-", 80))

	if !*skipConfirm {
		fmt.Print("确认删除？(输入 y 继续，其他任意键取消): ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			log.Println("[CANCEL] 已取消，未执行任何删除。")
			return
		}
	}

	// 2. 二次确认后真正删除
	// 严格按"主键 + username"双条件删，避免误删其他角色同名账号
	deleted := 0
	for _, u := range admins {
		tx := database.Where("id = ? AND role = ?", u.ID, model.SystemUserRoleAdmin).Delete(&model.SystemUser{})
		if tx.Error != nil {
			log.Fatalf("  ✗ 删除 id=%d username=%s 失败: %v", u.ID, u.Username, tx.Error)
		}
		deleted += int(tx.RowsAffected)
		log.Printf("  ✓ 已删除 id=%d username=%s", u.ID, u.Username)
	}

	log.Printf("=== 清理完成：共删除 %d 个超管账号 ===", deleted)

	// 3. 可选：清空 install.lock
	if *wipeInstallLock {
		wipeLock()
	} else {
		log.Println("[HINT] 如需重走安装向导，请加 --wipe-install-lock 同时清空 install.lock")
	}
}

func wipeLock() {
	lockPath := install.GetInstallLockPath()
	if err := os.Remove(lockPath); err != nil {
		if os.IsNotExist(err) {
			log.Printf("[OK] install.lock 不存在，无需清理 (path=%s)", lockPath)
			return
		}
		log.Fatalf("  ✗ 删除 install.lock 失败 (path=%s): %v", lockPath, err)
	}
	log.Printf("  ✓ 已清空 install.lock (path=%s)，可重走 InitSetup 向导", lockPath)
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "未登录"
	}
	return t.Format("2006-01-02 15:04:05")
}

// initDB 独立初始化数据库连接（参考 cmd/seed/main.go，避免依赖全局 db.InitDB）
func initDB() *gorm.DB {
	cfg := config.GetAppConfig()
	pg := cfg.Database.Postgres
	if pg.Host == "" {
		// 兜底：开发环境默认值
		pg.Host = "localhost"
		pg.Port = 8202
		pg.User = "admin"
		pg.Password = "password123"
		pg.DBName = "user_db"
		pg.SSLMode = "disable"
	}
	// 优先用 .env 中的真实密码覆盖
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		pg.Password = v
	}
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Shanghai",
		pg.Host, pg.User, pg.Password, pg.DBName, pg.Port, pg.SSLMode)
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogger.Warn),
	})
	if err != nil {
		log.Fatalf("数据库连接失败（host=%s port=%d db=%s）: %v", pg.Host, pg.Port, pg.DBName, err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		log.Fatalf("获取 SQL DB 失败: %v", err)
	}
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(5)
	log.Printf("数据库已连接：host=%s port=%d db=%s", pg.Host, pg.Port, pg.DBName)
	return database
}
