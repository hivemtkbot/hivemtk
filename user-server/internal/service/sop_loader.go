package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"gorm.io/gorm"
)

// IndustrySOP 行业 SOP
type IndustrySOP struct {
	ID       string                   `json:"id"`
	Name     string                   `json:"name"`
	Industry string                   `json:"industry"`
	Category string                   `json:"category"`
	Steps    []map[string]interface{} `json:"steps"`
	KPI      map[string]interface{}   `json:"kpi"`
	ToolRefs []string                 `json:"tool_refs"`
}

// SOPLoader SOP 加载器
type SOPLoader struct {
	db *gorm.DB
}

func NewSOPLoader(db *gorm.DB) *SOPLoader {
	return &SOPLoader{db: db}
}

func (l *SOPLoader) LoadSOP(ctx context.Context, sopID string) (*IndustrySOP, error) {
	if data, found := LoadAssetFromDB(l.db, "industry_sop", sopID); found {
		var s IndustrySOP
		if err := json.Unmarshal(data, &s); err == nil {
			s.ID = sopID
			return &s, nil
		}
		slog.Warn("SOPLoader invalid JSON, fallback", "asset_id", sopID)
	}
	return loadDefaultSOP(sopID)
}

func (l *SOPLoader) ListAllSOPs(ctx context.Context) ([]*IndustrySOP, error) {
	var result []*IndustrySOP
	rows, _ := ListAssetsFromDB(l.db, "industry_sop")
	seen := map[string]bool{}
	for _, r := range rows {
		var s IndustrySOP
		if err := json.Unmarshal(r.Data, &s); err == nil {
			s.ID = r.AssetID
			s.Name = r.Name
			result = append(result, &s)
			seen[s.ID] = true
		}
	}
	for id, s := range defaultSOPs() {
		if !seen[id] {
			result = append(result, s)
		}
	}
	return result, nil
}

func loadDefaultSOP(id string) (*IndustrySOP, error) {
	if s, ok := defaultSOPs()[id]; ok {
		return s, nil
	}
	return nil, errors.New("sop not found: " + id)
}

func defaultSOPs() map[string]*IndustrySOP {
	return map[string]*IndustrySOP{
		"default-reception": {
			ID:       "default-reception",
			Name:     "默认客户接待 SOP",
			Category: "客户接待",
			Steps: []map[string]interface{}{
				{"order": 1, "name": "识别来源", "action": "tag_source"},
				{"order": 2, "name": "需求分析", "action": "ask_need"},
				{"order": 3, "name": "产品推荐", "action": "recommend"},
				{"order": 4, "name": "促成下单", "action": "close"},
			},
			KPI: map[string]interface{}{"conversion": ">15%"},
		},
	}
}
