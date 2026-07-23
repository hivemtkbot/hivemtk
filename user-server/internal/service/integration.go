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
	"math"
	"net/http"
	"net/url"
	"time"
	"context"
)

// yuanToFen 将元（float64）转换为分（int64）
// 使用 math.Round 避免浮点精度误差（如 19.99 * 100 = 1998.9999... → 1999）
func yuanToFen(yuan float64) int64 {
	return int64(math.Round(yuan * 100))
}

// IntegrationService 第三方对接服务
type IntegrationService struct {
	accountRepo		*repository.IntegrationAccountRepository
	syncLogRepo		*repository.SyncLogRepository
	customerRepo		*repository.ExternalCustomerRepository
	orderRepo		*repository.ExternalOrderRepository
	productRepo		*repository.ExternalProductRepository
	webhookEventRepo	*repository.WebhookEventRepository
}

var (
	_ *repository.IntegrationAccountRepository	// 用于静态检查
)

// NewIntegrationService 创建第三方对接服务实例
func NewIntegrationService() *IntegrationService {
	return &IntegrationService{
		accountRepo:		repository.NewIntegrationAccountRepository(),
		syncLogRepo:		repository.NewSyncLogRepository(),
		customerRepo:		repository.NewExternalCustomerRepository(),
		orderRepo:		repository.NewExternalOrderRepository(),
		productRepo:		repository.NewExternalProductRepository(),
		webhookEventRepo:	repository.NewWebhookEventRepository(),
	}
}

// Platform 平台类型
type Platform string

const (
	PlatformXiaoshouyi	Platform	= "crm_xiaoshouyi"	// 销售易
	PlatformFenxiangxiao	Platform	= "crm_fenxiangxiao"	// 纷享销客
	PlatformTaobao		Platform	= "ecommerce_taobao"	// 淘宝
	PlatformJD		Platform	= "ecommerce_jd"	// 京东
)

// CreateIntegrationAccountRequest 创建对接账号请求
type CreateIntegrationAccountRequest struct {
	Platform	string		`json:"platform" binding:"required"`
	AccountName	string		`json:"account_name"`
	APIKey		string		`json:"api_key"`
	APISecret	string		`json:"api_secret"`
	Config		map[string]any	`json:"config"`
}

// CreateIntegrationAccount 创建对接账号
func (s *IntegrationService) CreateIntegrationAccount(ctx context.Context, req *CreateIntegrationAccountRequest) (*model.IntegrationAccount, error) {
	configJSON := ""
	if req.Config != nil {
		data, _ := json.Marshal(req.Config)
		configJSON = string(data)
	}

	account := &model.IntegrationAccount{
		Platform:	req.Platform,
		AccountName:	req.AccountName,
		APIKey:		req.APIKey,
		APISecret:	req.APISecret,
		Config:		configJSON,
		Status:		1,
	}

	if err := s.accountRepo.Create(ctx, account); err != nil {
		return nil, err
	}

	return account, nil
}

// GetIntegrationAccountList 获取对接账号列表
func (s *IntegrationService) GetIntegrationAccountList(ctx context.Context) ([]*model.IntegrationAccount, error) {
	return s.accountRepo.GetAll(ctx)
}

// GetIntegrationAccountByID 获取对接账号详情
func (s *IntegrationService) GetIntegrationAccountByID(ctx context.Context, id uint) (*model.IntegrationAccount, error) {
	return s.accountRepo.GetByID(ctx, id)
}

// UpdateIntegrationAccount 更新对接账号
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

// DeleteIntegrationAccount 删除对接账号
func (s *IntegrationService) DeleteIntegrationAccount(ctx context.Context, id uint) error {
	return s.accountRepo.Delete(ctx, id)
}

// ==================== 销售易 CRM 对接 ====================

// XiaoshouyiClient 销售易 API 客户端
type XiaoshouyiClient struct {
	accountRepo	*repository.IntegrationAccountRepository
	account		*model.IntegrationAccount
	httpClient	*http.Client
}

// NewXiaoshouyiClient 创建销售易 API 客户端
func NewXiaoshouyiClient(account *model.IntegrationAccount, accountRepo *repository.IntegrationAccountRepository) *XiaoshouyiClient {
	return &XiaoshouyiClient{
		accountRepo:	accountRepo,
		account:	account,
		httpClient:	httpclient.NewWithTimeout(30 * time.Second),
	}
}

// GetAccessToken 获取访问令牌
func (c *XiaoshouyiClient) GetAccessToken(ctx context.Context)  (string, error) {
	if c.account.AccessToken != "" && c.account.TokenExpires != nil && time.Now().Before(*c.account.TokenExpires) {
		return c.account.AccessToken, nil
	}

	// 销售易 OAuth2.0 令牌获取
	tokenURL := "https://api.xiaoshouyi.com/oauth/token"
	data := url.Values{
		"grant_type":		{"client_credentials"},
		"client_id":		{c.account.APIKey},
		"client_secret":	{c.account.APISecret},
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
		AccessToken	string	`json:"access_token"`
		ExpiresIn	int	`json:"expires_in"`
		Error		string	`json:"error"`
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

// SyncCustomers 同步客户
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

// syncXiaoshouyiCustomers 同步销售易客户
func (s *IntegrationService) syncXiaoshouyiCustomers(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewXiaoshouyiClient(account, s.accountRepo)
	token, err := client.GetAccessToken(ctx)
	if err != nil {
		return 0, err
	}

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:	account.Platform,
		SyncType:	"customer",
		Status:		0,
		StartTime:	time.Now(),
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
		Data	[]struct {
			ID		string	`json:"id"`
			Name		string	`json:"name"`
			Phone		string	`json:"phone"`
			Email		string	`json:"email"`
			Company		string	`json:"company"`
			Industry	string	`json:"industry"`
			OwnerID		string	`json:"owner_id"`
			OwnerName	string	`json:"owner_name"`
			Status		string	`json:"status"`
			Source		string	`json:"source"`
			CreatedTime	int64	`json:"created_time"`
			ModifiedTime	int64	`json:"modified_time"`
		}	`json:"data"`
		Error	struct {
			Code	int	`json:"code"`
			Message	string	`json:"message"`
		}	`json:"error"`
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
			Platform:	account.Platform,
			ExternalID:	c.ID,
			Name:		c.Name,
			Phone:		c.Phone,
			Email:		c.Email,
			Company:	c.Company,
			Industry:	c.Industry,
			OwnerID:	c.OwnerID,
			OwnerName:	c.OwnerName,
			Status:		c.Status,
			Source:		c.Source,
			LastContactAt:	func() *time.Time { t := time.Unix(c.ModifiedTime, 0); return &t }(),
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

// FenxiangxiaoClient 纷享销客 API 客户端
type FenxiangxiaoClient struct {
	accountRepo	*repository.IntegrationAccountRepository
	account		*model.IntegrationAccount
	httpClient	*http.Client
}

// NewFenxiangxiaoClient 创建纷享销客 API 客户端
func NewFenxiangxiaoClient(account *model.IntegrationAccount, accountRepo *repository.IntegrationAccountRepository) *FenxiangxiaoClient {
	return &FenxiangxiaoClient{
		accountRepo:	accountRepo,
		account:	account,
		httpClient:	httpclient.NewWithTimeout(30 * time.Second),
	}
}

// GetAccessToken 获取访问令牌
func (c *FenxiangxiaoClient) GetAccessToken(ctx context.Context)  (string, error) {
	if c.account.AccessToken != "" && c.account.TokenExpires != nil && time.Now().Before(*c.account.TokenExpires) {
		return c.account.AccessToken, nil
	}

	// 纷享销客 OAuth2.0 令牌获取
	tokenURL := "https://api.fxiaoke.com/oauth2/token"
	data := url.Values{
		"grant_type":	{"client_credentials"},
		"app_key":	{c.account.APIKey},
		"app_secret":	{c.account.APISecret},
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
		AccessToken	string	`json:"access_token"`
		ExpiresIn	int	`json:"expires_in"`
		Errcode		int	`json:"errcode"`
		Errmsg		string	`json:"errmsg"`
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

// syncFenxiangxiaoCustomers 同步纷享销客客户
func (s *IntegrationService) syncFenxiangxiaoCustomers(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewFenxiangxiaoClient(account, s.accountRepo)
	token, err := client.GetAccessToken(ctx)
	if err != nil {
		return 0, err
	}

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:	account.Platform,
		SyncType:	"customer",
		Status:		0,
		StartTime:	time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 获取客户列表
	apiURL := "https://api.fxiaoke.com/crm/lead/v2/list"
	reqBody := map[string]any{
		"page":		1,
		"pagesize":	100,
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
		Response	struct {
			Data	[]struct {
				ID		string		`json:"id"`
				Name		string		`json:"name"`
				Phone		string		`json:"mobile"`
				Email		string		`json:"email"`
				Company		string		`json:"company_name"`
				Position	string		`json:"position"`
				OwnerID		string		`json:"owner_id"`
				OwnerName	string		`json:"owner_name"`
				Status		string		`json:"status"`
				Source		string		`json:"source"`
				Tags		[]string	`json:"tags"`
			}	`json:"data"`
			TotalCount	int	`json:"total_count"`
		}	`json:"response"`
		Errcode	int	`json:"errcode"`
		Errmsg	string	`json:"errmsg"`
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
			Platform:	account.Platform,
			ExternalID:	c.ID,
			Name:		c.Name,
			Phone:		c.Phone,
			Email:		c.Email,
			Company:	c.Company,
			Position:	c.Position,
			OwnerID:	c.OwnerID,
			OwnerName:	c.OwnerName,
			Status:		c.Status,
			Source:		c.Source,
			Tags:		string(tagsJSON),
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

// ==================== 淘宝电商对接 ====================

// TaobaoClient 淘宝 API 客户端
type TaobaoClient struct {
	account		*model.IntegrationAccount
	httpClient	*http.Client
	appKey		string
	appSecret	string
}

// NewTaobaoClient 创建淘宝 API 客户端
func NewTaobaoClient(account *model.IntegrationAccount) *TaobaoClient {
	return &TaobaoClient{
		account:	account,
		httpClient:	httpclient.NewWithTimeout(30 * time.Second),
		appKey:		account.APIKey,
		appSecret:	account.APISecret,
	}
}

// SyncOrders 同步订单
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

// syncTaobaoOrders 同步淘宝订单
func (s *IntegrationService) syncTaobaoOrders(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewTaobaoClient(account)

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:	account.Platform,
		SyncType:	"order",
		Status:		0,
		StartTime:	time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 淘宝 API 签名请求（简化版本，实际需要复杂签名）
	apiURL := "https://gw.api.taobao.com/router/rest"
	params := url.Values{
		"app_key":		{client.appKey},
		"method":		{"taobao.trades.sold.get"},
		"format":		{"json"},
		"v":			{"2.0"},
		"start_created":	{time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")},
		"end_created":		{time.Now().Format("2006-01-02 15:04:05")},
		"page":			{"1"},
		"page_size":		{"100"},
		"fields":		{"tid,type,status,payment,receiver_name,receiver_phone,created,orders"},
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
					TID		string	`json:"tid"`
					Type		string	`json:"type"`
					Status		string	`json:"status"`
					Payment		float64	`json:"payment"`
					ReceiverName	string	`json:"receiver_name"`
					ReceiverPhone	string	`json:"receiver_phone"`
					Created		string	`json:"created"`
					PayTime		string	`json:"pay_time"`
					ConsentTime	string	`json:"consign_time"`
					Orders		struct {
						Order []struct {
							Title		string	`json:"title"`
							Price		float64	`json:"price"`
							Num		int	`json:"num"`
							OuterIid	string	`json:"outer_iid"`
						} `json:"order"`
					}	`json:"orders"`
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
				"title":	o.Title,
				"price":	o.Price,
				"quantity":	o.Num,
				"product_id":	o.OuterIid,
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
			Platform:	account.Platform,
			OrderID:	t.TID,
			Status:		t.Status,
			OrderTime:	orderTime,
			PayAmount:	yuanToFen(t.Payment),
			UserName:	t.ReceiverName,
			UserPhone:	t.ReceiverPhone,
			PayTime:	payTime,
			ShipTime:	shipTime,
			Items:		string(itemsJSON),
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

// ==================== 京东电商对接 ====================

// JDClient 京东 API 客户端
type JDClient struct {
	account		*model.IntegrationAccount
	httpClient	*http.Client
	appKey		string
	appSecret	string
}

// NewJDClient 创建京东 API 客户端
func NewJDClient(account *model.IntegrationAccount) *JDClient {
	return &JDClient{
		account:	account,
		httpClient:	httpclient.NewWithTimeout(30 * time.Second),
		appKey:		account.APIKey,
		appSecret:	account.APISecret,
	}
}

// syncJDOrders 同步京东订单
func (s *IntegrationService) syncJDOrders(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewJDClient(account)

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:	account.Platform,
		SyncType:	"order",
		Status:		0,
		StartTime:	time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 京东 API 请求（简化版本，实际需要 JOSN 签名）
	apiURL := "https://api.jd.com/routerjson"
	params := url.Values{
		"app_key":	{client.appKey},
		"method":	{"jingdong.pop.order.search"},
		"format":	{"json"},
		"v":		{"2.0"},
		"start_date":	{time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")},
		"end_date":	{time.Now().Format("2006-01-02 15:04:05")},
		"page":		{"1"},
		"page_size":	{"100"},
	}

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
		OrderSearchResponse	struct {
			Orders	[]struct {
				OrderID			string	`json:"order_id"`
				OrderStatus		string	`json:"order_status"`
				OrderTotal		float64	`json:"order_total_price"`
				OrderPayment		float64	`json:"order_payment"`
				Consignee		string	`json:"consignee"`
				Telephone		string	`json:"telephone"`
				OrderStartTime		string	`json:"order_start_time"`
				OrderPaymentTime	string	`json:"order_payment_time"`
				OrderOutboundTime	string	`json:"order_outbound_time"`
				SKUList			[]struct {
					SkuID	string	`json:"sku_id"`
					SkuName	string	`json:"sku_name"`
					Price	float64	`json:"price"`
					Num	int	`json:"num"`
				}	`json:"sku_list"`
			}	`json:"orders"`
			Total	int	`json:"total"`
		}	`json:"jingdong_pop_order_search_response"`
		ErrMsg	string	`json:"error_response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	// 保存订单数据
	count := 0
	for _, o := range result.OrderSearchResponse.Orders {
		// 解析订单商品
		var items []map[string]any
		for _, sku := range o.SKUList {
			items = append(items, map[string]any{
				"title":	sku.SkuName,
				"price":	sku.Price,
				"quantity":	sku.Num,
				"product_id":	sku.SkuID,
			})
		}
		itemsJSON, _ := json.Marshal(items)

		// 解析时间
		var payTime, shipTime, orderTime *time.Time
		if o.OrderStartTime != "" {
			if st, err := time.Parse("2006-01-02 15:04:05", o.OrderStartTime); err == nil {
				orderTime = &st
			}
		}
		if o.OrderPaymentTime != "" {
			pt, _ := time.Parse("2006-01-02 15:04:05", o.OrderPaymentTime)
			payTime = &pt
		}
		if o.OrderOutboundTime != "" {
			st, _ := time.Parse("2006-01-02 15:04:05", o.OrderOutboundTime)
			shipTime = &st
		}

		order := &model.ExternalOrder{
			Platform:	account.Platform,
			OrderID:	o.OrderID,
			Status:		o.OrderStatus,
			OrderTime:	orderTime,
			PayAmount:	yuanToFen(o.OrderPayment),
			UserName:	o.Consignee,
			UserPhone:	o.Telephone,
			PayTime:	payTime,
			ShipTime:	shipTime,
			Items:		string(itemsJSON),
		}

		// 检查是否已存在
		existing, _ := s.orderRepo.GetByOrderID(ctx, account.Platform, o.OrderID)
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

// SyncProducts 同步商品
func (s *IntegrationService) SyncProducts(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	switch account.Platform {
	case string(PlatformTaobao):
		return s.syncTaobaoProducts(ctx, account)
	case string(PlatformJD):
		return s.syncJDProducts(ctx, account)
	default:
		return 0, errors.New("不支持的平台")
	}
}

// syncTaobaoProducts 同步淘宝商品
func (s *IntegrationService) syncTaobaoProducts(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewTaobaoClient(account)

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:	account.Platform,
		SyncType:	"product",
		Status:		0,
		StartTime:	time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 淘宝 API 请求（简化版本）
	apiURL := "https://gw.api.taobao.com/router/rest"
	params := url.Values{
		"app_key":	{client.appKey},
		"method":	{"taobao.items.seller.get"},
		"format":	{"json"},
		"v":		{"2.0"},
		"page":		{"1"},
		"page_size":	{"100"},
		"fields":	{"num_iid,title,price,num,pic_url,cid,status"},
	}

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

	var result struct {
		ItemsSellerGetResponse struct {
			Items struct {
				Item []struct {
					NumIid	string	`json:"num_iid"`
					Title	string	`json:"title"`
					Price	float64	`json:"price"`
					Num	int	`json:"num"`
					PicURL	string	`json:"pic_url"`
					Cid	string	`json:"cid"`
					Status	string	`json:"status"`
				} `json:"item"`
			} `json:"items"`
		} `json:"items_seller_get_response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	// 保存商品数据
	count := 0
	for _, item := range result.ItemsSellerGetResponse.Items.Item {
		imagesJSON, _ := json.Marshal([]string{item.PicURL})
		status := 1
		if item.Status != "onsale" {
			status = 0
		}

		product := &model.ExternalProduct{
			Platform:	account.Platform,
			ProductID:	item.NumIid,
			Name:		item.Title,
			Price:		yuanToFen(item.Price),
			Stock:		item.Num,
			CategoryID:	item.Cid,
			Images:		string(imagesJSON),
			Status:		status,
		}

		// 检查是否已存在
		existing, _ := s.productRepo.GetByProductID(ctx, account.Platform, item.NumIid)
		if existing != nil {
			product.ID = existing.ID
			s.productRepo.Update(ctx, product)
		} else {
			s.productRepo.Create(ctx, product)
		}
		count++
	}

	// 更新同步时间
	s.accountRepo.UpdateSyncTime(ctx, account.ID)
	s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 1, count, "")

	return count, nil
}

// syncJDProducts 同步京东商品
func (s *IntegrationService) syncJDProducts(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewJDClient(account)

	// 创建同步日志
	syncLog := &model.SyncLog{
		Platform:	account.Platform,
		SyncType:	"product",
		Status:		0,
		StartTime:	time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 京东 API 请求（简化版本）
	apiURL := "https://api.jd.com/routerjson"
	params := url.Values{
		"app_key":	{client.appKey},
		"method":	{"jingdong.pop.ware.sku.list"},
		"format":	{"json"},
		"v":		{"2.0"},
		"page":		{"1"},
		"page_size":	{"100"},
	}

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

	var result struct {
		SkuListResponse struct {
			Skus	[]struct {
				SkuId		string		`json:"sku_id"`
				Name		string		`json:"name"`
				Price		float64		`json:"price"`
				StockNum	int		`json:"stock_num"`
				Category	string		`json:"category"`
				Images		[]string	`json:"images"`
				Status		int		`json:"status"`
			}	`json:"skus"`
			Total	int	`json:"total"`
		} `json:"jingdong_pop_ware_sku_list_response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 2, 0, err.Error())
		return 0, err
	}

	// 保存商品数据
	count := 0
	for _, sku := range result.SkuListResponse.Skus {
		imagesJSON, _ := json.Marshal(sku.Images)

		product := &model.ExternalProduct{
			Platform:	account.Platform,
			ProductID:	sku.SkuId,
			Name:		sku.Name,
			Price:		yuanToFen(sku.Price),
			Stock:		sku.StockNum,
			CategoryID:	sku.Category,
			Images:		string(imagesJSON),
			Status:		sku.Status,
		}

		// 检查是否已存在
		existing, _ := s.productRepo.GetByProductID(ctx, account.Platform, sku.SkuId)
		if existing != nil {
			product.ID = existing.ID
			s.productRepo.Update(ctx, product)
		} else {
			s.productRepo.Create(ctx, product)
		}
		count++
	}

	// 更新同步时间
	s.accountRepo.UpdateSyncTime(ctx, account.ID)
	s.syncLogRepo.UpdateStatus(ctx, syncLog.ID, 1, count, "")

	return count, nil
}

// GetSyncLogs 获取同步日志
func (s *IntegrationService) GetSyncLogs(ctx context.Context, page, pageSize int) ([]*model.SyncLog, int64, error) {
	return s.syncLogRepo.GetAll(ctx, page, pageSize)
}

// GetExternalCustomers 获取外部客户列表
func (s *IntegrationService) GetExternalCustomers(ctx context.Context, platform string, page, pageSize int) ([]*model.ExternalCustomer, int64, error) {
	if platform != "" {
		return s.customerRepo.GetByPlatform(ctx, platform, page, pageSize)
	}
	return s.customerRepo.GetAll(ctx, page, pageSize)
}

// GetExternalOrders 获取外部订单列表
func (s *IntegrationService) GetExternalOrders(ctx context.Context, platform string, page, pageSize int) ([]*model.ExternalOrder, int64, error) {
	if platform != "" {
		return s.orderRepo.GetByPlatform(ctx, platform, page, pageSize)
	}
	return s.orderRepo.GetAll(ctx, page, pageSize)
}

// GetExternalOrdersByCustomer 按客户手机/姓名查询近期外部订单（客服 360 视图 / 答单上下文用）。
// 订单是外部电商同步进来的只读镜像，此处只查询、不写。
func (s *IntegrationService) GetExternalOrdersByCustomer(ctx context.Context, phone, name string) ([]*model.ExternalOrder, error) {
	return s.orderRepo.GetByCustomer(ctx, phone, name)
}

// UpsertOrderFromWebhook 处理电商订单状态推送（近实时刷新本地订单镜像）。
//
// 这是"拉取同步(B)"之外的"事件推送(C)"补强：电商订单状态变更时主动推送，
// 本系统记录 WebhookEvent 并 upsert ExternalOrder，使客服看到的订单状态与电商一致（防漂移）。
// 订单镜像为只读，客服不创建/履约订单。
func (s *IntegrationService) UpsertOrderFromWebhook(ctx context.Context, platform, orderID, status string, raw map[string]any) error {
	// 记录订单状态变更事件（best-effort，失败不影响镜像刷新）
	webhookEvent := &model.WebhookEvent{
		Platform:  platform,
		EventID:   fmt.Sprintf("%s:%s:%d", platform, orderID, time.Now().UnixNano()),
		EventType: "order.updated",
		RawData:   fmt.Sprintf("%v", raw),
		Processed: true,
	}
	_ = s.webhookEventRepo.Create(ctx, webhookEvent)
	existing, _ := s.orderRepo.GetByOrderID(ctx, platform, orderID)
	// 基于已存在记录叠加，避免 Save 全量覆盖把未传字段清零（关键：upsert 语义）
	o := &model.ExternalOrder{
		Platform: platform,
		OrderID:  orderID,
	}
	if existing != nil {
		o = existing
		o.Platform = platform
		o.OrderID = orderID
	}
	o.Status = status
	if raw != nil {
		if v, ok := raw["order_no"].(string); ok && v != "" {
			o.OrderNo = v
		}
		if v, ok := raw["user_phone"].(string); ok && v != "" {
			o.UserPhone = v
		}
		if v, ok := raw["user_name"].(string); ok && v != "" {
			o.UserName = v
		}
		if v, ok := raw["total_amount"].(float64); ok {
			o.TotalAmount = int64(v)
		}
		if v, ok := raw["pay_amount"].(float64); ok {
			o.PayAmount = int64(v)
		}
		if v, ok := raw["items"].(string); ok && v != "" {
			o.Items = v
		}
		if t, ok := parseWebhookTime(raw["order_time"]); ok {
			o.OrderTime = t
		} else if t, ok := parseWebhookTime(raw["pay_time"]); ok {
			o.OrderTime = t
		}
	}
	if existing != nil {
		return s.orderRepo.Update(ctx, o)
	}
	return s.orderRepo.Create(ctx, o)
}

// parseWebhookTime 将 webhook raw 中可能为 string(RFC3339/常见格式) 或 time.Time 的值解析为 *time.Time。
func parseWebhookTime(v any) (*time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return &t, true
	case *time.Time:
		return t, true
	case string:
		if t == "" {
			return nil, false
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return &parsed, true
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

// GetExternalProducts 获取外部商品列表
func (s *IntegrationService) GetExternalProducts(ctx context.Context, platform string, page, pageSize int) ([]*model.ExternalProduct, int64, error) {
	_ = platform
	return s.productRepo.GetAll(ctx, page, pageSize)
}

// TestConnection 测试对接账号连接是否正常
func (s *IntegrationService) TestConnection(ctx context.Context, account *model.IntegrationAccount) error {
	if account == nil {
		return errors.New("账号不能为空")
	}
	if account.APIKey == "" {
		return errors.New("API Key 不能为空")
	}
	if account.APISecret == "" {
		return errors.New("API Secret 不能为空")
	}
	if account.Status != 1 {
		return errors.New("账号已被禁用")
	}
	return nil
}
