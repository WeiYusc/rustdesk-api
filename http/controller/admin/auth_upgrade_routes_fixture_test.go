package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/router"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminAuthUpgradeRouteFixture(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite auth upgrade route db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate auth upgrade route db: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	service.New(&global.Config, db, global.Logger, nil, nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/admin")
	router.PasskeyBind(group)
	router.EmailBind(group)
	return engine
}

func TestAdminPasskeyLoginBeginDisabledByDefault(t *testing.T) {
	engine := setupAdminAuthUpgradeRouteFixture(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/login/begin", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	assertAuthUpgradeRouteCode(t, recorder, 101)
	if !strings.Contains(recorder.Body.String(), "PasskeyDisabled") {
		t.Fatalf("passkey disabled response body = %q", recorder.Body.String())
	}
}

func TestAdminEmailVerificationRoutesRequireLogin(t *testing.T) {
	engine := setupAdminAuthUpgradeRouteFixture(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/email/verification/send", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	assertAuthUpgradeRouteCode(t, recorder, 403)
	if !strings.Contains(recorder.Body.String(), "NeedLogin") && !strings.Contains(recorder.Body.String(), "Please log in") {
		t.Fatalf("email verification unauthenticated body = %q", recorder.Body.String())
	}
}

func assertAuthUpgradeRouteCode(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != want {
		t.Fatalf("code = %d, want %d; body=%q", payload.Code, want, recorder.Body.String())
	}
}
