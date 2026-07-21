package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/httpclient"
	"marketing/internal/repository"
)

// SmsService 短信服务接口
type SmsService interface {
	// 配置相关
	GetConfig() (*dto.SmsConfigResponse, error)
	SaveConfig(req *dto.SmsConfigRequest) error
	// IsProviderConfigured 检查指定提供商的凭证是否已配置（非空）
	IsProviderConfigured(provider string) (bool, error)

	// 短信记录相关
	GetSmsList(req *dto.SmsListRequest) ([]*model.SmsRecord, int64, error)
	GetSmsByID(id uint) (*model.SmsRecord, error)
	SendSms(req *dto.SmsSendRequest) error
	ResendSms(id uint) error

	// 草稿相关
	GetDraftList(req *dto.SmsDraftListRequest) ([]*model.SmsDraft, int64, error)
	GetDraftByID(id uint) (*model.SmsDraft, error)
	CreateDraft(req *dto.SmsDraftCreateRequest) error
	UpdateDraft(id uint, req *dto.SmsDraftUpdateRequest) error
	DeleteDraft(id uint) error
	SendDraft(id uint, phone string) error

	// 任务相关
	GetJobList(req *dto.SmsJobListRequest) ([]*model.SmsJob, int64, error)
	GetJobByID(id uint) (*model.SmsJob, error)
	CreateJob(req *dto.SmsJobCreateRequest) error
	PauseJob(id uint) error
	ResumeJob(id uint) error
	StopJob(id uint) error
	DeleteJob(id uint) error
	GetJobRecords(id uint, page, limit int) ([]*model.SmsJobDetail, int64, error)
}

// smsService 短信服务实现
type smsService struct {
	repo repository.SmsRepository
}

// NewSmsService 创建短信服务
func NewSmsService(repo repository.SmsRepository) SmsService {
	return &smsService{repo: repo}
}

// GetConfig 获取短信配置
func (s *smsService) GetConfig() (*dto.SmsConfigResponse, error) {
	// 获取基本配置
	config, err := s.repo.GetConfig()
	if err != nil {
		return nil, err
	}

	// 获取阿里云配置
	aliyunConfig, err := s.repo.GetAliyunConfig()
	if err != nil {
		return nil, err
	}

	// 获取腾讯云配置
	tencentConfig, err := s.repo.GetTencentConfig()
	if err != nil {
		return nil, err
	}

	// 获取华为云配置
	huaweiConfig, err := s.repo.GetHuaweiConfig()
	if err != nil {
		return nil, err
	}

	return &dto.SmsConfigResponse{
		DefaultProvider: config.DefaultProvider,
		RateLimit:       config.RateLimit,
		DailyLimit:      config.DailyLimit,
		RetryTimes:      config.RetryTimes,
		Aliyun: dto.SmsAliyunConfig{
			AccessKeyId:     aliyunConfig.AccessKeyID,
			AccessKeySecret: aliyunConfig.AccessKeySecret,
			SignName:        aliyunConfig.SignName,
		},
		Tencent: dto.SmsTencentConfig{
			SecretId:  tencentConfig.SecretID,
			SecretKey: tencentConfig.SecretKey,
			AppId:     tencentConfig.AppID,
			SignName:  tencentConfig.SignName,
		},
		Huawei: dto.SmsHuaweiConfig{
			AppKey:    huaweiConfig.AppKey,
			AppSecret: huaweiConfig.AppSecret,
			Sender:    huaweiConfig.Sender,
			Signature: huaweiConfig.Signature,
		},
	}, nil
}

// SaveConfig 保存短信配置
func (s *smsService) SaveConfig(req *dto.SmsConfigRequest) error {
	// 保存基本配置
	config := &model.SmsConfig{
		DefaultProvider: req.DefaultProvider,
		RateLimit:       req.RateLimit,
		DailyLimit:      req.DailyLimit,
		RetryTimes:      req.RetryTimes,
	}
	if err := s.repo.SaveConfig(config); err != nil {
		return err
	}

	// 保存阿里云配置
	aliyunConfig := &model.SmsAliyunConfig{
		AccessKeyID:     req.Aliyun.AccessKeyId,
		AccessKeySecret: req.Aliyun.AccessKeySecret,
		SignName:        req.Aliyun.SignName,
	}
	if err := s.repo.SaveAliyunConfig(aliyunConfig); err != nil {
		return err
	}

	// 保存腾讯云配置
	tencentConfig := &model.SmsTencentConfig{
		SecretID:  req.Tencent.SecretId,
		SecretKey: req.Tencent.SecretKey,
		AppID:     req.Tencent.AppId,
		SignName:  req.Tencent.SignName,
	}
	if err := s.repo.SaveTencentConfig(tencentConfig); err != nil {
		return err
	}

	// 保存华为云配置
	huaweiConfig := &model.SmsHuaweiConfig{
		AppKey:    req.Huawei.AppKey,
		AppSecret: req.Huawei.AppSecret,
		Sender:    req.Huawei.Sender,
		Signature: req.Huawei.Signature,
	}
	return s.repo.SaveHuaweiConfig(huaweiConfig)
}

// IsProviderConfigured 检查指定提供商的凭证是否已配置（非空）
func (s *smsService) IsProviderConfigured(provider string) (bool, error) {
	switch provider {
	case "aliyun":
		cfg, err := s.repo.GetAliyunConfig()
		if err != nil {
			return false, err
		}
		return cfg.AccessKeyID != "" && cfg.AccessKeySecret != "", nil
	case "tencent":
		cfg, err := s.repo.GetTencentConfig()
		if err != nil {
			return false, err
		}
		return cfg.SecretID != "" && cfg.SecretKey != "", nil
	case "huawei":
		cfg, err := s.repo.GetHuaweiConfig()
		if err != nil {
			return false, err
		}
		return cfg.AppKey != "" && cfg.AppSecret != "", nil
	default:
		return false, fmt.Errorf("unknown sms provider: %s", provider)
	}
}

// GetSmsList 获取短信列表
func (s *smsService) GetSmsList(req *dto.SmsListRequest) ([]*model.SmsRecord, int64, error) {
	return s.repo.GetSmsList(req.Page, req.Limit, req.Phone, req.Status, req.StartDate, req.EndDate)
}

// GetSmsByID 根据ID获取短信
func (s *smsService) GetSmsByID(id uint) (*model.SmsRecord, error) {
	return s.repo.GetSmsByID(id)
}

// SendSms 发送短信
func (s *smsService) SendSms(req *dto.SmsSendRequest) error {
	// 获取默认提供商
	config, err := s.repo.GetConfig()
	if err != nil {
		return err
	}

	// 创建短信记录
	record := &model.SmsRecord{
		Phone:    req.Phone,
		Content:  req.Content,
		Provider: config.DefaultProvider,
		Status:   "sending",
	}

	if err := s.repo.CreateSmsRecord(record); err != nil {
		return err
	}

	// 真实调用三方 SMS API
	sentTime, errCode, errMsg, err := s.dispatchToProvider(req.Phone, req.Content, config.DefaultProvider)
	if err != nil {
		record.Status = "failed"
		record.ErrorCode = errCode
		record.ErrorMsg = errMsg
		record.SendTime = &time.Time{}
		_ = s.repo.UpdateSmsRecord(record)
		return fmt.Errorf("send sms failed: %w", err)
	}

	record.SendTime = &sentTime
	record.Status = "sent"

	// 更新记录
	return s.repo.UpdateSmsRecord(record)
}

// dispatchToProvider 调度到具体 SMS 提供商
func (s *smsService) dispatchToProvider(phone, content, provider string) (time.Time, string, string, error) {
	switch provider {
	case "aliyun":
		return s.sendAliyun(phone, content)
	case "tencent":
		return s.sendTencent(phone, content)
	case "huawei":
		return s.sendHuawei(phone, content)
	default:
		return time.Time{}, "", "", fmt.Errorf("unknown sms provider: %s", provider)
	}
}

// sendAliyun 通过阿里云发送短信
func (s *smsService) sendAliyun(phone, content string) (time.Time, string, string, error) {
	cfg, err := s.repo.GetAliyunConfig()
	if err != nil {
		return time.Time{}, "", "", err
	}
	if cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" {
		return time.Time{}, "", "", errors.New("aliyun sms config missing")
	}

	// 阿里云短信 v1 API：使用简单的 HTTP 调用 + 签名
	// 生产环境建议使用官方 SDK（aliyun-go-sdk-dysmsapi），这里采用 REST 直调以避免引入额外依赖
	apiURL := "https://dysmsapi.aliyuncs.com/"

	params := url.Values{}
	params.Set("PhoneNumbers", phone)
	params.Set("SignName", cfg.SignName)
	params.Set("TemplateCode", "SMS_000000001") // 通用模板码（应由 config 传入）
	params.Set("TemplateParam", `{"content":"`+escapeJSON(content)+`"}`)
	params.Set("AccessKeyId", cfg.AccessKeyID)
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("Format", "JSON")
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", randomNonce())
	params.Set("Action", "SendSms")
	params.Set("Version", "2017-05-25")
	params.Set("RegionId", "cn-hangzhou")

	// 阿里云签名：按参数排序拼接后 HMAC-SHA1
	_ = specialURLEncode(apiURL) + "&" + specialURLEncode(percentEncode(params.Encode()))
	mac := hmac.New(sha256.New, []byte(cfg.AccessKeySecret+"&"))
	// 阿里云新版短信支持 HMAC-SHA256，但为兼容使用 SHA1 签名（v1 API 规定）
	_ = mac
	sign := signAliyun(params, cfg.AccessKeySecret)
	params.Set("Signature", sign)

	// 使用统一全局 HTTP 客户端（带超时 + 连接池），并做有限重试（网络错误 / 5xx）
	client := httpclient.NewWithTimeout(30 * time.Second)
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt <= 2; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		resp, lastErr = client.PostForm(apiURL, params)
		if lastErr != nil {
			continue
		}
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("aliyun sms http status %d", resp.StatusCode)
			continue
		}
		break
	}
	if resp == nil {
		return time.Time{}, "", "", fmt.Errorf("aliyun sms request failed after retries: %w", lastErr)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Code      string `json:"Code"`
		Message   string `json:"Message"`
		RequestID string `json:"RequestId"`
		BizID     string `json:"BizId"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return time.Time{}, "", "", fmt.Errorf("decode aliyun response: %w", err)
	}
	if result.Code != "OK" {
		return time.Time{}, result.Code, result.Message, fmt.Errorf("aliyun sms error: %s", result.Message)
	}
	return time.Now(), result.Code, result.Message, nil
}

// sendTencent 通过腾讯云发送短信
func (s *smsService) sendTencent(phone, content string) (time.Time, string, string, error) {
	cfg, err := s.repo.GetTencentConfig()
	if err != nil {
		return time.Time{}, "", "", err
	}
	if cfg.SecretID == "" || cfg.SecretKey == "" {
		return time.Time{}, "", "", errors.New("tencent sms config missing")
	}

	// 腾讯云短信 v3 API：使用 TC3-HMAC-SHA256 签名
	apiHost := "sms.tencentcloudapi.com"
	apiURL := "https://" + apiHost + "/"
	service := "sms"
	version := "2021-01-11"
	action := "SendSms"
	algorithm := "TC3-HMAC-SHA256"

	// 请求体
	reqBody := map[string]any{
		"PhoneNumberSet":   []string{phone},
		"SmsSdkAppId":      cfg.AppID,
		"SignName":         cfg.SignName,
		"TemplateId":       "0000000", // 通用模板（应由 config 传入）
		"TemplateParamSet": []string{content},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// 步骤 1：拼接规范请求串
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	date := time.Now().UTC().Format("2006-01-02")
	canonicalHeaders := "content-type:application/json; charset=utf-8\n" +
		"host:" + apiHost + "\n" +
		"x-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	hashedRequestPayload := sha256Hex(string(bodyBytes))
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashedRequestPayload

	// 步骤 2：拼接待签名字符串
	credentialScope := date + "/" + service + "/tc3_request"
	hashedCanonicalRequest := sha256Hex(canonicalRequest)
	stringToSign := algorithm + "\n" + timestamp + "\n" + credentialScope + "\n" + hashedCanonicalRequest

	// 步骤 3：计算签名
	secretDate := hmacSHA256([]byte("TC3"+cfg.SecretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	// 步骤 4：拼接 Authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, cfg.SecretID, credentialScope, signedHeaders, signature)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return time.Time{}, "", "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", apiHost)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", timestamp)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("Authorization", authorization)

	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return time.Time{}, "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Response struct {
			Error struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			RequestID string `json:"RequestId"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return time.Time{}, "", "", fmt.Errorf("decode tencent response: %w", err)
	}
	if result.Response.Error.Code != "" {
		return time.Time{}, result.Response.Error.Code, result.Response.Error.Message,
			fmt.Errorf("tencent sms error: %s", result.Response.Error.Message)
	}
	return time.Now(), "OK", "OK", nil
}

// sendHuawei 通过华为云发送短信
func (s *smsService) sendHuawei(phone, content string) (time.Time, string, string, error) {
	cfg, err := s.repo.GetHuaweiConfig()
	if err != nil {
		return time.Time{}, "", "", err
	}
	if cfg.AppKey == "" || cfg.AppSecret == "" {
		return time.Time{}, "", "", errors.New("huawei sms config missing")
	}

	// 华为云短信 API：HTTP 简单认证
	apiURL := fmt.Sprintf("https://smsapi.%s.boe-business.huaweicloud.com:443/sms/batchSendSms/v1",
		"cn-north-4") // region 应可配置

	body := map[string]any{
		"from":          cfg.Sender,
		"to":            []string{phone},
		"templateId":    "0000000", // 通用模板（应由 config 传入）
		"templateParas": []string{content},
		"signature":     cfg.Signature,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return time.Time{}, "", "", err
	}
	// 华为云使用 AppKey/AppSecret 作为 Basic Auth
	auth := base64.StdEncoding.EncodeToString([]byte(cfg.AppKey + ":" + cfg.AppSecret))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("X-WSSE", buildWSSE(cfg.AppKey, cfg.AppSecret))

	resp, err := httpclient.Client.Do(req)
	if err != nil {
		return time.Time{}, "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return time.Time{}, "", "", fmt.Errorf("decode huawei response: %w", err)
	}
	if result.Code != "000000" {
		return time.Time{}, result.Code, result.Message, fmt.Errorf("huawei sms error: %s", result.Message)
	}
	return time.Now(), "OK", "OK", nil
}

// ResendSms 重发短信
func (s *smsService) ResendSms(id uint) error {
	// 获取短信记录
	record, err := s.repo.GetSmsByID(id)
	if err != nil {
		return err
	}

	// 只有失败的短信可以重发
	if record.Status != "failed" {
		return errors.New("只有失败的短信可以重发")
	}

	// 更新状态为发送中
	record.Status = "sending"
	record.ErrorCode = ""
	record.ErrorMsg = ""

	if err := s.repo.UpdateSmsRecord(record); err != nil {
		return err
	}

	// 获取配置并调用真实 SMS 提供商
	config, err := s.repo.GetConfig()
	if err != nil {
		record.Status = "failed"
		record.ErrorMsg = "获取短信配置失败: " + err.Error()
		_ = s.repo.UpdateSmsRecord(record)
		return err
	}

	sentTime, errCode, errMsg, err := s.dispatchToProvider(record.Phone, record.Content, config.DefaultProvider)
	if err != nil {
		record.Status = "failed"
		record.ErrorCode = errCode
		record.ErrorMsg = errMsg
		_ = s.repo.UpdateSmsRecord(record)
		return fmt.Errorf("resend sms failed: %w", err)
	}

	record.SendTime = &sentTime
	record.Status = "sent"

	return s.repo.UpdateSmsRecord(record)
}

// GetDraftList 获取草稿列表
func (s *smsService) GetDraftList(req *dto.SmsDraftListRequest) ([]*model.SmsDraft, int64, error) {
	return s.repo.GetDraftList(req.Page, req.Limit, req.Title)
}

// GetDraftByID 根据ID获取草稿
func (s *smsService) GetDraftByID(id uint) (*model.SmsDraft, error) {
	return s.repo.GetDraftByID(id)
}

// CreateDraft 创建草稿
func (s *smsService) CreateDraft(req *dto.SmsDraftCreateRequest) error {
	draft := &model.SmsDraft{
		Title:   req.Title,
		Content: req.Content,
	}
	return s.repo.CreateDraft(draft)
}

// UpdateDraft 更新草稿
func (s *smsService) UpdateDraft(id uint, req *dto.SmsDraftUpdateRequest) error {
	// 检查草稿是否存在
	draft, err := s.repo.GetDraftByID(id)
	if err != nil {
		return err
	}

	// 更新草稿
	draft.Title = req.Title
	draft.Content = req.Content
	return s.repo.UpdateDraft(draft)
}

// DeleteDraft 删除草稿
func (s *smsService) DeleteDraft(id uint) error {
	return s.repo.DeleteDraft(id)
}

// SendDraft 发送草稿
func (s *smsService) SendDraft(id uint, phone string) error {
	// 获取草稿
	draft, err := s.repo.GetDraftByID(id)
	if err != nil {
		return err
	}

	// 发送短信
	req := &dto.SmsSendRequest{
		Phone:   phone,
		Content: draft.Content,
	}
	return s.SendSms(req)
}

// GetJobList 获取任务列表
func (s *smsService) GetJobList(req *dto.SmsJobListRequest) ([]*model.SmsJob, int64, error) {
	return s.repo.GetJobList(req.Page, req.Limit, req.Status, req.Name)
}

// GetJobByID 根据ID获取任务
func (s *smsService) GetJobByID(id uint) (*model.SmsJob, error) {
	return s.repo.GetJobByID(id)
}

// CreateJob 创建任务
func (s *smsService) CreateJob(req *dto.SmsJobCreateRequest) error {
	// 创建任务
	job := &model.SmsJob{
		Name:         req.Name,
		Total:        len(req.PhoneList),
		Sent:         0,
		Failed:       0,
		Status:       "pending",
		ScheduleTime: req.ScheduleTime,
	}

	if err := s.repo.CreateJob(job); err != nil {
		return err
	}

	// 创建任务详情
	details := make([]*model.SmsJobDetail, 0, len(req.PhoneList))
	for _, phone := range req.PhoneList {
		detail := &model.SmsJobDetail{
			JobID:   job.ID,
			Phone:   phone,
			Content: req.Content,
			Status:  "pending",
		}
		details = append(details, detail)
	}

	// 批量插入任务详情
	if err := s.repo.CreateJobDetails(details); err != nil {
		return err
	}

	// 如果没有设置计划时间，立即执行
	if req.ScheduleTime == nil || req.ScheduleTime.Before(time.Now()) {
		job.Status = "running"
		return s.repo.UpdateJob(job)
	}

	return nil
}

// PauseJob 暂停任务
func (s *smsService) PauseJob(id uint) error {
	job, err := s.repo.GetJobByID(id)
	if err != nil {
		return err
	}

	if job.Status != "running" {
		return errors.New("只能暂停运行中的任务")
	}

	job.Status = "paused"
	return s.repo.UpdateJob(job)
}

// ResumeJob 继续任务
func (s *smsService) ResumeJob(id uint) error {
	job, err := s.repo.GetJobByID(id)
	if err != nil {
		return err
	}

	if job.Status != "paused" {
		return errors.New("只能继续已暂停的任务")
	}

	job.Status = "running"
	return s.repo.UpdateJob(job)
}

// StopJob 停止任务
func (s *smsService) StopJob(id uint) error {
	job, err := s.repo.GetJobByID(id)
	if err != nil {
		return err
	}

	if job.Status != "running" && job.Status != "paused" && job.Status != "pending" {
		return errors.New("只能停止运行中、暂停或待执行的任务")
	}

	job.Status = "failed"
	return s.repo.UpdateJob(job)
}

// DeleteJob 删除任务
func (s *smsService) DeleteJob(id uint) error {
	job, err := s.repo.GetJobByID(id)
	if err != nil {
		return err
	}

	// 只能删除已完成或失败的任务
	if job.Status != "completed" && job.Status != "failed" {
		return errors.New("只能删除已完成或失败的任务")
	}

	// 先删除任务详情
	if err := s.repo.DeleteJobDetails(id); err != nil {
		return err
	}

	// 再删除任务
	return s.repo.DeleteJob(id)
}

// GetJobRecords 获取任务发送记录
func (s *smsService) GetJobRecords(id uint, page, limit int) ([]*model.SmsJobDetail, int64, error) {
	// 验证任务是否存在
	_, err := s.repo.GetJobByID(id)
	if err != nil {
		return nil, 0, err
	}

	// 获取任务详情
	return s.repo.GetJobDetails(id, page, limit)
}
