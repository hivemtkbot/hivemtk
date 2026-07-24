package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	whatsapp "github.com/Rhymen/go-whatsapp"
	"github.com/google/uuid"
	"marketing/internal/model"
	"marketing/internal/repository"
	"sync"
	"time"
)

type WhatsappService struct {
	repo     repository.WhatsappRepository
	clueRepo repository.ClueRepository
	connMu   sync.RWMutex
	conns    map[uuid.UUID]*whatsapp.Conn
	qrMu     sync.RWMutex
	qrs      map[uuid.UUID]string
}

// NewWhatsappService 创建 WhatsappService。
// P2-2 修复：原构造函数接收 repo 参数导致 controller 越层 new repository，
// 现改为内部构造 repo，保持五层架构（Controller → Service → Repository）。
func NewWhatsappService() *WhatsappService {
	return &WhatsappService{
		repo:     repository.NewWhatsappRepository(),
		clueRepo: repository.NewClueRepository(),
		conns:    make(map[uuid.UUID]*whatsapp.Conn),
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
	// create connection
	wac, err := whatsapp.NewConn(timeout)
	if err != nil {
		return "", err
	}
	s.connMu.Lock()
	s.conns[accountID] = wac
	s.connMu.Unlock()

	qrChan := make(chan string)
	go func() {
		sess, err := wac.Login(qrChan)
		if err != nil {
			return
		}
		// persist session
		b, _ := json.Marshal(sess)
		_ = s.repo.UpsertSession(ctx, &model.WhatsappSession{AccountID: accountID.String(), SessionJSON: string(b)})
		// update account status
		acc, _ := s.repo.GetAccount(ctx, accountID)
		if acc != nil {
			acc.Status = model.WhatsappStatusOnline
			_ = s.repo.UpdateAccount(ctx, acc)
		}
	}()

	// capture first QR code string
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

func (s *WhatsappService) ensureConn(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (*whatsapp.Conn, error) {
	s.connMu.RLock()
	c := s.conns[accountID]
	s.connMu.RUnlock()
	if c != nil {
		return c, nil
	}
	// try restore
	sess, err := s.repo.GetSession(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, errors.New("no session for account")
	}
	wac, err := whatsapp.NewConn(timeout)
	if err != nil {
		return nil, err
	}
	var ws whatsapp.Session
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
func (s *WhatsappService) SendTextMessage(ctx context.Context, accountID uuid.UUID, toJID, text string) (string, error) {
	wac, err := s.ensureConn(ctx, accountID, 30*time.Second)
	if err != nil {
		return "", err
	}
	msg := whatsapp.TextMessage{Info: whatsapp.MessageInfo{RemoteJid: toJID}, Text: text}
	resp, err := wac.Send(msg)
	if err != nil {
		return "", err
	}
	// go-whatsapp 库返回 msgID 字符串,空字符串表示发送未返回 ID
	return resp, nil
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
const ClueTypeWhatsapp int64 = 7

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

	// load contacts
	clues, total, err := s.clueRepo.GetClueAllList(ctx, ClueTypeWhatsapp)
	if err != nil {
		return nil, err
	}
	job.Total = total
	_ = s.repo.UpdateJob(ctx, job)

	// round-robin accounts
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, errors.New("no accounts available")
	}

	// start sending in background
	go func() {
		job.Status = model.WhatsappJobRunning
		_ = s.repo.UpdateJob(ctx, job)
		idx := 0
		for _, clue := range clues {
			acc := accounts[idx%len(accounts)]
			idx++
			toJid := fmt.Sprintf("%s@s.whatsapp.net", clue.Account)
			detail := &model.WhatsappJobDetail{JobID: job.ID, AccountID: acc.ID, ToJid: toJid}
			_ = s.repo.CreateJobDetail(ctx, detail)
			// ensure conn
			wac, err := s.ensureConn(ctx, acc.ID, 30*time.Second)
			if err != nil {
				detail.Status = model.WhatsappJobDetailFailed
				detail.ErrorMsg = err.Error()
				_ = s.repo.UpdateJobDetail(ctx, detail)
				job.Failed++
				_ = s.repo.UpdateJob(ctx, job)
				continue
			}
			// send
			msg := whatsapp.TextMessage{Info: whatsapp.MessageInfo{RemoteJid: toJid}, Text: draft.Content}
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
	}()

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
