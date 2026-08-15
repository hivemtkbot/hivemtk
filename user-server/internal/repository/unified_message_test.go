package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupUnifiedMessageTestDB 设置统一消息测试数据库
func setupUnifiedMessageTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.UnifiedMessage{},
		&model.UnifiedReply{},
		&model.PlatformAccount{},
	)
	db.SetTestDB(database)
	return database
}

// setupUnifiedMessageRepositories 创建测试用的仓库实例
func setupUnifiedMessageRepositories(t *testing.T) (UnifiedMessageRepository, UnifiedReplyRepository, PlatformAccountRepository) {
	setupUnifiedMessageTestDB(t)
	return NewUnifiedMessageRepository(), NewUnifiedReplyRepository(), NewPlatformAccountRepository()
}

// TestUnifiedMessageRepository_Create 测试创建消息
func TestUnifiedMessageRepository_Create(t *testing.T) {
	msgRepo, _, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		msg     *model.UnifiedMessage
		wantErr bool
	}{
		{
			name: "create message success",
			msg: &model.UnifiedMessage{
				MessageID:   "msg-001",
				Platform:    model.PlatformDouyin,
				AccountID:   "account-123",
				AccountName: "Test Account",
				ChatID:      "chat-456",
				ChatType:    model.ChatTypePrivate,
				SenderID:    "sender-789",
				SenderName:  "Test Sender",
				Content:     "Hello, World!",
				ContentType: model.MessageTypeText,
			},
			wantErr: false,
		},
		{
			name: "create message with media",
			msg: &model.UnifiedMessage{
				MessageID:   "msg-002",
				Platform:    model.PlatformKuaishou,
				ChatID:      "chat-789",
				Content:     "Image message",
				ContentType: model.MessageTypeImage,
				MediaURL:    "https://example.com/image.jpg",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := msgRepo.Create(ctx, tt.msg)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.msg.ID == 0 {
				t.Error("Expected message ID to be set after creation")
			}
		})
	}
}

// TestUnifiedMessageRepository_GetByID 测试根据 ID 获取消息
func TestUnifiedMessageRepository_GetByID(t *testing.T) {
	msgRepo, _, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	msg := &model.UnifiedMessage{
		MessageID: "msg-getbyid",
		Platform:  model.PlatformDouyin,
		ChatID:    "chat-123",
		Content:   "GetByID Test",
	}
	msgRepo.Create(ctx, msg)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing message",
			id:      msg.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing message",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := msgRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.Content != "GetByID Test" {
					t.Errorf("Expected content 'GetByID Test', got '%s'", result.Content)
				}
			}
		})
	}
}

// TestUnifiedMessageRepository_GetByMessageID 测试根据 MessageID 获取消息
func TestUnifiedMessageRepository_GetByMessageID(t *testing.T) {
	msgRepo, _, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	msg := &model.UnifiedMessage{
		MessageID: "unique-msg-id",
		Platform:  model.PlatformDouyin,
		Content:   "Unique Message",
	}
	msgRepo.Create(ctx, msg)

	result, err := msgRepo.GetByMessageID(context.Background(), "unique-msg-id")
	if err != nil {
		t.Errorf("GetByMessageID() error = %v", err)
	}

	if result.ID != msg.ID {
		t.Errorf("Expected ID %d, got %d", msg.ID, result.ID)
	}

	_, err = msgRepo.GetByMessageID(context.Background(), "non-existing-id")
	if err == nil {
		t.Error("Expected error for non-existing message ID")
	}
}

// TestUnifiedMessageRepository_GetByChat 测试获取会话下的消息列表
func TestUnifiedMessageRepository_GetByChat(t *testing.T) {
	msgRepo, _, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	for i := 1; i <= 4; i++ {
		msgRepo.Create(ctx, &model.UnifiedMessage{
			MessageID: string(rune('a' + i - 1)),
			Platform:  model.PlatformDouyin,
			ChatID:    "chat-123",
			Content:   string(rune('A' + i - 1)),
		})
	}

	msgRepo.Create(ctx, &model.UnifiedMessage{
		MessageID: "msg-other-chat",
		Platform:  model.PlatformDouyin,
		ChatID:    "chat-456",
		Content:   "Other chat",
	})

	tests := []struct {
		name       string
		merchantID string
		chatID     string
		page       int
		pageSize   int
		wantCount  int
		wantErr    bool
	}{
		{
			name: "get messages from chat-123",

			chatID:    "chat-123",
			page:      1,
			pageSize:  3,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "get second page",

			chatID:    "chat-123",
			page:      2,
			pageSize:  3,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "get messages from different chat",

			chatID:    "chat-456",
			page:      1,
			pageSize:  10,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := msgRepo.GetByChat(context.Background(), tt.chatID, tt.page, tt.pageSize)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByChat() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d results, got %d", tt.wantCount, len(results))
			}

			if tt.page == 1 && tt.chatID == "chat-123" && int(total) != 4 {
				t.Errorf("Expected total 4 for chat-123, got %d", total)
			}
		})
	}
}

// TestUnifiedMessageRepository_UpdateStatus 测试更新消息状态
func TestUnifiedMessageRepository_UpdateStatus(t *testing.T) {
	msgRepo, _, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	msg := &model.UnifiedMessage{
		MessageID: "msg-status",
		Platform:  model.PlatformDouyin,
		Status:    model.MessageStatusPending,
	}
	msgRepo.Create(ctx, msg)

	tests := []struct {
		name      string
		newStatus model.MessageStatus
		wantErr   bool
	}{
		{
			name:      "update to processing",
			newStatus: model.MessageStatusProcessing,
			wantErr:   false,
		},
		{
			name:      "update to replied",
			newStatus: model.MessageStatusReplied,
			wantErr:   false,
		},
		{
			name:      "update to failed",
			newStatus: model.MessageStatusFailed,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := msgRepo.UpdateStatus(context.Background(), msg.ID, tt.newStatus)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateStatus() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := msgRepo.GetByID(ctx, msg.ID)
				if updated.Status != tt.newStatus {
					t.Errorf("Expected status '%v', got '%v'", tt.newStatus, updated.Status)
				}
			}
		})
	}
}

// TestUnifiedMessageRepository_Delete 测试删除消息
func TestUnifiedMessageRepository_Delete(t *testing.T) {
	msgRepo, _, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	msg := &model.UnifiedMessage{
		MessageID: "msg-delete",
		Platform:  model.PlatformDouyin,
		Content:   "To be deleted",
	}
	msgRepo.Create(ctx, msg)

	err := msgRepo.Delete(ctx, msg.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = msgRepo.GetByID(ctx, msg.ID)
	if err == nil {
		t.Error("Expected message to be deleted")
	}
}

// TestUnifiedReplyRepository_Create 测试创建回复
func TestUnifiedReplyRepository_Create(t *testing.T) {
	_, replyRepo, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		reply   *model.UnifiedReply
		wantErr bool
	}{
		{
			name: "create reply success",
			reply: &model.UnifiedReply{
				ReplyID:     "reply-001",
				MessageID:   "msg-001",
				Platform:    model.PlatformDouyin,
				AccountID:   "account-123",
				ChatID:      "chat-456",
				Content:     "Hello, this is a reply!",
				ContentType: model.MessageTypeText,
				ReplyType:   "rule",
				Confidence:  0.95,
				Status:      model.ReplyStatusPending,
			},
			wantErr: false,
		},
		{
			name: "create reply with error",
			reply: &model.UnifiedReply{
				ReplyID:      "reply-002",
				MessageID:    "msg-002",
				Content:      "Failed reply",
				ReplyType:    "llm",
				Status:       model.ReplyStatusFailed,
				ErrorMessage: "API error",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := replyRepo.Create(ctx, tt.reply)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.reply.ID == 0 {
				t.Error("Expected reply ID to be set after creation")
			}
		})
	}
}

// TestUnifiedReplyRepository_GetByID 测试根据 ID 获取回复
func TestUnifiedReplyRepository_GetByID(t *testing.T) {
	_, replyRepo, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	reply := &model.UnifiedReply{
		ReplyID:   "reply-getbyid",
		MessageID: "msg-123",
		Content:   "GetByID Reply",
		Status:    model.ReplyStatusSent,
	}
	replyRepo.Create(ctx, reply)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing reply",
			id:      reply.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing reply",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := replyRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.Content != "GetByID Reply" {
					t.Errorf("Expected content 'GetByID Reply', got '%s'", result.Content)
				}
			}
		})
	}
}

// TestUnifiedReplyRepository_GetByMessageID 测试根据 MessageID 获取回复列表
func TestUnifiedReplyRepository_GetByMessageID(t *testing.T) {
	_, replyRepo, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	replyRepo.Create(ctx, &model.UnifiedReply{
		ReplyID:   "reply-1",
		MessageID: "msg-target",
		Content:   "First reply",
	})
	replyRepo.Create(ctx, &model.UnifiedReply{
		ReplyID:   "reply-2",
		MessageID: "msg-target",
		Content:   "Second reply",
	})
	replyRepo.Create(ctx, &model.UnifiedReply{
		ReplyID:   "reply-3",
		MessageID: "msg-other",
		Content:   "Other message reply",
	})

	results, err := replyRepo.GetByMessageID(context.Background(), "msg-target")
	if err != nil {
		t.Errorf("GetByMessageID() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 replies, got %d", len(results))
	}
}

// TestUnifiedReplyRepository_UpdateStatus 测试更新回复状态
func TestUnifiedReplyRepository_UpdateStatus(t *testing.T) {
	_, replyRepo, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	reply := &model.UnifiedReply{
		ReplyID:   "reply-status",
		MessageID: "msg-123",
		Status:    model.ReplyStatusPending,
	}
	replyRepo.Create(ctx, reply)

	tests := []struct {
		name      string
		newStatus model.ReplyStatus
		wantErr   bool
	}{
		{
			name:      "update to sent",
			newStatus: model.ReplyStatusSent,
			wantErr:   false,
		},
		{
			name:      "update to failed",
			newStatus: model.ReplyStatusFailed,
			wantErr:   false,
		},
		{
			name:      "update to discarded",
			newStatus: model.ReplyStatusDiscarded,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := replyRepo.UpdateStatus(context.Background(), reply.ID, tt.newStatus)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateStatus() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := replyRepo.GetByID(ctx, reply.ID)
				if updated.Status != tt.newStatus {
					t.Errorf("Expected status '%v', got '%v'", tt.newStatus, updated.Status)
				}
			}
		})
	}
}

// TestUnifiedReplyRepository_Delete 测试删除回复
func TestUnifiedReplyRepository_Delete(t *testing.T) {
	_, replyRepo, _ := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	reply := &model.UnifiedReply{
		ReplyID:   "reply-delete",
		MessageID: "msg-123",
		Content:   "To be deleted",
	}
	replyRepo.Create(ctx, reply)

	err := replyRepo.Delete(ctx, reply.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = replyRepo.GetByID(ctx, reply.ID)
	if err == nil {
		t.Error("Expected reply to be deleted")
	}
}

// TestPlatformAccountRepository_Create 测试创建平台账号
func TestPlatformAccountRepository_Create(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		account *model.PlatformAccount
		wantErr bool
	}{
		{
			name: "create account success",
			account: &model.PlatformAccount{
				Platform:      model.PlatformDouyin,
				AccountID:     "dy-account-123",
				AccountName:   "Test Account",
				AccountAvatar: "https://example.com/avatar.jpg",
				Config:        `{"key": "value"}`,
				Status:        1,
			},
			wantErr: false,
		},
		{
			name: "create account with token",
			account: &model.PlatformAccount{
				Platform:  model.PlatformKuaishou,
				AccountID: "ks-account-456",
				Token:     "encrypted-token-here",
				Status:    1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := accountRepo.Create(ctx, tt.account)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.account.ID == 0 {
				t.Error("Expected account ID to be set after creation")
			}
		})
	}
}

// TestPlatformAccountRepository_GetByID 测试根据 ID 获取账号
func TestPlatformAccountRepository_GetByID(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	account := &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "dy-123",
		AccountName: "GetByID Account",
	}
	accountRepo.Create(ctx, account)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing account",
			id:      account.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing account",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := accountRepo.GetByID(ctx, tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if result.ID != tt.id {
					t.Errorf("Expected ID %d, got %d", tt.id, result.ID)
				}
				if result.AccountName != "GetByID Account" {
					t.Errorf("Expected account name 'GetByID Account', got '%s'", result.AccountName)
				}
			}
		})
	}
}

// TestPlatformAccountRepository_GetAll 测试获取所有账号列表
func TestPlatformAccountRepository_GetAll(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	accountRepo.Create(ctx, &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "dy-1",
		AccountName: "Douyin Account 1",
	})
	accountRepo.Create(ctx, &model.PlatformAccount{
		Platform:    model.PlatformKuaishou,
		AccountID:   "ks-1",
		AccountName: "Kuaishou Account 1",
	})

	results, err := accountRepo.GetAll(ctx)
	if err != nil {
		t.Errorf("GetByMerchant() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(results))
	}
}

// TestPlatformAccountRepository_GetByPlatform 测试根据平台获取账号
func TestPlatformAccountRepository_GetByPlatform(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	accountRepo.Create(ctx, &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "dy-1",
		AccountName: "Douyin 1",
	})
	accountRepo.Create(ctx, &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "dy-2",
		AccountName: "Douyin 2",
	})
	accountRepo.Create(ctx, &model.PlatformAccount{
		Platform:    model.PlatformKuaishou,
		AccountID:   "ks-1",
		AccountName: "Kuaishou 1",
	})

	tests := []struct {
		name      string
		platform  model.Platform
		wantCount int
	}{
		{
			name:      "get douyin accounts",
			platform:  model.PlatformDouyin,
			wantCount: 2,
		},
		{
			name:      "get kuaishou accounts",
			platform:  model.PlatformKuaishou,
			wantCount: 1,
		},
		{
			name:      "get xiaohongshu accounts (none)",
			platform:  model.PlatformXiaohongshu,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := accountRepo.GetByPlatform(context.Background(), tt.platform)

			if err != nil {
				t.Errorf("GetByPlatform() error = %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("Expected %d accounts, got %d", tt.wantCount, len(results))
			}
		})
	}
}

// TestPlatformAccountRepository_Update 测试更新账号
func TestPlatformAccountRepository_Update(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	account := &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "dy-123",
		AccountName: "Original Name",
		Status:      1,
	}
	accountRepo.Create(ctx, account)

	account.AccountName = "Updated Name"
	account.Status = 0

	err := accountRepo.Update(ctx, account)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := accountRepo.GetByID(ctx, account.ID)
	if updated.AccountName != "Updated Name" {
		t.Errorf("Expected account name 'Updated Name', got '%s'", updated.AccountName)
	}
	if updated.Status != 0 {
		t.Errorf("Expected status 0, got %d", updated.Status)
	}
}

// TestPlatformAccountRepository_Delete 测试删除账号
func TestPlatformAccountRepository_Delete(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	account := &model.PlatformAccount{
		Platform:    model.PlatformDouyin,
		AccountID:   "dy-delete",
		AccountName: "To Delete",
	}
	accountRepo.Create(ctx, account)

	err := accountRepo.Delete(ctx, account.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = accountRepo.GetByID(ctx, account.ID)
	if err == nil {
		t.Error("Expected account to be deleted")
	}
}

// TestPlatformAccountRepository_UpdateStatus 测试更新账号状态
func TestPlatformAccountRepository_UpdateStatus(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	account := &model.PlatformAccount{
		Platform:  model.PlatformDouyin,
		AccountID: "dy-status",
		Status:    1,
	}
	accountRepo.Create(ctx, account)

	tests := []struct {
		name      string
		newStatus int
		wantErr   bool
	}{
		{
			name:      "update to disabled",
			newStatus: 0,
			wantErr:   false,
		},
		{
			name:      "update to enabled",
			newStatus: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := accountRepo.UpdateStatus(context.Background(), account.ID, tt.newStatus)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateStatus() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				updated, _ := accountRepo.GetByID(ctx, account.ID)
				if updated.Status != tt.newStatus {
					t.Errorf("Expected status %d, got %d", tt.newStatus, updated.Status)
				}
			}
		})
	}
}

// TestPlatformAccountRepository_UpdateLastSync 测试更新时间
func TestPlatformAccountRepository_UpdateLastSync(t *testing.T) {
	_, _, accountRepo := setupUnifiedMessageRepositories(t)
	ctx := context.Background()

	account := &model.PlatformAccount{
		Platform:  model.PlatformDouyin,
		AccountID: "dy-sync",
	}
	accountRepo.Create(ctx, account)

	err := accountRepo.UpdateLastSync(context.Background(), account.ID)
	if err != nil {
		t.Errorf("UpdateLastSync() error = %v", err)
	}

	updated, _ := accountRepo.GetByID(ctx, account.ID)
	if updated.LastSyncAt == nil {
		t.Error("Expected LastSyncAt to be updated")
	}
}

