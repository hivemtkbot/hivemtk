package model

import (
	"testing"
	"time"
)

func TestDomainPool_TableName(t *testing.T) {
	pool := &DomainPool{}
	tableName := pool.TableName()
	if tableName != "domain_pool" {
		t.Errorf("Expected table name 'domain_pool', got %s", tableName)
	}
}

func TestDomainPool_BasicFields(t *testing.T) {
	now := time.Now()
	pool := &DomainPool{
		ID:        1,
		Domain:    "example.com",
		Port:      8080,
		Purpose:   "API endpoint",
		Status:    1,
		LastCheck: now,
	}

	if pool.ID != 1 {
		t.Errorf("Expected ID 1, got %d", pool.ID)
	}
	if pool.Domain != "example.com" {
		t.Errorf("Expected Domain 'example.com', got %s", pool.Domain)
	}
	if pool.Port != 8080 {
		t.Errorf("Expected Port 8080, got %d", pool.Port)
	}
	if pool.Purpose != "API endpoint" {
		t.Errorf("Expected Purpose 'API endpoint', got %s", pool.Purpose)
	}
	if pool.Status != 1 {
		t.Errorf("Expected Status 1, got %d", pool.Status)
	}
}

func TestDomainPool_DefaultValues(t *testing.T) {
	pool := &DomainPool{}

	if pool.Port != 0 {
		t.Logf("Port is %d (expected 0 before save, default is 80)", pool.Port)
	}
	if pool.Status != 0 {
		t.Logf("Status is %d (expected 0 before save, default is 1)", pool.Status)
	}
}

func TestDomainPool_WithStatusValues(t *testing.T) {
	statuses := []int{1, 2}
	statusNames := map[int]string{
		1: "正常",
		2: "不可访问",
	}

	for _, status := range statuses {
		pool := &DomainPool{
			Status: status,
		}
		if pool.Status != status {
			t.Errorf("Expected Status %d (%s), got %d", status, statusNames[status], pool.Status)
		}
	}
}

func TestDomainPool_WithPortValues(t *testing.T) {
	ports := []int{80, 443, 8080, 3000}

	for _, port := range ports {
		pool := &DomainPool{
			Port: port,
		}
		if pool.Port != port {
			t.Errorf("Expected Port %d, got %d", port, pool.Port)
		}
	}
}

func TestDomainPool_WithDomain(t *testing.T) {
	domains := []string{
		"example.com",
		"api.example.com",
		"test.example.cn",
		"sub.domain.example.org",
	}

	for _, domain := range domains {
		pool := &DomainPool{
			Domain: domain,
		}
		if pool.Domain != domain {
			t.Errorf("Expected Domain %s, got %s", domain, pool.Domain)
		}
	}
}

func TestDomainPool_WithPurpose(t *testing.T) {
	pool := &DomainPool{
		Domain:  "api.example.com",
		Purpose: "用于 API 服务的域名池",
	}

	if pool.Purpose != "用于 API 服务的域名池" {
		t.Errorf("Expected Purpose '用于 API 服务的域名池', got %s", pool.Purpose)
	}
}

func TestDomainPool_WithEmptyPurpose(t *testing.T) {
	pool := &DomainPool{
		Domain: "example.com",
	}

	if pool.Purpose != "" {
		t.Errorf("Expected empty Purpose, got %s", pool.Purpose)
	}
}

