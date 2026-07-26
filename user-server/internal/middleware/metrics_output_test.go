package middleware

// metrics_output_test.go 验证 /metrics 端点输出的 Prometheus 文本格式正确
//
// 回归测试覆盖：
//  1. float64 值序列化正确（原实现 float64ToString 将小数位逆序输出，导致
//     `1.5` 被错误序列化为 `1.000005`，破坏所有 latency/size 指标）
//  2. Prometheus 标签值转义正确（`"` `\` `\n` 必须转义，否则 scrape 失败）
//  3. /metrics 端点返回 200 + 正确 Content-Type

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"marketing/internal/pkg/metrics"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestFloatValuesSerializedCorrectly 验证 float64 指标值的小数位顺序正确
func TestFloatValuesSerializedCorrectly(t *testing.T) {
	// 准备一个带 float 值的 HistogramVec
	hv := &metrics.HistogramVec{}
	hv.Observe("GET|/test|200", 0.5)
	hv.Observe("GET|/test|200", 1.25)
	hv.Observe("GET|/test|200", 0.85)
	metrics.GlobalMetrics.RequestDuration = hv
	defer func() {
		// 恢复默认值，避免污染其他测试
		metrics.GlobalMetrics.RequestDuration = metrics.NewHistogramVec()
	}()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", MetricsHandler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()

	// sum = 0.5 + 1.25 + 0.85 = 2.6
	if !strings.Contains(body, "http_request_duration_seconds_sum{method=\"GET\",path=\"/test\",status=\"200\"} 2.600000") {
		t.Errorf("float sum not serialized correctly.\nbody snippet:\n%s", body)
	}

	// count 应为 3
	if !strings.Contains(body, "http_request_duration_seconds_count{method=\"GET\",path=\"/test\",status=\"200\"} 3\n") {
		t.Errorf("count not serialized correctly.\nbody snippet:\n%s", body)
	}
}

// TestLabelValuesEscaped 验证 Prometheus 标签值被正确转义
//
// 复现 bug：原 formatHTTPLabels 不转义 `"` `\` `\n`，导致 /metrics 输出格式损坏。
func TestLabelValuesEscaped(t *testing.T) {
	// 构造包含特殊字符的 label
	// 注意：splitLabels 用 "|" 分隔，所以这里用包含特殊字符的 path
	cv := &metrics.CounterVec{}
	cv.Inc(`GET|/path/"quoted"|200`)
	cv.Inc(`GET|/path\backslash|200`)
	cv.Inc("GET|/path\nnewline|200")
	metrics.GlobalMetrics.RequestTotal = cv
	defer func() {
		metrics.GlobalMetrics.RequestTotal = metrics.NewCounterVec()
	}()

	r := gin.New()
	r.GET("/metrics", MetricsHandler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()

	// 验证转义后的输出存在
	if !strings.Contains(body, `path="/path/\"quoted\""`) {
		t.Errorf("double-quote not escaped.\nbody snippet:\n%s", body)
	}
	if !strings.Contains(body, `path="/path\\backslash"`) {
		t.Errorf("backslash not escaped.\nbody snippet:\n%s", body)
	}
	// \n 在 Prometheus 文本格式中应被转义为字面量 \n（反斜杠+n）
	if !strings.Contains(body, `path="/path\nnewline"`) {
		t.Errorf("newline not escaped.\nbody snippet:\n%s", body)
	}
}

// TestMetricsContentType 验证 Content-Type 符合 Prometheus exposition format
func TestMetricsContentType(t *testing.T) {
	r := gin.New()
	r.GET("/metrics", MetricsHandler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %s", ct)
	}
	if !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("expected version=0.0.4 in content-type, got %s", ct)
	}
}
