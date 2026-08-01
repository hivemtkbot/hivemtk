// Command reset-admin-password 重置 user-server 超管账号密码
//
// 用途：
//   - 超管密码遗忘 / 需要强制改密时，用命令行安全地重设某个 admin 账号的密码。
//   - 密码使用与业务完全一致的 bcrypt(DefaultCost) 哈希，避免手动生成哈希导致格式不符。
//
// 用法：
//
//	cd user-server && \
//	  POSTGRES_PASSWORD=xxx go run ./cmd/reset-admin-password --username=admin --password='Seed@123'
//
// 安全护栏：
//   - 仅对 role='admin' 的账号生效，且必须按 username 精确匹配
//   - 默认不改动账号状态；可通过 --enable 一并启用（status=1, enabled=true）
//   - 私域合规基线：密码不落配置文件，DB 密码仅由环境变量 POSTGRES_PASSWORD 注入
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/bcrypt"
	"marketing/internal/pkg/utils/config"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

func main() {
	var (
		username = flag.String("username", "admin", "要重置密码的超管 username（精确匹配，role 必须为 admin）")
		password = flag.String("password", "", "新的明文密码（必填；建议使用环境变量传入避免落命令行历史）")
		enable   = flag.Bool("enable", true, "重置后一并启用该账号（status=1, enabled=true）")
	)
	flag.Parse()

	if *password == "" {
		// 也允许从环境变量 ADMIN_PASSWORD 读取（避免明文出现在进程参数）
		*password = os.Getenv("ADMIN_PASSWORD")
	}
	if *password == "" {
		log.Fatalf("必须指定 --password 或环境变量 ADMIN_PASSWORD")
	}

	database := initDB()

	// 1. 定位目标 admin
	var admin model.SystemUser
	if err := database.Where("username = ? AND role = ?", *username, model.SystemUserRoleAdmin).
		First(&admin).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Fatalf("未找到 role=admin 且 username=%q 的账号，已取消", *username)
		}
		log.Fatalf("查询超管失败: %v", err)
	}

	// 2. 用项目统一 bcrypt 哈希（DefaultCost，与 BeforeCreate / 登录校验一致）
	hashed, err := bcrypt.HashPassword(*password)
	if err != nil {
		log.Fatalf("口令哈希失败: %v", err)
	}

	// 3. 更新密码（不触发 BeforeCreate 钩子，直接 UPDATE）
	updates := map[string]interface{}{
		"password":   hashed,
		"updated_at": time.Now(),
	}
	if *enable {
		updates["status"] = 1
		updates["enabled"] = true
	}
	tx := database.Model(&model.SystemUser{}).
		Where("id = ? AND role = ?", admin.ID, model.SystemUserRoleAdmin).
		Updates(updates)
	if tx.Error != nil {
		log.Fatalf("更新密码失败: %v", tx.Error)
	}
	if tx.RowsAffected == 0 {
		log.Fatalf("未更新任何记录（id=%d 不匹配或已非 admin）", admin.ID)
	}

	log.Printf("=== 已重置超管密码：username=%s id=%d (%d 行受影响) ===", admin.Username, admin.ID, tx.RowsAffected)
	if *enable {
		log.Printf("[OK] 账号已置为启用状态（status=1, enabled=true）")
	}
	log.Printf("[HINT] 请使用新密码登录；若之前开启 MFA，仍需完成二次验证")
}

// initDB 独立初始化数据库连接（参考 cmd/reset-admin/main.go）
func initDB() *gorm.DB {
	cfg := config.GetAppConfig()
	pg := cfg.Database.Postgres
	if pg.Host == "" {
		log.Fatalf("[FATAL] 缺少 config.yaml 或 database.postgres.host 为空；请先准备好 user-server/config.yaml")
	}
	// 私域合规基线 §7.2：密码不落配置文件，缺配置时由运行时环境变量 POSTGRES_PASSWORD 注入
	if v := os.Getenv("POSTGRES_PASSWORD"); v != "" {
		pg.Password = v
	}
	if pg.Password == "" {
		log.Fatalf("[FATAL] 缺少 POSTGRES_PASSWORD 环境变量（合规基线 §7.2，密码不落配置文件）")
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
