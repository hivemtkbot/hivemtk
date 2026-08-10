// 拆分自 marketing_flow.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hivemtk-user/internal/content/model"
	reachmodel "hivemtk-user/internal/model"
	_type "hivemtk-user/internal/pkg/utils/type"
	"io"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

func (s *MarketingFlowService) sendActionSendSms(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if smsSenderFunc == nil {
		return nil, errors.New("短信发送器未注册，请确认 internal/service 包已调用 SetSmsSender")
	}

	// 提取手机号
	phone, _ := config["phone"].(string)
	if phone == "" {
		// 从 data 中回退取值
		if p, ok := data["phone"].(string); ok {
			phone = p
		} else if p, ok := data["user_phone"].(string); ok {
			phone = p
		}
	}
	if phone == "" {
		return nil, errors.New("phone 未指定")
	}

	// 提取短信内容
	content, _ := config["content"].(string)
	if content == "" {
		return nil, errors.New("content 未指定")
	}

	// 调用注入的 SMS 发送实现
	if err := smsSenderFunc(phone, content); err != nil {
		return nil, fmt.Errorf("发送短信失败：%w", err)
	}

	return map[string]any{
		"success": true,
		"phone":   phone,
		"content": content,
		"user_id": userID,
	}, nil
}

// sendActionUpdateLead 更新线索动作
// 配置参数：
//   - clue_id: string 线索 ID（必填，若为空则尝试从 data 中取 clue_id）
//   - fields: map[string]interface{} 需要更新的字段及其值
//
// 支持更新的字段：name / city / address / desc / is_verify / type / source_id / account
func (s *MarketingFlowService) sendActionUpdateLead(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.clueRepo == nil {
		return nil, errors.New("线索仓库未初始化")
	}

	// 提取线索 ID
	clueID, _ := config["clue_id"].(string)
	if clueID == "" {
		if id, ok := data["clue_id"].(string); ok {
			clueID = id
		} else if id, ok := data["lead_id"].(string); ok {
			clueID = id
		}
	}
	if clueID == "" {
		return nil, errors.New("clue_id 未指定")
	}

	// 提取待更新字段
	fieldsRaw, ok := config["fields"].(map[string]any)
	if !ok || len(fieldsRaw) == 0 {
		return nil, errors.New("fields 配置格式错误或为空")
	}

	// 白名单过滤，仅允许更新 Clue 模型中存在的字段
	allowedFields := map[string]bool{
		"name":      true,
		"city":      true,
		"address":   true,
		"desc":      true,
		"is_verify": true,
		"type":      true,
		"source_id": true,
		"account":   true,
	}
	updates := make(map[string]any)
	for k, v := range fieldsRaw {
		if !allowedFields[k] {
			continue
		}
		// 类型修正：is_verify 与 type 在 model 中为 int64
		if k == "is_verify" || k == "type" {
			switch vv := v.(type) {
			case float64:
				updates[k] = int64(vv)
			case int:
				updates[k] = int64(vv)
			case int64:
				updates[k] = vv
			case string:
				if n, err := strconv.ParseInt(vv, 10, 64); err == nil {
					updates[k] = n
				}
			default:
				return nil, fmt.Errorf("字段 %s 的值类型不合法", k)
			}
			continue
		}
		updates[k] = v
	}

	if len(updates) == 0 {
		return nil, errors.New("没有有效的可更新字段（请检查字段名是否在白名单内）")
	}

	// 调用仓库更新
	if err := s.clueRepo.UpdateByID(ctx, clueID, updates); err != nil {
		return nil, fmt.Errorf("更新线索失败：%w", err)
	}

	return map[string]any{
		"success": true,
		"clue_id": clueID,
		"updates": updates,
		"user_id": userID,
	}, nil
}

// sendActionCreateOrder 创建订单动作
// 配置参数：
//   - price: string 订单金额（必填）
//   - tg_id: float64 Telegram 用户 ID（必填，作为业务侧用户标识）
//   - account_id: string 账号 ID（必填）
//   - status: float64/string 初始订单状态（可选，默认 0=待支付）
func (s *MarketingFlowService) sendActionCreateOrder(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	if s.orderRepo == nil {
		return nil, errors.New("订单仓库未初始化")
	}

	// 提取金额
	price, _ := config["price"].(string)
	if price == "" {
		// 兼容 float64 形式
		if p, ok := config["price"].(float64); ok {
			price = strconv.FormatFloat(p, 'f', -1, 64)
		}
	}
	if price == "" {
		return nil, errors.New("price 未指定")
	}

	// 提取 Telegram 用户 ID
	var tgID int64
	switch v := config["tg_id"].(type) {
	case float64:
		tgID = int64(v)
	case string:
		if v != "" {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("tg_id 格式无效：%w", err)
			}
			tgID = id
		}
	}
	if tgID == 0 {
		// 从 data 中回退取值
		if v, ok := data["tg_id"].(float64); ok {
			tgID = int64(v)
		} else if v, ok := data["tg_id"].(string); ok && v != "" {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				tgID = id
			}
		}
	}
	if tgID == 0 {
		return nil, errors.New("tg_id 未指定")
	}

	// 提取账号 ID
	accountID, _ := config["account_id"].(string)
	if accountID == "" {
		if v, ok := data["account_id"].(string); ok {
			accountID = v
		}
	}
	if accountID == "" {
		return nil, errors.New("account_id 未指定")
	}

	// 解析订单状态（默认 0=待支付）
	var statusInt int
	switch v := config["status"].(type) {
	case float64:
		statusInt = int(v)
	case string:
		if v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				statusInt = n
			}
		}
	}

	// 构建订单（ID 在 BeforeCreate 钩子中自动生成）
	order := &reachmodel.Order{
		Price:     price,
		TgID:      tgID,
		AccountID: accountID,
		Status:    _type.OrderStatusType(statusInt),
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, fmt.Errorf("创建订单失败：%w", err)
	}

	return map[string]any{
		"success":    true,
		"order_id":   order.ID,
		"price":      price,
		"tg_id":      tgID,
		"account_id": accountID,
		"status":     int(order.Status),
		"user_id":    userID,
	}, nil
}

// firstNonEmpty 返回第一个非空字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// sendActionWebhook Webhook 动作
func (s *MarketingFlowService) sendActionWebhook(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return nil, errors.New("webhook URL 未指定")
	}

	// 设置默认方法为 POST
	method, _ := config["method"].(string)
	if method == "" {
		method = "POST"
	}

	// 构建请求体
	var bodyReader io.Reader
	if data != nil && len(data) > 0 {
		body, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("JSON 序列化失败：%w", err)
		}
		bodyReader = bytes.NewBuffer(body)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败：%w", err)
	}

	// 设置默认请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Marketing-Flow-Webhook/1.0")

	// 添加自定义请求头
	if headers, ok := config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if strVal, ok := v.(string); ok {
				req.Header.Set(k, strVal)
			}
		}
	}

	// 添加流程上下文信息
	req.Header.Set("X-Flow-User-ID", userID)

	// 执行请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送 Webhook 请求失败：%w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败：%w", err)
	}

	// 检查状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Webhook 返回错误状态码：%d, 响应：%s", resp.StatusCode, string(respBody))
	}

	// 尝试解析 JSON 响应
	var result map[string]any
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			// 非 JSON 响应，返回原始内容
			return map[string]any{
				"status_code": resp.StatusCode,
				"body":        string(respBody),
			}, nil
		}
		result["status_code"] = resp.StatusCode
		return result, nil
	}

	return map[string]any{
		"status_code": resp.StatusCode,
	}, nil
}

// sendActionSendEmail 发送邮件动作
func (s *MarketingFlowService) sendActionSendEmail(ctx context.Context, config map[string]any, userID string, data map[string]any) (map[string]any, error) {
	// 获取 SMTP 配置
	smtpHost, _ := config["smtp_host"].(string)
	smtpUser, _ := config["smtp_user"].(string)
	smtpPass, _ := config["smtp_pass"].(string)

	// 验证 SMTP 配置
	if smtpHost == "" {
		return nil, errors.New("SMTP 主机未配置，请在动作配置或环境变量中设置 smtp_host")
	}
	if smtpUser == "" || smtpPass == "" {
		return nil, errors.New("SMTP 用户名或密码未配置")
	}

	// 解析 SMTP 端口（支持 int 和 float64 类型）
	var smtpPort int
	switch v := config["smtp_port"].(type) {
	case int:
		smtpPort = v
	case float64:
		smtpPort = int(v)
	default:
		smtpPort = 587
	}

	// 获取邮件参数
	to, ok := config["to"].(string)
	if !ok || to == "" {
		return nil, errors.New("收件人邮箱未指定")
	}

	subject, _ := config["subject"].(string)
	if subject == "" {
		subject = "营销邮件"
	}

	body, _ := config["body"].(string)
	if body == "" {
		return nil, errors.New("邮件正文未指定")
	}

	from, _ := config["from"].(string)
	if from == "" {
		from = smtpUser
	}

	// 构建邮件内容
	mime := "MIME-version: 1.0\r\n"
	contentType := "Content-Type: text/html; charset=\"UTF-8\"\r\n"
	if isHTML, ok := config["is_html"].(bool); ok && !isHTML {
		contentType = "Content-Type: text/plain; charset=\"UTF-8\"\r\n"
	}

	// 修复：邮件头字段 from/to/subject 去除 CR/LF，防止 CRLF 注入导致邮件头或收件人被注入
	mailHeader := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n%s%s\r\n",
		strings.NewReplacer("\r", "", "\n", "").Replace(from),
		strings.NewReplacer("\r", "", "\n", "").Replace(to),
		strings.NewReplacer("\r", "", "\n", "").Replace(subject),
		mime, contentType)
	message := mailHeader + body

	// 发送邮件
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(message))
	if err != nil {
		return nil, fmt.Errorf("发送邮件失败：%w", err)
	}

	return map[string]any{
		"sent": true,
		"to":   to,
	}, nil
}

// evaluateCondition 评估条件
func (s *MarketingFlowService) evaluateCondition(node model.FlowNode, data map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	for k, v := range data {
		result[k] = v
	}

	// 获取条件表达式
	conditionRaw, ok := node.Config["condition"].(string)
	if !ok || conditionRaw == "" {
		// 空条件视为始终匹配，返回 true
		result["_condition_matched"] = true
		if len(node.NextNodes) > 0 {
			result["_next_node"] = node.NextNodes[0]
		}
		return result, nil
	}

	// 解析条件表达式："field operator value"
	field, operator, value, err := parseCondition(conditionRaw)
	if err != nil {
		return nil, err
	}

	// 获取字段值
	fieldValue, exists := data[field]
	if !exists {
		// 字段不存在，返回 false，不报错
		result["_condition_matched"] = false
		if len(node.NextNodes) > 1 {
			result["_next_node"] = node.NextNodes[1]
		} else if len(node.NextNodes) == 1 {
			result["_next_node"] = node.NextNodes[0]
		}
		return result, nil
	}

	// 评估条件
	matched, err := evaluateOperator(fieldValue, operator, value)
	if err != nil {
		return nil, err
	}

	result["_condition_matched"] = matched

	// 根据条件结果选择下一个节点
	if matched {
		if len(node.NextNodes) > 0 {
			result["_next_node"] = node.NextNodes[0]
		}
	} else {
		if len(node.NextNodes) > 1 {
			result["_next_node"] = node.NextNodes[1]
		} else if len(node.NextNodes) == 1 {
			result["_next_node"] = node.NextNodes[0]
		}
	}

	return result, nil
}

// parseCondition 解析条件表达式 "field operator value"
func parseCondition(condition string) (field, operator, value string, err error) {
	condition = strings.TrimSpace(condition)

	// 支持的运算符列表（按长度降序排列，优先匹配长的运算符）
	operators := []string{"contains", "gte", "lte", "eq", "ne", "gt", "lt", "in"}

	for _, op := range operators {
		// 查找运算符位置
		idx := strings.Index(condition, " "+op+" ")
		if idx != -1 {
			field = strings.TrimSpace(condition[:idx])
			rest := strings.TrimSpace(condition[idx+len(op)+2:])
			return field, op, rest, nil
		}
	}

	return "", "", "", errors.New("无效的条件表达式：未识别的运算符")
}

// ParseCondition 公开版 parseCondition(供跨包调用,如 service/sop_condition.go)
func ParseCondition(condition string) (field, operator, value string, err error) {
	return parseCondition(condition)
}

// evaluateOperator 评估运算符
func evaluateOperator(fieldValue any, operator, value string) (bool, error) {
	switch operator {
	case "eq":
		return evalEq(fieldValue, value)
	case "ne":
		return evalNe(fieldValue, value)
	case "gt":
		return evalGt(fieldValue, value)
	case "lt":
		return evalLt(fieldValue, value)
	case "gte":
		return evalGte(fieldValue, value)
	case "lte":
		return evalLte(fieldValue, value)
	case "contains":
		return evalContains(fieldValue, value)
	case "in":
		return evalIn(fieldValue, value)
	default:
		return false, fmt.Errorf("不支持的运算符：%s", operator)
	}
}

// EvaluateOperator 公开版 evaluateOperator(供跨包调用,如 service/sop_condition.go)
func EvaluateOperator(fieldValue any, operator, value string) (bool, error) {
	return evaluateOperator(fieldValue, operator, value)
}

// evalEq 等于比较
func evalEq(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case string:
		return strings.EqualFold(fv, value), nil
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv == numValue, nil
	default:
		return strings.EqualFold(fmt.Sprintf("%v", fv), value), nil
	}
}

// evalNe 不等于比较
func evalNe(fieldValue any, value string) (bool, error) {
	result, err := evalEq(fieldValue, value)
	if err != nil {
		return false, err
	}
	return !result, nil
}

// evalGt 大于比较
func evalGt(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv > numValue, nil
	case int:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return float64(fv) > numValue, nil
	default:
		return false, errors.New("gt 运算符仅支持数字类型")
	}
}

// evalLt 小于比较
func evalLt(fieldValue any, value string) (bool, error) {
	switch fv := fieldValue.(type) {
	case float64:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return fv < numValue, nil
	case int:
		numValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, fmt.Errorf("数字比较时值解析失败：%v", err)
		}
		return float64(fv) < numValue, nil
	default:
		return false, errors.New("lt 运算符仅支持数字类型")
	}
}

// evalGte 大于等于比较
func evalGte(fieldValue any, value string) (bool, error) {
	result, err := evalGt(fieldValue, value)
	if err != nil {
		return false, err
	}
	if result {
		return true, nil
	}
	// 检查是否等于
	return evalEq(fieldValue, value)
}

// evalLte 小于等于比较
func evalLte(fieldValue any, value string) (bool, error) {
	result, err := evalLt(fieldValue, value)
	if err != nil {
		return false, err
	}
	if result {
		return true, nil
	}
	// 检查是否等于
	return evalEq(fieldValue, value)
}

// evalContains 包含比较（大小写不敏感）
func evalContains(fieldValue any, value string) (bool, error) {
	strValue := fmt.Sprintf("%v", fieldValue)
	return strings.Contains(strings.ToLower(strValue), strings.ToLower(value)), nil
}

// evalIn 列表成员比较
func evalIn(fieldValue any, value string) (bool, error) {
	// 解析列表 [a,b,c]
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return false, errors.New("in 运算符的列表格式错误，应该为 [a,b,c]")
	}

	// 去掉方括号
	listStr := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	items := strings.Split(listStr, ",")

	for _, item := range items {
		item = strings.TrimSpace(item)
		matched, err := evalEq(fieldValue, item)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}

	return false, nil
}

// handleDelay 处理延迟
func (s *MarketingFlowService) handleDelay(ctx context.Context, node model.FlowNode) (map[string]any, error) {
	duration, _ := node.Config["duration"].(float64)
	if duration > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(duration) * time.Second):
		}
	}
	return nil, nil
}

// GetActiveFlows 获取所有激活的流程
func (s *MarketingFlowService) GetActiveFlows() ([]*model.MarketingFlow, error) {
	return s.flowRepo.GetByStatus(model.FlowStatusActive)
}

// EvaluateCondition 公开版 evaluateCondition(供跨包测试使用)
func (s *MarketingFlowService) EvaluateCondition(node model.FlowNode, data map[string]any) (map[string]any, error) {
	return s.evaluateCondition(node, data)
}
