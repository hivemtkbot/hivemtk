package account

import (
	"errors"
	"fmt"
	"marketing/internal/model"
	logger "marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/tgbot"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/service"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/shopspring/decimal"
)

var PayCommandString = "/pay"
var StartCommandString = "/start"
var JoinGroupCommandString = "/join_group"

type accountData struct {
	BotToken    string
	EpayConfig  _type.EpayConfig
	Price       decimal.Decimal
	AccountID   string
	GroupID     int64
	AccountName string
	Bot         *tgbotapi.BotAPI
	BotUrl      string
}

var (
	globalAccounts = make(map[string]accountData)
	dictLock       sync.RWMutex
)

func InitAllAccount() error {
	accountSer := service.NewAccountService()

	accountList, err := accountSer.GetAccountList()
	if err != nil {
		return err
	}
	for _, account := range accountList {
		// 初始化账号
		botName, err := InitOneAccount(account)
		if err != nil {
			logger.Info(fmt.Sprintf("初始化账号ID %s 失败: %s", account.ID, err.Error()))
			// 更新失败信息
			accountSer.UpdateAccountStatusById(account.ID, _type.AccountStatusInactive, err.Error())
		} else {
			// 更新成功信息
			logger.Info(fmt.Sprintf("初始化账号ID %s 成功: %s", account.ID, botName))
			accountSer.UpdateAccountStatusById(account.ID, _type.AccountStatusActive, "")
			//更新name
			accountSer.UpdateAccountTgNameById(account.ID, botName)
		}
		// 更新成功信息
	}
	return nil
}

func InitOneAccount(account *model.Account) (string, error) {
	// 字符串 0 1 转 bot
	bot, err := tgbot.InitTGBot(account.TgBotToken, account.GroupID, account.ProxyEnableProxy, account.ProxyProtoclo, account.ProxyHost, account.ProxyPort)
	if err != nil {
		return "", err
	}
	botName := bot.Self.FirstName
	account.TgName = botName

	// build data
	accountData, err := FormateAccountDictData(account, bot)
	accountData.AccountName = botName
	if err != nil {
		return botName, err
	}
	SetAccount(account.TgBotToken, accountData)
	// 运行
	go RunTgBot(accountData)
	return botName, nil
}

func FormateEpayConfigByAccount(account *model.Account) _type.EpayConfig {
	config := _type.EpayConfig{
		Pid:       account.EpayPid,
		Key:       account.EpayKey,
		Type:      account.EpayPayType,
		NotifyUrl: fmt.Sprintf("%s%s", account.EpayURL, "/api/epay_notify"),
		ReturnUrl: account.URL,
		QueryUrl:  account.EpayQueryUrl,
		EpayUrl:   account.EpayURL,
	}
	return config
}

func FormateAccountDictData(account *model.Account, bot *tgbotapi.BotAPI) (accountData, error) {
	payconfig := FormateEpayConfigByAccount(account)

	price, err := decimal.NewFromString(account.Price)
	if err != nil {
		return accountData{}, err
	}
	info := accountData{
		BotToken:    account.TgBotToken,
		EpayConfig:  payconfig,
		Price:       price,
		AccountID:   account.ID,
		GroupID:     account.GroupID,
		AccountName: account.TgName,
		Bot:         bot,
		BotUrl:      account.URL,
	}
	return info, nil
}

func GetAccount(token string) (accountData, error) {
	dictLock.RLock()
	defer dictLock.RUnlock()
	accountData, ok := globalAccounts[token]
	if !ok {
		return accountData, errors.New("bot not found")
	}
	return accountData, nil
}

func SetAccount(token string, account accountData) {
	dictLock.Lock()
	defer dictLock.Unlock()
	globalAccounts[token] = account
}

func RunTgBot(account accountData) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := account.Bot.GetUpdatesChan(u)

	for update := range updates {
		go handleUpdate(account, update)
	}
}

func handleUpdate(account accountData, update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(fmt.Errorf("机器人处理消息崩溃: %v", r), "goroutine panic recovered")
		}
	}()

	var from_id = update.Message.From.ID
	var from_FirstName = update.Message.From.FirstName
	var from_LastName = update.Message.From.LastName
	var from_UserName = update.Message.From.UserName
	var text = update.Message.Text

	// 保存用户
	userSer := service.NewUserService()
	user_id, err := userSer.InitUser(account.AccountID, int64(from_id), from_FirstName, from_LastName, from_UserName)
	if err != nil {
		logger.Info(err.Error())
	}
	// 保存聊天记录
	messageSer := service.NewMessageService()
	_, err = messageSer.InitMessage(account.AccountID, user_id, int64(from_id), text)
	if err != nil {
		logger.Info(err.Error())
	}

	if update.Message != nil && update.Message.Chat.IsPrivate() { // 只处理私聊信息
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start": //start
				msgText := BuildAccountStartNoticeMsg(account.AccountName)
				tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
			case "pay":
				orderService := service.NewOrderService()
				// 检查是否付费
				is_pay := orderService.LastOrderIsPay(account.AccountID, update.Message.Chat.ID, account.EpayConfig)
				if is_pay {
					msgText := BuildAccountJoinGroupMsg(account.AccountName)
					tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
					return
				}
				// 发起付费
				payUrl, err := orderService.CreatePay(account.AccountID, account.Price, update.Message.Chat.ID, account.EpayConfig)
				if err != nil {
					logger.Info("发起付费失败")
					msgText := BuildAccountNotPayErrorNoticeMsg(account.AccountName)
					tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
				}

				msgText := BuildAccountPayUrlMsg(payUrl)
				tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
			// 	PayCommand(account, update)
			case "join_group":
				orderService := service.NewOrderService()
				// 检查是否付费
				is_pay := orderService.LastOrderIsPay(account.AccountID, update.Message.Chat.ID, account.EpayConfig)
				if !is_pay {
					msgText := BuildAccountNotPaidNoticeMsg(account.AccountName)
					tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
				} else {
					tgbot.SendInviteJoinGroup(account.Bot, account.GroupID, update.Message.Chat.ID)
				}
			default:
				msgText := BuildAccountStartNoticeMsg(account.AccountName)
				tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
			}
		} else {
			msgText := BuildAccountStartNoticeMsg(account.AccountName)
			tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
		}
	}

	// 校验新成员
	if update.Message.NewChatMembers != nil {
		msgText := BuildAccountStartNoticeMsg(account.AccountName)
		tgbot.SendTgMsg(account.Bot, msgText, update.Message.Chat.ID)
	}
}

func BuildAccountStartNoticeMsg(TgName string) string {
	msgText := fmt.Sprintf(`欢迎使用: %s,未付费: %s, 已付费: %s`, TgName, PayCommandString, JoinGroupCommandString)
	return msgText
}

func BuildAccountNotPaidNoticeMsg(TgName string) string {
	msgText := fmt.Sprintf(`欢迎使用: %s,您尚未付费,请先付款: %s`, TgName, PayCommandString)
	return msgText
}

func BuildAccountJoinGroupMsg(TgName string) string {
	msgText := fmt.Sprintf(`欢迎使用: %s,您已付款点击进群 %s`, TgName, JoinGroupCommandString)
	return msgText
}

func BuildAccountNotPayErrorNoticeMsg(TgName string) string {
	msgText := fmt.Sprintf(`欢迎使用: %s,程序异常,请重试: %s`, TgName, PayCommandString)
	return msgText
}

func BuildAccountPayUrlMsg(payUrl string) string {
	msgText := fmt.Sprintf("<a href=\"%s\">点击支付</a>", payUrl)
	return msgText
}

func SendMsgBYBootToken(token string, msgText string, chatID int64) error {
	account, err := GetAccount(token)
	if err != nil {
		return err
	}
	tgbot.SendTgMsg(account.Bot, msgText, chatID)
	return nil
}
