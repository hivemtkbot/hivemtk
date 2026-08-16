package tooluse

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)


// ToolDiscovery 工具发现接口
type ToolDiscovery interface {
	Search(query string, limit int) ([]Tool, error)

	ListByTag(tag string) ([]Tool, error)

	GetTool(name string) (Tool, error)

	ListAll() []ToolInfo
}

// ToolInfo 工具元信息（不包含实际工具实例）
type ToolInfo struct {
	Name        string       `json:"name"`
	Category    ToolCategory `json:"category"`
	Description string       `json:"description"`
	Tags        []string     `json:"tags,omitempty"`
	Loaded      bool         `json:"loaded"` 
}


// DefaultToolDiscovery 默认工具发现实现
type DefaultToolDiscovery struct {
	registry *ToolRegistry
	index    *ToolIndex
}

// ToolIndex 工具索引（支持快速搜索）
type ToolIndex struct {
	byName     map[string]*ToolInfo
	byTag      map[string][]string 
	byCategory map[ToolCategory][]string
	mu         sync.RWMutex
}

// NewToolIndex 创建工具索引
func NewToolIndex() *ToolIndex {
	return &ToolIndex{
		byName:     make(map[string]*ToolInfo),
		byTag:      make(map[string][]string),
		byCategory: make(map[ToolCategory][]string),
	}
}

// Add 添加工具到索引
func (idx *ToolIndex) Add(info *ToolInfo) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.byName[info.Name] = info

	for _, tag := range info.Tags {
		idx.byTag[tag] = append(idx.byTag[tag], info.Name)
	}

	idx.byCategory[info.Category] = append(idx.byCategory[info.Category], info.Name)
}

// Remove 从索引移除工具
func (idx *ToolIndex) Remove(name string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	info, exists := idx.byName[name]
	if !exists {
		return
	}

	delete(idx.byName, name)

	for _, tag := range info.Tags {
		names := idx.byTag[tag]
		for i, n := range names {
			if n == name {
				idx.byTag[tag] = append(names[:i], names[i+1:]...)
				break
			}
		}
	}

	names := idx.byCategory[info.Category]
	for i, n := range names {
		if n == name {
			idx.byCategory[info.Category] = append(names[:i], names[i+1:]...)
			break
		}
	}
}

// Search 搜索工具
func (idx *ToolIndex) Search(query string, limit int) []*ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	query = strings.ToLower(query)
	results := make([]*ToolInfo, 0)

	if info, exists := idx.byName[query]; exists {
		results = append(results, info)
		if limit > 0 && len(results) >= limit {
			return results
		}
	}

	for _, info := range idx.byName {
		if strings.Contains(strings.ToLower(info.Name), query) ||
			strings.Contains(strings.ToLower(info.Description), query) {
			duplicate := false
			for _, r := range results {
				if r.Name == info.Name {
					duplicate = true
					break
				}
			}
			if !duplicate {
				results = append(results, info)
				if limit > 0 && len(results) >= limit {
					return results
				}
			}
		}
	}

	return results
}

// ListByTag 按标签列出
func (idx *ToolIndex) ListByTag(tag string) []*ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	names := idx.byTag[tag]
	infos := make([]*ToolInfo, 0, len(names))
	for _, name := range names {
		if info, exists := idx.byName[name]; exists {
			infos = append(infos, info)
		}
	}
	return infos
}

// ListByCategory 按分类列出
func (idx *ToolIndex) ListByCategory(category ToolCategory) []*ToolInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	names := idx.byCategory[category]
	infos := make([]*ToolInfo, 0, len(names))
	for _, name := range names {
		if info, exists := idx.byName[name]; exists {
			infos = append(infos, info)
		}
	}
	return infos
}


// NewDefaultToolDiscovery 创建默认工具发现实例
func NewDefaultToolDiscovery(registry *ToolRegistry) *DefaultToolDiscovery {
	d := &DefaultToolDiscovery{
		registry: registry,
		index:    NewToolIndex(),
	}

	d.rebuildIndex()

	return d
}

// rebuildIndex 重建索引
func (d *DefaultToolDiscovery) rebuildIndex() {
	d.index.mu.Lock()
	defer d.index.mu.Unlock()

	d.index.byName = make(map[string]*ToolInfo)
	d.index.byTag = make(map[string][]string)
	d.index.byCategory = make(map[ToolCategory][]string)

	tools := d.registry.List()
	for _, tool := range tools {
		info := &ToolInfo{
			Name:        tool.Name(),
			Category:    tool.Category(),
			Description: tool.Description(),
			Loaded:      true,
		}
		d.index.byName[info.Name] = info
		for _, tag := range info.Tags {
			d.index.byTag[tag] = append(d.index.byTag[tag], info.Name)
		}
		d.index.byCategory[info.Category] = append(d.index.byCategory[info.Category], info.Name)
	}
}

// Search 根据查询搜索相关工具
func (d *DefaultToolDiscovery) Search(query string, limit int) ([]Tool, error) {
	infos := d.index.Search(query, limit)

	tools := make([]Tool, 0, len(infos))
	for _, info := range infos {
		tool, err := d.registry.Get(info.Name)
		if err != nil {
			continue
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

// ListByTag 按标签列出工具
func (d *DefaultToolDiscovery) ListByTag(tag string) ([]Tool, error) {
	infos := d.index.ListByTag(tag)

	tools := make([]Tool, 0, len(infos))
	for _, info := range infos {
		tool, err := d.registry.Get(info.Name)
		if err != nil {
			continue
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

// GetTool 获取单个工具
func (d *DefaultToolDiscovery) GetTool(name string) (Tool, error) {
	return d.registry.Get(name)
}

// ListAll 列出所有可用工具
func (d *DefaultToolDiscovery) ListAll() []ToolInfo {
	d.index.mu.RLock()
	defer d.index.mu.RUnlock()

	infos := make([]ToolInfo, 0, len(d.index.byName))
	for _, info := range d.index.byName {
		infos = append(infos, *info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos
}

// Refresh 刷新索引
func (d *DefaultToolDiscovery) Refresh() {
	d.rebuildIndex()
}


// ToolLoader 工具加载器接口
type ToolLoader interface {
	Load(name string) (Tool, error)
	LoadAll() ([]Tool, error)
	IsLoaded(name string) bool
}

// LazyToolLoader 延迟加载工具加载器
type LazyToolLoader struct {
	registry  *ToolRegistry
	factories map[string]ToolFactory
	mu        sync.RWMutex
}

// ToolFactory 工具工厂函数
type ToolFactory func() (Tool, error)

// NewLazyToolLoader 创建延迟加载工具加载器
func NewLazyToolLoader(registry *ToolRegistry) *LazyToolLoader {
	return &LazyToolLoader{
		registry:  registry,
		factories: make(map[string]ToolFactory),
	}
}

// RegisterFactory 注册工具工厂
func (l *LazyToolLoader) RegisterFactory(name string, factory ToolFactory) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.factories[name] = factory
}

// Load 加载指定工具
func (l *LazyToolLoader) Load(name string) (Tool, error) {
	if l.registry.Has(name) {
		return l.registry.Get(name)
	}

	l.mu.RLock()
	factory, exists := l.factories[name]
	l.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	tool, err := factory()
	if err != nil {
		return nil, fmt.Errorf("load tool %s failed: %w", name, err)
	}

	if err := l.registry.Register(tool); err != nil {
		return nil, fmt.Errorf("register tool %s failed: %w", name, err)
	}

	return tool, nil
}

// LoadAll 加载所有工具
func (l *LazyToolLoader) LoadAll() ([]Tool, error) {
	l.mu.RLock()
	factories := make(map[string]ToolFactory)
	for k, v := range l.factories {
		factories[k] = v
	}
	l.mu.RUnlock()

	tools := make([]Tool, 0, len(factories))
	for name := range factories {
		tool, err := l.Load(name)
		if err != nil {
			continue
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

// IsLoaded 检查工具是否已加载
func (l *LazyToolLoader) IsLoaded(name string) bool {
	return l.registry.Has(name)
}


// LazyToolRegistry 延迟加载注册中心
type LazyToolRegistry struct {
	*ToolRegistry
	discovery *DefaultToolDiscovery
	loader    *LazyToolLoader
	cache     map[string]Tool
	mu        sync.RWMutex
}

// NewLazyToolRegistry 创建延迟加载注册中心
func NewLazyToolRegistry() *LazyToolRegistry {
	registry := NewToolRegistry()
	return &LazyToolRegistry{
		ToolRegistry: registry,
		discovery:    NewDefaultToolDiscovery(registry),
		loader:       NewLazyToolLoader(registry),
		cache:        make(map[string]Tool),
	}
}

// Register 注册工具并刷新索引
func (r *LazyToolRegistry) Register(t Tool) error {
	err := r.ToolRegistry.Register(t)
	if err == nil {
		r.discovery.Refresh()
	}
	return err
}

// MustRegister 注册工具，出错 panic
func (r *LazyToolRegistry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// GetTool 获取工具（支持延迟加载）
func (r *LazyToolRegistry) GetTool(name string) (Tool, error) {
	r.mu.RLock()
	if tool, exists := r.cache[name]; exists {
		r.mu.RUnlock()
		return tool, nil
	}
	r.mu.RUnlock()

	tool, err := r.loader.Load(name)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[name] = tool
	r.mu.Unlock()

	return tool, nil
}

// Search 搜索工具
func (r *LazyToolRegistry) Search(query string, limit int) ([]Tool, error) {
	return r.discovery.Search(query, limit)
}

// ListByTag 按标签列出工具
func (r *LazyToolRegistry) ListByTag(tag string) ([]Tool, error) {
	return r.discovery.ListByTag(tag)
}

// ListAll 列出所有工具信息
func (r *LazyToolRegistry) ListAll() []ToolInfo {
	return r.discovery.ListAll()
}

// RegisterFactory 注册工具工厂（用于延迟加载）
func (r *LazyToolRegistry) RegisterFactory(name string, factory ToolFactory) {
	r.loader.RegisterFactory(name, factory)
}

// Refresh 刷新工具索引
func (r *LazyToolRegistry) Refresh() {
	r.discovery.Refresh()
}

// ClearCache 清空缓存
func (r *LazyToolRegistry) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]Tool)
}

