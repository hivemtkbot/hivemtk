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

const testJWTSecret = "test-jwt-secret-do-not-use-in-prod-32+chars"

var dotenvLoaded bool

func LoadDotEnv(path string) {
	if dotenvLoaded {
		return
	}
	dotenvLoaded = true
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if i := strings.Index(line, " #"); i > 0 {
			line = strings.TrimSpace(line[:i])
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" || os.Getenv(k) != "" {
			continue
		}
		_ = os.Setenv(k, v)
	}
}

func isTestProcess() bool {
	// Windows 下 go test 编译产物为 xxx.test.exe，需同时兼容两种后缀
	return strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe")
}

func loadJWTSecret() string {
	LoadDotEnv(".env")
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
	DataScope    string `json:"data_scope,omitempty"`
	DepartmentID uint   `json:"department_id,omitempty"`
	TeamID       uint   `json:"team_id,omitempty"`
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(j.config.SecretKey))
}

// ParseToken 解析JWT令牌
func (j *JWTUtils) ParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("无效的签名方法")
		}
		return []byte(j.config.SecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("无效的令牌")
}

// RefreshToken 刷新令牌
func (j *JWTUtils) RefreshToken(tokenString string) (string, error) {
	claims, err := j.ParseToken(tokenString)
	if err != nil {
		return "", err
	}

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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)

	return token.SignedString([]byte(j.config.SecretKey))
}
