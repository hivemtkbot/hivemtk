package utils

import (
	"errors"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"

	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig JWT配置
type JWTConfig struct {
	SecretKey    string
	ExpiresHours int
	Issuer       string
}

// testJWTSecret 仅在 go test 进程下作为最后兜底使用的固定测试密钥。
// 绝不用于生产：loadJWTSecret 在检测到非测试模式且环境变量未配置时仍会 panic。
// 长度 ≥ 32 字符以满足默认配置强度。
const testJWTSecret = "test-jwt-secret-do-not-use-in-prod-32+chars"

// isTestProcess 判断当前进程是否由 `go test` 启动。
// 依据：go test 编译出的二进制文件名后缀为 ".test"（如 pkg.test）。
// 这是在 init 阶段唯一可靠的判断方法（flag.Lookup 此时还未注册）。
func isTestProcess() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

// loadJWTSecret 从环境变量加载 JWT 密钥。
// 规则：
//  1. 优先使用 USER_JWT_SECRET（生产密钥）
//  2. 兼容 JWT_SECRET（部分旧中间件使用）
//  3. 二者皆未配置：若当前是 go test 进程，使用测试 fallback 密钥并打 warning；
//     否则 panic（生产环境必须显式配置）
func loadJWTSecret() string {
	secret := os.Getenv("USER_JWT_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if secret == "" {
		if isTestProcess() {
			logger.Warnf("[SECURITY][WARN] USER_JWT_SECRET/JWT_SECRET 未配置，go test 进程使用固定测试密钥（仅用于单元测试）")
			return testJWTSecret
		}
		panic("[SECURITY] USER_JWT_SECRET 未配置或长度不足 32 字符，禁止使用硬编码密钥")
	}
	if len(secret) < 32 {
		if isTestProcess() {
			logger.Warnf("[SECURITY][WARN] JWT secret 长度不足 32 字符，go test 进程继续使用该值（仅用于单元测试）")
			return secret
		}
		panic("[SECURITY] USER_JWT_SECRET 未配置或长度不足 32 字符，禁止使用硬编码密钥")
	}
	return secret
}

// DefaultJWTConfig 默认 JWT 配置（从环境变量加载）
var DefaultJWTConfig = JWTConfig{
	SecretKey:    loadJWTSecret(),
	ExpiresHours: 24,
	Issuer:       "marketing-system",
}

// CustomClaims 自定义JWT声明
type CustomClaims struct {
	UserID       uint   `json:"user_id,string"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	DataScope    string `json:"data_scope,omitempty"`    // 数据范围 all/department/team/self
	DepartmentID uint   `json:"department_id,omitempty"` // 部门 ID
	TeamID       uint   `json:"team_id,omitempty"`       // 团队 ID
	jwt.RegisteredClaims
}

// JWTUtils JWT工具类
type JWTUtils struct {
	config JWTConfig
}

// NewJWTUtils 创建JWT工具实例
func NewJWTUtils(config JWTConfig) *JWTUtils {
	return &JWTUtils{config: config}
}

// GenerateToken 生成JWT令牌
func (j *JWTUtils) GenerateToken(userID uint, username, role string) (string, error) {
	return j.GenerateTokenWithScope(userID, username, role, "", 0, 0)
}

// GenerateTokenWithScope 生成带数据范围的 JWT 令牌（行级权限）
func (j *JWTUtils) GenerateTokenWithScope(userID uint, username, role, dataScope string, departmentID, teamID uint) (string, error) {
	// 创建声明
	claims := CustomClaims{
		UserID:       userID,
		Username:     username,
		Role:         role,
		DataScope:    dataScope,
		DepartmentID: departmentID,
		TeamID:       teamID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(j.config.ExpiresHours))),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    j.config.Issuer,
		},
	}

	// 创建令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名令牌
	return token.SignedString([]byte(j.config.SecretKey))
}

// ParseToken 解析JWT令牌
func (j *JWTUtils) ParseToken(tokenString string) (*CustomClaims, error) {
	// 解析令牌
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (any, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("无效的签名方法")
		}
		return []byte(j.config.SecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	// 验证令牌并获取声明
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("无效的令牌")
}

// RefreshToken 刷新令牌
func (j *JWTUtils) RefreshToken(tokenString string) (string, error) {
	// 解析旧令牌
	claims, err := j.ParseToken(tokenString)
	if err != nil {
		return "", err
	}

	// 创建新声明（保留 data_scope 等扩展字段）
	newClaims := CustomClaims{
		UserID:       claims.UserID,
		Username:     claims.Username,
		Role:         claims.Role,
		DataScope:    claims.DataScope,
		DepartmentID: claims.DepartmentID,
		TeamID:       claims.TeamID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * time.Duration(j.config.ExpiresHours))),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    j.config.Issuer,
		},
	}

	// 创建新令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)

	// 签名新令牌
	return token.SignedString([]byte(j.config.SecretKey))
}
