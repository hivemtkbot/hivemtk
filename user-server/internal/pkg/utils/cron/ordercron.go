package cron

import (
	"fmt"
	utilsaccount "marketing/internal/pkg/utils/account"
	"marketing/internal/pkg/utils/epay"
	"marketing/internal/pkg/utils/logger"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/service"
)

func OrderCron() {
	orderService := service.NewOrderService()
	orderList, err := orderService.GetRecentOrderList()
	if err != nil {
		logger.Info(fmt.Sprintf("获取订单列表失败 %s", err.Error()))
		return
	}

	for _, order := range orderList {
		accountService := service.NewAccountService()
		epayConfig, err := accountService.GetEpayConfigByID(order.AccountID)
		if err != nil {
			logger.Info(fmt.Sprintf("获取账号支付配置失败 %s", err.Error()))
			continue
		}
		account, err := accountService.GetAccount(order.AccountID)
		if err != nil {
			logger.Info(fmt.Sprintf("获取账号信息失败 %s", err.Error()))
			continue
		}
		is_pay, err := epay.EpayQuery(order.ID, epayConfig)
		if err != nil {
			logger.Info(fmt.Sprintf("查询订单失败 %s", err.Error()))
			continue
		}
		if is_pay {
			// 更新数据库状态
			var status = _type.OrderStatusSuccess
			orderService.UpdateOrderStatusById(order.ID, status)

			// 发消息
			msgText := utilsaccount.BuildAccountJoinGroupMsg(account.TgName)
			utilsaccount.SendMsgBYBootToken(account.TgBotToken, msgText, order.TgID)
		}
	}
}
