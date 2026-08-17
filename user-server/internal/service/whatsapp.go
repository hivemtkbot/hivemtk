package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hivemtk-user/internal/channelbot/core"
	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/repository"
	"strings"
	"sync"
	"time"

	gowa "github.com/Rhymen/go-whatsapp"
	"github.com/google/uuid"
)

type WhatsappService struct {
	repo     repository.WhatsappRepository
	clueRepo repository.ClueRepository
	connMu   sync.RWMutex
	conns    map[uuid.UUID]*gowa.Conn
	qrMu     sync.RWMutex
	qrs      map[uuid.UUID]string
}

// NewWhatsappService 创建 WhatsappService。
// 修复：原构造函数接收 repo 参数导致 controller 越层 new repository，
// 现改为内部构造 repo，保持五层架构（Controller → Service → Repository）。
func NewWhatsappService() *WhatsappService {
	return &WhatsappService{
		repo:     repository.NewWhatsappRepository(),
		clueRepo: repository.NewClueRepository(),
		conns:    make(map[uuid.UUID]*gowa.Conn),
		qrs:      make(map[uuid.UUID]string),
	}
}

// Accounts
func (s *WhatsappService) CreateAccount(ctx context.Context, name, remark string) (*model.WhatsappAccount, error) {
	acc := &model.WhatsappAccount{Name: name, Remark: remark, Status: model.WhatsappStatusPending}
	return acc, s.repo.CreateAccount(ctx, acc)
}

func (s *WhatsappService) ListAccounts(ctx context.Context) ([]*model.WhatsappAccount, error) {
	return s.repo.ListAccounts(ctx)
}

func (s *WhatsappService) UpdateAccount(ctx context.Context, acc *model.WhatsappAccount) error {
	return s.repo.UpdateAccount(ctx, acc)
}

func (s *WhatsappService) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAccount(ctx, id)
}

func (s *WhatsappService) GetAccount(ctx context.Context, id uuid.UUID) (*model.WhatsappAccount, error) {
	return s.repo.GetAccount(ctx, id)
}

// Login
func (s *WhatsappService) StartLogin(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (string, error) {
	wac, err := gowa.NewConn(timeout)
	if err != nil {
		return "", err
	}
	s.connMu.Lock()
	s.conns[accountID] = wac
	s.connMu.Unlock()

	qrChan := make(chan string)
	utils.SafeGo(ctx, "whatsapp.StartLogin", func(ctx context.Context) {
		sess, err := wac.Login(qrChan)
		if err != nil {
			return
		}
		b, _ := json.Marshal(sess)
		_ = s.repo.UpsertSession(ctx, &model.WhatsappSession{AccountID: accountID.String(), SessionJSON: string(b)})
		acc, _ := s.repo.GetAccount(ctx, accountID)
		if acc != nil {
			acc.Status = model.WhatsappStatusOnline
			_ = s.repo.UpdateAccount(ctx, acc)
		}
	})

	qr := <-qrChan
	s.qrMu.Lock()
	s.qrs[accountID] = qr
	s.qrMu.Unlock()
	return qr, nil
}

func (s *WhatsappService) GetLoginQR(ctx context.Context, accountID uuid.UUID) (string, bool) {
	s.qrMu.RLock()
	defer s.qrMu.RUnlock()
	qr, ok := s.qrs[accountID]
	return qr, ok
}

func (s *WhatsappService) LoginStatus(ctx context.Context, accountID uuid.UUID) (bool, error) {
	sess, err := s.repo.GetSession(ctx, accountID)
	if err != nil {
		return false, err
	}
	return sess != nil, nil
}

func (s *WhatsappService) ensureConn(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (*gowa.Conn, error) {
	s.connMu.RLock()
	c := s.conns[accountID]
	s.connMu.RUnlock()
	if c != nil {
		return c, nil
	}
	sess, err := s.repo.GetSession(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, errors.New("no session for account")
	}
	wac, err := gowa.NewConn(timeout)
	if err != nil {
		return nil, err
	}
	var ws gowa.Session
	if err := json.Unmarshal([]byte(sess.SessionJSON), &ws); err != nil {
		return nil, err
	}
	_, err = wac.RestoreWithSession(ws)
	if err != nil {
		return nil, err
	}
	s.connMu.Lock()
	s.conns[accountID] = wac
	s.connMu.Unlock()
	return wac, nil
}

// SendTextMessage 发送文本消息到指定 JID
// 兼容个人版 (go-whatsapp) 与 Cloud API 两种账号
func (s *WhatsappService) SendTextMessage(ctx context.Context, accountID uuid.UUID, toJID, text string) (string, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", errors.New("whatsapp account not found")
	}

	// Cloud API 账号：使用 Meta Graph API 发送（24h 客服窗口内免费）
	if acc.Type == "cloud" && acc.PhoneID != "" && acc.Token != "" {
		phone := toJID
		if strings.Contains(phone, "@") {
			phone = strings.Split(phone, "@")[0]
		}
		if !strings.HasPrefix(phone, "+") {
			phone = "+" + phone
		}
		cli := whatsapp.NewCloudClient(acc.PhoneID, acc.Token)
		return cli.SendText(ctx, phone, text)
	}

	// 个人版账号：使用 go-whatsapp 库
	wac, err := s.ensureConn(ctx, accountID, 30*time.Second)
	if err != nil {
		return "", err
	}
	msg := gowa.TextMessage{Info: gowa.MessageInfo{RemoteJid: toJID}, Text: text}
	resp, err := wac.Send(msg)
	if err != nil {
		return "", err
	}
	return resp, nil
}

// SendTemplateMessage 发送模板消息（客服窗口外必须走模板）
func (s *WhatsappService) SendTemplateMessage(ctx context.Context, accountID uuid.UUID, to, templateName, language string, params map[string]string) (string, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acc == nil {
		return "", errors.New("whatsapp account not found")
	}
	if acc.PhoneID == "" || acc.Token == "" {
		return "", errors.New("cloud api credentials missing")
	}
	phone := to
	if strings.Contains(phone, "@") {
		phone = strings.Split(phone, "@")[0]
	}
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}

	var components []core.TemplateComponent
	if len(params) > 0 {
		var bodyParams []core.TemplateParameter
		for _, v := range params {
			bodyParams = append(bodyParams, core.TemplateParameter{Type: "text", Text: v})
		}
		components = append(components, core.TemplateComponent{
			Type:       "body",
			Parameters: bodyParams,
		})
	}

	cli := whatsapp.NewCloudClient(acc.PhoneID, acc.Token)
	return cli.SendTemplate(ctx, phone, templateName, language, components)
}

// Drafts
func (s *WhatsappService) CreateDraft(ctx context.Context, title, content string) (*model.WhatsappDraft, error) {
	d := &model.WhatsappDraft{Title: title, Content: content}
	return d, s.repo.CreateDraft(ctx, d)
}
func (s *WhatsappService) ListDrafts(ctx context.Context) ([]*model.WhatsappDraft, error) {
	return s.repo.ListDrafts(ctx)
}

func (s *WhatsappService) UpdateDraft(ctx context.Context, d *model.WhatsappDraft) error {
	return s.repo.UpdateDraft(ctx, d)
}

func (s *WhatsappService) DeleteDraft(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDraft(ctx, id)
}

func (s *WhatsappService) GetDraft(ctx context.Context, id uuid.UUID) (*model.WhatsappDraft, error) {
	return s.repo.GetDraft(ctx, id)
}

// Bulk send
// Note: ClueTypeWhatsapp is defined in clue.go (value=5). Legacy type=7 is also queried for backward compat.

func (s *WhatsappService) CreateBulkJob(ctx context.Context, draftID uuid.UUID) (*model.WhatsappJob, error) {
	draft, err := s.repo.GetDraft(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, errors.New("draft not found")
	}
	job := &model.WhatsappJob{DraftID: draft.ID, Status: model.WhatsappJobPending}
	if err := s.repo.CreateJob(ctx, job); err != nil {
		return nil, err
	}

	// 查询 WhatsApp 线索，兼容历史 type=7 和标准 type=5
	clues, total, err := s.clueRepo.GetWhatsappClues(ctx)
	if err != nil {
		return nil, err
	}
	job.Total = total
	_ = s.repo.UpdateJob(ctx, job)

	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, errors.New("no accounts available")
	}

	utils.SafeGo(ctx, "whatsapp.SendBulk", func(ctx context.Context) {
		job.Status = model.WhatsappJobRunning
		_ = s.repo.UpdateJob(ctx, job)
		idx := 0
		for _, clue := range clues {
			acc := accounts[idx%len(accounts)]
			idx++
			toJid := fmt.Sprintf("%s@s.whatsapp.net", clue.Account)
			detail := &model.WhatsappJobDetail{JobID: job.ID, AccountID: acc.ID, ToJid: toJid}
			_ = s.repo.CreateJobDetail(ctx, detail)
			wac, err := s.ensureConn(ctx, acc.ID, 30*time.Second)
			if err != nil {
				detail.Status = model.WhatsappJobDetailFailed
				detail.ErrorMsg = err.Error()
				_ = s.repo.UpdateJobDetail(ctx, detail)
				job.Failed++
				_ = s.repo.UpdateJob(ctx, job)
				continue
			}
			msg := gowa.TextMessage{Info: gowa.MessageInfo{RemoteJid: toJid}, Text: draft.Content}
			_, err = wac.Send(msg)
			if err != nil {
				detail.Status = model.WhatsappJobDetailFailed
				detail.ErrorMsg = err.Error()
				_ = s.repo.UpdateJobDetail(ctx, detail)
				job.Failed++
				_ = s.repo.UpdateJob(ctx, job)
				continue
			}
			detail.Status = model.WhatsappJobDetailSuccess
			_ = s.repo.UpdateJobDetail(ctx, detail)
			job.Success++
			_ = s.repo.UpdateJob(ctx, job)
			time.Sleep(500 * time.Millisecond)
		}
		job.Status = model.WhatsappJobFinished
		_ = s.repo.UpdateJob(ctx, job)
	})

	return job, nil
}

// ListJobs lists all bulk send jobs
func (s *WhatsappService) ListJobs(ctx context.Context) ([]*model.WhatsappJob, error) {
	return s.repo.ListJobs(ctx)
}

// GetJob returns a single bulk send job by ID
func (s *WhatsappService) GetJob(ctx context.Context, id uuid.UUID) (*model.WhatsappJob, error) {
	return s.repo.GetJob(ctx, id)
}

// DeleteJob deletes a bulk send job by ID
func (s *WhatsappService) DeleteJob(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteJob(ctx, id)
}

