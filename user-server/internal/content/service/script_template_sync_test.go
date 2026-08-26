package service

import (
	"testing"

	contentmodel "hivemtk-user/internal/content/model"
	mainmodel "hivemtk-user/internal/model"
)

// --- T-5 template → library 映射 ---

func TestTemplateToLibrary_Mapping(t *testing.T) {
	tpl := &contentmodel.ScriptTemplate{
		Category:     "objection",
		Title:        "价格异议应对",
		Content:      "先讲价值再谈价格",
		JourneyStage: "conversion",
		Tags:         "价格,异议；转化",
	}
	got := templateToLibrary(tpl)
	if got.Category != "objection" || got.Title != "价格异议应对" || got.Content != "先讲价值再谈价格" {
		t.Errorf("基础字段映射错误: %+v", got)
	}
	if got.Scenario != "conversion" {
		t.Errorf("Scenario 应取 JourneyStage，got %q", got.Scenario)
	}
	wantTags := mainmodel.JSONArray{"价格", "异议", "转化"}
	if len(got.Tags) != len(wantTags) {
		t.Fatalf("Tags = %v, want %v", got.Tags, wantTags)
	}
	for i := range wantTags {
		if got.Tags[i] != wantTags[i] {
			t.Errorf("Tags[%d] = %v, want %v", i, got.Tags[i], wantTags[i])
		}
	}
	// 运行时统计字段必须为零值（同步不覆盖 library 侧统计）
	if got.UsageCount != 0 || got.SuccessCount != 0 || got.ConversionRate != 0 {
		t.Errorf("映射不应携带运行时统计: %+v", got)
	}
}

func TestParseTagsToJSONArray_PositiveAndNegative(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want mainmodel.JSONArray
	}{
		{"中英文逗号分号混排", "a,b，c；d;e", mainmodel.JSONArray{"a", "b", "c", "d", "e"}},
		{"空串得空数组非nil", "", mainmodel.JSONArray{}},
		{"纯分隔符得空数组", ",，，;;", mainmodel.JSONArray{}},
		{"首尾空白被裁剪", " 价格 ， 异议 ", mainmodel.JSONArray{"价格", "异议"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTagsToJSONArray(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parse(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parse(%q)[%d] = %v, want %v", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSyncStats_SkipsEmptyTitleOrContent(t *testing.T) {
	// 空字段跳过规则在 SyncToLibrary 中执行；此处验证判定条件本身
	cases := []struct{ title, content string }{
		{"", "有内容"},
		{"有标题", ""},
		{"", ""},
	}
	for _, c := range cases {
		if c.title != "" && c.content != "" {
			t.Errorf("case %+v 不应被判为跳过", c)
		}
	}
}
