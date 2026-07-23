package dto

import (
	_type "marketing/internal/pkg/utils/type"
)

type CreateAccountRequest struct {
	TgBotToken       string `json:"tg_bot_token" binding:"required"`
	Price            string `json:"price" binding:"required"`
	GroupID          int64  `json:"group_id" binding:"required"`
	ProxyEnableProxy bool   `json:"proxy_enable_proxy"`
	ProxyProtoclo    string `json:"proxy_protoclo"`
	ProxyHost        string `json:"proxy_host"`
	ProxyPort        int    `json:"proxy_port"`

	// 自动回复平台无头模式设置
	DouyinHeadless      *bool `json:"douyin_headless"`
	KuaishouHeadless    *bool `json:"kuaishou_headless"`
	XiaohongshuHeadless *bool `json:"xiaohongshu_headless"`
	XianyuHeadless      *bool `json:"xianyu_headless"`
}

type AccountResponse struct {
	ID               string                  `json:"id"`
	TgName           string                  `json:"tg_name"`
	TgBotToken       string                  `json:"tg_bot_token"`
	Price            string                  `json:"price"`
	GroupID          int64                   `json:"group_id"`
	ProxyEnableProxy bool                    `json:"proxy_enable_proxy"`
	ProxyProtoclo    string                  `json:"proxy_protoclo"`
	ProxyHost        string                  `json:"proxy_host"`
	ProxyPort        int                     `json:"proxy_port"`
	Status           _type.AccountStatusType `json:"status"`
	CreateTime       int64                   `json:"create_time"`
	Msg              string                  `json:"msg"`
	URL              string                  `json:"url"`

	// 自动回复平台无头模式设置
	DouyinHeadless      *bool `json:"douyin_headless"`
	KuaishouHeadless    *bool `json:"kuaishou_headless"`
	XiaohongshuHeadless *bool `json:"xiaohongshu_headless"`
	XianyuHeadless      *bool `json:"xianyu_headless"`
}

type GetAccountListResponse struct {
	Total int64              `json:"total"`
	List  []*AccountResponse `json:"list"`
}

type UpdateAccountRequest struct {
	ID               string `json:"id" binding:"required"`
	TgName           string `json:"tg_name"`
	TgBotToken       string `json:"tg_bot_token"`
	Price            string `json:"price"`
	GroupID          int64  `json:"group_id"`
	ProxyEnableProxy bool   `json:"proxy_enable_proxy"`
	ProxyProtoclo    string `json:"proxy_protoclo"`
	ProxyHost        string `json:"proxy_host"`
	ProxyPort        int    `json:"proxy_port"`

	// 自动回复平台无头模式设置
	DouyinHeadless      *bool `json:"douyin_headless"`
	KuaishouHeadless    *bool `json:"kuaishou_headless"`
	XiaohongshuHeadless *bool `json:"xiaohongshu_headless"`
	XianyuHeadless      *bool `json:"xianyu_headless"`
}

type DeleteAccountRequest struct {
	ID string `uri:"id" binding:"required"`
}

type GetAccountRequest struct {
	ID string `form:"id" binding:"required"`
}
