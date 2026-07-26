package cron

import (
	"context"
	"fmt"
	"marketing/internal/email/service"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/mail"
	"time"

	"github.com/google/uuid"
)

func EmailListCron() {
	// 读取未发送的 email 列表
	emailListService := email.NewEmailListService()
	emailListList, err := emailListService.GetUnsentEmailList(context.Background(), 10)
	if err != nil {
		logger.Info(fmt.Sprintf("获取未发送的email列表失败 %s", err.Error()))
		return
	}

	emailSmtpService := email.NewEmailSmtpService()

	// 循环发送
	for _, emailList := range emailListList {
		// 发送消息
		emailSmtp, err := emailSmtpService.GetRandEmailSmtp(context.Background())
		if err != nil {
			logger.Info(fmt.Sprintf("获取随机smtp失败 %s", err.Error()))
			continue
		}

		// 发送邮件
		cfg := mail.Config{
			From:     emailSmtp.Name,
			Password: emailSmtp.Password,
		}

		err = mail.SendMail(cfg, []string{emailList.To}, emailList.Subject, emailList.Content, true)

		is_success := 1
		if err != nil {
			is_success = 0
			logger.Info(fmt.Sprintf("发送失败:%s", err.Error()))
		}

		// 更新有邮件状态
		emailList.IsSend = 1
		emailList.SendTime = time.Now()
		emailList.From = emailSmtp.Name
		emailList.IsSuccess = is_success
		emailListService.UpdateEmailList(context.Background(), *emailList)

		// 更新jobs 统计
		jobs_id := emailList.JobsID
		// Add nil check for jobs_id
		if jobs_id == uuid.Nil {
			logger.Error(fmt.Errorf("Email list %s not found", emailList.ID), "查找邮件列表失败")
			continue
		}
		emailJobService := email.NewEmailJobsService()
		emailJobService.IncreaseSendTotal(context.Background(), jobs_id)
		//增加 success_total
		if is_success == 1 {
			emailJobService.IncreaseSuccessTotal(context.Background(), jobs_id)
		}
		//增加 fail_total
		if is_success == 0 {
			emailJobService.IncreaseFailTotal(context.Background(), jobs_id)
		}
	}
}
