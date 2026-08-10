package service

import (
	"bytes"

	"context"

	"encoding/json"

	"errors"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/httpclient"

	"hivemtk-user/internal/repository"

	"io"

	"math"

	"net/http"

	"net/url"

	"time"
)

func yuanToFen(yuan float64) int64 {
	return int64(math.Round(yuan * 100))
}

type IntegrationService struct {
	accountRepo      *repository.IntegrationAccountRepository
	syncLogRepo      *repository.SyncLogRepository
	customerRepo     *repository.ExternalCustomerRepository
	orderRepo        *repository.ExternalOrderRepository
	productRepo      *repository.ExternalProductRepository
	webhookEventRepo *repository.WebhookEventRepository
}

var (
	_ *repository.IntegrationAccountRepository // 用于静态检查

)

func NewIntegrationService() *IntegrationService {
	return &IntegrationService{
		accountRepo:      repository.NewIntegrationAccountRepository(),
		syncLogRepo:      repository.NewSyncLogRepository(),
		customerRepo:     repository.NewExternalCustomerRepository(),
		orderRepo:        repository.NewExternalOrderRepository(),
		productRepo:      repository.NewExternalProductRepository(),
		webhookEventRepo: repository.NewWebhookEventRepository(),
	}
}

type Platform string

const (
	PlatformXiaoshouyi Platform = "crm_xiaoshouyi" // 销售易

	PlatformFenxiangxiao Platform = "crm_fenxiangxiao" // 纷享销客

	PlatformTaobao Platform = "ecommerce_taobao" // 淘宝

	PlatformJD Platform = "ecommerce_jd" // 京东

)

type CreateIntegrationAccountRequest struct {
	Platform    string         `json:"platform" binding:"required"`
	AccountName string         `json:"account_name"`
	APIKey      string         `json:"api_key"`
	APISecret   string         `json:"api_secret"`
	Config      map[string]any `json:"config"`
}

func (s *IntegrationService) CreateIntegrationAccount(ctx context.Context, req *CreateIntegrationAccountRequest) (*model.IntegrationAccount, error) {
	configJSON := ""
	if req.Config != nil {
		data, _ := json.Marshal(req.Config)
		configJSON = string(data)
	}

	account := &model.IntegrationAccount{
		Platform:    req.Platform,
		AccountName: req.AccountName,
		APIKey:      req.APIKey,
		APISecret:   req.APISecret,
		Config:      configJSON,
		Status:      1,
	}

	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

func (s *IntegrationService) GetIntegrationAccountList(ctx context.Context) ([]*model.IntegrationAccount, error) {
	return s.accountRepo.GetAll(ctx)
}

func (s *IntegrationService) GetIntegrationAccountByID(ctx context.Context, id uint) (*model.IntegrationAccount, error) {
	return s.accountRepo.GetByID(ctx, id)
}

func (s *IntegrationService) UpdateIntegrationAccount(ctx context.Context, id uint, req *CreateIntegrationAccountRequest) (*model.IntegrationAccount, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	account.AccountName = req.AccountName
	account.APIKey = req.APIKey
	account.APISecret = req.APISecret
	if req.Config != nil {
		data, _ := json.Marshal(req.Config)
		account.Config = string(data)
	}

	if err := s.accountRepo.Update(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

func (s *IntegrationService) DeleteIntegrationAccount(ctx context.Context, id uint) error {
	return s.accountRepo.Delete(ctx, id)
}

type XiaoshouyiClient struct {
	accountRepo *repository.IntegrationAccountRepository
	account     *model.IntegrationAccount
	httpClient  *http.Client
}

func NewXiaoshouyiClient(account *model.IntegrationAccount, accountRepo *repository.IntegrationAccountRepository) *XiaoshouyiClient {
	return &XiaoshouyiClient{
		accountRepo: accountRepo,
		account:     account,
		httpClient:  httpclient.NewWithTimeout(30 * time.Second),
	}
}

func (c *XiaoshouyiClient) GetAccessToken(ctx context.Context) (string, error) {
	if c.account.AccessToken != "" && c.account.TokenExpires != nil && time.Now().Before(*c.account.TokenExpires) {
		return c.account.AccessToken, nil
	}

	// 销售易 OAuth2.0 令牌获取
	tokenURL := "https://api.xiaoshouyi.com/oauth/token"
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.account.APIKey},
		"client_secret": {c.account.APISecret},
	}

	resp, err := c.httpClient.PostForm(tokenURL, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if result.Error != "" {
		return "", errors.New(result.Error)
	}

	// 更新令牌
	expiresTime := time.Now().Add(time.Duration(result.ExpiresIn-600) * time.Second)
	c.accountRepo.UpdateToken(ctx, c.account.ID, result.AccessToken, &expiresTime)

	return result.AccessToken, nil
}

func (s *IntegrationService) SyncCustomers(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	switch account.Platform {
	case string(PlatformXiaoshouyi):
		return s.syncXiaoshouyiCustomers(ctx, account)
	case string(PlatformFenxiangxiao):
		return s.syncFenxiangxiaoCustomers(ctx, account)
	default:
		return 0, errors.New("不支持的平台")
	}
}

func (s *IntegrationService) syncXiaoshouyiCustomers(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewXiaoshouyiClient(account, s.accountRepo)
	token, err := client.GetAccessToken(ctx)
	if err != nil {
		return 0, err
	}

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:  account.Platform,
		SyncType:  "customer",
		Status:    0,
		StartTime: time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 获取客户列表
	apiURL := "https://api.xiaoshouyi.com/crm/v2/leads"
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	var result struct {
		Data []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Phone        string `json:"phone"`
			Email        string `json:"email"`
			Company      string `json:"company"`
			Industry     string `json:"industry"`
			OwnerID      string `json:"owner_id"`
			OwnerName    string `json:"owner_name"`
			Status       string `json:"status"`
			Source       string `json:"source"`
			CreatedTime  int64  `json:"created_time"`
			ModifiedTime int64  `json:"modified_time"`
		} `json:"data"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	if result.Error.Code != 0 {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, result.Error.Message)
		return 0, errors.New(result.Error.Message)
	}

	// 保存客户数据
	count := 0
	for _, c := range result.Data {
		customer := &model.ExternalCustomer{
			Platform:      account.Platform,
			ExternalID:    c.ID,
			Name:          c.Name,
			Phone:         c.Phone,
			Email:         c.Email,
			Company:       c.Company,
			Industry:      c.Industry,
			OwnerID:       c.OwnerID,
			OwnerName:     c.OwnerName,
			Status:        c.Status,
			Source:        c.Source,
			LastContactAt: func() *time.Time { t := time.Unix(c.ModifiedTime, 0); return &t }(),
		}

		// 检查是否已存在
		existing, _ := s.customerRepo.GetByExternalID(ctx, account.Platform, c.ID)
		if existing != nil {
			customer.ID = existing.ID
			s.customerRepo.Update(ctx, customer)
		} else {
			s.customerRepo.Create(ctx, customer)
		}
		count++
	}

	// 更新同步时间
	s.accountRepo.UpdateSyncTime(ctx, account.ID)
	s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 1, count, "")

	return count, nil
}

type FenxiangxiaoClient struct {
	accountRepo *repository.IntegrationAccountRepository
	account     *model.IntegrationAccount
	httpClient  *http.Client
}

func NewFenxiangxiaoClient(account *model.IntegrationAccount, accountRepo *repository.IntegrationAccountRepository) *FenxiangxiaoClient {
	return &FenxiangxiaoClient{
		accountRepo: accountRepo,
		account:     account,
		httpClient:  httpclient.NewWithTimeout(30 * time.Second),
	}
}

func (c *FenxiangxiaoClient) GetAccessToken(ctx context.Context) (string, error) {
	if c.account.AccessToken != "" && c.account.TokenExpires != nil && time.Now().Before(*c.account.TokenExpires) {
		return c.account.AccessToken, nil
	}

	// 纷享销客 OAuth2.0 令牌获取
	tokenURL := "https://api.fxiaoke.com/oauth2/token"
	data := url.Values{
		"grant_type": {"client_credentials"},
		"app_key":    {c.account.APIKey},
		"app_secret": {c.account.APISecret},
	}

	resp, err := c.httpClient.PostForm(tokenURL, data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Errcode     int    `json:"errcode"`
		Errmsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if result.Errcode != 0 {
		return "", errors.New(result.Errmsg)
	}

	// 更新令牌
	expiresTime := time.Now().Add(time.Duration(result.ExpiresIn-600) * time.Second)
	c.accountRepo.UpdateToken(ctx, c.account.ID, result.AccessToken, &expiresTime)

	return result.AccessToken, nil
}

func (s *IntegrationService) syncFenxiangxiaoCustomers(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewFenxiangxiaoClient(account, s.accountRepo)
	token, err := client.GetAccessToken(ctx)
	if err != nil {
		return 0, err
	}

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:  account.Platform,
		SyncType:  "customer",
		Status:    0,
		StartTime: time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 获取客户列表
	apiURL := "https://api.fxiaoke.com/crm/lead/v2/list"
	reqBody := map[string]any{
		"page":     1,
		"pagesize": 100,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	var result struct {
		Response struct {
			Data []struct {
				ID        string   `json:"id"`
				Name      string   `json:"name"`
				Phone     string   `json:"mobile"`
				Email     string   `json:"email"`
				Company   string   `json:"company_name"`
				Position  string   `json:"position"`
				OwnerID   string   `json:"owner_id"`
				OwnerName string   `json:"owner_name"`
				Status    string   `json:"status"`
				Source    string   `json:"source"`
				Tags      []string `json:"tags"`
			} `json:"data"`
			TotalCount int `json:"total_count"`
		} `json:"response"`
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	if result.Errcode != 0 {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, result.Errmsg)
		return 0, errors.New(result.Errmsg)
	}

	// 保存客户数据
	count := 0
	for _, c := range result.Response.Data {
		tagsJSON, _ := json.Marshal(c.Tags)
		customer := &model.ExternalCustomer{
			Platform:   account.Platform,
			ExternalID: c.ID,
			Name:       c.Name,
			Phone:      c.Phone,
			Email:      c.Email,
			Company:    c.Company,
			Position:   c.Position,
			OwnerID:    c.OwnerID,
			OwnerName:  c.OwnerName,
			Status:     c.Status,
			Source:     c.Source,
			Tags:       string(tagsJSON),
		}

		// 检查是否已存在
		existing, _ := s.customerRepo.GetByExternalID(ctx, account.Platform, c.ID)
		if existing != nil {
			customer.ID = existing.ID
			s.customerRepo.Update(ctx, customer)
		} else {
			s.customerRepo.Create(ctx, customer)
		}
		count++
	}

	// 更新同步时间
	s.accountRepo.UpdateSyncTime(ctx, account.ID)
	s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 1, count, "")

	return count, nil
}

type TaobaoClient struct {
	account    *model.IntegrationAccount
	httpClient *http.Client
	appKey     string
	appSecret  string
}

func NewTaobaoClient(account *model.IntegrationAccount) *TaobaoClient {
	return &TaobaoClient{
		account:    account,
		httpClient: httpclient.NewWithTimeout(30 * time.Second),
		appKey:     account.APIKey,
		appSecret:  account.APISecret,
	}
}

func (s *IntegrationService) SyncOrders(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	switch account.Platform {
	case string(PlatformTaobao):
		return s.syncTaobaoOrders(ctx, account)
	case string(PlatformJD):
		return s.syncJDOrders(ctx, account)
	default:
		return 0, errors.New("不支持的平台")
	}
}

func (s *IntegrationService) syncTaobaoOrders(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewTaobaoClient(account)

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:  account.Platform,
		SyncType:  "order",
		Status:    0,
		StartTime: time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 淘宝 API 签名请求（简化版本，实际需要复杂签名）
	apiURL := "https://gw.api.taobao.com/router/rest"
	params := url.Values{
		"app_key":       {client.appKey},
		"method":        {"taobao.trades.sold.get"},
		"format":        {"json"},
		"v":             {"2.0"},
		"start_created": {time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")},
		"end_created":   {time.Now().Format("2006-01-02 15:04:05")},
		"page":          {"1"},
		"page_size":     {"100"},
		"fields":        {"tid,type,status,payment,receiver_name,receiver_phone,created,orders"},
	}

	// Add signature calculation (requires app_secret)
	// In production, this would generate an HMAC signature for the API request
	resp, err := client.httpClient.Get(apiURL + "?" + params.Encode())
	if err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	// 解析响应（简化版本）
	var result struct {
		TradesSoldGetResponse struct {
			Trades struct {
				Trade []struct {
					TID           string  `json:"tid"`
					Type          string  `json:"type"`
					Status        string  `json:"status"`
					Payment       float64 `json:"payment"`
					ReceiverName  string  `json:"receiver_name"`
					ReceiverPhone string  `json:"receiver_phone"`
					Created       string  `json:"created"`
					PayTime       string  `json:"pay_time"`
					ConsentTime   string  `json:"consign_time"`
					Orders        struct {
						Order []struct {
							Title    string  `json:"title"`
							Price    float64 `json:"price"`
							Num      int     `json:"num"`
							OuterIid string  `json:"outer_iid"`
						} `json:"order"`
					} `json:"orders"`
				} `json:"trade"`
			} `json:"trades"`
		} `json:"trades_sold_get_response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	// 保存订单数据
	count := 0
	for _, t := range result.TradesSoldGetResponse.Trades.Trade {
		// 解析订单商品
		var items []map[string]any
		for _, o := range t.Orders.Order {
			items = append(items, map[string]any{
				"title":      o.Title,
				"price":      o.Price,
				"quantity":   o.Num,
				"product_id": o.OuterIid,
			})
		}
		itemsJSON, _ := json.Marshal(items)

		// 解析时间
		var payTime, shipTime, orderTime *time.Time
		if t.Created != "" {
			if ct, err := time.Parse("2006-01-02 15:04:05", t.Created); err == nil {
				orderTime = &ct
			}
		}
		if t.PayTime != "" {
			pt, _ := time.Parse("2006-01-02 15:04:05", t.PayTime)
			payTime = &pt
		}
		if t.ConsentTime != "" {
			st, _ := time.Parse("2006-01-02 15:04:05", t.ConsentTime)
			shipTime = &st
		}

		order := &model.ExternalOrder{
			Platform:  account.Platform,
			OrderID:   t.TID,
			Status:    t.Status,
			OrderTime: orderTime,
			PayAmount: yuanToFen(t.Payment),
			UserName:  t.ReceiverName,
			UserPhone: t.ReceiverPhone,
			PayTime:   payTime,
			ShipTime:  shipTime,
			Items:     string(itemsJSON),
		}

		// 检查是否已存在
		existing, _ := s.orderRepo.GetByOrderID(ctx, account.Platform, t.TID)
		if existing != nil {
			order.ID = existing.ID
			s.orderRepo.Update(ctx, order)
		} else {
			s.orderRepo.Create(ctx, order)
		}
		count++
	}

	// 更新同步时间
	s.accountRepo.UpdateSyncTime(ctx, account.ID)
	s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 1, count, "")

	return count, nil
}

type JDClient struct {
	account    *model.IntegrationAccount
	httpClient *http.Client
	appKey     string
	appSecret  string
}

func NewJDClient(account *model.IntegrationAccount) *JDClient {
	return &JDClient{
		account:    account,
		httpClient: httpclient.NewWithTimeout(30 * time.Second),
		appKey:     account.APIKey,
		appSecret:  account.APISecret,
	}
}
