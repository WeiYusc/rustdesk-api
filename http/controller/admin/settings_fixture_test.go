package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/router"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type adminSettingsFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
}

func setupAdminSettingsFixture(t *testing.T) adminSettingsFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite settings fixture db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate settings fixture models: %v", err)
	}

	global.Config = config.Config{Lang: "en"}
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.Jwt = jwt.NewJwt("", 0)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)

	createAdminSettingsUser(t, db, "settings-admin", true, "settings-admin-token")
	createAdminSettingsUser(t, db, "settings-user", false, "settings-user-token")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/admin")
	router.SettingsBind(group)

	return adminSettingsFixture{db: db, router: engine, adminToken: "settings-admin-token", nonAdminToken: "settings-user-token"}
}

func createAdminSettingsUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) {
	t.Helper()
	user := &model.User{Username: username, Email: username + "@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create settings user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create settings token: %v", err)
	}
}

func adminSettingsRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminSettingsRoutesRequireAdminPrivilege(t *testing.T) {
	fixture := setupAdminSettingsFixture(t)

	unauthenticated := adminSettingsRequest(fixture.router, http.MethodGet, "/api/admin/settings/smtp", "", "")
	assertAdminSettingsResponseCode(t, unauthenticated, 403)

	nonAdmin := adminSettingsRequest(fixture.router, http.MethodGet, "/api/admin/settings/smtp", "", fixture.nonAdminToken)
	assertAdminSettingsResponseCode(t, nonAdmin, 403)
}

func TestAdminSettingsSMTPReadUpdateAndMaskPassword(t *testing.T) {
	fixture := setupAdminSettingsFixture(t)

	getDefault := adminSettingsRequest(fixture.router, http.MethodGet, "/api/admin/settings/smtp", "", fixture.adminToken)
	assertAdminSettingsResponseCode(t, getDefault, 0)
	var defaultPayload struct {
		Code int `json:"code"`
		Data struct {
			Enabled        bool   `json:"enabled"`
			Port           int    `json:"port"`
			Security       string `json:"security"`
			HasPassword    bool   `json:"has_password"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getDefault.Body.Bytes(), &defaultPayload); err != nil {
		t.Fatalf("unmarshal default smtp payload: %v; body=%q", err, getDefault.Body.String())
	}
	if defaultPayload.Data.Enabled || defaultPayload.Data.Port != 587 || defaultPayload.Data.Security != service.SMTPSecurityStartTLS || defaultPayload.Data.HasPassword || defaultPayload.Data.TimeoutSeconds != 10 {
		t.Fatalf("default SMTP payload = %#v", defaultPayload.Data)
	}

	update := adminSettingsRequest(fixture.router, http.MethodPost, "/api/admin/settings/smtp", `{"enabled":true,"host":"smtp.example.test","port":465,"security":"tls","username":"smtp-user","password":"secret-password","from_email":"noreply@example.test","from_name":"RustDesk API","timeout_seconds":20}`, fixture.adminToken)
	assertAdminSettingsResponseCode(t, update, 0)

	getUpdated := adminSettingsRequest(fixture.router, http.MethodGet, "/api/admin/settings/smtp", "", fixture.adminToken)
	assertAdminSettingsResponseCode(t, getUpdated, 0)
	var updatedPayload struct {
		Code int `json:"code"`
		Data struct {
			Host        string `json:"host"`
			Password    string `json:"password"`
			HasPassword bool   `json:"has_password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getUpdated.Body.Bytes(), &updatedPayload); err != nil {
		t.Fatalf("unmarshal updated smtp payload: %v; body=%q", err, getUpdated.Body.String())
	}
	if updatedPayload.Data.Host != "smtp.example.test" || updatedPayload.Data.Password != "" || !updatedPayload.Data.HasPassword {
		t.Fatalf("updated SMTP payload = %#v", updatedPayload.Data)
	}
}

func assertAdminSettingsResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != want {
		t.Fatalf("response code = %d, want %d; body=%q", payload.Code, want, recorder.Body.String())
	}
}
