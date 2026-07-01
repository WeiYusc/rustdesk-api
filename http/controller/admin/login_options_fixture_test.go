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

func setupAdminLoginOptionsFixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite login options db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.Oauth{}, &model.User{}, &model.UserPasskey{}); err != nil {
		t.Fatalf("migrate login options fixture models: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.DB = db
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
	router.LoginBind(group)
	return engine, db
}

func TestAdminLoginOptionsExposePasskeyAndEmailFlags(t *testing.T) {
	engine, db := setupAdminLoginOptionsFixture(t)
	if err := service.AllService.SettingsService.SavePasskey(service.PasskeySettings{Enabled: true, RPID: "admin.example.test", AllowedOrigins: []string{"https://admin.example.test"}, DiscoverableLoginEnabled: true}, 1); err != nil {
		t.Fatalf("save passkey settings: %v", err)
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true}, 1); err != nil {
		t.Fatalf("save email settings: %v", err)
	}
	isAdmin := true
	adminUser := &model.User{Username: "admin", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin, WebauthnUserHandle: "stable-handle"}
	if err := db.Create(adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := db.Create(&model.UserPasskey{UserId: adminUser.Id, Name: "admin key", CredentialID: "credential-id", UserHandle: "stable-handle", PublicKey: "public-key"}).Error; err != nil {
		t.Fatalf("create admin passkey: %v", err)
	}
	if err := service.AllService.SettingsService.SaveAuthPolicy(service.AuthPolicySettings{DisablePasswordLogin: true}, 1); err != nil {
		t.Fatalf("save auth policy settings: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/login-options", strings.NewReader(""))
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login-options status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			PasskeyEnabled                  bool `json:"passkey_enabled"`
			PasskeyDiscoverableLoginEnabled bool `json:"passkey_discoverable_login_enabled"`
			EmailVerificationEnabled        bool `json:"email_verification_enabled"`
			DisablePwd                      bool `json:"disable_pwd"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal login-options: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 0 || !payload.Data.PasskeyEnabled || !payload.Data.PasskeyDiscoverableLoginEnabled || !payload.Data.EmailVerificationEnabled || !payload.Data.DisablePwd {
		t.Fatalf("login-options payload = %#v", payload)
	}
}
