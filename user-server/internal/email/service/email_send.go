package email

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"github.com/google/uuid"
	"gopkg.in/gomail.v2"
)

// 邮件状态常量
const (
	EmailStatusPending = 0 
	EmailStatusSent    = 1 
	EmailStatusFailed  = 2 
)

type EmailSendService struct {
	repo     repository.EmailSendRepository
	smtpRepo repository.EmailSmtpRepository
}

func NewEmailSendService() *EmailSendService {
	return &EmailSendService{
		repo:     repository.NewEmailSendRepository(),
		smtpRepo: repository.NewEmailSmtpRepository(),
	}
}

// 发送邮件
func (s *EmailSendService) SendEmail(ctx context.Context, req dto.SendEmailRequest) (*model.EmailSend, error) {

	emailSend := &model.EmailSend{
		ID:          uuid.New().String(),
		To:          req.To,
		Subject:     req.Subject,
		Content:     req.Content,
		Attachments: strings.Join(req.Attachments, ","),
		SmtpID:      req.SmtpId,
		Status:      EmailStatusPending,
	}

	if req.ImmediateSend {
		now := time.Now()
		emailSend.SendTime = &now
	} else if req.SendTime != nil {
		emailSend.SendTime = req.SendTime
	}

	if err := s.repo.Create(ctx, emailSend); err != nil {
		return nil, err
	}

	if req.ImmediateSend {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("邮件发送异步协程 panic [%s]: %v", emailSend.ID, r)
				}
			}()
			sendCtx := context.WithoutCancel(ctx)
			err := s.sendActualEmail(sendCtx, emailSend)
			emailUUID, parseErr := uuid.Parse(emailSend.ID)
			if parseErr != nil {
				logger.Errorf("邮件 ID 解析失败：%v", parseErr)
				return
			}
			if err != nil {
				logger.Errorf("邮件发送失败 [%s]: %v", emailSend.ID, err)
				s.repo.UpdateStatus(sendCtx, emailUUID, EmailStatusFailed)
			} else {
				s.repo.UpdateStatus(sendCtx, emailUUID, EmailStatusSent)
			}
		}()
	}

	return emailSend, nil
}

// 处理待发送邮件的定时任务
func (s *EmailSendService) ProcessPendingEmails(ctx context.Context) error {
	pendingEmails, err := s.repo.GetPendingEmails(ctx)
	if err != nil {
		return err
	}

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
	// 1) 优先使用 DB 中的 SMTP 配置（按 SmtpID）
	var smtpConfig *model.EmailSmtp
	if email.SmtpID != "" {
		cfg, err := s.smtpRepo.GetByID(ctx, email.SmtpID)
		if err == nil && cfg != nil && cfg.Server != "" && cfg.Username != "" {
			smtpConfig = cfg
		} else if err != nil {
			logger.Warnf("邮件 SMTP 配置读取失败（SmtpID=%s），尝试环境变量兜底: %v", email.SmtpID, err)
		}
	}

	if smtpConfig == nil {
		smtpConfig = resolveSmtpFromEnv()
	}

	if smtpConfig == nil {
		logger.Errorf("邮件发送失败 [%s]: 无任何可用 SMTP 配置（SmtpID=%q 且未配置 EMAIL_163_*/QQ_EMAIL_*/SMTP_* 环境变量）",
			email.ID, email.SmtpID)
		return fmt.Errorf("邮件发送失败：未找到可用的 SMTP 配置（请配置 SmtpID 或 EMAIL_163_*/QQ_EMAIL_*/SMTP_* 环境变量）")
	}

	m := gomail.NewMessage()
	m.SetHeader("From", smtpConfig.Username)
	m.SetHeader("To", email.To)
	m.SetHeader("Subject", email.Subject)
	m.SetBody("text/html", email.Content)

	if email.Attachments != "" {
		attachments := strings.Split(email.Attachments, ",")
		for _, attachment := range attachments {
			if attachment != "" {
				m.Attach(attachment)
			}
		}
	}

	d := gomail.NewDialer(smtpConfig.Server, smtpConfig.Port, smtpConfig.Username, smtpConfig.Password)

	return d.DialAndSend(m)
}

// resolveSmtpFromEnv 从环境变量构造 SMTP 发信配置（按优先级）：
//   - EMAIL_163_USER / EMAIL_163_PASSWORD / EMAIL_163_SMTP_HOST（默认 smtp.163.com，端口 465）
//   - QQ_EMAIL / QQ_EMAIL_PASSWORD / QQ_NUMBER（QQ 邮箱，smtp.qq.com，端口 465）
//   - SMTP_HOST / SMTP_PORT / SMTP_USER / SMTP_PASSWORD（通用兜底）
//
// 任一来源字段完整即返回配置；都不完整返回 nil（由调用方显式报错）。
// 此为基于 SmtpID 的 DB 路径的兼容兜底，不取代原有路径。
func resolveSmtpFromEnv() *model.EmailSmtp {
	if user := os.Getenv("EMAIL_163_USER"); user != "" {
		if pwd := os.Getenv("EMAIL_163_PASSWORD"); pwd != "" {
			host := os.Getenv("EMAIL_163_SMTP_HOST")
			if host == "" {
				host = "smtp.163.com"
			}
			return &model.EmailSmtp{
				Name:     "env:163",
				Server:   host,
				Port:     465,
				Username: user,
				Password: pwd,
			}
		}
	}
	if qq := os.Getenv("QQ_EMAIL"); qq != "" {
		if pwd := os.Getenv("QQ_EMAIL_PASSWORD"); pwd != "" {
			return &model.EmailSmtp{
				Name:     "env:qq",
				Server:   "smtp.qq.com",
				Port:     465,
				Username: qq,
				Password: pwd,
			}
		}
	}
	if host := os.Getenv("SMTP_HOST"); host != "" {
		if user := os.Getenv("SMTP_USER"); user != "" {
			if pwd := os.Getenv("SMTP_PASSWORD"); pwd != "" {
				port := 465
				if p := os.Getenv("SMTP_PORT"); p != "" {
					if n, err := strconv.Atoi(p); err == nil && n > 0 {
						port = n
					}
				}
				return &model.EmailSmtp{
					Name:     "env:smtp",
					Server:   host,
					Port:     port,
					Username: user,
					Password: pwd,
				}
			}
		}
	}
	return nil
}

