// Package testutil 提供统一的 PostgreSQL 测试数据库辅助
// 本系统仅使用 PostgreSQL，测试环境必须连接真实 PG（不允许任何非 PG 数据库）。
// 通过 TEST_DATABASE_URL 或 POSTGRES_* 环境变量配置测试连接。
package testutil

import (
	"fmt"
	"os"
	"regexp"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// dbnameRe 匹配 DSN 中的 dbname 段，用于切换到维护库（postgres）或进程级测试库。
var dbnameRe = regexp.MustCompile(`dbname=[^\s]+`)

// 进程级唯一测试库（避免多个 go test 进程并行执行 AutoMigrate 冲突）
// 设计要点：
//   - 每个 go test 进程拥有独立库名 user_db_test_<pid>，跨进程天然隔离，
//     彻底消除 `go test ./...`（跨包默认并行）下多进程并发 DDL 互相踩踏的隐患
//     （典型报错：duplicate key "pg_type_typname_nsp_index" / relation does not exist）。
//   - 同一进程内的所有测试共享该库，与原始「共享 user_db_test」行为一致，
//     保留同包内测试间可能的状态共享语义，不产生回归。
//   - 进程结束后库随连接关闭而留存在 PG 中（与原始实现同样不自动 DROP 整个库），
//     下次同 PID 进程启动时会先 DROP 同名残留再重建，避免崩溃残留累积。
var (
	procDBName    string
	procDBInit    sync.Once
	procDBInitErr error
)

// NewTestDB 创建并初始化（或复用）当前进程的独立 PostgreSQL 测试数据库。
//
// 行为：
//  1. 首次调用时在维护库（postgres）创建 user_db_test_<pid> 独立库（先清理同名残留）。
//  2. 连接该库，启用 pgvector 扩展（向量字段必需）。
//  3. 在测试 session 内禁用外键约束（避免跨域共享模型引用时缺失依赖记录）。
//  4. 对传入的 models 执行 AutoMigrate（先 DROP 目标表再重建，幂等）。
//  5. 通过 t.Cleanup 注册清理：断开本测试连接（不 DROP 整个进程库，供同进程后续测试复用）。
//
// 注意：本辅助不调用 db.SetTestDB，由调用方按需将 DB 注入到全局 db.DB 或 Service。
func NewTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()

	ensureProcTestDB(t)
	testDSN := dbnameRe.ReplaceAllString(getTestDSN(), "dbname="+procDBName)

	database, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("连接 PostgreSQL 测试库失败（dsn=%s）: %v", maskedDSN(testDSN), err)
	}

	// 启用 pgvector 扩展（业务模型中含 vector 列，缺扩展时 AutoMigrate 报 42704）
	if sqlDB, dbErr := database.DB(); dbErr == nil {
		if _, execErr := sqlDB.Exec("CREATE EXTENSION IF NOT EXISTS vector"); execErr != nil {
			t.Logf("启用 pgvector 扩展提示（需 postgres 镜像含 vector 包）: %v", execErr)
		}
		// 关闭外键约束检查（业务域拆分后,跨域共享的 model.License 等不在 content 测试 setup 中,
		// 关闭 session_replication_role 可避免外键引用失败）
		if _, execErr := sqlDB.Exec("SET session_replication_role = 'replica'"); execErr != nil {
			t.Logf("禁用外键约束提示: %v", execErr)
		}
	}

	if len(models) > 0 {
		// 先 DROP 全部目标表（幂等重建），再 AutoMigrate 创建
		for _, m := range models {
			if dropErr := database.Migrator().DropTable(m); dropErr != nil {
				// DROP 不存在表不视为错误
				t.Logf("DropTable 提示 %T: %v", m, dropErr)
			}
		}
		if migrateErr := database.AutoMigrate(models...); migrateErr != nil {
			t.Fatalf("AutoMigrate 失败: %v", migrateErr)
		}
	}

	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return database
}

// ensureProcTestDB 保证当前进程级测试库已存在（仅首次调用时创建）。
func ensureProcTestDB(t *testing.T) {
	procDBInit.Do(func() {
		name := fmt.Sprintf("user_db_test_%d", os.Getpid())
		maintDSN := dbnameRe.ReplaceAllString(getTestDSN(), "dbname=postgres")
		m, err := gorm.Open(postgres.Open(maintDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			procDBInitErr = err
			return
		}
		defer func() {
			if s, e := m.DB(); e == nil {
				_ = s.Close()
			}
		}()
		s, _ := m.DB()
		// 清理同 PID 上次（崩溃）残留，再创建干净独立库
		if _, e := s.Exec(fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, name)); e != nil {
			t.Logf("清理残留测试库提示 %s: %v", name, e)
		}
		if _, e := s.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, name)); e != nil {
			procDBInitErr = e
			return
		}
		procDBName = name
	})
	if procDBInitErr != nil {
		t.Fatalf("初始化进程级测试库失败: %v", procDBInitErr)
	}
}

// getTestDSN 读取测试数据库连接串
// 优先级：POSTGRES_TEST_DSN > 组合 POSTGRES_* 默认值（docker-compose-example.yml 默认值）
// 默认端口 8202 对应 docker-compose-example.yml 中 postgres-user 服务的 port=8202 配置
func getTestDSN() string {
	if v := os.Getenv("POSTGRES_TEST_DSN"); v != "" {
		return v
	}
	host := getEnvOr("POSTGRES_TEST_HOST", "127.0.0.1")
	port := getEnvOr("POSTGRES_TEST_PORT", "8202")
	user := getEnvOr("POSTGRES_TEST_USER", "admin")
	password := getEnvOr("POSTGRES_TEST_PASSWORD", "password123")
	dbname := getEnvOr("POSTGRES_TEST_DBNAME", "user_db_test")
	sslmode := getEnvOr("POSTGRES_TEST_SSLMODE", "disable")
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=Asia/Shanghai",
		host, port, user, password, dbname, sslmode)
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// maskedDSN 在 DSN 中隐藏密码，避免在测试日志中泄露
func maskedDSN(dsn string) string {
	out := []byte(dsn)
	// 简易处理：password=xxxx -> password=***
	for i := 0; i < len(out)-8; i++ {
		if string(out[i:i+9]) == "password=" {
			j := i + 9
			for j < len(out) && out[j] != ' ' {
				out[j] = '*'
				j++
			}
			break
		}
	}
	return string(out)
}
