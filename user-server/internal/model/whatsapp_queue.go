package model

import "time"

type QueuedMessage struct {
	ID          string     `json:"id"`
	LeadID      string     `json:"lead_id"`
	PhoneNumber string     `json:"phone_number"`
	Content     string     `json:"content"`
	TemplateID  string     `json:"template_id"`
	ScheduleAt  *time.Time `json:"schedule_at"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}

type QueueStatus struct {
	QueueID string    `json:"queue_id"`
	Total   int       `json:"total"`
	Sent    int       `json:"sent"`
	Failed  int       `json:"failed"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

