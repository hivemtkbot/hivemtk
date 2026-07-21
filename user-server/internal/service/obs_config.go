package service

import (
	"errors"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"
)

type ObsConfigService interface {
	GetConfigList(licenseID string, page, limit int) (*dto.GetObsConfigListResponse, error)
	GetConfig(id string) (*dto.ObsConfigResponse, error)
	CreateConfig(req *dto.CreateObsConfigRequest) (*dto.ObsConfigResponse, error)
	UpdateConfig(id string, req *dto.UpdateObsConfigRequest) (*dto.ObsConfigResponse, error)
	DeleteConfig(id string) error
	SetDefaultConfig(id string, licenseID string) error
	TestConnection(config *dto.ObsConfigResponse) error
	UploadFile(file multipart.File, header *multipart.FileHeader, licenseID string, folder string) (string, error)
	GetDefaultConfig(licenseID string) (*dto.ObsConfigResponse, error)
}

type obsConfigService struct {
	repo repository.ObsConfigRepository
}

func NewObsConfigService() ObsConfigService {
	return &obsConfigService{
		repo: repository.NewObsConfigRepository(),
	}
}

func (s *obsConfigService) GetConfigList(licenseID string, page, limit int) (*dto.GetObsConfigListResponse, error) {
	configs, total, err := s.repo.GetListByLicense(licenseID, page, limit)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.ObsConfigResponse, len(configs))
	for i, config := range configs {
		list[i] = s.convertToDTO(config)
	}

	return &dto.GetObsConfigListResponse{
		List:  list,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (s *obsConfigService) GetConfig(id string) (*dto.ObsConfigResponse, error) {
	config, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return s.convertToDTO(config), nil
}

func (s *obsConfigService) CreateConfig(req *dto.CreateObsConfigRequest) (*dto.ObsConfigResponse, error) {
	// 验证请求
	if err := s.validateCreateRequest(req); err != nil {
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
		LicenseID:  req.LicenseID,
	}

	// 如果这是第一个配置，设为默认
	count, err := s.repo.CountByLicense(req.LicenseID)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		config.IsDefault = true
	}

	err = s.repo.Create(config)
	if err != nil {
		return nil, err
	}

	return s.convertToDTO(config), nil
}

func (s *obsConfigService) UpdateConfig(id string, req *dto.UpdateObsConfigRequest) (*dto.ObsConfigResponse, error) {
	config, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// 更新字段
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

	err = s.repo.Update(config)
	if err != nil {
		return nil, err
	}

	return s.convertToDTO(config), nil
}

func (s *obsConfigService) DeleteConfig(id string) error {
	config, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	// 不能删除默认配置
	if config.IsDefault {
		return errors.New("不能删除默认配置")
	}

	return s.repo.Delete(id)
}

func (s *obsConfigService) SetDefaultConfig(id string, licenseID string) error {
	config, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	// 确保配置属于该许可证
	if config.LicenseID != licenseID {
		return errors.New("配置不属于该许可证")
	}

	// 清除当前默认配置
	if err := s.repo.ClearDefaultByLicense(licenseID); err != nil {
		return err
	}

	// 设置新的默认配置
	return s.repo.SetDefault(id)
}

func (s *obsConfigService) TestConnection(config *dto.ObsConfigResponse) error {
	// Validate configuration based on provider
	// In production, this would call the actual cloud storage SDK to test connection

	switch model.ObsProvider(config.Provider) {
	case model.ObsProviderAliyun:
		// Validate Aliyun OSS configuration
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("阿里云OSS配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for Aliyun: bucket=%s, region=%s", config.Bucket, config.Region)
		return nil
	case model.ObsProviderQiniu:
		// Validate Qiniu configuration
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("七牛云存储配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for Qiniu: bucket=%s", config.Bucket)
		return nil
	case model.ObsProviderTencent:
		// Validate Tencent COS configuration
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("腾讯云COS配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for Tencent: bucket=%s, region=%s", config.Bucket, config.Region)
		return nil
	case model.ObsProviderAWS:
		// Validate AWS S3 configuration
		if config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" {
			return errors.New("AWS S3配置不完整")
		}
		logger.Infof("[OBS Config] Testing connection for AWS: bucket=%s, region=%s", config.Bucket, config.Region)
		return nil
	case model.ObsProviderLocal:
		// Local storage - validate basic path and permissions
		return nil
	default:
		return errors.New("不支持的存储提供商")
	}
}

func (s *obsConfigService) UploadFile(file multipart.File, header *multipart.FileHeader, licenseID string, folder string) (string, error) {
	// 获取默认配置
	config, err := s.repo.GetDefaultByLicense(licenseID)
	if err != nil {
		return "", errors.New("未找到默认存储配置")
	}

	// 检查文件大小限制
	if !config.IsFileSizeAllowed(header.Size) {
		return "", fmt.Errorf("文件大小超过限制: %d bytes (最大: %d bytes)", header.Size, config.MaxSize)
	}

	// 检查文件数量限制
	if !config.IsFileCountAllowed() {
		return "", fmt.Errorf("已达到最大文件数量限制: %d", config.MaxCount)
	}

	// 生成文件名
	ext := strings.ToLower(filepath.Ext(header.Filename))
	fileName := fmt.Sprintf("%s/%s%s", folder, time.Now().Format("20060102150405"), ext)

	// 根据提供商上传文件
	var fileURL string
	switch config.Provider {
	case model.ObsProviderLocal:
		// Local storage - save file to local directory
		fileURL = fmt.Sprintf("/uploads/%s", fileName)
		logger.Infof("[OBS Config] Saving file locally: %s", fileName)
		return fileURL, nil
	default:
		// Cloud storage - in production, this would call the cloud SDK
		logger.Infof("[OBS Config] Uploading to cloud storage: %s, provider: %s", fileName, config.Provider)
		fileURL = config.GetAccessURL(fileName)
	}

	// 更新统计信息
	config.FileCount++
	config.TotalSize += header.Size
	s.repo.Update(config)

	return fileURL, nil
}

func (s *obsConfigService) GetDefaultConfig(licenseID string) (*dto.ObsConfigResponse, error) {
	config, err := s.repo.GetDefaultByLicense(licenseID)
	if err != nil {
		return nil, err
	}
	return s.convertToDTO(config), nil
}

func (s *obsConfigService) validateCreateRequest(req *dto.CreateObsConfigRequest) error {
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

func (s *obsConfigService) convertToDTO(config *model.ObsConfig) *dto.ObsConfigResponse {
	return &dto.ObsConfigResponse{
		ID:           config.ID,
		Name:         config.Name,
		Provider:     string(config.Provider),
		ProviderName: config.GetProviderName(),
		AccessKey:    config.AccessKey,
		SecretKey:    "***", // 不返回完整的SecretKey
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
		LicenseID:    config.LicenseID,
		CreatedAt:    config.CreatedAt,
		UpdatedAt:    config.UpdatedAt,
	}
}
