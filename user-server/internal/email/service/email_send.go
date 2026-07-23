package email

import (
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/gomail.v2"
	"context"
)

// 邮件状态常量
const (
	EmailStatusPending	= 0	// 待发送
	EmailStatusSent		= 1	// 已发送
	EmailStatusFailed	= 2	// 发送失败
)

type EmailSendService struct {
	repo		repository.EmailSendRepository
	smtpRepo	repository.EmailSmtpRepository
}

func NewEmailSendService() *EmailSendService {
	return &EmailSendService{
		repo:		repository.NewEmailSendRepository(),
		smtpRepo:	repository.NewEmailSmtpRepository(),
	}
}

// 发送邮件
func (s *EmailSendService) SendEmail(ctx context.Context, req dto.SendEmailRequest) (*model.EmailSend, error) {

	// 创建邮件记录
	emailSend := &model.EmailSend{
		ID:		uuid.New().String(),
		To:		req.To,
		Subject:	req.Subject,
		Content:	req.Content,
		Attachments:	strings.Join(req.Attachments, ","),
		SmtpID:		req.SmtpId,
		Status:		EmailStatusPending,
	}

	// 设置发送时间：立即发送或计划发送
	if req.ImmediateSend {
		now := time.Now()
		emailSend.SendTime = &now
	} else if req.SendTime != nil {
		emailSend.SendTime = req.SendTime
	}

	// 保存到数据库
	if err := s.repo.Create(ctx, emailSend); err != nil {
		return nil, err
	}

	// 如果是立即发送，立即发送邮件（异步，避免阻塞请求）
	if req.ImmediateSend {
		go func() {
			// panic 兜底：异步发送异常（如 SMTP 配置为空）不能拖垮主流程/进程
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("邮件发送异步协程 panic [%s]: %v", emailSend.ID, r)
				}
			}()
			err := s.sendActualEmail(ctx, emailSend)
			emailUUID, parseErr := uuid.Parse(emailSend.ID)
			if parseErr != nil {
				logger.Errorf("邮件 ID 解析失败：%v", parseErr)
				return
			}
			if err != nil {
				logger.Errorf("邮件发送失败 [%s]: %v", emailSend.ID, err)
				s.repo.UpdateStatus(ctx, emailUUID, EmailStatusFailed)
			} else {
				s.repo.UpdateStatus(ctx, emailUUID, EmailStatusSent)
			}
		}()
	}

	return emailSend, nil
}

// 处理待发送邮件的定时任务
func (s *EmailSendService) ProcessPendingEmails(ctx context.Context) error {
	// 获取所有待发送的邮件（status=0 且 sendTime <= 现在）
	pendingEmails, err := s.repo.GetPendingEmails(ctx)
	if err != nil {
		return err
	}

	// 遍历发送
	for _, email := range pendingEmails {
		err := s.sendActualEmail(ctx, email)
		emailUUID, parseErr := uuid.Parse(email.ID)
		if parseErr != nil {
			logger.Errorf("邮件 ID 解析失败 [%s]: %v", email.ID, parseErr)
			continue
		}
		if err != nil {
			logger.Errorf("邮件发送失败 [%s]: %v", email.ID, err)
			s.repo.UpdateStatus(ctx, emailUUID, EmailStatusFailed)
		} else {
			s.repo.UpdateStatus(ctx, emailUUID, EmailStatusSent)
		}
	}

	return nil
}

// sendActualEmail 实际发送邮件
func (s *EmailSendService) sendActualEmail(ctx context.Context, email *model.EmailSend) error {
	// 获取 SMTP 配置（直接使用 string ID）
	smtpConfig, err := s.smtpRepo.GetByID(ctx, email.SmtpID)
	if err != nil {
		return err
	}

	// 创建 gomail 消息
	m := gomail.NewMessage()
	m.SetHeader("From", smtpConfig.Username)
	m.SetHeader("To", email.To)
	m.SetHeader("Subject", email.Subject)
	m.SetBody("text/html", email.Content)

	// 添加附件
	if email.Attachments != "" {
		attachments := strings.Split(email.Attachments, ",")
		for _, attachment := range attachments {
			if attachment != "" {
				m.Attach(attachment)
			}
		}
	}

	// 创建拨号器
	d := gomail.NewDialer(smtpConfig.Server, smtpConfig.Port, smtpConfig.Username, smtpConfig.Password)

	// 发送邮件
	// 注意：在实际生产环境中，这里会真正连接 SMTP 服务器发送邮件
	// 在测试环境中，由于 SMTP 服务器不可达，会返回连接错误
	return d.DialAndSend(m)
}
