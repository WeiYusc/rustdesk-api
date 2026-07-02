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

func setupAdminRegisterEmailContractFixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite register fixture db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.Oauth{}, &model.User{}, &model.UserToken{}, &model.LoginLog{}); err != nil {
		t.Fatalf("migrate register fixture models: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.Config.App.Register = true
	global.Config.App.RegisterStatus = int(model.COMMON_STATUS_DISABLED)
	global.DB = db
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.ApiInitValidator()
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	service.New(&global.Config, db, global.Logger, nil, nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.Init(engine)
	return engine, db
}

func decodeAdminRegisterResponseCode(t *testing.T, body []byte) int {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal response code: %v; body=%q", err, string(body))
	}
	return payload.Code
}

func postAdminRegisterFixture(engine *gin.Engine, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/user/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminLoginOptionsExposeRegisterEmailRequired(t *testing.T) {
	engine, _ := setupAdminRegisterEmailContractFixture(t)
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true, RequireForRegister: true}, 1); err != nil {
		t.Fatalf("save email verification settings: %v", err)
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
			EmailVerificationEnabled            bool `json:"email_verification_enabled"`
			EmailVerificationRequireForRegister bool `json:"email_verification_require_for_register"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal login-options: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 0 || !payload.Data.EmailVerificationEnabled || !payload.Data.EmailVerificationRequireForRegister {
		t.Fatalf("login-options email verification payload = %#v", payload)
	}
}

func TestAdminRegisterRequiresEmailWhenVerificationRequiresRegister(t *testing.T) {
	engine, db := setupAdminRegisterEmailContractFixture(t)
	if err := service.AllService.SettingsService.SaveRegisterPolicy(service.RegisterPolicySettings{Enabled: true, DefaultStatus: int(model.COMMON_STATUS_DISABLED)}, 1); err != nil {
		t.Fatalf("save register policy: %v", err)
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true, RequireForRegister: true}, 1); err != nil {
		t.Fatalf("save email verification settings: %v", err)
	}

	for name, body := range map[string]string{
		"missing":    `{"username":"email-required-user","password":"pass1234","confirm_password":"pass1234"}`,
		"whitespace": `{"username":"email-whitespace-user","password":"pass1234","confirm_password":"pass1234","email":"   "}`,
	} {
		recorder := postAdminRegisterFixture(engine, body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s register status = %d, want %d; body=%q", name, recorder.Code, http.StatusOK, recorder.Body.String())
		}
		if code := decodeAdminRegisterResponseCode(t, recorder.Body.Bytes()); code != 101 {
			t.Fatalf("%s register without required email code = %d, want 101; body=%q", name, code, recorder.Body.String())
		}
	}
	var count int64
	if err := db.Model(&model.User{}).Where("username in ?", []string{"email-required-user", "email-whitespace-user"}).Count(&count).Error; err != nil {
		t.Fatalf("count created users: %v", err)
	}
	if count != 0 {
		t.Fatalf("register without required email created %d users", count)
	}
}

func TestAdminRegisterAllowsValidEmailWhenVerificationRequiresRegister(t *testing.T) {
	engine, db := setupAdminRegisterEmailContractFixture(t)
	if err := service.AllService.SettingsService.SaveRegisterPolicy(service.RegisterPolicySettings{Enabled: true, DefaultStatus: int(model.COMMON_STATUS_DISABLED)}, 1); err != nil {
		t.Fatalf("save register policy: %v", err)
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true, RequireForRegister: true}, 1); err != nil {
		t.Fatalf("save email verification settings: %v", err)
	}

	recorder := postAdminRegisterFixture(engine, `{"username":"valid-email-required-user","password":"pass1234","confirm_password":"pass1234","email":" valid@example.test "}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if code := decodeAdminRegisterResponseCode(t, recorder.Body.Bytes()); code != 101 {
		t.Fatalf("register with valid required email code = %d, want wait-admin code 101; body=%q", code, recorder.Body.String())
	}
	var user model.User
	if err := db.Where("username = ?", "valid-email-required-user").First(&user).Error; err != nil {
		t.Fatalf("query registered user: %v", err)
	}
	if user.Email != "valid@example.test" || user.Status != model.COMMON_STATUS_DISABLED {
		t.Fatalf("registered user = %#v, want trimmed email and disabled status", user)
	}
}

func TestAdminRegisterAllowsMissingEmailWhenVerificationDoesNotRequireRegister(t *testing.T) {
	engine, db := setupAdminRegisterEmailContractFixture(t)
	if err := service.AllService.SettingsService.SaveRegisterPolicy(service.RegisterPolicySettings{Enabled: true, DefaultStatus: int(model.COMMON_STATUS_DISABLED)}, 1); err != nil {
		t.Fatalf("save register policy: %v", err)
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true, RequireForRegister: false}, 1); err != nil {
		t.Fatalf("save email verification settings: %v", err)
	}

	recorder := postAdminRegisterFixture(engine, `{"username":"optional-email-user","password":"pass1234","confirm_password":"pass1234"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if code := decodeAdminRegisterResponseCode(t, recorder.Body.Bytes()); code != 101 {
		t.Fatalf("register with optional missing email code = %d, want wait-admin code 101; body=%q", code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&model.User{}).Where("username = ?", "optional-email-user").Count(&count).Error; err != nil {
		t.Fatalf("count created users: %v", err)
	}
	if count != 1 {
		t.Fatalf("register with optional missing email created %d users, want 1", count)
	}
}

func TestAdminRegisterRejectsInvalidOptionalEmail(t *testing.T) {
	engine, db := setupAdminRegisterEmailContractFixture(t)
	if err := service.AllService.SettingsService.SaveRegisterPolicy(service.RegisterPolicySettings{Enabled: true, DefaultStatus: int(model.COMMON_STATUS_DISABLED)}, 1); err != nil {
		t.Fatalf("save register policy: %v", err)
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: false, RequireForRegister: false}, 1); err != nil {
		t.Fatalf("save email verification settings: %v", err)
	}

	recorder := postAdminRegisterFixture(engine, `{"username":"invalid-email-user","password":"pass1234","confirm_password":"pass1234","email":"not-an-email"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if code := decodeAdminRegisterResponseCode(t, recorder.Body.Bytes()); code != 101 {
		t.Fatalf("register with invalid optional email code = %d, want 101; body=%q", code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&model.User{}).Where("username = ?", "invalid-email-user").Count(&count).Error; err != nil {
		t.Fatalf("count created users: %v", err)
	}
	if count != 0 {
		t.Fatalf("register with invalid optional email created %d users", count)
	}
}
