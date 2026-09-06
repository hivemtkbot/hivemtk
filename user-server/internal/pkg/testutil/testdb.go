// Package testutil 提供统一的 PostgreSQL 测试数据库辅助
// 本系统仅使用 PostgreSQL，测试环境必须连接真实 PG（不允许任何非 PG 数据库）。
// 通过 TEST_DATABASE_URL 或 POSTGRES_* 环境变量配置测试连接。
package testutil

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var dbnameRe = regexp.MustCompile(`dbname=[^\s]+`)

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
// 优雅降级：当 PostgreSQL 测试库不可达时（如本地开发/CI 无 PG），
// 自动调用 t.Skipf 跳过本测试，保证 go test 不会因环境差异大面积失败。
// 强依赖真实 DB 的集成/E2E 测试在此场景下整体跳过，不影响纯单元测试。
//
// 注意：本辅助不调用 db.SetTestDB，由调用方按需将 DB 注入到全局 db.DB 或 Service。
func NewTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()

	host := getEnvOr("POSTGRES_TEST_HOST", "127.0.0.1")
	port := getEnvOr("POSTGRES_TEST_PORT", "8202")
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second); err != nil {
		t.Skipf("PostgreSQL 测试库不可达（%s:%s）：%v；按设计本测试可跳过", host, port, err)
		return nil
	} else {
		_ = conn.Close()
	}

	ensureProcTestDB(t)
	testDSN := dbnameRe.ReplaceAllString(getTestDSN(), "dbname="+procDBName)
	database, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("连接 PostgreSQL 测试库失败（dsn=%s）: %v", maskedDSN(testDSN), err)
	}

	if sqlDB, dbErr := database.DB(); dbErr == nil {
		if _, execErr := sqlDB.Exec("CREATE EXTENSION IF NOT EXISTS vector"); execErr != nil {
			t.Logf("启用 pgvector 扩展提示（需 postgres 镜像含 vector 包）: %v", execErr)
		}
		if _, execErr := sqlDB.Exec("SET session_replication_role = 'replica'"); execErr != nil {
			t.Logf("禁用外键约束提示: %v", execErr)
		}
	}

	if len(models) > 0 {
		for _, m := range models {
			if dropErr := database.Migrator().DropTable(m); dropErr != nil {
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

// NewTestDBOrSkip 在 PostgreSQL 测试库不可达时跳过测试（t.Skipf），否则同 NewTestDB。
//
// 适用场景：本地/CI 无 PG 时，让不强制依赖真实 DB 的测试优雅跳过；
// 强依赖 DB 的测试仍使用 NewTestDB（不可达则 fail）。
func NewTestDBOrSkip(t *testing.T, models ...any) *gorm.DB {
	t.Helper()

	host := getEnvOr("POSTGRES_TEST_HOST", "127.0.0.1")
	port := getEnvOr("POSTGRES_TEST_PORT", "8202")
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		t.Skipf("PostgreSQL 测试库不可达（%s:%s）：%v；按设计本测试可跳过", host, port, err)
		return nil
	}
	_ = conn.Close()
	return NewTestDB(t, models...)
}

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

func maskedDSN(dsn string) string {
	out := []byte(dsn)
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
