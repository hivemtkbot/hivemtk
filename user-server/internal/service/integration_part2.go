// 拆分自 integration.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hivemtk-user/internal/model"
	"io"
	"net/url"
	"time"
)

func (s *IntegrationService) syncJDOrders(ctx context.Context, account *model.IntegrationAccount) (int, error) {
	client := NewJDClient(account)

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

	// 京东 API 请求（简化版本，实际需要 JOSN 签名）
	apiURL := "https://api.jd.com/routerjson"
	params := url.Values{
		"app_key":    {client.appKey},
		"method":     {"jingdong.pop.order.search"},
		"format":     {"json"},
		"v":          {"2.0"},
		"start_date": {time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")},
		"end_date":   {time.Now().Format("2006-01-02 15:04:05")},
		"page":       {"1"},
		"page_size":  {"100"},
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
		OrderSearchResponse struct {
			Orders []struct {
				OrderID           string  `json:"order_id"`
				OrderStatus       string  `json:"order_status"`
				OrderTotal        float64 `json:"order_total_price"`
				OrderPayment      float64 `json:"order_payment"`
				Consignee         string  `json:"consignee"`
				Telephone         string  `json:"telephone"`
				OrderStartTime    string  `json:"order_start_time"`
				OrderPaymentTime  string  `json:"order_payment_time"`
				OrderOutboundTime string  `json:"order_outbound_time"`
				SKUList           []struct {
					SkuID   string  `json:"sku_id"`
					SkuName string  `json:"sku_name"`
					Price   float64 `json:"price"`
					Num     int     `json:"num"`
				} `json:"sku_list"`
			} `json:"orders"`
			Total int `json:"total"`
		} `json:"jingdong_pop_order_search_response"`
		ErrMsg string `json:"error_response"`
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
				"title":      sku.SkuName,
				"price":      sku.Price,
				"quantity":   sku.Num,
				"product_id": sku.SkuID,
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
			Platform:  account.Platform,
			OrderID:   o.OrderID,
			Status:    o.OrderStatus,
			OrderTime: orderTime,
			PayAmount: yuanToFen(o.OrderPayment),
			UserName:  o.Consignee,
			UserPhone: o.Telephone,
			PayTime:   payTime,
			ShipTime:  shipTime,
			Items:     string(itemsJSON),
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
		Platform:  account.Platform,
		SyncType:  "product",
		Status:    0,
		StartTime: time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 淘宝 API 请求（简化版本）
	apiURL := "https://gw.api.taobao.com/router/rest"
	params := url.Values{
		"app_key":   {client.appKey},
		"method":    {"taobao.items.seller.get"},
		"format":    {"json"},
		"v":         {"2.0"},
		"page":      {"1"},
		"page_size": {"100"},
		"fields":    {"num_iid,title,price,num,pic_url,cid,status"},
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
					NumIid string  `json:"num_iid"`
					Title  string  `json:"title"`
					Price  float64 `json:"price"`
					Num    int     `json:"num"`
					PicURL string  `json:"pic_url"`
					Cid    string  `json:"cid"`
					Status string  `json:"status"`
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
			Platform:   account.Platform,
			ProductID:  item.NumIid,
			Name:       item.Title,
			Price:      yuanToFen(item.Price),
			Stock:      item.Num,
			CategoryID: item.Cid,
			Images:     string(imagesJSON),
			Status:     status,
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
		Platform:  account.Platform,
		SyncType:  "product",
		Status:    0,
		StartTime: time.Now(),
	}
	if err := s.syncLogRepo.Create(ctx, syncLog); err != nil {
		return 0, err
	}

	// 京东 API 请求（简化版本）
	apiURL := "https://api.jd.com/routerjson"
	params := url.Values{
		"app_key":   {client.appKey},
		"method":    {"jingdong.pop.ware.sku.list"},
		"format":    {"json"},
		"v":         {"2.0"},
		"page":      {"1"},
		"page_size": {"100"},
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
			Skus []struct {
				SkuId    string   `json:"sku_id"`
				Name     string   `json:"name"`
				Price    float64  `json:"price"`
				StockNum int      `json:"stock_num"`
				Category string   `json:"category"`
				Images   []string `json:"images"`
				Status   int      `json:"status"`
			} `json:"skus"`
			Total int `json:"total"`
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
			Platform:   account.Platform,
			ProductID:  sku.SkuId,
			Name:       sku.Name,
			Price:      yuanToFen(sku.Price),
			Stock:      sku.StockNum,
			CategoryID: sku.Category,
			Images:     string(imagesJSON),
			Status:     sku.Status,
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
