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

func setupAdminForgotPasswordFixture(t *testing.T, lang string) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite forgot password db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate forgot password fixture models: %v", err)
	}
	global.Config = config.Config{Lang: lang}
	global.DB = db
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, locale := range []string{"en", "zh_CN"} {
		if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", locale+".toml")); err != nil {
			t.Fatalf("load %s locale: %v", locale, err)
		}
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	service.New(&global.Config, db, global.Logger, nil, nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/admin")
	router.LoginBind(group)
	return engine, db
}

func TestAdminForgotPasswordRequestExplainsSMTPUnavailable(t *testing.T) {
	engine, _ := setupAdminForgotPasswordFixture(t, "zh_CN")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/forgot-password/request", strings.NewReader(`{"email":"user@example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "zh_CN")
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal forgot password response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 101 || !strings.Contains(payload.Message, "管理员尚未配置SMTP，服务不可用") {
		t.Fatalf("forgot password response = %#v", payload)
	}
}
