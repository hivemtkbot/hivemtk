package db

import (
	"fmt"
	"hivemtk-user/internal/config"
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

	pgPassword := appConfig.Database.Postgres.Password
	if pgPassword == "" {
		pgPassword = os.Getenv("POSTGRES_PASSWORD")
	}
	if pgPassword == "" {
		panic("数据库连接密码缺失：配置文件未保留 password 字段，必须由运行时环境变量 POSTGRES_PASSWORD 注入")
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Shanghai",
		appConfig.Database.Postgres.Host,
		appConfig.Database.Postgres.User,
		pgPassword,
		appConfig.Database.Postgres.DBName,
		appConfig.Database.Postgres.Port,
		appConfig.Database.Postgres.SSLMode,
	)

	logLevel := gormLogger.Warn
	if os.Getenv("APP_ENV") == "development" || os.Getenv("GIN_MODE") == "debug" {
		logLevel = gormLogger.Info
	}
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   gormLogger.Default.LogMode(logLevel),
		DisableForeignKeyConstraintWhenMigrating: true,
	})

	if err != nil {
		panic(fmt.Sprintf("Failed to connect to database: %v", err))
	}

	poolConfig := appConfig.Database.Pool
	if poolConfig.MaxIdleConns == 0 {
		poolConfig = config.DefaultPoolConfig
	}

	if poolConfig.MaxOpenConns == 0 {
		poolConfig.MaxOpenConns = 20
	}
	if poolConfig.ConnMaxLifetime == 0 {
		poolConfig.ConnMaxLifetime = int((30 * time.Minute).Seconds())
	}

	sqlDB, err := db.DB()
	if err != nil {
		panic(fmt.Sprintf("Failed to get database instance: %v", err))
	}

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
