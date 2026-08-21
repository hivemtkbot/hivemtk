package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/geo/dto"
	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
)

// 发布状态常量
const (
	PublishStatusPending = "pending"
	PublishStatusCopied  = "copied"
	PublishStatusFailed  = "failed"
)

// platformDef 内置平台定义
type platformDef struct {
	Name        string
	DisplayName string
	URL         string
	Path        string
	Branch      string
	AuthType    string
	Enabled     bool
}

// defaultPlatforms 迁移自 AIGEOTOOLS DefaultCopyPlatforms
func defaultPlatforms() []platformDef {
	return []platformDef{
		{Name: "github_readme", DisplayName: "GitHub README", URL: "https://github.com", Path: "README.md", Branch: "main", AuthType: "token", Enabled: true},
		{Name: "github_blog", DisplayName: "GitHub Blog", URL: "https://github.com", Path: "blog/", Branch: "main", AuthType: "token", Enabled: true},
		{Name: "juejin", DisplayName: "掘金", URL: "https://juejin.cn", AuthType: "cookie", Enabled: true},
		{Name: "zhihu", DisplayName: "知乎", URL: "https://zhihu.com", AuthType: "cookie", Enabled: true},
		{Name: "csdn", DisplayName: "CSDN", URL: "https://csdn.net", AuthType: "cookie", Enabled: true},
		{Name: "weibo", DisplayName: "微博", URL: "https://weibo.com", AuthType: "oauth", Enabled: true},
		{Name: "xiaohongshu", DisplayName: "小红书", URL: "https://xiaohongshu.com", AuthType: "cookie", Enabled: true},
		{Name: "douyin", DisplayName: "抖音", URL: "https://douyin.com", AuthType: "oauth", Enabled: true},
		{Name: "toutiao", DisplayName: "今日头条", URL: "https://toutiao.com", AuthType: "cookie", Enabled: true},
		{Name: "medium", DisplayName: "Medium", URL: "https://medium.com", AuthType: "oauth", Enabled: true},
		{Name: "wordpress", DisplayName: "WordPress", AuthType: "xmlrpc", Enabled: true},
		{Name: "custom", DisplayName: "自定义平台", AuthType: "custom", Enabled: false},
	}
}

func findPlatform(name string) *platformDef {
	defs := defaultPlatforms()
	for i := range defs {
		if defs[i].Name == name {
			return &defs[i]
		}
	}
	return nil
}

// PlatformService GEO 平台同步发布服务（迁移自 AIGEOTOOLS platform/service.go）
type PlatformService struct {
	accountRepo repository.GeoPlatformAccountRepository
	recordRepo  repository.GeoPublishRecordRepository
	articleRepo repository.GeoArticleRepository
	httpClient  *http.Client
}

// NewPlatformService 创建平台同步发布服务
func NewPlatformService(
	ar repository.GeoPlatformAccountRepository,
	rr repository.GeoPublishRecordRepository,
	gr repository.GeoArticleRepository,
) *PlatformService {
	return &PlatformService{
		accountRepo: ar,
		recordRepo:  rr,
		articleRepo: gr,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListPlatforms 获取支持的平台列表
func (s *PlatformService) ListPlatforms(ctx context.Context) []dto.PlatformInfo {
	defs := defaultPlatforms()
	result := make([]dto.PlatformInfo, 0, len(defs))
	for _, p := range defs {
		result = append(result, dto.PlatformInfo{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			URL:         p.URL,
			AuthType:    p.AuthType,
			Enabled:     p.Enabled,
		})
	}
	return result
}

// validateAccount 校验账号入参（迁移自 ValidateAccount）
func (s *PlatformService) validateAccount(req *dto.SavePlatformAccountRequest) error {
	switch req.Platform {
	case "github_readme", "github_blog":
		if (req.Credentials == nil || (req.Credentials["api_key"] == "" && req.Credentials["access_token"] == "")) &&
			os.Getenv("GEO_GITHUB_TOKEN") == "" {
			return fmt.Errorf("GitHub 平台需要提供 access_token 或配置 GEO_GITHUB_TOKEN")
		}
	case "wordpress":
		if req.Credentials == nil || req.Credentials["api_key"] == "" {
			return fmt.Errorf("WordPress 需要提供 api_key")
		}
	}
	return nil
}

// SaveAccount 新增/更新平台账号（同平台同名账号覆盖更新）
func (s *PlatformService) SaveAccount(ctx context.Context, req *dto.SavePlatformAccountRequest) (*model.GeoPlatformAccount, error) {
	if err := s.validateAccount(req); err != nil {
		return nil, err
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	configJSON := ""
	if len(req.Credentials) > 0 {
		b, err := json.Marshal(req.Credentials)
		if err != nil {
			return nil, fmt.Errorf("序列化凭证失败: %w", err)
		}
		configJSON = string(b)
	}

	// 同平台+同名视为更新
	existing, total, err := s.accountRepo.GetList(req.Platform, 1, 100)
	if err == nil {
		for _, acc := range existing {
			if acc.AccountName == req.AccountName && total > 0 {
				acc.Status = status
				acc.Config = configJSON
				if req.AccountID != "" {
					acc.AccountID = req.AccountID
				}
				if err := s.accountRepo.Update(acc); err != nil {
					return nil, err
				}
				return acc, nil
			}
		}
	}

	account := &model.GeoPlatformAccount{
		Platform:    req.Platform,
		AccountID:   req.AccountID,
		AccountName: req.AccountName,
		Status:      status,
		Config:      configJSON,
	}
	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}
	return account, nil
}

// ListAccounts 平台账号列表（分页）
func (s *PlatformService) ListAccounts(ctx context.Context, platform string, page, limit int) ([]*model.GeoPlatformAccount, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.accountRepo.GetList(platform, page, limit)
}

// DeleteAccount 删除平台账号
func (s *PlatformService) DeleteAccount(ctx context.Context, id string) error {
	return s.accountRepo.Delete(id)
}

// Publish 发布文章到平台（迁移自 Publish + publishGitHub）
func (s *PlatformService) Publish(ctx context.Context, req *dto.PublishRequest) (*dto.PublishResponse, error) {
	// 加载文章内容
	article, err := s.articleRepo.GetByID(req.ArticleID)
	if err != nil {
		return nil, fmt.Errorf("文章不存在: %w", err)
	}

	def := findPlatform(req.Platform)

	// 补全 path / branch / commit message
	path := req.Path
	if path == "" && def != nil {
		path = def.Path
	}
	if path == "" {
		path = req.Filename
	}
	if path == "" {
		path = "README.md"
	}
	branch := req.Branch
	if branch == "" && def != nil {
		branch = def.Branch
	}
	if branch == "" {
		branch = "main"
	}
	commitMsg := req.CommitMsg
	if commitMsg == "" {
		commitMsg = "update " + path
	}

	switch req.Platform {
	case "github_readme", "github_blog", "github":
		return s.publishGitHub(ctx, article, req, path, branch, commitMsg)
	}

	// 非 GitHub 平台：登记待发布记录（复制模式，由用户手动粘贴）
	publishID := fmt.Sprintf("%s_%d", req.Platform, time.Now().UnixMilli())
	record := &model.GeoPublishRecord{
		ArticleID: article.ID,
		Platform:  req.Platform,
		Status:    PublishStatusPending,
	}
	if err := s.recordRepo.Create(record); err != nil {
		return nil, err
	}
	url := ""
	if def != nil {
		url = def.URL
	}
	return &dto.PublishResponse{
		ID:           record.ID,
		Status:       PublishStatusPending,
		PublishedURL: url,
		Message:      publishID + ": 请手动粘贴内容到目标平台",
	}, nil
}

// publishGitHub 通过 GitHub Contents API 发布
func (s *PlatformService) publishGitHub(
	ctx context.Context,
	article *model.GeoArticle,
	req *dto.PublishRequest,
	path, branch, commitMsg string,
) (*dto.PublishResponse, error) {
	token := req.AuthToken
	if token == "" {
		token = os.Getenv("GEO_GITHUB_TOKEN")
	}
	user := os.Getenv("GEO_GITHUB_USER")
	repo := req.Repo
	if token == "" || user == "" {
		return nil, fmt.Errorf("未配置 GitHub 凭证（GEO_GITHUB_TOKEN/GEO_GITHUB_USER）")
	}
	if repo == "" {
		return nil, fmt.Errorf("缺少 repo 参数")
	}

	apiBase := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", user, repo, path)

	// 查询已有文件 SHA（存在则为更新）
	var sha string
	if getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiBase, nil); getReq != nil {
		getReq.Header.Set("Authorization", "Bearer "+token)
		getReq.Header.Set("Accept", "application/vnd.github+json")
		if resp, err := s.httpClient.Do(getReq); err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var meta struct {
					SHA string `json:"sha"`
				}
				if json.Unmarshal(body, &meta) == nil {
					sha = meta.SHA
				}
			}
		}
	}

	payload := map[string]any{
		"message": commitMsg,
		"content": base64.StdEncoding.EncodeToString([]byte(article.Content)),
		"branch":  branch,
	}
	if sha != "" {
		payload["sha"] = sha
	}
	body, _ := json.Marshal(payload)
	putReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, apiBase, bytes.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer "+token)
	putReq.Header.Set("Accept", "application/vnd.github+json")
	putReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(putReq)
	if err != nil {
		s.saveFailedRecord(article.ID, req.Platform, err.Error())
		return nil, fmt.Errorf("GitHub 请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(respBody))
		s.saveFailedRecord(article.ID, req.Platform, msg)
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, msg)
	}

	var r struct {
		Content struct {
			HTMLURL string `json:"html_url"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		s.saveFailedRecord(article.ID, req.Platform, "解析 GitHub 响应失败: "+err.Error())
		return nil, fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}

	record := &model.GeoPublishRecord{
		ArticleID:    article.ID,
		Platform:     req.Platform,
		Status:       PublishStatusCopied,
		PublishedURL: r.Content.HTMLURL,
		PublishedAt:  time.Now(),
	}
	if err := s.recordRepo.Create(record); err != nil {
		return nil, err
	}
	return &dto.PublishResponse{
		ID:           record.ID,
		Status:       PublishStatusCopied,
		PublishedURL: r.Content.HTMLURL,
	}, nil
}

// saveFailedRecord 记录失败发布
func (s *PlatformService) saveFailedRecord(articleID, platform, errMsg string) {
	_ = s.recordRepo.Create(&model.GeoPublishRecord{
		ArticleID: articleID,
		Platform:  platform,
		Status:    PublishStatusFailed,
	})
}

// ListPublishRecords 发布记录列表（分页）
func (s *PlatformService) ListPublishRecords(ctx context.Context, articleID, platform string, page, limit int) ([]*model.GeoPublishRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.recordRepo.GetList(articleID, platform, page, limit)
}
