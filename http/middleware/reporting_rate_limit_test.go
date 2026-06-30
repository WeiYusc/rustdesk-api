package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestReportingRateLimitRejectsRequestsOverLimitPerPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ReportingRateLimit(2, time.Minute))
	r.POST("/api/heartbeat", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d; body=%q", i+1, w.Code, http.StatusOK, w.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/heartbeat", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want %d; body=%q", w.Code, http.StatusTooManyRequests, w.Body.String())
	}
}

func TestReportingRateLimitIsScopedPerPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ReportingRateLimit(1, time.Minute))
	r.POST("/api/heartbeat", func(c *gin.Context) { c.String(http.StatusOK, "heartbeat") })
	r.POST("/api/sysinfo", func(c *gin.Context) { c.String(http.StatusOK, "sysinfo") })

	for _, path := range []string{"/api/heartbeat", "/api/sysinfo"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body=%q", path, w.Code, http.StatusOK, w.Body.String())
		}
	}
}
