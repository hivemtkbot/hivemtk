package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/httpclient"
	"marketing/internal/repository"
	"time"

	"gorm.io/gorm"
)

type WeComService struct {
	accountRepo  *repository.WeComAccountRepository
	customerRepo *repository.WeComCustomerRepository
	groupRepo    *repository.WeComGroupRepository
	memberRepo   *repository.WeComGroupMemberRepository
	messageRepo  *repository.WeComMessageRepository
	tagRepo      *repository.WeComTagRepository
}

// NewWeComService 创建企业微信服务实例
func NewWeComService(db *gorm.DB) *WeComService {
	return &WeComService{
		accountRepo:  repository.NewWeComAccountRepository(),
		customerRepo: repository.NewWeComCustomerRepository(),
		groupRepo:    repository.NewWeComGroupRepository(),
		memberRepo:   repository.NewWeComGroupMemberRepository(),
		messageRepo:  repository.NewWeComMessageRepository(),
		tagRepo:      repository.NewWeComTagRepository(db),
	}
}

// WeComTokenResponse 获取访问令牌响应
type WeComTokenResponse struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// GetAccessToken 获取访问令牌
func (s *WeComService) GetAccessToken(account *model.WeComAccount) (string, error) {
	if account == nil {
		return "", errors.New("账户不能为空")
	}

	if account.AccessToken != "" && time.Now().Before(account.TokenExpires) {
		return account.AccessToken, nil
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		account.CorpID, account.CorpSecret)

	resp, err := httpclient.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var tokenResp WeComTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.ErrCode != 0 {
		return "", errors.New(tokenResp.ErrMsg)
	}

	expiresTime := time.Now().Add(time.Duration(tokenResp.ExpiresIn-600) * time.Second)
	s.accountRepo.UpdateToken(account.ID, tokenResp.AccessToken, expiresTime)

	return tokenResp.AccessToken, nil
}

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	CorpID      string `json:"corp_id" binding:"required"`
	CorpSecret  string `json:"corp_secret" binding:"required"`
	AgentID     int    `json:"agent_id"`
	AgentSecret string `json:"agent_secret"`
}

// CreateAccount 创建企业微信账号
func (s *WeComService) CreateAccount(req *CreateAccountRequest) (*model.WeComAccount, error) {
	account := &model.WeComAccount{
		CorpID:      req.CorpID,
		CorpSecret:  req.CorpSecret,
		AgentID:     req.AgentID,
		AgentSecret: req.AgentSecret,
		Status:      1,
	}

	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}

	return account, nil
}

// GetAccountList 获取账号列表
func (s *WeComService) GetAccountList() ([]*model.WeComAccount, error) {
	return s.accountRepo.GetByMerchant()
}

// GetAccountByID 获取账号详情
func (s *WeComService) GetAccountByID(id uint) (*model.WeComAccount, error) {
	return s.accountRepo.GetByID(id)
}

// UpdateAccount 更新账号
func (s *WeComService) UpdateAccount(id uint, req *CreateAccountRequest) (*model.WeComAccount, error) {
	account, err := s.accountRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	account.CorpID = req.CorpID
	account.CorpSecret = req.CorpSecret
	account.AgentID = req.AgentID
	account.AgentSecret = req.AgentSecret

	if err := s.accountRepo.Update(account); err != nil {
		return nil, err
	}

	return account, nil
}

// DeleteAccount 删除账号
func (s *WeComService) DeleteAccount(id uint) error {
	return s.accountRepo.Delete(id)
}

// SyncCustomers 同步客户
func (s *WeComService) SyncCustomers(account *model.WeComAccount) (int, error) {
	token, err := s.GetAccessToken(account)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/externalcontact/list?access_token=%s", token)
	resp, err := httpclient.Client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		ErrCode        int      `json:"errcode"`
		ErrMsg         string   `json:"errmsg"`
		ExternalUserID []string `json:"external_userid_list"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.ErrCode != 0 {
		return 0, errors.New(result.ErrMsg)
	}

	count := 0
	for _, userID := range result.ExternalUserID {
		customer, err := s.getCustomerDetail(token, userID)
		if err != nil {
			continue
		}
		customer.AccountID = account.ID

		existing, _ := s.customerRepo.GetByExternalUserID(userID)
		if existing != nil {
			customer.ID = existing.ID
			s.customerRepo.Update(customer)
		} else {
			s.customerRepo.Create(customer)
		}
		count++
	}

	s.accountRepo.UpdateSyncTime(account.ID)
	return count, nil
}

// getCustomerDetail 获取客户详情
func (s *WeComService) getCustomerDetail(token, userID string) (*model.WeComCustomer, error) {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/externalcontact/get?access_token=%s&external_userid=%s", token, userID)
	resp, err := httpclient.Client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		ErrCode         int    `json:"errcode"`
		ErrMsg          string `json:"errmsg"`
		ExternalContact struct {
			ExternalUserID string `json:"external_userid"`
			Name           string `json:"name"`
			Avatar         string `json:"avatar"`
			Gender         int    `json:"gender"`
			UnionID        string `json:"union"`
			Type           int    `json:"type"`
		} `json:"external_contact"`
		FollowUser []struct {
			UserID   string `json:"userid"`
			Nickname string `json:"nickname"`
			AddTime  int64  `json:"add_time"`
		} `json:"follow_user"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	customer := &model.WeComCustomer{
		ExternalUserID: result.ExternalContact.ExternalUserID,
		Name:           result.ExternalContact.Name,
		Avatar:         result.ExternalContact.Avatar,
		Gender:         result.ExternalContact.Gender,
		UnionID:        result.ExternalContact.UnionID,
		Type:           result.ExternalContact.Type,
	}

	if len(result.FollowUser) > 0 {
		customer.EmployeeID = result.FollowUser[0].UserID
		customer.EmployeeName = result.FollowUser[0].Nickname
		customer.AddTime = time.Unix(result.FollowUser[0].AddTime, 0)
	}

	return customer, nil
}

// GetCustomerList 获取客户列表
func (s *WeComService) GetCustomerList(page, pageSize int) ([]*model.WeComCustomer, int64, error) {
	return s.customerRepo.GetByMerchant(page, pageSize)
}

// SyncGroups 同步客户群
func (s *WeComService) SyncGroups(account *model.WeComAccount) (int, error) {
	token, err := s.GetAccessToken(account)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/externalcontact/groupchat/list?access_token=%s", token)

	reqBody := map[string]any{
		"status_filter": 0,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := httpclient.Client.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		ErrCode       int    `json:"errcode"`
		ErrMsg        string `json:"errmsg"`
		GroupChatList []struct {
			ChatID string `json:"chat_id"`
			Status int    `json:"status"`
		} `json:"group_chat_list"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.ErrCode != 0 {
		return 0, errors.New(result.ErrMsg)
	}

	count := 0
	for _, g := range result.GroupChatList {
		group, err := s.getGroupDetail(token, g.ChatID)
		if err != nil {
			continue
		}
		group.AccountID = account.ID

		existing, _ := s.groupRepo.GetByChatID(g.ChatID)
		if existing != nil {
			group.ID = existing.ID
			s.groupRepo.Update(group)
		} else {
			s.groupRepo.Create(group)
		}
		count++
	}

	return count, nil
}

// getGroupDetail 获取群详情
func (s *WeComService) getGroupDetail(token, chatID string) (*model.WeComGroup, error) {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/externalcontact/groupchat/get?access_token=%s", token)

	reqBody := map[string]any{
		"chat_id": chatID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := httpclient.Client.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		GroupChat struct {
			ChatID      string `json:"chat_id"`
			Name        string `json:"name"`
			Owner       string `json:"owner"`
			MemberCount int    `json:"member_count"`
			CreateTime  int64  `json:"create_time"`
		} `json:"group_chat"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	group := &model.WeComGroup{
		ChatID:      result.GroupChat.ChatID,
		Name:        result.GroupChat.Name,
		OwnerID:     result.GroupChat.Owner,
		MemberCount: result.GroupChat.MemberCount,
		CreateTime:  time.Unix(result.GroupChat.CreateTime, 0),
		Status:      1,
	}

	return group, nil
}

// GetGroupList 获取群列表
func (s *WeComService) GetGroupList(page, pageSize int) ([]*model.WeComGroup, int64, error) {
	return s.groupRepo.GetByMerchant(page, pageSize)
}

// WeComSendMessageRequest 发送消息请求
type WeComSendMessageRequest struct {
	ToUser  string `json:"to_user"`
	ToParty string `json:"to_party"`
	MsgType string `json:"msg_type" binding:"required"`
	Content string `json:"content"`
	MediaID string `json:"media_id"`
	Title   string `json:"title"`
	Desc    string `json:"desc"`
	URL     string `json:"url"`
	PicURL  string `json:"pic_url"`
}

// SendMessage 发送消息
func (s *WeComService) SendMessage(account *model.WeComAccount, req *WeComSendMessageRequest) (string, error) {
	token, err := s.GetAccessToken(account)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/externalcontact/message/send?access_token=%s", token)

	msgData := map[string]any{
		"to_user":  req.ToUser,
		"to_party": req.ToParty,
		"msgtype":  req.MsgType,
		"agentid":  account.AgentID,
	}

	switch req.MsgType {
	case "text":
		msgData["text"] = map[string]string{"content": req.Content}
	case "image":
		msgData["image"] = map[string]string{"media_id": req.MediaID}
	case "link":
		msgData["link"] = map[string]string{
			"title":  req.Title,
			"desc":   req.Desc,
			"url":    req.URL,
			"picurl": req.PicURL,
		}
	}

	bodyBytes, _ := json.Marshal(msgData)
	resp, err := httpclient.Client.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MsgID   string `json:"msgid"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if result.ErrCode != 0 {
		return "", errors.New(result.ErrMsg)
	}

	now := time.Now()
	message := &model.WeComMessage{
		AccountID: account.ID,
		MsgID:     result.MsgID,
		MsgType:   req.MsgType,
		ToUser:    req.ToUser,
		Content:   req.Content,
		Status:    1,
		SendTime:  &now,
	}
	s.messageRepo.Create(message)

	return result.MsgID, nil
}

// GetMessageList 获取消息列表
func (s *WeComService) GetMessageList(page, pageSize int) ([]*model.WeComMessage, int64, error) {
	return s.messageRepo.GetByMerchant(page, pageSize)
}

// GetTagList 获取标签列表
func (s *WeComService) GetTagList() ([]*model.WeComTag, error) {
	return s.tagRepo.GetByMerchant()
}

// SyncTags 同步标签
func (s *WeComService) SyncTags(account *model.WeComAccount) (int, error) {
	token, err := s.GetAccessToken(account)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/tag/list?access_token=%s", token)
	resp, err := httpclient.Client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		TagList []struct {
			TagID     int    `json:"tagid"`
			TagName   string `json:"tagname"`
			UserCount int    `json:"user_count"`
		} `json:"taglist"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	if result.ErrCode != 0 {
		return 0, errors.New(result.ErrMsg)
	}

	count := 0
	for _, t := range result.TagList {
		tag := &model.WeComTag{
			AccountID:     account.ID,
			TagID:         fmt.Sprintf("%d", t.TagID),
			TagName:       t.TagName,
			CustomerCount: t.UserCount,
		}
		s.tagRepo.Create(tag)
		count++
	}

	return count, nil
}
