package tooluse

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// registry.go 工具注册中心（PRD §5.2）
//
// 设计目标：
//  1. 全局唯一 ToolRegistry 单例
//  2. 支持按 name 查询 / 按 category 列表
//  3. 支持导出为 LLM Function Calling 格式
//  4. 线程安全（sync.RWMutex）

// ErrToolNotFound 工具未找到
var ErrToolNotFound = errors.New("tool not found")

// ErrToolAlreadyExists 工具已存在
var ErrToolAlreadyExists = errors.New("tool already exists")

// ToolRegistry 工具注册中心
type ToolRegistry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewToolRegistry 创建新的工具注册中心
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
// 同名工具重复注册返回 ErrToolAlreadyExists
func (r *ToolRegistry) Register(t Tool) error {
	if t == nil {
		return errors.New("cannot register nil tool")
	}
	if t.Name() == "" {
		return errors.New("tool name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("%w: %s", ErrToolAlreadyExists, t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

// MustRegister 注册工具，出错 panic（用于初始化时）
func (r *ToolRegistry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(fmt.Sprintf("注册工具 %s 失败：%v", t.Name(), err))
	}
}

// Unregister 注销工具
func (r *ToolRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	delete(r.tools, name)
	return nil
}

// Get 按 name 获取工具
func (r *ToolRegistry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return t, nil
}

// Has 判断工具是否存在
func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.tools[name]
	return exists
}

// List 列出所有工具（按 name 排序）
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

// ListByCategory 按 category 列出工具（按 name 排序）
func (r *ToolRegistry) ListByCategory(category ToolCategory) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Tool, 0)
	for _, t := range r.tools {
		if t.Category() == category {
			list = append(list, t)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

// Count 返回已注册工具数量
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// CountByCategory 按 category 统计工具数量
func (r *ToolRegistry) CountByCategory(category ToolCategory) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, t := range r.tools {
		if t.Category() == category {
			count++
		}
	}
	return count
}

// ToLLMFunctions 导出所有工具为 LLM Function Calling 格式
func (r *ToolRegistry) ToLLMFunctions() []LLMFunction {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]LLMFunction, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, ToLLMFunction(t))
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// ToLLMFunctionsByCategory 按 category 导出 LLM Function Calling 格式
func (r *ToolRegistry) ToLLMFunctionsByCategory(category ToolCategory) []LLMFunction {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]LLMFunction, 0)
	for _, t := range r.tools {
		if t.Category() == category {
			list = append(list, ToLLMFunction(t))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// ListNames 列出所有工具名称（按 name 排序）
func (r *ToolRegistry) ListNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ===== 全局注册中心 =====

var (
	globalRegistry     *ToolRegistry
	globalRegistryOnce sync.Once
)

// GetGlobalRegistry 获取全局工具注册中心
func GetGlobalRegistry() *ToolRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewToolRegistry()
	})
	return globalRegistry
}

// RegisterGlobal 注册到全局注册中心
func RegisterGlobal(t Tool) error {
	return GetGlobalRegistry().Register(t)
}

// MustRegisterGlobal 注册到全局注册中心，出错 panic
func MustRegisterGlobal(t Tool) {
	GetGlobalRegistry().MustRegister(t)
}
