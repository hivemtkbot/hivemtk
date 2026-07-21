package pagination

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCtx(query string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/?"+query, nil)
	c.Request = req
	return c
}

func TestParse_Defaults(t *testing.T) {
	c := newCtx("")
	page, pageSize, err := Parse(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != DefaultPage {
		t.Errorf("page = %d, want %d", page, DefaultPage)
	}
	if pageSize != DefaultPageSize {
		t.Errorf("pageSize = %d, want %d", pageSize, DefaultPageSize)
	}
}

func TestParse_SnakeCase(t *testing.T) {
	c := newCtx("page=3&page_size=25")
	page, pageSize, err := Parse(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page != 3 {
		t.Errorf("page = %d, want 3", page)
	}
	if pageSize != 25 {
		t.Errorf("pageSize = %d, want 25", pageSize)
	}
}

func TestParse_CamelCase(t *testing.T) {
	c := newCtx("page=2&pageSize=15")
	_, pageSize, err := Parse(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 15 {
		t.Errorf("pageSize = %d, want 15", pageSize)
	}
}

func TestParse_LimitAlias(t *testing.T) {
	c := newCtx("limit=50")
	_, pageSize, err := Parse(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 50 {
		t.Errorf("pageSize = %d, want 50", pageSize)
	}
}

func TestParse_PageZero_Rejected(t *testing.T) {
	c := newCtx("page=0")
	_, _, err := Parse(c)
	if err != ErrInvalidPage {
		t.Errorf("err = %v, want ErrInvalidPage", err)
	}
}

func TestParse_PageNegative_Rejected(t *testing.T) {
	c := newCtx("page=-5")
	_, _, err := Parse(c)
	if err != ErrInvalidPage {
		t.Errorf("err = %v, want ErrInvalidPage", err)
	}
}

func TestParse_PageNonNumeric_Rejected(t *testing.T) {
	c := newCtx("page=abc")
	_, _, err := Parse(c)
	if err != ErrInvalidPage {
		t.Errorf("err = %v, want ErrInvalidPage", err)
	}
}

func TestParse_PageSizeZero_Rejected(t *testing.T) {
	c := newCtx("page_size=0")
	_, _, err := Parse(c)
	if err != ErrInvalidPageSize {
		t.Errorf("err = %v, want ErrInvalidPageSize", err)
	}
}

func TestParse_PageSizeNegative_Rejected(t *testing.T) {
	c := newCtx("page_size=-10")
	_, _, err := Parse(c)
	if err != ErrInvalidPageSize {
		t.Errorf("err = %v, want ErrInvalidPageSize", err)
	}
}

func TestParse_PageSizeExceedsMax_Rejected(t *testing.T) {
	c := newCtx("page_size=500")
	_, _, err := Parse(c)
	if err != ErrInvalidPageSize {
		t.Errorf("err = %v, want ErrInvalidPageSize", err)
	}
}

func TestParse_PageSizeAtMax_Accepted(t *testing.T) {
	c := newCtx("page_size=100")
	_, pageSize, err := Parse(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 100 {
		t.Errorf("pageSize = %d, want 100", pageSize)
	}
}

func TestParseWithMax_StricterLimit(t *testing.T) {
	c := newCtx("page_size=50")
	_, pageSize, err := ParseWithMax(c, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 50 {
		t.Errorf("pageSize = %d, want 50", pageSize)
	}

	// 51 should be rejected when max=50
	c = newCtx("page_size=51")
	_, _, err = ParseWithMax(c, 50)
	if err != ErrInvalidPageSize {
		t.Errorf("err = %v, want ErrInvalidPageSize", err)
	}
}

func TestParseWithMax_InvalidInputClampedToGlobalMax(t *testing.T) {
	// maxPageSize > MaxPageSize should be clamped
	c := newCtx("page_size=100")
	_, pageSize, err := ParseWithMax(c, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageSize != 100 {
		t.Errorf("pageSize = %d, want 100", pageSize)
	}
}

func TestParseOffset_Correct(t *testing.T) {
	c := newCtx("page=3&page_size=20")
	offset, limit, err := ParseOffset(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if offset != 40 {
		t.Errorf("offset = %d, want 40", offset)
	}
	if limit != 20 {
		t.Errorf("limit = %d, want 20", limit)
	}
}

func TestParseOffset_PageOne(t *testing.T) {
	c := newCtx("")
	offset, limit, err := ParseOffset(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
	if limit != DefaultPageSize {
		t.Errorf("limit = %d, want %d", limit, DefaultPageSize)
	}
}
