package platform

import (
	"fmt"
	"sync"

	"hivemtk-user/internal/model"
)

// Adapter 平台适配器接口(供 service / controller 通过 registry 使用)
type Adapter interface {
	GetPlatform() model.Platform
	Login(credentials map[string]string) (*model.PlatformAccount, error)
	GetMessages(accountID string, opts *model.MessageQueryOptions) ([]*model.UnifiedMessage, error)
	SendMessage(accountID, chatID, content string, opts *model.SendOptions) (*model.UnifiedReply, error)
	SendImage(accountID, chatID, imageURL string) (*model.UnifiedReply, error)
	CheckLoginStatus(accountID string) (bool, error)
	Logout(accountID string) error
	RefreshToken(accountID string) error
	GetUserInfo(accountID, userID string) (*model.PlatformUser, error)
	GetChatInfo(accountID, chatID string) (*model.ChatInfo, error)
	ParseWebhook(data []byte) (*model.UnifiedMessage, error)
	GetWebhookURL(accountID string) string
}

// AdapterRegistry 平台适配器注册中心(单例)
// 进程启动时由 init() 注册各平台工厂,后续 service / controller 通过 Get 获取
type AdapterRegistry struct {
	mu        sync.RWMutex
	adapters  map[model.Platform]Adapter
	platforms []model.Platform // 注册顺序,方便前端展示
}

// 全局注册中心(单例)
var (
	registryOnce sync.Once
	registryInst *AdapterRegistry
)

// init 进程启动时把各平台适配器注册到 registry
func init() {
	registryInst = newAdapterRegistry()
	MustRegister(model.PlatformDouyin, NewBrowserAdapter(model.PlatformDouyin))
	MustRegister(model.PlatformKuaishou, NewBrowserAdapter(model.PlatformKuaishou))
	MustRegister(model.PlatformXiaohongshu, NewBrowserAdapter(model.PlatformXiaohongshu))
	MustRegister(model.PlatformXianyu, NewBrowserAdapter(model.PlatformXianyu))
	MustRegister(model.PlatformTiktok, NewBrowserAdapter(model.PlatformTiktok))
}

func newAdapterRegistry() *AdapterRegistry {
	return &AdapterRegistry{adapters: make(map[model.Platform]Adapter)}
}

// NewAdapterRegistry 创建一个新的适配器注册中心(供测试使用，自动注册 4 个内置平台)
func NewAdapterRegistry() *AdapterRegistry {
	r := newAdapterRegistry()
	_ = r.Register(model.PlatformDouyin, NewBrowserAdapter(model.PlatformDouyin))
	_ = r.Register(model.PlatformKuaishou, NewBrowserAdapter(model.PlatformKuaishou))
	_ = r.Register(model.PlatformXiaohongshu, NewBrowserAdapter(model.PlatformXiaohongshu))
	_ = r.Register(model.PlatformXianyu, NewBrowserAdapter(model.PlatformXianyu))
	return r
}

// GetAdapterRegistry 获取全局注册中心
func GetAdapterRegistry() *AdapterRegistry {
	registryOnce.Do(func() {
		if registryInst == nil {
			registryInst = newAdapterRegistry()
		}
	})
	return registryInst
}

// Register 注册一个平台适配器
func (r *AdapterRegistry) Register(p model.Platform, a Adapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[p]; exists {
		return fmt.Errorf("platform %s already registered", p)
	}
	r.adapters[p] = a
	r.platforms = append(r.platforms, p)
	return nil
}

// MustRegister 注册失败时 panic(通常在 init 中调用)
func MustRegister(p model.Platform, a Adapter) {
	if err := GetAdapterRegistry().Register(p, a); err != nil {
		panic(err)
	}
}

// Get 根据平台取适配器
func (r *AdapterRegistry) Get(p model.Platform) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[p]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s", p)
	}
	return a, nil
}

// GetPlatforms 返回所有已注册平台(按注册顺序)
func (r *AdapterRegistry) GetPlatforms() []model.Platform {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Platform, len(r.platforms))
	copy(out, r.platforms)
	return out
}

// GetAll 返回所有已注册的适配器
func (r *AdapterRegistry) GetAll() map[model.Platform]Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[model.Platform]Adapter, len(r.adapters))
	for k, v := range r.adapters {
		out[k] = v
	}
	return out
}

// GetAdapter 是 GetAdapterRegistry().Get 的快捷封装
// 接受 model.Platform 类型的入参(常见于 service 直接用 account.Platform)
func GetAdapter(p model.Platform) (Adapter, error) {
	return GetAdapterRegistry().Get(p)
}
