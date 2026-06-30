package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBodyLimitRejectsOversizedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(4))
	r.POST("/echo", func(c *gin.Context) {
		if _, err := c.GetRawData(); err != nil {
			c.String(http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("12345"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%q", w.Code, http.StatusRequestEntityTooLarge, w.Body.String())
	}
}

func TestBodyLimitAllowsRequestsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(4))
	r.POST("/echo", func(c *gin.Context) {
		body, err := c.GetRawData()
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("1234"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "1234" {
		t.Fatalf("status/body = %d/%q, want 200/1234", w.Code, w.Body.String())
	}
}

func TestBodyLimitSkipsConfiguredPathPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(4, "/api/admin/file/upload"))
	r.POST("/api/admin/file/upload", func(c *gin.Context) {
		body, err := c.GetRawData()
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/file/upload", strings.NewReader("12345"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "12345" {
		t.Fatalf("status/body = %d/%q, want 200/12345 for skipped upload path", w.Code, w.Body.String())
	}
}
