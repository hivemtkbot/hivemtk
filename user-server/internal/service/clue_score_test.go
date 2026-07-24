package service

import (
	"context"
	"marketing/internal/model"
	"testing"
	"time"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
	"marketing/internal/pkg/utils/db"
)

func setupClueScoreTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Clue{},
		&model.ClueScore{},
		&model.ClueEngagementEvent{},
	)
	db.SetTestDB(database)
	return database
}

func TestClueScoreService_ScoreClue_Verify(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	// 使用近期时间，让 recency 维度有分（线索创建时间=现在，recency 满分）
	clue := &model.Clue{
		ID:         "test-clue-1",
		Account:    "acc-1",
		Type:       3, // 电话（高分渠道）
		IsVerify:   1,
		Name:       "Alice",
		City:       "上海",
		Address:    "徐汇区",
		Desc:       "高质量客户",
		CreateTime: time.Now().Unix(),
	}
	score, err := svc.ScoreClue(clue)
	if err != nil {
		t.Fatalf("ScoreClue failed: %v", err)
	}
	if score == nil {
		t.Fatal("expected non-nil score")
	}
	if score.TotalScore < 70 {
		t.Errorf("expected total >= 70 for high quality clue, got %d", score.TotalScore)
	}
	if score.Grade != "S" && score.Grade != "A" && score.Grade != "B" {
		t.Errorf("expected grade S/A/B, got %s", score.Grade)
	}
}

func TestClueScoreService_ScoreClue_LowQuality(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	clue := &model.Clue{
		ID:       "test-clue-2",
		Account:  "acc-2",
		Type:     6, // twitter（低分渠道）
		IsVerify: 0,
		// 全部字段为空
	}
	score, err := svc.ScoreClue(clue)
	if err != nil {
		t.Fatalf("ScoreClue failed: %v", err)
	}
	if score.TotalScore > 60 {
		t.Errorf("expected total <= 60 for low quality clue, got %d", score.TotalScore)
	}
}

func TestClueScoreService_ScoreClue_ChannelScore(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	// 电话应该得高分
	c1 := &model.Clue{ID: "c-phone", Account: "a1", Type: 3, IsVerify: 1, Name: "A"}
	// twitter 应该得低分
	c2 := &model.Clue{ID: "c-tw", Account: "a2", Type: 6, IsVerify: 1, Name: "A"}

	s1, _ := svc.ScoreClue(c1)
	s2, _ := svc.ScoreClue(c2)

	if s1.ChannelScore <= s2.ChannelScore {
		t.Errorf("phone channel score should be > twitter; got phone=%d tw=%d", s1.ChannelScore, s2.ChannelScore)
	}
}

func TestClueScoreService_RecordEngagement(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	err := svc.RecordEngagement("c-eng-1", "reply", "wechat", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("RecordEngagement failed: %v", err)
	}
}

func TestClueScoreService_GetByClueID(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	clue := &model.Clue{ID: "c-get", Account: "a", Type: 2, IsVerify: 1, Name: "A"}
	created, err := svc.ScoreClue(clue)
	if err != nil {
		t.Fatalf("ScoreClue failed: %v", err)
	}

	got, err := svc.GetByClueID("c-get")
	if err != nil {
		t.Fatalf("GetByClueID failed: %v", err)
	}
	if got.TotalScore != created.TotalScore {
		t.Errorf("expected %d, got %d", created.TotalScore, got.TotalScore)
	}
}

func TestClueScoreService_ListByGrade(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	// 高分
	svc.ScoreClue(&model.Clue{ID: "g1", Account: "g1", Type: 3, IsVerify: 1, Name: "A", City: "B", Address: "C", Desc: "D"})
	// 低分
	svc.ScoreClue(&model.Clue{ID: "g2", Account: "g2", Type: 6, IsVerify: 0})

	list, total, err := svc.ListByGrade("", 1, 10)
	if err != nil {
		t.Fatalf("ListByGrade failed: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
}

func TestClueScoreService_LoadClueForScoring(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	// Clue.BeforeCreate 会覆盖 ID 为 UUID，因此创建后用实际 ID 重新查找
	clue := &model.Clue{Account: "la", Type: 2, IsVerify: 1, Name: "load"}
	err := svc.clueRepo.Create(context.Background(), clue)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	actualID := clue.ID
	if actualID == "" {
		t.Fatal("created clue should have an auto-generated ID")
	}
	got, err := svc.LoadClueForScoring(actualID)
	if err != nil {
		t.Fatalf("LoadClueForScoring failed: %v", err)
	}
	if got.Account != "la" {
		t.Errorf("expected account 'la', got %s", got.Account)
	}
}

func TestClueScoreService_LoadClueForScoring_NotFound(t *testing.T) {
	_ = setupClueScoreTestDB(t)

	svc := NewClueScoreService()
	_, err := svc.LoadClueForScoring("non-existent")
	if err == nil {
		t.Error("expected error for non-existent clue")
	}
}

func TestCalcGradeFromScore(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{100, "S"},
		{90, "S"},
		{89, "A"},
		{75, "A"},
		{74, "B"},
		{60, "B"},
		{59, "C"},
		{40, "C"},
		{39, "D"},
		{0, "D"},
	}
	for _, c := range cases {
		if got := model.CalcGradeFromScore(c.score); got != c.want {
			t.Errorf("score=%d expected %s, got %s", c.score, c.want, got)
		}
	}
}
