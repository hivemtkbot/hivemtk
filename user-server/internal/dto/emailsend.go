package dto

import (
	"time"
)

// SendEmailRequest 发送邮件请求
type SendEmailRequest struct {
	DraftId       string     `json:"draftId"`
	To            string     `json:"to" binding:"required"`
	Subject       string     `json:"subject" binding:"required"`
	Content       string     `json:"content" binding:"required"`
	Attachments   []string   `json:"attachments"`
	SendTime      *time.Time `json:"sendTime,omitempty"`
	ImmediateSend bool       `json:"immediateSend"`
	SmtpId        string     `json:"smtpId" binding:"required"`
}

// EmailSendResponse 邮件发送响应
type EmailSendResponse struct {
	ID        string     `json:"id"`
	To        string     `json:"to"`
	Subject   string     `json:"subject"`
	Status    int        `json:"status"`
	SendTime  *time.Time `json:"send_time,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
