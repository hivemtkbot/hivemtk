package service

import (
	"errors"
	"fmt"
	"sync"

	contentsvc "marketing/internal/content/service"
	"marketing/internal/dto"
	"marketing/internal/repository"
)

// marketing_flow_sms_init.go 营销流程 send_sms 动作的 SMS 发送实现注入
//
// 背景：
//   marketing_flow.go 中的 sendActionSendSms 通过包级函数变量 smsSenderFunc 调用 SMS 发送能力。
//   因 internal/service 反向依赖 internal/content/service（sop_condition / sales_engine_adapters），
//   本包是 contentsvc.SetSmsSender 的唯一调用方，必须在 init 阶段注册真实实现。
//
// 实现策略：
//   1. init() 阶段调用 contentsvc.SetSmsSender 注册 lazySender
//   2. lazySender 在第一次调用时延迟初始化 SmsService（避免 init 阶段 DB 未就绪）
//   3. 复用同一 SmsService 实例以减少对象创建开销
//   4. 若 DB 未就绪则返回明确错误，便于调用方感知
//
// 五层架构合规：
//   - 本文件不再 import db 包，也不再调用 db 包
//   - DB 访问完全委托给 SmsRepository（其内部自行获取 db 连接）
//   - 服务层只通过 repository 暴露的接口完成数据访问

var (
	smsSvcOnce     sync.Once
	smsSvcInstance SmsService
	smsSvcInitErr  error
)

// init 注册 SMS 发送实现到 contentsvc
func init() {
	contentsvc.SetSmsSender(lazySendSms)
}

// initSmsService 延迟初始化 SmsService（线程安全）
//
// 通过 NewSmsService(NewSmsRepository()) 注入仓储；
// 仓储自身负责获取 db 连接，Service 层不感知 db 包。
func initSmsService() (SmsService, error) {
	smsSvcOnce.Do(func() {
		smsSvcInstance = NewSmsService(repository.NewSmsRepository())
		if smsSvcInstance == nil {
			smsSvcInitErr = errors.New("初始化短信服务失败：仓储创建返回 nil")
		}
	})
	return smsSvcInstance, smsSvcInitErr
}

// lazySendSms 营销流程 send_sms 动作的实际 SMS 发送实现
// 延迟初始化 SmsService 并调用其 SendSms 方法。
func lazySendSms(phone, content string) error {
	if phone == "" {
		return errors.New("phone 不能为空")
	}
	if content == "" {
		return errors.New("content 不能为空")
	}

	svc, err := initSmsService()
	if err != nil {
		return fmt.Errorf("初始化短信服务失败：%w", err)
	}

	req := &dto.SmsSendRequest{
		Phone:   phone,
		Content: content,
	}
	if err := svc.SendSms(ctx, req); err != nil {
		return fmt.Errorf("发送短信失败：%w", err)
	}
	return nil
}
