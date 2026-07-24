package service

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupCustomerSessionServiceTestDB 设置测试数据库
func setupCustomerSessionServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.AgentStatus{},
		&model.AISuggestion{},
		&model.QuickReply{},
		&model.SessionTag{},
	)
	db.SetTestDB(database)
	return database
}

// setupCustomerSessionService 设置测试服务
func setupCustomerSessionService(t *testing.T) *CustomerSessionService {
	setupCustomerSessionServiceTestDB(t)
	return NewCustomerSessionService()
}

// TestNewCustomerSessionService 测试创建服务实例
func TestNewCustomerSessionService(t *testing.T) {
	service := NewCustomerSessionService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.sessionRepo == nil {
		t.Error("Expected sessionRepo to be initialized")
	}
	if service.messageRepo == nil {
		t.Error("Expected messageRepo to be initialized")
	}
	if service.agentRepo == nil {
		t.Error("Expected agentRepo to be initialized")
	}
	if service.suggestionRepo == nil {
		t.Error("Expected suggestionRepo to be initialized")
	}
}

// TestCustomerSessionService_CreateSession_Success 测试创建会话成功
func TestCustomerSessionService_CreateSession_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	req := &CreateSessionRequest{
		Platform:   model.PlatformDouyin,
		AccountID:  "account_123",
		UserID:     "user_123",
		UserName:   "Test User",
		UserAvatar: "https://example.com/avatar.jpg",
		UserPhone:  "1234567890",
		UserEmail:  "test@example.com",
	}

	session, err := service.CreateSession(req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.SessionID == "" {
		t.Error("Expected SessionID to be set")
	}

	if session.Platform != model.PlatformDouyin {
		t.Errorf("Expected platform douyin, got %v", session.Platform)
	}
	if session.Status != model.SessionStatusPending {
		t.Errorf("Expected status pending, got %v", session.Status)
	}
}

// TestCustomerSessionService_GetSessions_Success 测试获取会话列表
func TestCustomerSessionService_GetSessions_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	for i := 0; i < 5; i++ {
		req := &CreateSessionRequest{
			Platform:  model.PlatformDouyin,
			AccountID: "account_" + string(rune('0'+i)),
			UserID:    "user_" + string(rune('0'+i)),
		}
		service.CreateSession(req)
	}

	// 获取列表
	sessions, total, err := service.GetSessions(model.SessionStatusPending, 1, 10)
	if err != nil {
		t.Fatalf("GetSessions failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(sessions) != 5 {
		t.Errorf("Expected 5 sessions, got %d", len(sessions))
	}
}

// TestCustomerSessionService_GetSessionByID_Success 测试获取会话详情
func TestCustomerSessionService_GetSessionByID_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	created, _ := service.CreateSession(req)

	// 获取会话详情
	session, err := service.GetSessionByID(created.ID)
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}

	if session.SessionID != created.SessionID {
		t.Errorf("Expected session_id %s, got %s", created.SessionID, session.SessionID)
	}
}

// TestCustomerSessionService_GetSessionByID_SingleTenant 单租户访问验证
// 单租户私有部署：所有数据归当前部署实例所有，GetSessionByID 不再做跨租户权限校验。
func TestCustomerSessionService_GetSessionByID_SingleTenant(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	created, err := service.CreateSession(req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// 正常访问应返回会话
	got, err := service.GetSessionByID(created.ID)
	if err != nil {
		t.Fatalf("GetSessionByID failed: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("Expected session ID %d, got %v", created.ID, got)
	}
}

// TestCustomerSessionService_AssignSession_Success 测试分配会话成功
func TestCustomerSessionService_AssignSession_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 0,
	}
	service.agentRepo.Create(agent)

	// 创建会话
	sessionReq := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(sessionReq)

	// 分配会话
	assignReq := &AssignSessionRequest{
		SessionID: session.ID,
		AgentID:   123,
	}
	err := service.AssignSession(assignReq)
	if err != nil {
		t.Fatalf("AssignSession failed: %v", err)
	}

	// 验证会话已分配
	updated, _ := service.GetSessionByID(session.ID)
	if updated.AgentID != 123 {
		t.Errorf("Expected agent_id 123, got %d", updated.AgentID)
	}
}

// TestCustomerSessionService_AssignSession_AgentNotFound 测试客服不存在
func TestCustomerSessionService_AssignSession_AgentNotFound(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 分配会话给不存在的客服
	assignReq := &AssignSessionRequest{
		SessionID: session.ID,
		AgentID:   999999,
	}
	err := service.AssignSession(assignReq)
	if err == nil {
		t.Error("Expected error for non-existent agent")
	}
}

// TestCustomerSessionService_AssignSession_AgentOffline 测试客服离线
func TestCustomerSessionService_AssignSession_AgentOffline(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建离线客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "offline",
		MaxSessions:    5,
		ActiveSessions: 0,
	}
	service.agentRepo.Create(agent)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 分配会话
	assignReq := &AssignSessionRequest{
		SessionID: session.ID,
		AgentID:   123,
	}
	err := service.AssignSession(assignReq)
	if err == nil {
		t.Error("Expected error for offline agent")
	}
}

// TestCustomerSessionService_AutoAssign_Success 测试自动分配会话
func TestCustomerSessionService_AutoAssign_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建在线客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 0,
	}
	service.agentRepo.Create(agent)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 自动分配
	err := service.AutoAssign(session.ID)
	if err != nil {
		t.Fatalf("AutoAssign failed: %v", err)
	}

	// 验证会话已分配
	updated, _ := service.GetSessionByID(session.ID)
	if updated.AgentID != 123 {
		t.Errorf("Expected agent_id 123, got %d", updated.AgentID)
	}
}

// TestCustomerSessionService_AutoAssign_NoAgents 测试无可用客服
func TestCustomerSessionService_AutoAssign_NoAgents(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 自动分配
	err := service.AutoAssign(session.ID)
	if err == nil {
		t.Error("Expected error for no available agents")
	}
}

// TestCustomerSessionService_SendMessage_Success 测试发送消息成功
func TestCustomerSessionService_SendMessage_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 发送消息
	msgReq := &SendMessageRequest{
		SessionID:  session.SessionID,
		Content:    "Hello, World!",
		SenderType: "user",
		SenderID:   "user_123",
	}
	message, err := service.SendMessage(msgReq)
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if message.Content != "Hello, World!" {
		t.Errorf("Expected content 'Hello, World!', got '%s'", message.Content)
	}
	if message.SenderType != "user" {
		t.Errorf("Expected sender_type 'user', got '%s'", message.SenderType)
	}
}

// TestCustomerSessionService_SendMessage_SessionNotFound 测试会话不存在
func TestCustomerSessionService_SendMessage_SessionNotFound(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 发送消息到不存在的会话
	req := &SendMessageRequest{
		SessionID:  "non_existent_session",
		Content:    "Hello",
		SenderType: "user",
	}
	_, err := service.SendMessage(req)
	if err == nil {
		t.Error("Expected error for non-existent session")
	}
}

// TestCustomerSessionService_GetMessages_Success 测试获取消息列表
func TestCustomerSessionService_GetMessages_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	sessionReq := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(sessionReq)

	// 发送消息
	for i := 0; i < 5; i++ {
		msgReq := &SendMessageRequest{
			SessionID:  session.SessionID,
			Content:    "Message " + string(rune('0'+i)),
			SenderType: "user",
		}
		service.SendMessage(msgReq)
	}

	// 获取消息
	messages, total, err := service.GetMessages(session.SessionID, 1, 10)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}
}

// TestCustomerSessionService_UpdateSessionStatus_Success 测试更新会话状态
func TestCustomerSessionService_UpdateSessionStatus_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 分配客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 1,
	}
	service.agentRepo.Create(agent)
	service.sessionRepo.AssignAgent(session.ID, 123, "客服 A")

	// 更新状态为已解决
	err := service.UpdateSessionStatus(session.ID, model.SessionStatusResolved)
	if err != nil {
		t.Fatalf("UpdateSessionStatus failed: %v", err)
	}

	// 验证状态已更新
	updated, _ := service.GetSessionByID(session.ID)
	if updated.Status != model.SessionStatusResolved {
		t.Errorf("Expected status resolved, got %v", updated.Status)
	}
}

// TestCustomerSessionService_RateSession_Success 测试评价会话
func TestCustomerSessionService_RateSession_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 评价会话
	err := service.RateSession(session.ID, 5, "Excellent service!")
	if err != nil {
		t.Fatalf("RateSession failed: %v", err)
	}

	// 验证评价
	updated, _ := service.GetSessionByID(session.ID)
	if updated.Rating != 5 {
		t.Errorf("Expected rating 5, got %d", updated.Rating)
	}
	if updated.RatingComment != "Excellent service!" {
		t.Errorf("Expected comment 'Excellent service!', got '%s'", updated.RatingComment)
	}
}

// TestCustomerSessionService_TransferSession_Success 测试转接会话
func TestCustomerSessionService_TransferSession_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建两个客服
	agent1 := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 1,
	}
	agent2 := &model.AgentStatus{
		AgentID:        456,
		AgentName:      "客服 B",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 0,
	}
	service.agentRepo.Create(agent1)
	service.agentRepo.Create(agent2)

	// 创建会话并分配给客服 A
	sessionReq := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(sessionReq)
	service.sessionRepo.AssignAgent(session.ID, 123, "客服 A")

	// 转接给客服 B
	err := service.TransferSession(session.ID, 456)
	if err != nil {
		t.Fatalf("TransferSession failed: %v", err)
	}

	// 验证会话已转接
	updated, _ := service.GetSessionByID(session.ID)
	if updated.AgentID != 456 {
		t.Errorf("Expected agent_id 456, got %d", updated.AgentID)
	}
}

// TestCustomerSessionService_TagSession_Success 测试标记会话
func TestCustomerSessionService_TagSession_Success(t *testing.T) {
	service := setupCustomerSessionService(t)

	// 创建会话
	req := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(req)

	// 标记会话
	tags := []string{"urgent", "vip", "complaint"}
	err := service.TagSession(session.ID, tags)
	if err != nil {
		t.Fatalf("TagSession failed: %v", err)
	}

	// 验证标签
	updated, _ := service.GetSessionByID(session.ID)
	if updated.Tags == "" {
		t.Error("Expected tags to be set")
	}
}

// TestAgentStatusService_CreateAgent_Success 测试创建客服成功
func TestAgentStatusService_CreateAgent_Success(t *testing.T) {
	setupCustomerSessionService(t)
	agentService := NewAgentStatusService()

	req := &CreateAgentRequest{
		AgentID:     123,
		AgentName:   "客服 A",
		MaxSessions: 10,
	}

	agent, err := agentService.CreateAgent(req)
	if err != nil {
		t.Fatalf("CreateAgent failed: %v", err)
	}

	if agent.AgentID != 123 {
		t.Errorf("Expected agent_id 123, got %d", agent.AgentID)
	}
	if agent.AgentName != "客服 A" {
		t.Errorf("Expected agent_name '客服 A', got '%s'", agent.AgentName)
	}
	if agent.Status != "offline" {
		t.Errorf("Expected status 'offline', got '%s'", agent.Status)
	}
}

// TestAgentStatusService_GetAgentStatus_Success 测试获取客服状态
func TestAgentStatusService_GetAgentStatus_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	agentService := NewAgentStatusService()

	// 创建客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 2,
	}
	service.agentRepo.Create(agent)

	// 获取客服状态
	retrieved, err := agentService.GetAgentStatus(123)
	if err != nil {
		t.Fatalf("GetAgentStatus failed: %v", err)
	}

	if retrieved.AgentID != 123 {
		t.Errorf("Expected agent_id 123, got %d", retrieved.AgentID)
	}
	if retrieved.Status != "online" {
		t.Errorf("Expected status 'online', got '%s'", retrieved.Status)
	}
}

// TestAgentStatusService_GetOnlineAgents_Success 测试获取在线客服列表
func TestAgentStatusService_GetOnlineAgents_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	agentService := NewAgentStatusService()

	// 创建在线客服
	agent1 := &model.AgentStatus{
		AgentID:     123,
		AgentName:   "客服 A",
		Status:      "online",
		MaxSessions: 5,
	}
	agent2 := &model.AgentStatus{
		AgentID:     456,
		AgentName:   "客服 B",
		Status:      "offline",
		MaxSessions: 5,
	}
	service.agentRepo.Create(agent1)
	service.agentRepo.Create(agent2)

	// 获取在线客服
	agents, err := agentService.GetOnlineAgents()
	if err != nil {
		t.Fatalf("GetOnlineAgents failed: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("Expected 1 online agent, got %d", len(agents))
	}
	if agents[0].AgentID != 123 {
		t.Errorf("Expected agent_id 123, got %d", agents[0].AgentID)
	}
}

// TestAgentStatusService_GoOnline_Success 测试客服上线
func TestAgentStatusService_GoOnline_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	agentService := NewAgentStatusService()

	// 创建客服
	agent := &model.AgentStatus{
		AgentID:     123,
		AgentName:   "客服 A",
		Status:      "offline",
		MaxSessions: 5,
	}
	service.agentRepo.Create(agent)

	// 上线
	err := agentService.GoOnline(123)
	if err != nil {
		t.Fatalf("GoOnline failed: %v", err)
	}

	// 验证状态
	updated, _ := agentService.GetAgentStatus(123)
	if updated.Status != "online" {
		t.Errorf("Expected status 'online', got '%s'", updated.Status)
	}
}

// TestAgentStatusService_GoOffline_Success 测试客服下线
func TestAgentStatusService_GoOffline_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	agentService := NewAgentStatusService()

	// 创建客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 0,
	}
	service.agentRepo.Create(agent)

	// 下线
	err := agentService.GoOffline(123)
	if err != nil {
		t.Fatalf("GoOffline failed: %v", err)
	}

	// 验证状态
	updated, _ := agentService.GetAgentStatus(123)
	if updated.Status != "offline" {
		t.Errorf("Expected status 'offline', got '%s'", updated.Status)
	}
}

// TestAgentStatusService_GoOffline_WithActiveSessions 测试有活跃会话时下线
func TestAgentStatusService_GoOffline_WithActiveSessions(t *testing.T) {
	service := setupCustomerSessionService(t)
	agentService := NewAgentStatusService()

	// 创建客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 2,
	}
	service.agentRepo.Create(agent)

	// 下线
	err := agentService.GoOffline(123)
	if err == nil {
		t.Error("Expected error for active sessions")
	}
}

// TestAISuggestionService_CreateSuggestion_Success 测试创建 AI 建议
func TestAISuggestionService_CreateSuggestion_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	suggestionService := NewAISuggestionService()

	// 创建会话
	sessionReq := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(sessionReq)

	// 分配客服
	agent := &model.AgentStatus{
		AgentID:        123,
		AgentName:      "客服 A",
		Status:         "online",
		MaxSessions:    5,
		ActiveSessions: 1,
	}
	service.agentRepo.Create(agent)
	service.sessionRepo.AssignAgent(session.ID, 123, "客服 A")

	// 创建消息
	message := &model.SessionMessage{
		SessionID:  session.SessionID,
		Content:    "用户消息",
		SenderType: "user",
	}
	service.messageRepo.Create(message)

	// 创建 AI 建议
	suggestion, err := suggestionService.CreateSuggestion(session.SessionID,
		message.ID,
		"这是建议回复",
		0.95,
		"ai-model",
	)
	if err != nil {
		t.Fatalf("CreateSuggestion failed: %v", err)
	}

	if suggestion.Suggestion != "这是建议回复" {
		t.Errorf("Expected suggestion '这是建议回复', got '%s'", suggestion.Suggestion)
	}
	if suggestion.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", suggestion.Confidence)
	}
}

// TestAISuggestionService_GetSuggestions_Success 测试获取 AI 建议列表
func TestAISuggestionService_GetSuggestions_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	suggestionService := NewAISuggestionService()

	// 创建会话
	sessionReq := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(sessionReq)

	// 创建 AI 建议
	for i := 0; i < 3; i++ {
		suggestionService.CreateSuggestion(session.SessionID,
			1,
			"Suggestion "+string(rune('0'+i)),
			0.9,
			"ai-model",
		)
	}

	// 获取建议
	suggestions, err := suggestionService.GetSuggestions(session.SessionID)
	if err != nil {
		t.Fatalf("GetSuggestions failed: %v", err)
	}

	if len(suggestions) != 3 {
		t.Errorf("Expected 3 suggestions, got %d", len(suggestions))
	}
}

// TestAISuggestionService_UseSuggestion_Success 测试使用 AI 建议
func TestAISuggestionService_UseSuggestion_Success(t *testing.T) {
	service := setupCustomerSessionService(t)
	suggestionService := NewAISuggestionService()

	// 创建会话
	sessionReq := &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "account_123",
		UserID:    "user_123",
	}
	session, _ := service.CreateSession(sessionReq)

	// 创建 AI 建议
	suggestion, _ := suggestionService.CreateSuggestion(session.SessionID,
		1,
		"这是建议",
		0.9,
		"ai-model",
	)

	// 使用建议
	err := suggestionService.UseSuggestion(suggestion.ID, 123)
	if err != nil {
		t.Fatalf("UseSuggestion failed: %v", err)
	}

	// 验证已使用 - use GetBySessionID since GetByID doesn't exist
	suggestions, _ := suggestionService.suggestionRepo.GetBySessionID(session.SessionID)
	if len(suggestions) == 0 || !suggestions[0].IsUsed {
		t.Error("Expected suggestion to be marked as used")
	}
}

// TestQuickReplyService_CreateReply_Success 测试创建快捷回复
func TestQuickReplyService_CreateReply_Success(t *testing.T) {
	setupCustomerSessionService(t)
	replyService := NewQuickReplyService()

	req := &CreateReplyRequest{
		Category:  "问候语",
		Title:     "你好",
		Content:   "您好，有什么可以帮助您的吗？",
		SortOrder: 1,
		IsPublic:  true,
	}

	reply, err := replyService.CreateReply(123, req)
	if err != nil {
		t.Fatalf("CreateReply failed: %v", err)
	}

	if reply.Category != "问候语" {
		t.Errorf("Expected category '问候语', got '%s'", reply.Category)
	}
	if reply.Title != "你好" {
		t.Errorf("Expected title '你好', got '%s'", reply.Title)
	}
	if reply.CreatedBy != 123 {
		t.Errorf("Expected created_by 123, got %d", reply.CreatedBy)
	}
}

// TestQuickReplyService_UpdateReply_Success 测试更新快捷回复
func TestQuickReplyService_UpdateReply_Success(t *testing.T) {
	setupCustomerSessionService(t)
	replyService := NewQuickReplyService()

	// 创建快捷回复
	createReq := &CreateReplyRequest{
		Category: "问候语",
		Title:    "你好",
		Content:  "您好",
		IsPublic: true,
	}
	created, _ := replyService.CreateReply(123, createReq)

	// 更新快捷回复
	updateReq := &CreateReplyRequest{
		Category: "常用语",
		Title:    "您好！",
		Content:  "您好，有什么可以帮助您的吗？",
		IsPublic: true,
	}
	updated, err := replyService.UpdateReply(created.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateReply failed: %v", err)
	}

	if updated.Category != "常用语" {
		t.Errorf("Expected category '常用语', got '%s'", updated.Category)
	}
	if updated.Title != "您好！" {
		t.Errorf("Expected title '您好！', got '%s'", updated.Title)
	}
}

// TestQuickReplyService_DeleteReply_Success 测试删除快捷回复
func TestQuickReplyService_DeleteReply_Success(t *testing.T) {
	setupCustomerSessionService(t)
	replyService := NewQuickReplyService()

	// 创建快捷回复
	req := &CreateReplyRequest{
		Category: "问候语",
		Title:    "你好",
		Content:  "您好",
		IsPublic: true,
	}
	created, _ := replyService.CreateReply(123, req)

	// 删除快捷回复
	err := replyService.DeleteReply(created.ID)
	if err != nil {
		t.Fatalf("DeleteReply failed: %v", err)
	}

	// 验证已删除
	_, err = replyService.replyRepo.GetByID(created.ID)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

// TestQuickReplyService_GetReplies_Success 测试获取快捷回复列表
func TestQuickReplyService_GetReplies_Success(t *testing.T) {
	setupCustomerSessionService(t)
	replyService := NewQuickReplyService()

	// 创建快捷回复
	for i := 0; i < 3; i++ {
		req := &CreateReplyRequest{
			Category: "问候语",
			Title:    "问候 " + string(rune('0'+i)),
			Content:  "内容",
			IsPublic: true,
		}
		replyService.CreateReply(123, req)
	}

	// 获取列表
	replies, err := replyService.GetReplies("问候语")
	if err != nil {
		t.Fatalf("GetReplies failed: %v", err)
	}

	if len(replies) != 3 {
		t.Errorf("Expected 3 replies, got %d", len(replies))
	}
}

// TestQuickReplyService_GetCategories_Success 测试获取分类列表
func TestQuickReplyService_GetCategories_Success(t *testing.T) {
	setupCustomerSessionService(t)
	replyService := NewQuickReplyService()

	// 创建快捷回复
	req1 := &CreateReplyRequest{
		Category: "问候语",
		Title:    "你好",
		Content:  "您好",
		IsPublic: true,
	}
	req2 := &CreateReplyRequest{
		Category: "结束语",
		Title:    "再见",
		Content:  "再见",
		IsPublic: true,
	}
	replyService.CreateReply(123, req1)
	replyService.CreateReply(123, req2)

	// 获取分类
	categories, err := replyService.GetCategories()
	if err != nil {
		t.Fatalf("GetCategories failed: %v", err)
	}

	if len(categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(categories))
	}
}

// TestSessionTagService_CreateTag_Success 测试创建标签
func TestSessionTagService_CreateTag_Success(t *testing.T) {
	setupCustomerSessionService(t)
	tagService := NewSessionTagService()

	req := &CreateTagRequest{
		Name:      "VIP",
		Color:     "#FFD700",
		SortOrder: 1,
	}

	tag, err := tagService.CreateTag(req)
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if tag.Name != "VIP" {
		t.Errorf("Expected name 'VIP', got '%s'", tag.Name)
	}
	if tag.Color != "#FFD700" {
		t.Errorf("Expected color '#FFD700', got '%s'", tag.Color)
	}
}

// TestSessionTagService_CreateTag_DefaultColor 测试默认颜色
func TestSessionTagService_CreateTag_DefaultColor(t *testing.T) {
	setupCustomerSessionService(t)
	tagService := NewSessionTagService()

	req := &CreateTagRequest{
		Name:      "普通",
		SortOrder: 1,
	}

	tag, err := tagService.CreateTag(req)
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	if tag.Color != "#1890ff" {
		t.Errorf("Expected default color '#1890ff', got '%s'", tag.Color)
	}
}

// TestSessionTagService_UpdateTag_Success 测试更新标签
func TestSessionTagService_UpdateTag_Success(t *testing.T) {
	setupCustomerSessionService(t)
	tagService := NewSessionTagService()

	// 创建标签
	createReq := &CreateTagRequest{
		Name:      "旧标签",
		Color:     "#1890ff",
		SortOrder: 1,
	}
	created, _ := tagService.CreateTag(createReq)

	// 更新标签
	updateReq := &CreateTagRequest{
		Name:      "新标签",
		Color:     "#FF0000",
		SortOrder: 2,
	}
	updated, err := tagService.UpdateTag(created.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}

	if updated.Name != "新标签" {
		t.Errorf("Expected name '新标签', got '%s'", updated.Name)
	}
	if updated.Color != "#FF0000" {
		t.Errorf("Expected color '#FF0000', got '%s'", updated.Color)
	}
}

// TestSessionTagService_DeleteTag_Success 测试删除标签
func TestSessionTagService_DeleteTag_Success(t *testing.T) {
	setupCustomerSessionService(t)
	tagService := NewSessionTagService()

	// 创建标签
	req := &CreateTagRequest{
		Name:      "测试标签",
		Color:     "#1890ff",
		SortOrder: 1,
	}
	created, _ := tagService.CreateTag(req)

	// 删除标签
	err := tagService.DeleteTag(created.ID)
	if err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}

	// 验证已删除
	_, err = tagService.tagRepo.GetByID(created.ID)
	if err == nil {
		t.Error("Expected error after deletion")
	}
}

// TestSessionTagService_GetTags_Success 测试获取标签列表
func TestSessionTagService_GetTags_Success(t *testing.T) {
	setupCustomerSessionService(t)
	tagService := NewSessionTagService()

	// 创建标签（Code 必须唯一，否则 uniqueIndex 约束会让后续 Create 静默失败）
	codes := []string{"tag_a", "tag_b", "tag_c"}
	for i := 0; i < 3; i++ {
		req := &CreateTagRequest{
			Name:      "标签 " + string(rune('0'+i)),
			Code:      codes[i],
			Color:     "#1890ff",
			SortOrder: i,
		}
		if _, err := tagService.CreateTag(req); err != nil {
			t.Fatalf("CreateTag[%d] failed: %v", i, err)
		}
	}

	// 获取列表
	tags, err := tagService.GetTags()
	if err != nil {
		t.Fatalf("GetTags failed: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(tags))
	}
}

// TestGenerateSessionID 测试生成会话 ID
func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	if id1 == "" {
		t.Error("Expected non-empty session ID")
	}
	if id2 == "" {
		t.Error("Expected non-empty session ID")
	}
	if id1 == id2 {
		t.Error("Expected unique session IDs")
	}
}
