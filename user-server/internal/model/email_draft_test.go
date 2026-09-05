package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEmailJobs_TableName(t *testing.T) {
	job := &EmailJobs{}
	tableName := job.TableName()
	if tableName != "email_jobs" {
		t.Errorf("Expected table name 'email_jobs', got %s", tableName)
	}
}

func TestEmailList_TableName(t *testing.T) {
	emailList := &EmailList{}
	tableName := emailList.TableName()
	if tableName != "email_list" {
		t.Errorf("Expected table name 'email_list', got %s", tableName)
	}
}

func TestEmailDraft_BeforeCreate(t *testing.T) {
	draft := &EmailDraft{
		Subject: "Test Draft",
		Content: "Test content",
	}

	if draft.ID.String() != "" {
		t.Logf("ID before BeforeCreate: %s", draft.ID.String())
	}

	err := draft.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if draft.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if draft.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt after BeforeCreate")
	}
	if draft.UpdatedAt.IsZero() {
		t.Error("Expected non-zero UpdatedAt after BeforeCreate")
	}
}

func TestEmailJobs_BeforeCreate_Full(t *testing.T) {
	job := &EmailJobs{
		Subject: "Test Job",
	}

	err := job.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if job.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if job.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt after BeforeCreate")
	}
	if job.UpdatedAt.IsZero() {
		t.Error("Expected non-zero UpdatedAt after BeforeCreate")
	}
}

func TestEmailList_BeforeCreate_Full(t *testing.T) {
	emailList := &EmailList{
		Subject: "Test",
		From:    "test@example.com",
		To:      "recipient@example.com",
	}

	err := emailList.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if emailList.ID.String() == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if emailList.CreatedAt.IsZero() {
		t.Error("Expected non-zero CreatedAt after BeforeCreate")
	}
}

func TestEmailSend_BeforeCreate_Full(t *testing.T) {
	email := &EmailSend{
		To:      "test@example.com",
		Subject: "Test",
	}

	err := email.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if email.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	if len(email.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(email.ID))
	}
}

func TestEmailSend_BeforeCreate_NoChangeIfExists(t *testing.T) {
	email := &EmailSend{
		ID:      "existing-id",
		To:      "test@example.com",
		Subject: "Test",
	}

	err := email.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if email.ID != "existing-id" {
		t.Errorf("Expected ID to remain 'existing-id', got %s", email.ID)
	}
}

func TestEmailDraft_BasicFields(t *testing.T) {
	id := uuid.New()
	now := time.Now()

	draft := &EmailDraft{
		ID:          id,
		Subject:     "Test Subject",
		Content:     "Test content",
		Attachments: `["file1.pdf"]`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if draft.ID != id {
		t.Errorf("Expected ID %s, got %s", id, draft.ID)
	}
	if draft.Subject != "Test Subject" {
		t.Errorf("Expected Subject 'Test Subject', got %s", draft.Subject)
	}
	if draft.Content != "Test content" {
		t.Errorf("Expected Content 'Test content', got %s", draft.Content)
	}
}

func TestEmailJobs_BeforeCreate(t *testing.T) {
	job := &EmailJobs{
		Subject: "Test Job",
	}

	if job.ID.String() != "" {
		t.Logf("ID before BeforeCreate: %s", job.ID.String())
	}
}

func TestEmailJobs_BasicFields(t *testing.T) {
	id := uuid.New()

	job := &EmailJobs{
		ID:           id,
		Subject:      "Batch Email",
		SendTotal:    100,
		EmailTotal:   100,
		SuccessTotal: 95,
		FailTotal:    5,
		ReadTotal:    50,
	}

	if job.ID != id {
		t.Errorf("Expected ID %s, got %s", id, job.ID)
	}
	if job.Subject != "Batch Email" {
		t.Errorf("Expected Subject 'Batch Email', got %s", job.Subject)
	}
	if job.SendTotal != 100 {
		t.Errorf("Expected SendTotal 100, got %d", job.SendTotal)
	}
	if job.SuccessTotal != 95 {
		t.Errorf("Expected SuccessTotal 95, got %d", job.SuccessTotal)
	}
}

func TestEmailJobs_DefaultValues(t *testing.T) {
	job := &EmailJobs{}

	if job.SendTotal != 0 {
		t.Logf("SendTotal is %d (expected 0 before save)", job.SendTotal)
	}
	if job.EmailTotal != 0 {
		t.Logf("EmailTotal is %d (expected 0 before save)", job.EmailTotal)
	}
}

func TestEmailSend_BeforeCreate(t *testing.T) {
	email := &EmailSend{
		ID:      "",
		To:      "test@example.com",
		Subject: "Test",
	}

	if email.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", email.ID)
	}
}

func TestEmailSend_BasicFields(t *testing.T) {
	now := time.Now()

	email := &EmailSend{
		ID:          uuid.New().String(),
		To:          "test@example.com",
		Subject:     "Test Subject",
		Content:     "Test content",
		Attachments: `["file1.pdf"]`,
		Status:      1,
		SendTime:    &now,
		SmtpID:      "smtp-001",
	}

	if email.To != "test@example.com" {
		t.Errorf("Expected To 'test@example.com', got %s", email.To)
	}
	if email.Subject != "Test Subject" {
		t.Errorf("Expected Subject 'Test Subject', got %s", email.Subject)
	}
	if email.Status != 1 {
		t.Errorf("Expected Status 1, got %d", email.Status)
	}
}

func TestEmailSend_StatusValues(t *testing.T) {
	statuses := map[int]string{
		0: "待发送",
		1: "已发送",
		2: "发送失败",
	}

	for status, desc := range statuses {
		email := &EmailSend{
			Status: status,
		}
		if email.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, desc, email.Status)
		}
	}
}

func TestEmailList_BeforeCreate(t *testing.T) {
	emailList := &EmailList{
		Subject: "Test",
		From:    "test@example.com",
		To:      "recipient@example.com",
	}

	if emailList.ID.String() != "" {
		t.Logf("ID before BeforeCreate: %s", emailList.ID.String())
	}
}

func TestEmailList_BasicFields(t *testing.T) {
	id := uuid.New()
	jobsID := uuid.New()
	traceID := uuid.New()

	emailList := &EmailList{
		ID:        id,
		Subject:   "Email List Subject",
		Content:   "Email content",
		From:      "sender@example.com",
		To:        "recipient@example.com",
		IsSend:    0,
		IsRead:    0,
		IsSuccess: 0,
		JobsID:    jobsID,
		TraceID:   traceID,
	}

	if emailList.ID != id {
		t.Errorf("Expected ID %s, got %s", id, emailList.ID)
	}
	if emailList.From != "sender@example.com" {
		t.Errorf("Expected From 'sender@example.com', got %s", emailList.From)
	}
	if emailList.To != "recipient@example.com" {
		t.Errorf("Expected To 'recipient@example.com', got %s", emailList.To)
	}
	if emailList.IsSend != 0 {
		t.Errorf("Expected IsSend 0, got %d", emailList.IsSend)
	}
}

func TestEmailList_IsSendValues(t *testing.T) {
	statuses := map[int]string{
		0: "未发送",
		1: "已发送",
	}

	for status, desc := range statuses {
		emailList := &EmailList{
			IsSend: status,
		}
		if emailList.IsSend != status {
			t.Errorf("Expected IsSend %d (%s), got %d", status, desc, emailList.IsSend)
		}
	}
}

func TestEmailList_IsReadValues(t *testing.T) {
	statuses := map[int]string{
		0: "未读",
		1: "已读",
	}

	for status, desc := range statuses {
		emailList := &EmailList{
			IsRead: status,
		}
		if emailList.IsRead != status {
			t.Errorf("Expected IsRead %d (%s), got %d", status, desc, emailList.IsRead)
		}
	}
}
