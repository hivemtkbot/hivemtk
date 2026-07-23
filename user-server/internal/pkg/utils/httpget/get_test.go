package httpget

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetRequest_Success(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"key":   "value",
			"count": 42,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer testServer.Close()

	result, err := GetRequest(testServer.URL)
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("Expected key to be 'value', got %v", result["key"])
	}
}

func TestGetRequest_InvalidURL(t *testing.T) {
	_, err := GetRequest("http://127.0.0.1:1")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestGetRequest_InvalidJSON(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json{"))
	}))
	defer testServer.Close()

	result, err := GetRequest(testServer.URL)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if result != nil {
		t.Logf("Got non-nil result for invalid JSON: %v", result)
	}
}

func TestGetRequest_EmptyResponse(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}))
	defer testServer.Close()

	result, err := GetRequest(testServer.URL)
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result for empty JSON object")
	}
}

func TestGetRequest_ComplexResponse(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"string": "hello",
			"number": 123,
			"bool":   true,
			"array":  []any{1, 2, 3},
			"nested": map[string]any{"key": "value"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer testServer.Close()

	result, err := GetRequest(testServer.URL)
	if err != nil {
		t.Fatalf("GetRequest failed: %v", err)
	}
	if result["string"] != "hello" {
		t.Errorf("Expected string to be 'hello', got %v", result["string"])
	}
	if result["number"] != float64(123) {
		t.Errorf("Expected number to be 123, got %v", result["number"])
	}
	if result["bool"] != true {
		t.Errorf("Expected bool to be true, got %v", result["bool"])
	}
}
