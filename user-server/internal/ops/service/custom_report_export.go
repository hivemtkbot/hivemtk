package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"hivemtk-user/internal/ops/model"
)

// CSV 同步流式导出安全边界（M18 表 D-4：同步安全边界 csv30K 行）。
// 超过上限时拒绝同步导出，提示缩小时间范围/过滤条件（异步任务表本期不做）。
const CSVExportMaxRows = 30000

// ErrReportTooManyRows 报表行数超过 CSV 同步导出上限
var ErrReportTooManyRows = errors.New("报表数据超过 30000 行，CSV 同步导出已拒绝；请缩小时间范围或增加过滤条件后重试")

// ExportReportCSV 将报表查询结果以 CSV 流式写出。
//
// 决策源：docs/architecture/MASTER_COMPETITIVE_DECISIONS.md M18 表 D-4。
// csv.Writer 直接写 ResponseWriter（controller 层注入 io.Writer），不在内存中拼装
// 完整文件体；行数超过 CSVExportMaxRows 时返回 ErrReportTooManyRows（不写任何字节）。
func (s *CustomReportService) ExportReportCSV(ctx context.Context, w io.Writer, report *model.CustomReport, params map[string]any) error {
	data, err := s.QueryReportData(ctx, report, params)
	if err != nil {
		return err
	}
	return ExportReportDataCSV(w, data)
}

// ExportReportDataCSV 行数校验 + CSV 写出（独立导出便于单测覆盖上限语义）。
func ExportReportDataCSV(w io.Writer, data *model.ReportData) error {
	if data.Total > CSVExportMaxRows || int64(len(data.Data)) > CSVExportMaxRows {
		return ErrReportTooManyRows
	}
	return writeReportCSV(w, data)
}

func writeReportCSV(w io.Writer, data *model.ReportData) error {
	cw := csv.NewWriter(w)

	declared := make([]string, 0, len(data.Dimensions)+len(data.Metrics))
	declared = append(declared, data.Dimensions...)
	declared = append(declared, data.Metrics...)

	rowKeys := make(map[string]bool)
	for _, row := range data.Data {
		for k := range row {
			rowKeys[k] = true
		}
	}

	header := make([]string, 0, len(rowKeys))
	seen := make(map[string]bool)

	for _, d := range declared {
		if !seen[d] && rowKeys[d] {
			seen[d] = true
			header = append(header, d)
		}
	}

	extra := make([]string, 0, len(rowKeys))
	for k := range rowKeys {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	header = append(header, extra...)

	if err := cw.Write(header); err != nil {
		return err
	}

	for _, row := range data.Data {
		record := make([]string, len(header))
		for i, col := range header {
			record[i] = formatCSVCell(row[col])
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

func formatCSVCell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case time.Time:
		return t.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", t)
	}
}
