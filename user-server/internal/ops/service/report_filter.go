package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/ops/model"
	"gorm.io/gorm"
)

// R55 T2：报表过滤器 SQL 落地（此前 ReportFilter 存储/编辑完备但查询全不应用，
// 用户建报表带筛选条件得到的是未过滤的错误数据）。
//
// 安全约束：字段必须命中数据源白名单（标识符直接拼接前强校验，防注入）；
// 操作符仅支持 eq/ne/gt/lt/like；值走参数绑定，绝不拼接。

// filterFieldWhitelist 每数据源允许过滤的字段（列名硬白名单）
var filterFieldWhitelist = map[string]map[string]string{
	"sessions": {
		"status":     "status",
		"agent_name": "agent_name",
		"platform":   "platform",
		"handler_type": "handler_type",
	},
	"messages": {
		"content_type": "COALESCE(content_type, 'unknown')",
		"platform":     "platform",
		"status":       "status",
	},
	"clues": {
		"type":       "type",
		"is_verify":  "is_verify",
		"level":      "level",
		"is_group":   "is_group",
	},
	"users": {
		"churn_risk": "churn_risk",
	},
}

// filterOperatorSQL 操作符 → SQL 片段（值占位符由调用方绑定）
var filterOperatorSQL = map[string]string{
	"eq":   "= ?",
	"ne":   "!= ?",
	"gt":   "> ?",
	"lt":   "< ?",
	"like": "ILIKE '%' || ? || '%'",
}

// BuildReportFilterSQL 将报表过滤器解析为安全 SQL 片段与绑定值
//
// dataSource 用于字段白名单校验；返回的 conds 可直接 WHERE 拼接。
// 非法字段/操作符直接跳过该条（宽容降级，不因脏配置炸整个报表）。
func BuildReportFilterSQL(dataSource string, filtersJSON string) ([]string, []any) {
	whitelist, ok := filterFieldWhitelist[dataSource]
	if !ok || strings.TrimSpace(filtersJSON) == "" {
		return nil, nil
	}
	var filters []model.ReportFilter
	if err := json.Unmarshal([]byte(filtersJSON), &filters); err != nil || len(filters) == 0 {
		return nil, nil
	}
	conds := make([]string, 0, len(filters))
	args := make([]any, 0, len(filters))
	for _, f := range filters {
		col, ok := whitelist[f.Field]
		if !ok || f.Field == "" {
			continue
		}
		opSQL, ok := filterOperatorSQL[f.Operator]
		if !ok {
			continue
		}
		if f.Value == nil || fmt.Sprintf("%v", f.Value) == "" {
			continue
		}
		conds = append(conds, col+" "+opSQL)
		args = append(args, normalizeFilterValue(f.Value))
	}
	return conds, args
}

// normalizeFilterValue 归一化过滤值（前端传字符串，数字列需转数值）
func normalizeFilterValue(v any) any {
	switch t := v.(type) {
	case string:
		var i int64
		if !strings.ContainsAny(t, ".- ") && isAllDigits(t) {
			if _, err := fmt.Sscanf(t, "%d", &i); err == nil {
				return i
			}
		}
		return t
	default:
		return v
	}
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// applyConds 将条件应用到 gorm 查询
func applyConds(base *gorm.DB, conds []string, args []any) *gorm.DB {
	if len(conds) == 0 {
		return base
	}
	return base.Where(strings.Join(conds, " AND "), args...)
}
