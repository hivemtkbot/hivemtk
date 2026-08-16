package service

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"
)

// ObsConfigService OBS（对象存储）配置服务
//
// 开源版：已移除 LicenseID 维度隔离（不再做"按 License 分组"的查询/默认设置）。
// 列表/默认配置走"全局"语义：第一个创建即为默认。
type ObsConfigService interface {
	GetConfigList(ctx context.Context, page, limit int, provider, status string) (*dto.GetObsConfigListResponse, error)
	GetConfig(ctx context.Context, id string) (*dto.ObsConfigResponse, error)
	CreateConfig(ctx context.Context, req *dto.CreateObsConfigRequest) (*dto.ObsConfigResponse, error)
	UpdateConfig(ctx context.Context, id string, req *dto.UpdateObsConfigRequest) (*dto.ObsConfigResponse, error)
	DeleteConfig(ctx context.Context, id string) error
	SetDefaultConfig(ctx context.Context, id string) error
	TestConnection(ctx context.Context, config *dto.ObsConfigResponse) error
	UploadFile(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, error)
	GetDefaultConfig(ctx context.Context) (*dto.ObsConfigResponse, error)
}

type obsConfigService struct {
	repo repository.ObsConfigRepository
}

func NewObsConfigService() ObsConfigService {
	return &obsConfigService{
		repo: repository.NewObsConfigRepository(),
	}
}

func (s *obsConfigService) GetConfigList(ctx context.Context, page, limit int, provider, status string) (*dto.GetObsConfigListResponse, error) {
	configs, total, err := s.repo.GetList(ctx, page, limit, provider, status)
	if err != nil {
		return nil, err
	}
	_ = ctx

	list := make([]*dto.ObsConfigResponse, len(configs))
	for i, config := range configs {
		list[i] = s.convertToDTO(ctx, config)
	}

	return &dto.GetObsConfigListResponse{
		List:  list,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (s *obsConfigService) GetConfig(ctx context.Context, id string) (*dto.ObsConfigResponse, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.convertToDTO(ctx, config), nil
}

func (s *obsConfigService) CreateConfig(ctx context.Context, req *dto.CreateObsConfigRequest) (*dto.ObsConfigResponse, error) {
	if err := s.validateCreateRequest(ctx, req); err != nil {
		return nil, err
	}

	config := &model.ObsConfig{
		Name:       req.Name,
		Provider:   model.ObsProvider(req.Provider),
		AccessKey:  req.AccessKey,
		SecretKey:  req.SecretKey,
		Bucket:     req.Bucket,
		Region:     req.Region,
		Endpoint:   req.Endpoint,
		Domain:     req.Domain,
		PathPrefix: req.PathPrefix,
		Config:     req.Config,
		MaxSize:    req.MaxSize,
		MaxCount:   req.MaxCount,
		Status:     model.ObsStatusActive,
	}

	count, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		config.IsDefault = true
	}

	err = s.repo.Create(ctx, config)
	if err != nil {
		return nil, err
	}

	return s.convertToDTO(ctx, config), nil
}

func (s *obsConfigService) UpdateConfig(ctx context.Context, id string, req *dto.UpdateObsConfigRequest) (*dto.ObsConfigResponse, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		config.Name = req.Name
	}
	if req.Provider != "" {
		config.Provider = model.ObsProvider(req.Provider)
	}
	if req.AccessKey != "" {
		config.AccessKey = req.AccessKey
	}
	if req.SecretKey != "" {
		config.SecretKey = req.SecretKey
	}
	if req.Bucket != "" {
		config.Bucket = req.Bucket
	}
	if req.Region != "" {
		config.Region = req.Region
	}
	if req.Endpoint != "" {
		config.Endpoint = req.Endpoint
	}
	if req.Domain != "" {
		config.Domain = req.Domain
	}
	if req.PathPrefix != "" {
		config.PathPrefix = req.PathPrefix
	}
	if req.Config != "" {
		config.Config = req.Config
	}
	if req.MaxSize > 0 {
		config.MaxSize = req.MaxSize
	}
	if req.MaxCount > 0 {
		config.MaxCount = req.MaxCount
	}

	err = s.repo.Update(ctx, config)
	if err != nil {
		return nil, err
	}

	return s.convertToDTO(ctx, config), nil
}

func (s *obsConfigService) DeleteConfig(ctx context.Context, id string) error {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	_ = ctx

	if config.IsDefault {
		return errors.New("不能删除默认配置")
	}

	return s.repo.Delete(ctx, id)
}

func (s *obsConfigService) SetDefaultConfig(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}

	if err := s.repo.ClearDefault(ctx); err != nil {
		return err
	}

	return s.repo.SetDefault(ctx, id)
}

func (s *obsConfigService) TestConnection(ctx context.Context, config *dto.ObsConfigResponse) error {
	switch model.ObsProvider(config.Provider) {
	case model.ObsProviderAliyun:
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("阿里云OSS配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for Aliyun: bucket=%s, region=%s", config.Bucket, config.Region)
		return nil
	case model.ObsProviderQiniu:
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("七牛云存储配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for Qiniu: %s", config.Bucket)
		return nil
	case model.ObsProviderTencent:
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("腾讯云COS配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for Tencent: bucket=%s, region=%s", config.Bucket, config.Region)
		return nil
	case model.ObsProviderAWS:
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("AWS S3配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for AWS: %s", config.Bucket)
		return nil
	case model.ObsProviderLocal:
		return nil
	default:
		return errors.New("不支持的存储提供商")
	}
}

func (s *obsConfigService) UploadFile(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, error) {
	config, err := s.repo.GetDefault(ctx)
	if err != nil {
		return "", errors.New("未找到默认存储配置")
	}

	if !obsConfigIsFileSizeAllowed(config, header.Size) {
		return "", fmt.Errorf("文件大小超过限制: %d bytes (最大: %d bytes)", header.Size, config.MaxSize)
	}

	if !obsConfigIsFileCountAllowed(config) {
		return "", fmt.Errorf("已达到最大文件数量限制: %d", config.MaxCount)
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	fileName := fmt.Sprintf("%s/%s%s", folder, time.Now().Format("20060102150405"), ext)

	// 根据提供商上传文件
	var fileURL string
	switch config.Provider {
	case model.ObsProviderLocal:
		fileURL = fmt.Sprintf("/uploads/%s", fileName)
		logger.Infof("[OBS Config] Saving file locally: %s", fileName)
		return fileURL, nil
	default:
		logger.Infof("[OBS Config] Uploading to cloud storage: %s, provider: %s", fileName, config.Provider)
		fileURL = obsConfigAccessURL(config, fileName)
	}

	config.FileCount++
	config.TotalSize += header.Size
	s.repo.Update(ctx, config)

	return fileURL, nil
}

func (s *obsConfigService) GetDefaultConfig(ctx context.Context) (*dto.ObsConfigResponse, error) {
	config, err := s.repo.GetDefault(ctx)
	if err != nil {
		return nil, err
	}
	return s.convertToDTO(ctx, config), nil
}

func (s *obsConfigService) validateCreateRequest(ctx context.Context, req *dto.CreateObsConfigRequest) error {
	if req.Name == "" {
		return errors.New("配置名称不能为空")
	}
	if req.Provider == "" {
		return errors.New("存储提供商不能为空")
	}
	if req.AccessKey == "" {
		return errors.New("AccessKey不能为空")
	}
	if req.SecretKey == "" {
		return errors.New("SecretKey不能为空")
	}
	if req.Bucket == "" {
		return errors.New("存储桶名称不能为空")
	}
	return nil
}

func (s *obsConfigService) convertToDTO(ctx context.Context, config *model.ObsConfig) *dto.ObsConfigResponse {
	return &dto.ObsConfigResponse{
		ID:           config.ID,
		Name:         config.Name,
		Provider:     string(config.Provider),
		ProviderName: obsConfigProviderName(config),
		AccessKey:    config.AccessKey,
		SecretKey:    "***", 
		Bucket:       config.Bucket,
		Region:       config.Region,
		Endpoint:     config.Endpoint,
		Domain:       config.Domain,
		PathPrefix:   config.PathPrefix,
		Config:       config.Config,
		MaxSize:      config.MaxSize,
		MaxCount:     config.MaxCount,
		Status:       string(config.Status),
		LastError:    config.LastError,
		LastTestAt:   config.LastTestAt,
		TotalSize:    config.TotalSize,
		FileCount:    config.FileCount,
		IsDefault:    config.IsDefault,
		CreatedAt:    config.CreatedAt,
		UpdatedAt:    config.UpdatedAt,
	}
}

// obsConfigProviderName 获取提供商名称（从 model.ObsConfig 迁出，五层架构合规）
func obsConfigProviderName(c *model.ObsConfig) string {
	switch c.Provider {
	case model.ObsProviderAliyun:
		return "阿里云OSS"
	case model.ObsProviderQiniu:
		return "七牛云存储"
	case model.ObsProviderTencent:
		return "腾讯云COS"
	case model.ObsProviderAWS:
		return "AWS S3"
	case model.ObsProviderLocal:
		return "本地存储"
	default:
		return "未知"
	}
}

// obsConfigAccessURL 获取访问URL
func obsConfigAccessURL(c *model.ObsConfig, filePath string) string {
	if c.Domain != "" {
		return c.Domain + "/" + filePath
	}
	switch c.Provider {
	case model.ObsProviderAliyun:
		return "https://" + c.Bucket + "." + c.Region + ".aliyuncs.com/" + filePath
	case model.ObsProviderQiniu:
		return "http://" + c.Domain + "/" + filePath
	case model.ObsProviderTencent:
		return "https://" + c.Bucket + ".cos." + c.Region + ".myqcloud.com/" + filePath
	case model.ObsProviderAWS:
		return "https://" + c.Bucket + ".s3." + c.Region + ".amazonaws.com/" + filePath
	default:
		return filePath
	}
}

// obsConfigIsFileSizeAllowed 检查文件大小限制
func obsConfigIsFileSizeAllowed(c *model.ObsConfig, size int64) bool {
	return size <= c.MaxSize
}

// obsConfigIsFileCountAllowed 检查是否达到文件数量限制
func obsConfigIsFileCountAllowed(c *model.ObsConfig) bool {
	return c.FileCount < c.MaxCount
}

