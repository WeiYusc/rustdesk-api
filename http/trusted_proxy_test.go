package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfigureTrustedProxiesEmptyDisablesForwardedForTrust(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	if err := configureTrustedProxies(r, ""); err != nil {
		t.Fatalf("configureTrustedProxies returned error: %v", err)
	}

	r.GET("/ip", func(c *gin.Context) {
		c.String(stdhttp.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(stdhttp.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got == "198.51.100.99" {
		t.Fatalf("ClientIP trusted X-Forwarded-For with empty trust-proxy; got %q", got)
	}
	if got := w.Body.String(); got != "203.0.113.10" {
		t.Fatalf("ClientIP = %q, want remote addr without forwarded header trust", got)
	}
}

func TestConfigureTrustedProxiesUsesConfiguredProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	if err := configureTrustedProxies(r, "203.0.113.10"); err != nil {
		t.Fatalf("configureTrustedProxies returned error: %v", err)
	}

	r.GET("/ip", func(c *gin.Context) {
		c.String(stdhttp.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(stdhttp.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Body.String(); got != "198.51.100.99" {
		t.Fatalf("ClientIP = %q, want configured trusted proxy forwarded address", got)
	}
}
