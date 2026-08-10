package repository

import (
	"context"
	"encoding/json"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupCustomerEventTestDB sets up the test database for customer event tests
func setupCustomerEventTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.CustomerEvent{},
	)
	db.SetTestDB(database)
	return database
}

// setupCustomerEventRepository creates a test customer event repository instance
func setupCustomerEventRepository(t *testing.T) CustomerEventRepository {
	setupCustomerEventTestDB(t)
	return NewCustomerEventRepository()
}

// TestCustomerEventRepository_Record tests recording events
func TestCustomerEventRepository_Record(t *testing.T) {
	repo := setupCustomerEventRepository(t)

	tests := []struct {
		name    string
		event   *model.CustomerEvent
		wantErr bool
	}{
		{
			name: "record page view event",
			event: &model.CustomerEvent{
				CustomerID:  "test-customer-1",
				EventType:   model.EventTypePageView,
				EventSource: model.EventSourceWebsite,
			},
			wantErr: false,
		},
		{
			name: "record purchase event",
			event: &model.CustomerEvent{
				CustomerID:  "test-customer-2",
				EventType:   model.EventTypePurchase,
				EventSource: model.EventSourceApp,
			},
			wantErr: false,
		},
		{
			name: "record click event with data",
			event: &model.CustomerEvent{
				CustomerID:  "test-customer-3",
				EventType:   model.EventTypeClick,
				EventSource: model.EventSourceWechat,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Record(context.Background(), tt.event)

			if (err != nil) != tt.wantErr {
				t.Errorf("Record() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.event.ID == "" {
				t.Error("Expected event ID to be set after recording")
			}

			if !tt.wantErr && tt.event.OccurredAt.IsZero() {
				t.Error("Expected OccurredAt to be set after recording")
			}
		})
	}
}

// TestCustomerEventRepository_GetByCustomerID tests retrieving events by customer ID
func TestCustomerEventRepository_GetByCustomerID(t *testing.T) {
	repo := setupCustomerEventRepository(t)

	// Create test customer and events
	customerID := "test-customer"

	now := time.Now()
	events := []*model.CustomerEvent{
		{
			CustomerID:  customerID,
			EventType:   model.EventTypePageView,
			EventSource: model.EventSourceWebsite,
			OccurredAt:  now.Add(-2 * time.Hour),
		},
		{
			CustomerID:  customerID,
			EventType:   model.EventTypeClick,
			EventSource: model.EventSourceApp,
			OccurredAt:  now.Add(-1 * time.Hour),
		},
		{
			CustomerID:  customerID,
			EventType:   model.EventTypePurchase,
			EventSource: model.EventSourceWechat,
			OccurredAt:  now,
		},
	}

	for _, event := range events {
		repo.Record(context.Background(), event)
	}

	tests := []struct {
		name       string
		customerID string
		limit      int
		wantCount  int
	}{
		{
			name:       "get all events for customer",
			customerID: customerID,
			limit:      0,
			wantCount:  3,
		},
		{
			name:       "get limited events for customer",
			customerID: customerID,
			limit:      2,
			wantCount:  2,
		},
		{
			name:       "get events for non-existing customer",
			customerID: "non-existing",
			limit:      0,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByCustomerID(context.Background(), tt.customerID, tt.limit)

			if err != nil {
				t.Errorf("GetByCustomerID() error = %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("Expected %d events, got %d", tt.wantCount, len(result))
			}

			// Verify events are ordered by occurred_at DESC when limit is applied
			if tt.limit > 0 && len(result) > 1 {
				for i := 0; i < len(result)-1; i++ {
					if result[i].OccurredAt.Before(result[i+1].OccurredAt) {
						t.Error("Expected events to be ordered by occurred_at DESC")
					}
				}
			}
		})
	}
}

// TestCustomerEventRepository_GetByTimeRange tests retrieving events by time range
func TestCustomerEventRepository_GetByTimeRange(t *testing.T) {
	repo := setupCustomerEventRepository(t)

	now := time.Now()

	// Create events at different times
	events := []*model.CustomerEvent{
		{
			CustomerID:  "customer-1",
			EventType:   model.EventTypePageView,
			EventSource: model.EventSourceWebsite,
			OccurredAt:  now.Add(-48 * time.Hour), // 2 days ago
		},
		{
			CustomerID:  "customer-2",
			EventType:   model.EventTypeClick,
			EventSource: model.EventSourceApp,
			OccurredAt:  now.Add(-24 * time.Hour), // 1 day ago
		},
		{
			CustomerID:  "customer-3",
			EventType:   model.EventTypePurchase,
			EventSource: model.EventSourceWechat,
			OccurredAt:  now.Add(-12 * time.Hour), // 12 hours ago
		},
		{
			CustomerID:  "customer-4",
			EventType:   model.EventTypeSignup,
			EventSource: model.EventSourceWebsite,
			OccurredAt:  now,
		},
	}

	for _, event := range events {
		repo.Record(context.Background(), event)
	}

	tests := []struct {
		name      string
		start     time.Time
		end       time.Time
		wantCount int
	}{
		{
			name:      "get all events",
			start:     now.Add(-72 * time.Hour),
			end:       now,
			wantCount: 4,
		},
		{
			name:      "get last 24 hours",
			start:     now.Add(-24 * time.Hour),
			end:       now,
			wantCount: 3,
		},
		{
			name:      "get last 12 hours",
			start:     now.Add(-12 * time.Hour),
			end:       now,
			wantCount: 2,
		},
		{
			name:      "get events from far future (none expected)",
			start:     now.Add(24 * time.Hour),
			end:       now.Add(48 * time.Hour),
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.GetByTimeRange(context.Background(), tt.start, tt.end)
			if err != nil {
				t.Errorf("GetByTimeRange() error = %v", err)
			}
			if len(result) != tt.wantCount {
				t.Errorf("Expected %d events, got %d", tt.wantCount, len(result))
			}
		})
	}
}

// TestCustomerEventRepository_EventData tests event data serialization
func TestCustomerEventRepository_EventData(t *testing.T) {
	repo := setupCustomerEventRepository(t)

	eventData := map[string]any{
		"product_id": "prod-123",
		"quantity":   2,
		"price":      99.99,
	}
	dataJSON, _ := json.Marshal(eventData)

	event := &model.CustomerEvent{
		CustomerID:  "test-customer",
		EventType:   model.EventTypePurchase,
		EventSource: model.EventSourceApp,
		EventData:   string(dataJSON),
	}

	err := repo.Record(context.Background(), event)
	if err != nil {
		t.Errorf("Record() error = %v", err)
	}

	// Retrieve and verify event data
	result, err := repo.GetByCustomerID(context.Background(), event.CustomerID, 1)
	if err != nil {
		t.Errorf("GetByCustomerID() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result[0].EventData), &data); err != nil {
		t.Errorf("Failed to unmarshal event data: %v", err)
	}

	if data["product_id"] != "prod-123" {
		t.Errorf("Expected product_id 'prod-123', got '%v'", data["product_id"])
	}

	// Note: JSON unmarshals numbers as float64
	if data["quantity"] != float64(2) {
		t.Errorf("Expected quantity 2, got '%v'", data["quantity"])
	}
}

// TestCustomerEventRepository_AllEventTypes tests all event types
func TestCustomerEventRepository_AllEventTypes(t *testing.T) {
	repo := setupCustomerEventRepository(t)

	customerID := "test-customer"

	eventTypes := []model.EventType{
		model.EventTypePageView,
		model.EventTypeClick,
		model.EventTypePurchase,
		model.EventTypeAddToCart,
		model.EventTypeSignup,
		model.EventTypeLogin,
	}

	for _, eventType := range eventTypes {
		event := &model.CustomerEvent{
			CustomerID:  customerID,
			EventType:   eventType,
			EventSource: model.EventSourceWebsite,
		}
		err := repo.Record(context.Background(), event)
		if err != nil {
			t.Errorf("Record() error for event type %s = %v", eventType, err)
		}
	}

	events, err := repo.GetByCustomerID(context.Background(), customerID, 100)
	if err != nil {
		t.Errorf("GetByCustomerID() error = %v", err)
	}

	if len(events) != len(eventTypes) {
		t.Errorf("Expected %d events, got %d", len(eventTypes), len(events))
	}
}

// TestCustomerEventRepository_AllEventSources tests all event sources
func TestCustomerEventRepository_AllEventSources(t *testing.T) {
	repo := setupCustomerEventRepository(t)

	customerID := "test-customer"

	eventSources := []model.EventSource{
		model.EventSourceWechat,
		model.EventSourceDouyin,
		model.EventSourceXiaohongshu,
		model.EventSourceWebsite,
		model.EventSourceApp,
	}

	for _, source := range eventSources {
		event := &model.CustomerEvent{
			CustomerID:  customerID,
			EventType:   model.EventTypePageView,
			EventSource: source,
		}
		err := repo.Record(context.Background(), event)
		if err != nil {
			t.Errorf("Record() error for source %s = %v", source, err)
		}
	}

	events, err := repo.GetByCustomerID(context.Background(), customerID, 100)
	if err != nil {
		t.Errorf("GetByCustomerID() error = %v", err)
	}

	if len(events) != len(eventSources) {
		t.Errorf("Expected %d events, got %d", len(eventSources), len(events))
	}
}
