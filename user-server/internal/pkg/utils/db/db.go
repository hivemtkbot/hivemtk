package db

import (
	"fmt"
	"marketing/internal/pkg/utils/config"
	"os"
	"time"

	gormLogger "gorm.io/gorm/logger"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() {
	var db *gorm.DB
	var err error

	appConfig := config.GetAppConfig()

	// 私域合规基线 §7.2：密码不落配置文件。配置文件不保留 password 字段，
	// 连接密码仅由运行时从环境变量 POSTGRES_PASSWORD 注入（.env / 密钥管理）。
	pgPassword := appConfig.Database.Postgres.Password
	if pgPassword == "" {
		pgPassword = os.Getenv("POSTGRES_PASSWORD")
	}
	if pgPassword == "" {
		panic("数据库连接密码缺失：配置文件未保留 password 字段，必须由运行时环境变量 POSTGRES_PASSWORD 注入")
	}

	// 本系统仅使用 PostgreSQL，host/port/user/dbname 统一从配置文件读取，
	// 密码由上述运行时注入获取
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Shanghai",
		appConfig.Database.Postgres.Host,
		appConfig.Database.Postgres.User,
		pgPassword,
		appConfig.Database.Postgres.DBName,
		appConfig.Database.Postgres.Port,
		appConfig.Database.Postgres.SSLMode,
	)

	// P1-5 修复：生产环境默认 Warn 级别（不打印每条 SQL），开发环境才用 Info
	// 避免生产日志爆炸 + SQL 细节泄露
	logLevel := gormLogger.Warn
	if os.Getenv("APP_ENV") == "development" || os.Getenv("GIN_MODE") == "debug" {
		logLevel = gormLogger.Info
	}
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(logLevel),
	})

	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}

	// 获取连接池配置
	poolConfig := appConfig.Database.Pool
	if poolConfig.MaxIdleConns == 0 {
		poolConfig = config.DefaultPoolConfig
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		panic(fmt.Sprintf("Failed to get database instance: %v", err))
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(poolConfig.MaxIdleConns)
	sqlDB.SetMaxOpenConns(poolConfig.MaxOpenConns)
	sqlDB.SetConnMaxIdleTime(time.Duration(poolConfig.ConnMaxIdleTime) * time.Second)
	sqlDB.SetConnMaxLifetime(time.Duration(poolConfig.ConnMaxLifetime) * time.Second)

	DB = db
}

func GetDB() *gorm.DB {
	return DB
}

// SetTestDB 设置测试数据库（仅用于测试）
func SetTestDB(testDB *gorm.DB) {
	DB = testDB
}
