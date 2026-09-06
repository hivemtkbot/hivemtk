package model

import (
	"testing"

	"github.com/google/uuid"
)

func TestWhatsappAccount_TableName(t *testing.T) {
	account := &WhatsappAccount{}
	tableName := account.TableName()
	if tableName != "whatsapp_accounts" {
		t.Errorf("Expected table name 'whatsapp_accounts', got %s", tableName)
	}
}

func TestWhatsappJob_TableName(t *testing.T) {
	job := &WhatsappJob{}
	tableName := job.TableName()
	if tableName != "whatsapp_jobs" {
		t.Errorf("Expected table name 'whatsapp_jobs', got %s", tableName)
	}
}

func TestWhatsappJobDetail_TableName(t *testing.T) {
	detail := &WhatsappJobDetail{}
	tableName := detail.TableName()
	if tableName != "whatsapp_job_details" {
		t.Errorf("Expected table name 'whatsapp_job_details', got %s", tableName)
	}
}

func TestWhatsappSession_TableName(t *testing.T) {
	session := &WhatsappSession{}
	tableName := session.TableName()
	if tableName != "whatsapp_sessions" {
		t.Errorf("Expected table name 'whatsapp_sessions', got %s", tableName)
	}
}

func TestWhatsappDraft_TableName(t *testing.T) {
	draft := &WhatsappDraft{}
	tableName := draft.TableName()
	if tableName != "whatsapp_drafts" {
		t.Errorf("Expected table name 'whatsapp_drafts', got %s", tableName)
	}
}

func TestWhatsappAccount_BeforeCreate_GeneratesID(t *testing.T) {
	account := &WhatsappAccount{
		Name: "Test Account",
	}

	err := account.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if account.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(account.ID.String()) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(account.ID.String()))
	}
}

func TestWhatsappAccount_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := uuid.New()
	account := &WhatsappAccount{
		ID:   existingID,
		Name: "Test Account",
	}

	err := account.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if account.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, account.ID)
	}
}

func TestWhatsappAccount_BasicFields(t *testing.T) {
	id := uuid.New()

	account := &WhatsappAccount{
		ID:     id,
		Name:   "WhatsApp Account 1",
		Remark: "Test account",
		Status: WhatsappStatusOnline,
	}

	if account.ID != id {
		t.Errorf("Expected ID %s, got %s", id, account.ID)
	}
	if account.Name != "WhatsApp Account 1" {
		t.Errorf("Expected Name 'WhatsApp Account 1', got %s", account.Name)
	}
	if account.Status != WhatsappStatusOnline {
		t.Errorf("Expected Status 'online', got %s", account.Status)
	}
}

func TestWhatsappAccount_StatusValues(t *testing.T) {
	statuses := []WhatsappAccountStatus{
		WhatsappStatusPending,
		WhatsappStatusOnline,
		WhatsappStatusOffline,
	}

	for _, status := range statuses {
		account := &WhatsappAccount{
			Status: status,
		}
		if account.Status != status {
			t.Errorf("Expected Status %s, got %s", status, account.Status)
		}
	}
}

func TestWhatsappJob_BeforeCreate_GeneratesID(t *testing.T) {
	job := &WhatsappJob{}

	err := job.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if job.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(job.ID.String()) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(job.ID.String()))
	}
}

func TestWhatsappJob_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := uuid.New()
	job := &WhatsappJob{
		ID: existingID,
	}

	err := job.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if job.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, job.ID)
	}
}

func TestWhatsappJob_BasicFields(t *testing.T) {
	id := uuid.New()
	draftID := uuid.New()

	job := &WhatsappJob{
		ID:      id,
		DraftID: draftID,
		Status:  WhatsappJobPending,
		Total:   100,
		Success: 95,
		Failed:  5,
	}

	if job.ID != id {
		t.Errorf("Expected ID %s, got %s", id, job.ID)
	}
	if job.Total != 100 {
		t.Errorf("Expected Total 100, got %d", job.Total)
	}
	if job.Status != WhatsappJobPending {
		t.Errorf("Expected Status 'pending', got %s", job.Status)
	}
}

func TestWhatsappJob_StatusValues(t *testing.T) {
	statuses := []WhatsappJobStatus{
		WhatsappJobPending,
		WhatsappJobRunning,
		WhatsappJobFinished,
		WhatsappJobFailed,
	}

	for _, status := range statuses {
		job := &WhatsappJob{
			Status: status,
		}
		if job.Status != status {
			t.Errorf("Expected Status %s, got %s", status, job.Status)
		}
	}
}

func TestWhatsappJobDetail_BeforeCreate_GeneratesID(t *testing.T) {
	detail := &WhatsappJobDetail{
		ToJid: "1234567890@c.us",
	}

	err := detail.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if detail.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(detail.ID.String()) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(detail.ID.String()))
	}
}

func TestWhatsappJobDetail_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := uuid.New()
	detail := &WhatsappJobDetail{
		ID:    existingID,
		ToJid: "1234567890@c.us",
	}

	err := detail.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if detail.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, detail.ID)
	}
}

func TestWhatsappJobDetail_BasicFields(t *testing.T) {
	id := uuid.New()
	jobID := uuid.New()
	accountID := uuid.New()

	detail := &WhatsappJobDetail{
		ID:        id,
		JobID:     jobID,
		AccountID: accountID,
		ToJid:     "1234567890@c.us",
		Status:    WhatsappJobDetailSuccess,
		ErrorMsg:  "",
	}

	if detail.ID != id {
		t.Errorf("Expected ID %s, got %s", id, detail.ID)
	}
	if detail.ToJid != "1234567890@c.us" {
		t.Errorf("Expected ToJid '1234567890@c.us', got %s", detail.ToJid)
	}
	if detail.Status != WhatsappJobDetailSuccess {
		t.Errorf("Expected Status 'success', got %s", detail.Status)
	}
}

func TestWhatsappJobDetail_StatusValues(t *testing.T) {
	statuses := []WhatsappJobDetailStatus{
		WhatsappJobDetailPending,
		WhatsappJobDetailSuccess,
		WhatsappJobDetailFailed,
	}

	for _, status := range statuses {
		detail := &WhatsappJobDetail{
			Status: status,
		}
		if detail.Status != status {
			t.Errorf("Expected Status %s, got %s", status, detail.Status)
		}
	}
}

func TestWhatsappSession_BeforeCreate_GeneratesID(t *testing.T) {
	session := &WhatsappSession{
		SessionJSON: `{"session": "data"}`,
	}

	err := session.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if session.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(session.ID.String()) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(session.ID.String()))
	}
}

func TestWhatsappSession_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := uuid.New()
	session := &WhatsappSession{
		ID:          existingID,
		SessionJSON: `{"session": "data"}`,
	}

	err := session.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if session.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, session.ID)
	}
}

func TestWhatsappSession_BasicFields(t *testing.T) {
	id := uuid.New()
	accountID := uuid.New()

	session := &WhatsappSession{
		ID:          id,
		AccountID:   accountID.String(),
		SessionJSON: `{"session": "data"}`,
	}

	if session.ID != id {
		t.Errorf("Expected ID %s, got %s", id, session.ID)
	}
	if session.SessionJSON != `{"session": "data"}` {
		t.Errorf("Expected SessionJSON, got %s", session.SessionJSON)
	}
}

func TestWhatsappDraft_BeforeCreate_GeneratesID(t *testing.T) {
	draft := &WhatsappDraft{
		Title: "Test Draft",
	}

	err := draft.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if draft.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(draft.ID.String()) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(draft.ID.String()))
	}
}

func TestWhatsappDraft_BeforeCreate_NoChangeIfExists(t *testing.T) {
	existingID := uuid.New()
	draft := &WhatsappDraft{
		ID:    existingID,
		Title: "Test Draft",
	}

	err := draft.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if draft.ID != existingID {
		t.Errorf("Expected ID to remain %s, got %s", existingID, draft.ID)
	}
}

func TestWhatsappDraft_BasicFields(t *testing.T) {
	id := uuid.New()

	draft := &WhatsappDraft{
		ID:      id,
		Title:   "Draft Title",
		Content: "Draft content here",
	}

	if draft.ID != id {
		t.Errorf("Expected ID %s, got %s", id, draft.ID)
	}
	if draft.Title != "Draft Title" {
		t.Errorf("Expected Title 'Draft Title', got %s", draft.Title)
	}
	if draft.Content != "Draft content here" {
		t.Errorf("Expected Content 'Draft content here', got %s", draft.Content)
	}
}
