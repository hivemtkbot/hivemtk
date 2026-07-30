# AI Agent工具注入与代理循环全面重构设计文档

## 1. 背景与目标

### 1.1 背景
基于对LangChain、AutoGPT、MCP、OpenAI等主流框架的深入调研，发现当前项目与业界标准存在3个主要差距：
1. 缺少动态工具发现和延迟加载机制
2. 缺少流式输出支持
3. 缺少上下文压缩和工具调用历史管理

### 1.2 目标
- 实现动态工具发现和延迟加载，避免全量注入上下文窗口
- 实现流式输出，支持工具调用过程的实时反馈
- 实现上下文压缩，自动管理工具调用历史
- 保持向后兼容，不破坏现有功能

## 2. 设计方案

### 2.1 工具注入优化

#### 2.1.1 动态工具发现机制

**新增接口**：
```go
// ToolDiscovery 工具发现接口
type ToolDiscovery interface {
    // Search 根据查询搜索相关工具
    Search(query string, limit int) ([]Tool, error)
    // ListByTag 按标签列出工具
    ListByTag(tag string) ([]Tool, error)
    // GetTool 获取单个工具（延迟加载）
    GetTool(name string) (Tool, error)
}
```

**新增实现**：
```go
// LazyToolRegistry 延迟加载注册中心
type LazyToolRegistry struct {
    *ToolRegistry
    discovery ToolDiscovery
    loader    ToolLoader
    cache     map[string]Tool
    mu        sync.RWMutex
}
```

#### 2.1.2 工具定义标准化

**修改ToolParameters**：
```go
type ToolParameters struct {
    Type       string               `json:"type"`
    Properties map[string]ToolParam `json:"properties"`
    Required   []string             `json:"required,omitempty"`
    Definitions map[string]ToolParam `json:"definitions,omitempty"`
}
```

### 2.2 代理循环增强

#### 2.2.1 流式输出支持

**新增类型**：
```go
// StreamEvent 流式事件
type StreamEvent struct {
    Type      string      `json:"type"`
    ToolName  string      `json:"tool_name,omitempty"`
    Content   string      `json:"content,omitempty"`
    Timestamp time.Time   `json:"timestamp"`
}

// StreamHandler 流式处理器
type StreamHandler interface {
    OnEvent(event StreamEvent)
    OnError(err error)
    OnComplete()
}
```

**修改InferenceCycle**：
```go
func (c *InferenceCycle) RunOnceStream(ctx context.Context, payload CustomerMessagePayload, agentCtx *AgentContext, handler StreamHandler) (*InferenceDecision, error)
```

#### 2.2.2 上下文压缩机制

**新增接口**：
```go
// ContextCompressor 上下文压缩器
type ContextCompressor interface {
    // Compress 压缩历史消息
    Compress(messages []Message, maxTokens int) ([]Message, error)
    // ShouldCompress 判断是否需要压缩
    ShouldCompress(messages []Message, maxTokens int) bool
}
```

**新增实现**：
```go
// SummarizationCompressor 摘要压缩器
type SummarizationCompressor struct {
    llmClient LLMClient
    threshold float64
}
```

#### 2.2.3 工具调用历史管理

**新增类型**：
```go
// ToolCallHistory 工具调用历史
type ToolCallHistory struct {
    Calls    []ToolCallRecord `json:"calls"`
    Results  map[string]ToolResult `json:"results"`
    mu       sync.RWMutex
}

// ToolCallRecord 工具调用记录
type ToolCallRecord struct {
    CallID    string    `json:"call_id"`
    ToolName  string    `json:"tool_name"`
    Args      map[string]any `json:"args"`
    Result    ToolResult `json:"result"`
    ExecutedAt time.Time `json:"executed_at"`
}
```

## 3. 实施计划

### 阶段1：工具注入优化（1-2天）
1. 实现`LazyToolRegistry`和`ToolDiscovery`接口
2. 修改工具定义支持标准JSON Schema
3. 添加工具搜索和延迟加载功能

### 阶段2：代理循环增强（2-3天）
1. 实现`StreamHandler`和流式输出
2. 实现`ContextCompressor`上下文压缩
3. 实现`ToolCallHistory`历史管理

### 阶段3：集成测试（1天）
1. 单元测试覆盖
2. 集成测试验证
3. 性能测试对比

## 4. 风险与 mitigation

### 4.1 向后兼容风险
- **风险**：修改工具定义格式可能破坏现有工具
- **mitigation**：保持ToolParameters结构兼容，仅添加新字段

### 4.2 性能风险
- **风险**：动态工具发现可能增加延迟
- **mitigation**：实现本地缓存，减少重复查询

### 4.3 复杂度风险
- **风险**：全面重构增加代码复杂度
- **mitigation**：分阶段实施，充分测试

## 5. 验收标准

### 5.1 功能验收
- [ ] 支持动态工具发现和延迟加载
- [ ] 支持工具调用过程的流式输出
- [ ] 支持上下文自动压缩
- [ ] 支持工具调用历史管理

### 5.2 性能验收
- [ ] 工具发现延迟 < 100ms
- [ ] 流式输出延迟 < 50ms
- [ ] 上下文压缩不影响响应质量

### 5.3 质量验收
- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试通过
- [ ] 无破坏性变更