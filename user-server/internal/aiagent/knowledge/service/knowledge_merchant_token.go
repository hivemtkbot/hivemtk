package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/aiagent/knowledge/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)

// CreateTokenRequest 创建 Token
type CreateTokenRequest struct {
	Name      string     `json:"name"`
	ProductID string     `json:"product_id"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedBy string     `json:"created_by"`
}

// CreateToken 创建 Token（返回明文 token 仅此一次）
func (s *KnowledgeMerchantService) CreateToken(ctx context.Context, req *CreateTokenRequest) (*model.KnowledgeAPIToken, error) {
	if req.Name == "" {
		return nil, errors.New("name 不能为空")
	}
	if req.ProductID == "" {
		return nil, errors.New("product_id 不能为空")
	}
	s.ensureReposFromDB()
	plain, err := generateToken()
	if err != nil {
		return nil, err
	}
	hashed := hashToken(plain)
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"read", "write"}
	}
	scopesJSON, _ := json.Marshal(scopes)
	tok := &model.KnowledgeAPIToken{
		Name:       req.Name,
		Token:      hashed,
		TokenPlain: plain,
		Scopes:     string(scopesJSON),
		ProductID:  req.ProductID,
		Enabled:    1,
		ExpiresAt:  req.ExpiresAt,
		CreatedBy:  req.CreatedBy,
	}
	if err := s.tokenRepo.Create(ctx, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// ListTokens 列出 Token
func (s *KnowledgeMerchantService) ListTokens(ctx context.Context, productID string) ([]model.KnowledgeAPIToken, error) {
	s.ensureReposFromDB()
	list, err := s.tokenRepo.ListByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Token = ""
		list[i].TokenPlain = ""
	}
	return list, nil
}

// RevokeToken 吊销 Token
func (s *KnowledgeMerchantService) RevokeToken(ctx context.Context, id uint64) error {
	s.ensureReposFromDB()
	return s.tokenRepo.DisableByID(ctx, id, 0)
}

// ValidateToken 校验 Token（外部系统使用）
func (s *KnowledgeMerchantService) ValidateToken(ctx context.Context, plain string) (*model.KnowledgeAPIToken, error) {
	if plain == "" {
		return nil, fmt.Errorf("%w: token 不能为空", utils.ErrUnauthorized)
	}
	if s.db == nil {
		return nil, errors.New("数据库未初始化")
	}
	s.ensureReposFromDB()
	hashed := hashToken(plain)
	tok, err := s.tokenRepo.FindByToken(ctx, hashed)
	if err != nil {
		return nil, fmt.Errorf("%w: token 无效或不存在", utils.ErrUnauthorized)
	}
	if tok.Enabled != 1 {
		return nil, fmt.Errorf("%w: token 已禁用", utils.ErrUnauthorized)
	}
	if tok.ExpiresAt != nil && tok.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("%w: token 已过期", utils.ErrUnauthorized)
	}
	tokID := tok.ID
	go func(id uint64) {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("knowledge_merchant: async IncrementUsage recovered from panic: %v", r)
			}
		}()
		if err := s.tokenRepo.IncrementUsage(context.Background(), id); err != nil {
			logger.Errorf("knowledge_merchant: async IncrementUsage failed, token_id=%d: %v", id, err)
		}
	}(tokID)
	return tok, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "kbg_" + hex.EncodeToString(b), nil
}

func hashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// GenerateToken 公开版 generateToken(供跨包测试使用)
func GenerateToken() (string, error) {
	return generateToken()
}

// HashToken 公开版 hashToken(供跨包测试使用)
func HashToken(plain string) string {
	return hashToken(plain)
}

func tokenHasScope(scopes string, target string) bool {
	var arr []string
	if err := json.Unmarshal([]byte(scopes), &arr); err != nil {
		return false
	}
	for _, s := range arr {
		if s == target || s == "*" {
			return true
		}
	}
	return false
}

// TokenHasScope 公开版 tokenHasScope(供跨包测试使用)
func TokenHasScope(scopes string, target string) bool {
	return tokenHasScope(scopes, target)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BoolToInt 公开版 boolToInt(供跨包测试使用)
func BoolToInt(b bool) int {
	return boolToInt(b)
}
