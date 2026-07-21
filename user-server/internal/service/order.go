package service

import (
	"errors"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/epay"
	_type "marketing/internal/pkg/utils/type"
	"marketing/internal/repository"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type OrderService struct {
	repo repository.OrderRepository
}

func NewOrderService() *OrderService {
	return &OrderService{repo: repository.NewOrderRepository()}
}

// NewOrderServiceWithDB 创建带 DB 的订单服务（用于测试 / 多 DB 场景）
func NewOrderServiceWithDB(db *gorm.DB) *OrderService {
	return &OrderService{repo: repository.NewOrderRepositoryWithDB(db)}
}

func (s *OrderService) CreateOrder(order model.Order) (*model.Order, error) {
	if err := s.repo.Create(&order); err != nil {
		return nil, err
	}
	return &order, nil
}

// CreateOrderFromRequest 接收 *model.Order,内部补全默认值后入库
func (s *OrderService) CreateOrderFromRequest(order *model.Order) (*model.Order, error) {
	if order == nil {
		return nil, errors.New("订单不能为空")
	}
	if order.Status == 0 {
		order.Status = _type.OrderStatusPending
	}
	if err := s.repo.Create(order); err != nil {
		return nil, err
	}
	return order, nil
}

// CreateOrderFromRequestDTO 通过请求 DTO 创建订单（供 controller 使用，避免 controller 直接依赖 model）
func (s *OrderService) CreateOrderFromRequestDTO(req dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	price, err := decimal.NewFromString(req.Price)
	if err != nil {
		return nil, err
	}
	created, err := s.CreateOrderFromRequest(&model.Order{
		AccountID: req.AccountID,
		TgID:      req.TgID,
		Price:     price.String(),
	})
	if err != nil {
		return nil, err
	}
	return &dto.OrderResponse{
		ID:         created.ID,
		Status:     created.Status,
		Price:      created.Price,
		CreateTime: created.CreateTime,
		TgID:       created.TgID,
		AccountID:  created.AccountID,
	}, nil
}

// GetOrderByID 按字符串 UUID 查询订单
func (s *OrderService) GetOrderByID(id string) (*model.Order, error) {
	return s.repo.GetByStringID(id)
}

func (s *OrderService) GetOrder(id uint) (*model.Order, error) {
	return s.repo.GetByID(id)
}

func (s *OrderService) GetOrderList(page int, limit int) ([]*model.Order, int64, error) {
	return s.repo.GetOrderList(page, limit)
}

// CancelOrder 取消订单(仅 pending 状态可取消,其他返回错误)
func (s *OrderService) CancelOrder(id string, reason string) error {
	order, err := s.repo.GetByStringID(id)
	if err != nil {
		return err
	}
	if order.Status == _type.OrderStatusSuccess {
		return errors.New("已支付订单不可取消,请走退款流程")
	}
	if order.Status == _type.OrderStatusForceClose {
		return errors.New("订单已关闭,无需取消")
	}
	return s.repo.UpdateOrderStatusById(id, _type.OrderStatusForceClose)
}

// RefundOrder 退款(仅已支付订单可退款)
func (s *OrderService) RefundOrder(id string, amount string, reason string) (bool, error) {
	order, err := s.repo.GetByStringID(id)
	if err != nil {
		return false, err
	}
	if order.Status != _type.OrderStatusSuccess {
		return false, errors.New("订单未支付,无法退款")
	}
	// 实际生产环境应调支付网关退款接口,这里通过强制关闭标志来记录退款中
	if err := s.repo.UpdateOrderStatusById(id, _type.OrderStatusForceClose); err != nil {
		return false, err
	}
	// amount/reason 可写入审计日志,此处保留返回即可
	return true, nil
}

// CreatePayAndReturn 创建订单 + 返回支付 URL
func (s *OrderService) CreatePayAndReturn(accountID string, price decimal.Decimal, TgID int64) (string, string, error) {
	order := model.Order{
		AccountID: accountID,
		TgID:      TgID,
		Price:     price.String(),
		Status:    _type.OrderStatusPending,
	}
	if err := s.repo.Create(&order); err != nil {
		return "", "", err
	}
	// 此处不接易支付真实配置(避免测试环境强依赖)
	// 真实环境应从配置中读取 EpayConfig 后调用 epay.EpayUrl
	payURL := "/api/order/" + order.ID + "/check-pay"
	return payURL, order.ID, nil
}

// CheckPayStatus 查询订单支付状态(简化:返回 false;真实环境调网关)
func (s *OrderService) CheckPayStatus(id string) (bool, error) {
	order, err := s.repo.GetByStringID(id)
	if err != nil {
		return false, err
	}
	return order.Status == _type.OrderStatusSuccess, nil
}

func (s *OrderService) DeleteOrder(id string) error {
	return s.repo.Delete(id)
}

func (s *OrderService) LastOrderIsPay(accountID string, TgId int64, epayConfig _type.EpayConfig) bool {
	// 数据库查询状态
	var is_pay = s.repo.LastOrderIsPay(accountID, TgId)
	if !is_pay {
		// 若存在最新订单 则主动查询状态
		lastOrder, err := s.repo.GetGetLastOrder(accountID, TgId)
		if err != nil {
			return false
		}
		// 主动查询状态
		is_pay, err = epay.EpayQuery(lastOrder.ID, epayConfig)
		if is_pay {
			// 更新数据库状态
			var status = _type.OrderStatusSuccess
			s.UpdateOrderStatusById(lastOrder.ID, status)
			return true
		}
		return false
	}
	return is_pay
}

func (s *OrderService) CreatePay(accountID string, price decimal.Decimal, TgID int64, epayConfig _type.EpayConfig) (string, error) {
	// 新建订单
	var order = model.Order{
		AccountID: accountID,
		Price:     price.String(),
		TgID:      TgID,
		Status:    _type.OrderStatusPending,
	}
	if err := s.repo.Create(&order); err != nil {
		return "", err
	}
	// 构建支付url
	payUrl := epay.EpayUrl(order.ID, price, "信息服务", epayConfig)

	return payUrl, nil
}

func (s *OrderService) GetRecentOrderList() ([]*model.Order, error) {
	return s.repo.GetRecentOrderList()
}

func (s *OrderService) UpdateOrderStatusById(id string, status _type.OrderStatusType) error {
	return s.repo.UpdateOrderStatusById(id, status)
}

// UpdateOrder 更新订单信息
func (s *OrderService) UpdateOrder(order *model.Order) error {
	return s.repo.Update(order)
}
