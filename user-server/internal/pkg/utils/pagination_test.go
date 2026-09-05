package utils

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newPaginationCtx(query string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?"+query, nil)
	c.Request = req
	return c
}

func TestParsePagination_Defaults(t *testing.T) {
	page, pageSize, err := ParsePagination(newPaginationCtx(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 1 {
		t.Errorf("page = %d, want 1", page)
	}
	if pageSize != defaultDefaultPageSize {
		t.Errorf("pageSize = %d, want %d", pageSize, defaultDefaultPageSize)
	}
}

func TestParsePagination_PageNonNumeric_DefaultsToOne(t *testing.T) {
	page, _, err := ParsePagination(newPaginationCtx("page=abc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 1 {
		t.Errorf("page = %d, want 1", page)
	}
}

func TestParsePagination_PageNegative_DefaultsToOne(t *testing.T) {
	page, _, err := ParsePagination(newPaginationCtx("page=-3"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 1 {
		t.Errorf("page = %d, want 1", page)
	}
}

func TestParsePagination_PageSizeNonNumeric_FallbackToDefault(t *testing.T) {
	_, pageSize, err := ParsePagination(newPaginationCtx("page_size=xyz"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != defaultDefaultPageSize {
		t.Errorf("pageSize = %d, want %d", pageSize, defaultDefaultPageSize)
	}
}

func TestParsePagination_PageSizeBelowMin_Clamped(t *testing.T) {

	_, pageSize, err := ParsePagination(newPaginationCtx("page_size=2"),
		WithMinSize(5),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 5 {
		t.Errorf("pageSize = %d, want 5 (clamped to minSize)", pageSize)
	}
}

func TestParsePagination_PageSizeZero_FallbackToDefault(t *testing.T) {

	_, pageSize, err := ParsePagination(newPaginationCtx("page_size=0"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != defaultDefaultPageSize {
		t.Errorf("pageSize = %d, want %d (defaultSize fallback)", pageSize, defaultDefaultPageSize)
	}
}

func TestParsePagination_PageSizeNegative_FallbackToDefault(t *testing.T) {
	_, pageSize, err := ParsePagination(newPaginationCtx("page_size=-10"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != defaultDefaultPageSize {
		t.Errorf("pageSize = %d, want %d", pageSize, defaultDefaultPageSize)
	}
}

func TestParsePagination_PageSizeExceedsMax_Rejected(t *testing.T) {
	_, _, err := ParsePagination(newPaginationCtx("page_size=999"))
	if !IsInvalidPageSize(err) {
		t.Errorf("err = %v, want ErrInvalidPageSize", err)
	}
}

func TestParsePagination_PageSizeExceedsMax_ClampedWhenAllowed(t *testing.T) {

	_, pageSize, err := ParsePagination(newPaginationCtx("page_size=2000"),
		WithMaxSize(MaxPageSizeAdmin),
		WithAllowOverMax(true),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != MaxPageSizeAdmin {
		t.Errorf("pageSize = %d, want %d", pageSize, MaxPageSizeAdmin)
	}
}

func TestParsePagination_DefaultSizeOption(t *testing.T) {
	_, pageSize, err := ParsePagination(newPaginationCtx(""),
		WithDefaultSize(50),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 50 {
		t.Errorf("pageSize = %d, want 50", pageSize)
	}
}

func TestParsePagination_AdminScenario(t *testing.T) {

	page, pageSize, err := ParsePagination(newPaginationCtx("page=2&page_size=500"),
		WithMaxSize(MaxPageSizeAdmin),
		WithAllowOverMax(true),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 2 {
		t.Errorf("page = %d, want 2", page)
	}
	if pageSize != 500 {
		t.Errorf("pageSize = %d, want 500", pageSize)
	}

	_, pageSize, err = ParsePagination(newPaginationCtx("page_size=2000"),
		WithMaxSize(MaxPageSizeAdmin),
		WithAllowOverMax(true),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != MaxPageSizeAdmin {
		t.Errorf("pageSize = %d, want %d", pageSize, MaxPageSizeAdmin)
	}
}

func TestParsePagination_LimitAlias(t *testing.T) {
	_, pageSize, err := ParsePagination(newPaginationCtx("limit=33"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 33 {
		t.Errorf("pageSize = %d, want 33", pageSize)
	}
}

func TestParsePagination_CamelCaseAlias(t *testing.T) {
	_, pageSize, err := ParsePagination(newPaginationCtx("pageSize=27"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 27 {
		t.Errorf("pageSize = %d, want 27", pageSize)
	}
}

func TestParsePaginationOffset(t *testing.T) {
	offset, limit, err := ParsePaginationOffset(newPaginationCtx("page=3&page_size=15"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if offset != 30 {
		t.Errorf("offset = %d, want 30", offset)
	}
	if limit != 15 {
		t.Errorf("limit = %d, want 15", limit)
	}
}
