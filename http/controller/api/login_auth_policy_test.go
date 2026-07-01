package api_test

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
	controller "github.com/lejianwen/rustdesk-api/v2/http/controller/api"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAPILoginHonorsPersistedPasswordDisablePolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite api login db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.Oauth{}, &model.User{}, &model.UserPasskey{}); err != nil {
		t.Fatalf("migrate setting: %v", err)
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
	service.New(&global.Config, db, global.Logger, nil, nil)
	isAdmin := true
	adminUser := &model.User{Username: "admin", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin, WebauthnUserHandle: "stable-handle"}
	if err := db.Create(adminUser).Error; err != nil {
		t.Fatalf("create admin fallback user: %v", err)
	}
	if err := service.AllService.SettingsService.SavePasskey(service.PasskeySettings{Enabled: true, RPID: "rd.plumire.cyou", AllowedOrigins: []string{"https://rd.plumire.cyou"}, DiscoverableLoginEnabled: true, ResidentKeyRequirement: service.ResidentKeyRequired}, 1); err != nil {
		t.Fatalf("save passkey fallback settings: %v", err)
	}
	if err := db.Create(&model.UserPasskey{UserId: adminUser.Id, Name: "admin key", CredentialID: "credential-id", UserHandle: "stable-handle", PublicKey: "public-key"}).Error; err != nil {
		t.Fatalf("create admin fallback passkey: %v", err)
	}
	if err := service.AllService.SettingsService.SaveAuthPolicy(service.AuthPolicySettings{DisablePasswordLogin: true}, 1); err != nil {
		t.Fatalf("save auth policy: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/login", (&controller.Login{}).Login)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"password"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%q", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Error != "Password login disabled." {
		t.Fatalf("error = %q, want password disabled", payload.Error)
	}
}
