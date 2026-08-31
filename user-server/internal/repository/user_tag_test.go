package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupUserTagTestDB 设置用户标签测试数据库
func setupUserTagTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.UserTag{},
	)
	db.SetTestDB(database)
	return database
}

// setupUserTagRepository 创建测试用的仓库实例
func setupUserTagRepository(t *testing.T) *userTagRepo {
	setupUserTagTestDB(t)
	return &userTagRepo{db: db.GetDB()}
}

// TestUserTagRepository_AddTag 测试添加单个标签
func TestUserTagRepository_AddTag(t *testing.T) {
	_ = setupUserTagRepository(t)

	tests := []struct {
		name    string
		userID  string
		tagName string
		wantErr bool
	}{
		{
			name:    "add single tag success",
			userID:  "user-1",
			tagName: "VIP",
			wantErr: false,
		},
		{
			name:    "add tag with special characters",
			userID:  "user-1",
			tagName: "high-value-customer",
			wantErr: false,
		},
		{
			name:    "add duplicate tag should not error",
			userID:  "user-1",
			tagName: "VIP",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupUserTagRepository(t)

			err := repo.AddTag(context.Background(), tt.userID, tt.tagName)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddTag() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				tags, err := repo.GetTagsByUser(context.Background(), tt.userID)
				if err != nil {
					t.Fatalf("GetTagsByUser failed: %v", err)
				}

				found := false
				for _, tag := range tags {
					if tag == tt.tagName {
						found = true
						break
					}
				}
				if !found && tt.name != "add duplicate tag should not error" {
					t.Errorf("Expected tag %s to be added", tt.tagName)
				}
			}
		})
	}
}

// TestUserTagRepository_AddTags 测试批量添加标签
func TestUserTagRepository_AddTags(t *testing.T) {
	_ = setupUserTagRepository(t)

	tests := []struct {
		name      string
		userID    string
		tags      []string
		wantErr   bool
		wantCount int
	}{
		{
			name:      "add multiple tags",
			userID:    "user-2",
			tags:      []string{"tag1", "tag2", "tag3"},
			wantErr:   false,
			wantCount: 3,
		},
		{
			name:      "add empty tags",
			userID:    "user-3",
			tags:      []string{},
			wantErr:   false,
			wantCount: 0,
		},
		{
			name:      "add tags with duplicates",
			userID:    "user-2",
			tags:      []string{"tag4", "tag1"},
			wantErr:   false,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupUserTagRepository(t)

			err := repo.AddTags(context.Background(), tt.userID, tt.tags)

			if (err != nil) != tt.wantErr {
				t.Errorf("AddTags() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				tags, err := repo.GetTagsByUser(context.Background(), tt.userID)
				if err != nil {
					t.Fatalf("GetTagsByUser failed: %v", err)
				}

				if len(tags) != tt.wantCount {
					t.Errorf("Expected %d tags, got %d", tt.wantCount, len(tags))
				}
			}
		})
	}
}

// TestUserTagRepository_RemoveTag 测试移除单个标签
func TestUserTagRepository_RemoveTag(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTags(context.Background(), "user-4", []string{"tag1", "tag2", "tag3"})

	tests := []struct {
		name          string
		tagName       string
		wantErr       bool
		wantRemaining int
	}{
		{
			name:          "remove existing tag",
			tagName:       "tag2",
			wantErr:       false,
			wantRemaining: 2,
		},
		{
			name:          "remove non-existing tag",
			tagName:       "non-existing",
			wantErr:       false,
			wantRemaining: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.RemoveTag(context.Background(), "user-4", tt.tagName)

			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveTag() error = %v, wantErr %v", err, tt.wantErr)
			}

			tags, err := repo.GetTagsByUser(context.Background(), "user-4")
			if err != nil {
				t.Fatalf("GetTagsByUser failed: %v", err)
			}

			if len(tags) != tt.wantRemaining {
				t.Errorf("Expected %d remaining tags, got %d", tt.wantRemaining, len(tags))
			}
		})
	}
}

// TestUserTagRepository_RemoveTags 测试批量移除标签
func TestUserTagRepository_RemoveTags(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTags(context.Background(), "user-5", []string{"tag1", "tag2", "tag3", "tag4"})

	tests := []struct {
		name          string
		tags          []string
		wantErr       bool
		wantRemaining int
	}{
		{
			name:          "remove multiple tags",
			tags:          []string{"tag1", "tag3"},
			wantErr:       false,
			wantRemaining: 2,
		},
		{
			name:          "remove empty tags",
			tags:          []string{},
			wantErr:       false,
			wantRemaining: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.RemoveTags(context.Background(), "user-5", tt.tags)

			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveTags() error = %v, wantErr %v", err, tt.wantErr)
			}

			tags, err := repo.GetTagsByUser(context.Background(), "user-5")
			if err != nil {
				t.Fatalf("GetTagsByUser failed: %v", err)
			}

			if len(tags) != tt.wantRemaining {
				t.Errorf("Expected %d remaining tags, got %d", tt.wantRemaining, len(tags))
			}
		})
	}
}

// TestUserTagRepository_GetTagsByUser 测试获取用户标签
func TestUserTagRepository_GetTagsByUser(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTags(context.Background(), "user-6", []string{"vip", "active", "high-value"})

	tests := []struct {
		name     string
		userID   string
		wantTags []string
		wantErr  bool
	}{
		{
			name:     "get user tags",
			userID:   "user-6",
			wantTags: []string{"vip", "active", "high-value"},
			wantErr:  false,
		},
		{
			name:     "get non-existing user tags",
			userID:   "user-non-existing",
			wantTags: []string{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags, err := repo.GetTagsByUser(context.Background(), tt.userID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetTagsByUser() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(tags) != len(tt.wantTags) {
				t.Errorf("Expected %d tags, got %d", len(tt.wantTags), len(tags))
			}
		})
	}
}

// TestUserTagRepository_GetUsersByTag 测试获取具有指定标签的用户
func TestUserTagRepository_GetUsersByTag(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTag(context.Background(), "user-7", "vip")
	repo.AddTag(context.Background(), "user-8", "vip")
	repo.AddTag(context.Background(), "user-9", "active")

	tests := []struct {
		name      string
		tagName   string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "get users by tag",
			tagName:   "vip",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "get users by non-existing tag",
			tagName:   "non-existing",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users, err := repo.GetUsersByTag(context.Background(), tt.tagName)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUsersByTag() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(users) != tt.wantCount {
				t.Errorf("Expected %d users, got %d", tt.wantCount, len(users))
			}
		})
	}
}

// TestUserTagRepository_DeleteTagsByUser 测试删除用户的所有标签
func TestUserTagRepository_DeleteTagsByUser(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTags(context.Background(), "user-10", []string{"tag1", "tag2", "tag3"})

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "delete all user tags",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.DeleteTagsByUser(context.Background(), "user-10")

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteTagsByUser() error = %v, wantErr %v", err, tt.wantErr)
			}

			tags, err := repo.GetTagsByUser(context.Background(), "user-10")
			if err != nil {
				t.Fatalf("GetTagsByUser failed: %v", err)
			}

			if len(tags) != 0 {
				t.Errorf("Expected 0 tags after deletion, got %d", len(tags))
			}
		})
	}
}

// TestUserTagRepository_DeleteTagsByName 测试删除指定名称的标签
func TestUserTagRepository_DeleteTagsByName(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTag(context.Background(), "user-11", "global-tag")
	repo.AddTag(context.Background(), "user-12", "global-tag")
	repo.AddTag(context.Background(), "user-13", "other-tag")

	tests := []struct {
		name      string
		tagName   string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "delete tag by name",
			tagName:   "global-tag",
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.DeleteTagsByName(context.Background(), tt.tagName)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteTagsByName() error = %v, wantErr %v", err, tt.wantErr)
			}

			// 获取所有剩余标签
			var allTags []model.UserTag
			db.GetDB().Find(&allTags)

			if len(allTags) != tt.wantCount {
				t.Errorf("Expected %d tags remaining, got %d", tt.wantCount, len(allTags))
			}
		})
	}
}

// TestUserTagRepository_HasTag 测试检查用户是否有指定标签
func TestUserTagRepository_HasTag(t *testing.T) {
	repo := setupUserTagRepository(t)

	repo.AddTag(context.Background(), "user-14", "vip")

	tests := []struct {
		name    string
		userID  string
		tagName string
		wantHas bool
		wantErr bool
	}{
		{
			name:    "has tag",
			userID:  "user-14",
			tagName: "vip",
			wantHas: true,
			wantErr: false,
		},
		{
			name:    "does not have tag",
			userID:  "user-14",
			tagName: "non-existing",
			wantHas: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			has, err := repo.HasTag(context.Background(), tt.userID, tt.tagName)

			if (err != nil) != tt.wantErr {
				t.Errorf("HasTag() error = %v, wantErr %v", err, tt.wantErr)
			}

			if has != tt.wantHas {
				t.Errorf("Expected has=%v, got %v", tt.wantHas, has)
			}
		})
	}
}
