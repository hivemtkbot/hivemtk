package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	contentsvc "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/repository"
)


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

	ctx := context.Background()

	req := &dto.SmsSendRequest{
		Phone:   phone,
		Content: content,
	}
	if err := svc.SendSms(ctx, req); err != nil {
		return fmt.Errorf("发送短信失败：%w", err)
	}
	return nil
}

