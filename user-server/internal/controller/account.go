package controller

import (
	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AccountController struct {
	svc *service.AccountService
}

func NewAccountController() *AccountController {
	return &AccountController{svc: service.NewAccountService()}
}

func (c *AccountController) CreateAccount(ctx *gin.Context) {
	var req dto.CreateAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	account := model.Account{
		TgBotToken:          req.TgBotToken,
		Price:               req.Price,
		GroupID:             req.GroupID,
		EpayPid:             req.EpayPid,
		EpayKey:             req.EpayKey,
		EpayPayType:         req.EpayPayType,
		EpayQueryUrl:        req.EpayQueryUrl,
		EpayURL:             req.EpayURL,
		ProxyEnableProxy:    req.ProxyEnableProxy,
		ProxyProtoclo:       req.ProxyProtoclo,
		ProxyHost:           req.ProxyHost,
		ProxyPort:           req.ProxyPort,
		DouyinHeadless:      req.DouyinHeadless,
		KuaishouHeadless:    req.KuaishouHeadless,
		XiaohongshuHeadless: req.XiaohongshuHeadless,
		XianyuHeadless:      req.XianyuHeadless,
	}

	createdAccount, err := c.svc.CreateAccount(account)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	resp := dto.AccountResponse{
		ID:                  createdAccount.ID,
		TgBotToken:          createdAccount.TgBotToken,
		Price:               createdAccount.Price,
		GroupID:             createdAccount.GroupID,
		EpayPid:             createdAccount.EpayPid,
		EpayKey:             createdAccount.EpayKey,
		EpayPayType:         createdAccount.EpayPayType,
		EpayQueryUrl:        createdAccount.EpayQueryUrl,
		EpayURL:             createdAccount.EpayURL,
		ProxyEnableProxy:    createdAccount.ProxyEnableProxy,
		ProxyProtoclo:       createdAccount.ProxyProtoclo,
		ProxyHost:           createdAccount.ProxyHost,
		ProxyPort:           createdAccount.ProxyPort,
		Status:              createdAccount.Status,
		CreateTime:          createdAccount.CreateTime,
		DouyinHeadless:      createdAccount.DouyinHeadless,
		KuaishouHeadless:    createdAccount.KuaishouHeadless,
		XiaohongshuHeadless: createdAccount.XiaohongshuHeadless,
		XianyuHeadless:      createdAccount.XianyuHeadless,
	}
	response.Success(ctx, resp, "success")
}

func (c *AccountController) GetAccounts(ctx *gin.Context) {
	accounts, err := c.svc.GetAccountList()
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	resp := dto.GetAccountListResponse{
		Total: int64(len(accounts)),
		List:  []*dto.AccountResponse{},
	}
	for _, account := range accounts {
		resp.List = append(resp.List, &dto.AccountResponse{
			ID:                  account.ID,
			TgName:              account.TgName,
			TgBotToken:          account.TgBotToken,
			Price:               account.Price,
			GroupID:             account.GroupID,
			EpayPid:             account.EpayPid,
			EpayKey:             account.EpayKey,
			EpayPayType:         account.EpayPayType,
			EpayQueryUrl:        account.EpayQueryUrl,
			EpayURL:             account.EpayURL,
			ProxyEnableProxy:    account.ProxyEnableProxy,
			ProxyProtoclo:       account.ProxyProtoclo,
			ProxyHost:           account.ProxyHost,
			ProxyPort:           account.ProxyPort,
			Status:              account.Status,
			CreateTime:          account.CreateTime,
			DouyinHeadless:      account.DouyinHeadless,
			KuaishouHeadless:    account.KuaishouHeadless,
			XiaohongshuHeadless: account.XiaohongshuHeadless,
			XianyuHeadless:      account.XianyuHeadless,
		})
	}
	response.Success(ctx, resp, "success")
}

func (c *AccountController) GetAccount(ctx *gin.Context) {
	accountIDStr := ctx.Param("id")
	if accountIDStr == "" {
		response.Error(ctx, http.StatusBadRequest, "账户ID不能为空")
		return
	}

	account, err := c.svc.GetAccount(accountIDStr)
	if err != nil {
		if isNotFoundError(err) {
			response.Error(ctx, http.StatusNotFound, "账户不存在")
			return
		}
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	resp := dto.AccountResponse{
		ID:                  account.ID,
		TgName:              account.TgName,
		TgBotToken:          account.TgBotToken,
		Price:               account.Price,
		GroupID:             account.GroupID,
		EpayPid:             account.EpayPid,
		EpayKey:             account.EpayKey,
		EpayPayType:         account.EpayPayType,
		EpayQueryUrl:        account.EpayQueryUrl,
		EpayURL:             account.EpayURL,
		ProxyEnableProxy:    account.ProxyEnableProxy,
		ProxyProtoclo:       account.ProxyProtoclo,
		ProxyHost:           account.ProxyHost,
		ProxyPort:           account.ProxyPort,
		Status:              account.Status,
		CreateTime:          account.CreateTime,
		Msg:                 account.Msg,
		URL:                 account.URL,
		DouyinHeadless:      account.DouyinHeadless,
		KuaishouHeadless:    account.KuaishouHeadless,
		XiaohongshuHeadless: account.XiaohongshuHeadless,
		XianyuHeadless:      account.XianyuHeadless,
	}
	response.Success(ctx, resp, "success")
}

func (c *AccountController) UpdateAccount(ctx *gin.Context) {
	var req dto.UpdateAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	// 从URI参数获取ID
	accountIDStr := ctx.Param("id")
	if accountIDStr == "" {
		response.Error(ctx, http.StatusBadRequest, "账户ID不能为空")
		return
	}

	// 如果JSON中的ID与URI中的ID不一致，使用URI中的ID
	if req.ID != "" && req.ID != accountIDStr {
		req.ID = accountIDStr
	}

	account := model.Account{
		ID:                  accountIDStr,
		TgName:              req.TgName,
		TgBotToken:          req.TgBotToken,
		Price:               req.Price,
		GroupID:             req.GroupID,
		EpayPid:             req.EpayPid,
		EpayKey:             req.EpayKey,
		EpayPayType:         req.EpayPayType,
		EpayQueryUrl:        req.EpayQueryUrl,
		EpayURL:             req.EpayURL,
		ProxyEnableProxy:    req.ProxyEnableProxy,
		ProxyProtoclo:       req.ProxyProtoclo,
		ProxyHost:           req.ProxyHost,
		ProxyPort:           req.ProxyPort,
		DouyinHeadless:      req.DouyinHeadless,
		KuaishouHeadless:    req.KuaishouHeadless,
		XiaohongshuHeadless: req.XiaohongshuHeadless,
		XianyuHeadless:      req.XianyuHeadless,
	}
	err := c.svc.UpdateAccount(account)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "success")
}

func (c *AccountController) DeleteAccount(ctx *gin.Context) {
	var req dto.DeleteAccountRequest
	if err := ctx.ShouldBindUri(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := c.svc.DeleteAccount(req.ID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "success")
}
