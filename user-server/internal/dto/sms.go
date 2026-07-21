package dto

import "time"

// SmsConfigRequest 短信配置请求
type SmsConfigRequest struct {
	DefaultProvider string           `json:"defaultProvider" binding:"required,oneof=aliyun tencent huawei"`
	RateLimit       int              `json:"rateLimit" binding:"min=1,max=1000"`
	DailyLimit      int              `json:"dailyLimit" binding:"min=100,max=100000"`
	RetryTimes      int              `json:"retryTimes" binding:"min=0,max=5"`
	Aliyun          SmsAliyunConfig  `json:"aliyun"`
	Tencent         SmsTencentConfig `json:"tencent"`
	Huawei          SmsHuaweiConfig  `json:"huawei"`
}

// SmsConfigResponse 短信配置响应
type SmsConfigResponse struct {
	DefaultProvider string           `json:"defaultProvider"`
	RateLimit       int              `json:"rateLimit"`
	DailyLimit      int              `json:"dailyLimit"`
	RetryTimes      int              `json:"retryTimes"`
	Aliyun          SmsAliyunConfig  `json:"aliyun"`
	Tencent         SmsTencentConfig `json:"tencent"`
	Huawei          SmsHuaweiConfig  `json:"huawei"`
}

// SmsAliyunConfig 阿里云短信配置
type SmsAliyunConfig struct {
	AccessKeyId     string `json:"accessKeyId"`
	AccessKeySecret string `json:"accessKeySecret"`
	SignName        string `json:"signName"`
}

// SmsTencentConfig 腾讯云短信配置
type SmsTencentConfig struct {
	SecretId  string `json:"secretId"`
	SecretKey string `json:"secretKey"`
	AppId     string `json:"appId"`
	SignName  string `json:"signName"`
}

// SmsHuaweiConfig 华为云短信配置
type SmsHuaweiConfig struct {
	AppKey    string `json:"appKey"`
	AppSecret string `json:"appSecret"`
	Sender    string `json:"sender"`
	Signature string `json:"signature"`
}

// SmsListRequest 短信列表请求
type SmsListRequest struct {
	Page      int    `form:"page,default=1" binding:"min=1"`
	Limit     int    `form:"limit,default=10" binding:"min=1,max=100"`
	Phone     string `form:"phone"`
	Status    string `form:"status" binding:"omitempty,oneof=pending sending sent failed"`
	StartDate string `form:"startDate"`
	EndDate   string `form:"endDate"`
}

// SmsSendRequest 发送短信请求
type SmsSendRequest struct {
	Phone   string `json:"phone" binding:"required,len=11"`
	Content string `json:"content" binding:"required,min=1,max=500"`
}

// SmsDraftCreateRequest 创建草稿请求
type SmsDraftCreateRequest struct {
	Title   string `json:"title" binding:"required,min=1,max=100"`
	Content string `json:"content" binding:"required,min=1,max=500"`
}

// SmsDraftUpdateRequest 更新草稿请求
type SmsDraftUpdateRequest struct {
	Title   string `json:"title" binding:"required,min=1,max=100"`
	Content string `json:"content" binding:"required,min=1,max=500"`
}

// SmsDraftListRequest 草稿列表请求
type SmsDraftListRequest struct {
	Page  int    `form:"page,default=1" binding:"min=1"`
	Limit int    `form:"limit,default=10" binding:"min=1,max=100"`
	Title string `form:"title"`
}

// SmsJobCreateRequest 创建任务请求
type SmsJobCreateRequest struct {
	Name         string     `json:"name" binding:"required,min=1,max=100"`
	PhoneList    []string   `json:"phoneList" binding:"required,min=1"`
	Content      string     `json:"content" binding:"required,min=1,max=500"`
	ScheduleTime *time.Time `json:"scheduleTime"`
}

// SmsJobListRequest 任务列表请求
type SmsJobListRequest struct {
	Page   int    `form:"page,default=1" binding:"min=1"`
	Limit  int    `form:"limit,default=10" binding:"min=1,max=100"`
	Status string `form:"status" binding:"omitempty,oneof=pending running paused completed failed"`
	Name   string `form:"name"`
}
