package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupClientLoginOptionsFixture(t *testing.T, webSso bool) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite login options db: %v", err)
	}
	if err := db.AutoMigrate(&model.Oauth{}); err != nil {
		t.Fatalf("migrate oauth model: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.Config.App.WebSso = webSso
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(i18n.NewBundle(language.English)) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	service.New(&global.Config, db, global.Logger, nil, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/login-options", (&Login{}).LoginOptions)
	return router
}

func TestClientLoginOptionsExposeWebauthWhenWebSsoEnabled(t *testing.T) {
	router := setupClientLoginOptionsFixture(t, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/login-options", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	var options []string
	if err := json.Unmarshal(recorder.Body.Bytes(), &options); err != nil {
		t.Fatalf("unmarshal options: %v; body=%q", err, recorder.Body.String())
	}
	joined := strings.Join(options, "\n")
	if !strings.Contains(joined, "common-oidc/") || !strings.Contains(joined, `"name":"webauth"`) || !strings.Contains(joined, "oidc/webauth") {
		t.Fatalf("login options = %#v, want common-oidc and oidc webauth entries", options)
	}
}

func TestClientLoginOptionsHideWebauthWhenWebSsoDisabled(t *testing.T) {
	router := setupClientLoginOptionsFixture(t, false)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/login-options", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "webauth") {
		t.Fatalf("login options leaked webauth while web-sso disabled: %s", recorder.Body.String())
	}
}
