package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
)

func TestLimiterSkipPathIsExact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 1})
	global.LoginLimiter.RecordFailedAttempt("192.0.2.1")

	r := gin.New()
	r.Use(Limiter("/api/login"))
	r.POST("/api/login", func(c *gin.Context) { c.String(http.StatusOK, "login") })
	r.GET("/api/login-options", func(c *gin.Context) { c.String(http.StatusOK, "options") })

	login := httptest.NewRecorder()
	r.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/login", nil))
	if login.Code != http.StatusOK || login.Body.String() != "login" {
		t.Fatalf("login status/body = %d/%q, want 200/login", login.Code, login.Body.String())
	}

	options := httptest.NewRecorder()
	r.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/login-options", nil))
	if options.Code != http.StatusOK {
		t.Fatalf("login-options transport status = %d, want 200 response.Fail transport; body=%q", options.Code, options.Body.String())
	}
	if options.Body.String() == "options" {
		t.Fatalf("login-options unexpectedly bypassed limiter via prefix skip")
	}
}
