package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"marketing/internal/content/model"
	cdpmodel "marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	cdprepo "marketing/internal/repository"

	"gorm.io/gorm"
)

// BatchOperationService 批量操作服务
type BatchOperationService struct {
	db       *gorm.DB
	clueRepo cdprepo.ClueRepository
	userRepo cdprepo.UserRepository
}

// NewBatchOperationService 创建批量操作服务
func NewBatchOperationService() *BatchOperationService {
	return &BatchOperationService{
		db:       db.GetDB(),
		clueRepo: cdprepo.NewClueRepository(),
		userRepo: cdprepo.NewUserRepository(),
	}
}

// ImportType 导入类型
type ImportType string

const (
	ImportTypeClue    ImportType = "clue"
	ImportTypeUser    ImportType = "user"
	ImportTypeAccount ImportType = "account"
)

// ImportError 导入错误
type ImportError struct {
	Row     int    `json:"row"`
	Reason  string `json:"reason"`
	Content string `json:"content,omitempty"`
}

// ImportResult 导入结果
type ImportResult struct {
	ImportType ImportType    `json:"import_type"`
	Total      int           `json:"total"`
	Success    int           `json:"success"`
	Failed     int           `json:"failed"`
	Skipped    int           `json:"skipped"`
	Errors     []ImportError `json:"errors,omitempty"`
	ImportedAt string        `json:"imported_at"`
}

// ImportFromCSV 从 CSV 导入线索
func (s *BatchOperationService) ImportFromCSV(importType ImportType, file multipart.File) (*ImportResult, error) {
	result := &ImportResult{
		ImportType: importType,
		Errors:     make([]ImportError, 0),
		ImportedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // 允许可变列数
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失败: %w", err)
	}

	if len(rows) == 0 {
		return result, nil
	}

	// 跳过表头
	header := rows[0]
	dataRows := rows[1:]
	result.Total = len(dataRows)

	switch importType {
	case ImportTypeClue:
		s.importClues(dataRows, header, result)
	case ImportTypeUser:
		s.importUsers(dataRows, header, result)
	default:
		return nil, fmt.Errorf("不支持的导入类型: %s", importType)
	}

	return result, nil
}

// importClues 导入线索
func (s *BatchOperationService) importClues(rows [][]string, header []string, result *ImportResult) {
	// 解析列索引
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	requiredCols := []string{"姓名", "账户", "类型", "城市", "地址", "描述", "来源 ID"}
	for _, col := range requiredCols {
		if _, ok := colMap[col]; !ok {
			// 尝试英文列名
			if col == "姓名" {
				if _, ok := colMap["name"]; !ok {
					result.Errors = append(result.Errors, ImportError{
						Row:    0,
						Reason: fmt.Sprintf("缺少必要列: %s", col),
					})
					result.Failed = len(rows)
					return
				}
			}
		}
	}

	for i, row := range rows {
		if len(row) < 2 {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:     i + 2,
				Reason:  "行数据列数不足",
				Content: strings.Join(row, ","),
			})
			continue
		}

		getValue := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		name := getValue("姓名")
		if name == "" {
			name = getValue("name")
		}
		account := getValue("账户")
		if account == "" {
			account = getValue("account")
		}
		typeStr := getValue("类型")
		if typeStr == "" {
			typeStr = getValue("type")
		}
		city := getValue("城市")
		if city == "" {
			city = getValue("city")
		}
		address := getValue("地址")
		if address == "" {
			address = getValue("address")
		}
		desc := getValue("描述")
		if desc == "" {
			desc = getValue("desc")
		}
		sourceID := getValue("来源 ID")
		if sourceID == "" {
			sourceID = getValue("source_id")
		}

		if name == "" && account == "" {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:     i + 2,
				Reason:  "姓名和账户至少需要一个",
				Content: strings.Join(row, ","),
			})
			continue
		}

		clueType, _ := strconv.ParseInt(typeStr, 10, 64)

		clue := &cdpmodel.Clue{
			SourceID: sourceID,
			Account:  account,
			Type:     clueType,
			Name:     name,
			City:     city,
			Address:  address,
			Desc:     desc,
		}

		if err := s.clueRepo.Create(clue); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:     i + 2,
				Reason:  err.Error(),
				Content: strings.Join(row, ","),
			})
			continue
		}
		result.Success++
	}
}

// importUsers 导入用户
func (s *BatchOperationService) importUsers(rows [][]string, header []string, result *ImportResult) {
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}

	for i, row := range rows {
		if len(row) < 2 {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:     i + 2,
				Reason:  "行数据列数不足",
				Content: strings.Join(row, ","),
			})
			continue
		}

		getValue := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		username := getValue("用户名")
		if username == "" {
			username = getValue("username")
		}
		email := getValue("邮箱")
		if email == "" {
			email = getValue("email")
		}
		phone := getValue("手机号")
		if phone == "" {
			phone = getValue("phone")
		}
		realName := getValue("姓名")
		if realName == "" {
			realName = getValue("real_name")
		}

		if username == "" {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:     i + 2,
				Reason:  "用户名为空",
				Content: strings.Join(row, ","),
			})
			continue
		}

		user := &cdpmodel.User{
			Username: username,
			Email:    email,
			Phone:    phone,
			RealName: realName,
		}

		if err := s.userRepo.Create(user); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ImportError{
				Row:     i + 2,
				Reason:  err.Error(),
				Content: strings.Join(row, ","),
			})
			continue
		}
		result.Success++
	}
}

// ExportType 导出类型
type ExportType string

const (
	ExportTypeClue ExportType = "clue"
	ExportTypeUser ExportType = "user"
)

// GenerateCSV 生成 CSV 数据
func (s *BatchOperationService) GenerateCSV(exportType ExportType, ids []string) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	switch exportType {
	case ExportTypeClue:
		return s.exportClues(writer, &buf, ids)
	case ExportTypeUser:
		return s.exportUsers(writer, &buf, ids)
	default:
		return nil, errors.New("不支持的导出类型")
	}
}

func (s *BatchOperationService) exportClues(writer *csv.Writer, buf *bytes.Buffer, ids []string) (*bytes.Buffer, error) {
	headers := []string{"ID", "姓名", "账户", "类型", "城市", "地址", "描述", "来源ID", "是否验证", "创建时间"}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	clues, _, err := s.clueRepo.GetClueList(1, 10000)
	if err != nil {
		return nil, fmt.Errorf("查询线索失败: %w", err)
	}

	// 创建 ID 过滤集合
	idSet := make(map[string]bool)
	hasFilter := len(ids) > 0
	for _, id := range ids {
		idSet[strings.TrimSpace(id)] = true
	}

	for _, clue := range clues {
		if hasFilter && !idSet[clue.ID] {
			continue
		}
		row := []string{
			clue.ID,
			clue.Name,
			clue.Account,
			strconv.FormatInt(clue.Type, 10),
			clue.City,
			clue.Address,
			clue.Desc,
			clue.SourceID,
			strconv.FormatInt(clue.IsVerify, 10),
			time.Unix(clue.CreateTime, 0).Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *BatchOperationService) exportUsers(writer *csv.Writer, buf *bytes.Buffer, ids []string) (*bytes.Buffer, error) {
	headers := []string{"ID", "用户名", "邮箱", "手机号", "真实姓名", "状态", "创建时间"}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	users, _, err := s.userRepo.GetUserList(1, 10000)
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	idSet := make(map[string]bool)
	hasFilter := len(ids) > 0
	for _, id := range ids {
		idSet[strings.TrimSpace(id)] = true
	}

	for _, u := range users {
		if hasFilter && !idSet[u.ID] {
			continue
		}
		row := []string{
			u.ID,
			u.Username,
			u.Email,
			u.Phone,
			u.RealName,
			strconv.Itoa(int(u.Status)),
			time.Unix(u.CreateTime, 0).Format("2006-01-02 15:04:05"),
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf, nil
}

// TemplateDefinition 模板定义
type TemplateDefinition struct {
	Type        string
	Description string
	Headers     []string
	Examples    [][]string
}

// GetTemplate 获取模板定义
func (s *BatchOperationService) GetTemplate(importType ImportType) (*TemplateDefinition, error) {
	switch importType {
	case ImportTypeClue:
		return &TemplateDefinition{
			Type:        string(ImportTypeClue),
			Description: "线索导入模板 - 每行一条线索记录",
			Headers:     []string{"姓名", "账户", "类型", "城市", "地址", "描述", "来源ID"},
			Examples: [][]string{
				{"张三", "13800138000", "0", "北京", "北京市朝阳区", "示例客户", "source_001"},
				{"李四", "13900139000", "1", "上海", "上海市浦东新区", "潜在客户", "source_002"},
			},
		}, nil
	case ImportTypeUser:
		return &TemplateDefinition{
			Type:        string(ImportTypeUser),
			Description: "用户导入模板 - 每行一个用户记录",
			Headers:     []string{"用户名", "邮箱", "手机号", "真实姓名"},
			Examples: [][]string{
				{"user001", "user001@example.com", "13800138000", "用户一"},
			},
		}, nil
	default:
		return nil, errors.New("不支持的导入类型")
	}
}

// GenerateTemplateCSV 生成模板 CSV
func (s *BatchOperationService) GenerateTemplateCSV(importType ImportType) (*bytes.Buffer, error) {
	tmpl, err := s.GetTemplate(importType)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	if err := writer.Write(tmpl.Headers); err != nil {
		return nil, err
	}
	for _, ex := range tmpl.Examples {
		if err := writer.Write(ex); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// BatchDelete 删除多条线索
func (s *BatchOperationService) BatchDeleteClues(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ID 列表不能为空")
	}
	count := 0
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := s.clueRepo.Delete(id); err != nil {
			logger.Error(err, "删除线索失败: "+id)
			continue
		}
		count++
	}
	return count, nil
}

// BatchUpdateRequest 批量更新请求
type BatchUpdateRequest struct {
	IDs    []string          `json:"ids"`
	Fields map[string]string `json:"fields"`
}

// BatchUpdateClues 批量更新线索
func (s *BatchOperationService) BatchUpdateClues(req *BatchUpdateRequest) (int, error) {
	if len(req.IDs) == 0 {
		return 0, errors.New("ID 列表不能为空")
	}
	if len(req.Fields) == 0 {
		return 0, errors.New("更新字段不能为空")
	}

	// 构造可更新字段
	updates := make(map[string]any)
	if v, ok := req.Fields["name"]; ok {
		updates["name"] = v
	}
	if v, ok := req.Fields["city"]; ok {
		updates["city"] = v
	}
	if v, ok := req.Fields["address"]; ok {
		updates["address"] = v
	}
	if v, ok := req.Fields["desc"]; ok {
		updates["desc"] = v
	}
	if v, ok := req.Fields["is_verify"]; ok {
		if iv, err := strconv.ParseInt(v, 10, 64); err == nil {
			updates["is_verify"] = iv
		}
	}
	if v, ok := req.Fields["type"]; ok {
		if iv, err := strconv.ParseInt(v, 10, 64); err == nil {
			updates["type"] = iv
		}
	}

	if len(updates) == 0 {
		return 0, errors.New("无可更新字段")
	}

	tx := s.db.Begin()
	count := 0
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := tx.Model(&cdpmodel.Clue{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			logger.Error(err, "更新线索失败: "+id)
			continue
		}
		count++
	}
	if err := tx.Commit().Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ReadAllFromReader 工具函数：从 io.Reader 读取所有内容
func ReadAllFromReader(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// GetHistories 获取批量操作历史列表
func (s *BatchOperationService) GetHistories(page, pageSize int) ([]*model.BatchOperationHistory, int64, error) {
	var list []*model.BatchOperationHistory
	var total int64

	query := s.db.Model(&model.BatchOperationHistory{}).Where("1 = 1")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// GetHistoryByID 根据 ID 获取批量操作历史
func (s *BatchOperationService) GetHistoryByID(id uint) (*model.BatchOperationHistory, error) {
	var history model.BatchOperationHistory
	err := s.db.First(&history, id).Error
	return &history, err
}

// CancelHistory 取消批量操作（标记为已取消）
func (s *BatchOperationService) CancelHistory(id uint) error {
	now := time.Now()
	result := s.db.Model(&model.BatchOperationHistory{}).Where("id = ? AND status IN ?", id, []string{"pending", "running"}).
		Updates(map[string]any{
			"status":      "cancelled",
			"finished_at": now,
			"updated_at":  now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("操作记录不存在或状态不可取消")
	}
	return nil
}
