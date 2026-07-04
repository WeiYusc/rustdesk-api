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
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stage2AdminFixture struct {
	engine *gin.Engine
	db     *gorm.DB
}

func setupStage2AdminFixture(t *testing.T) stage2AdminFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite stage2 admin db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.Oauth{}, &model.User{}, &model.UserToken{}, &model.LoginLog{}, &model.UserPasskey{}); err != nil {
		t.Fatalf("migrate stage2 admin models: %v", err)
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
	global.Jwt = jwt.NewJwt("stage2-admin-secret", time.Hour)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.Init(engine)
	return stage2AdminFixture{engine: engine, db: db}
}

func TestAdminRegisterPendingApprovalIsSuccessfulWithoutToken(t *testing.T) {
	fixture := setupStage2AdminFixture(t)
	if err := service.AllService.SettingsService.SaveRegisterPolicy(service.RegisterPolicySettings{Enabled: true, DefaultStatus: int(model.COMMON_STATUS_DISABLED)}, 1); err != nil {
		t.Fatalf("save register policy: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/user/register", strings.NewReader(`{"username":"pending-user","password":"pass1234","confirm_password":"pass1234","email":"pending@example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	fixture.engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			PendingApproval bool   `json:"pending_approval"`
			Token           string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal register response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 0 || !payload.Data.PendingApproval || payload.Data.Token != "" {
		t.Fatalf("pending register response = %#v, want success pending_approval without token", payload)
	}
	var user model.User
	if err := fixture.db.Where("username = ?", "pending-user").First(&user).Error; err != nil {
		t.Fatalf("query pending user: %v", err)
	}
	if user.Status != model.COMMON_STATUS_DISABLED {
		t.Fatalf("pending user status = %d, want disabled", user.Status)
	}
	var tokenCount int64
	if err := fixture.db.Model(&model.UserToken{}).Where("user_id = ?", user.Id).Count(&tokenCount).Error; err != nil {
		t.Fatalf("count pending user tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("pending registration created %d tokens, want 0", tokenCount)
	}
}

func TestAdminLoginEmailVerificationRequireForLogin(t *testing.T) {
	cases := []struct {
		name             string
		settings         service.EmailVerificationSettings
		verified         bool
		wantCode         int
		wantTokenCreated bool
	}{
		{name: "default off allows unverified", settings: service.DefaultEmailVerificationSettings(), verified: false, wantCode: 0, wantTokenCreated: true},
		{name: "enabled without login requirement allows unverified", settings: service.EmailVerificationSettings{Enabled: true, RequireForLogin: false}, verified: false, wantCode: 0, wantTokenCreated: true},
		{name: "enabled and required denies unverified", settings: service.EmailVerificationSettings{Enabled: true, RequireForLogin: true}, verified: false, wantCode: 101, wantTokenCreated: false},
		{name: "enabled and required allows verified", settings: service.EmailVerificationSettings{Enabled: true, RequireForLogin: true}, verified: true, wantCode: 0, wantTokenCreated: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := setupStage2AdminFixture(t)
			if err := service.AllService.SettingsService.SaveSMTP(service.SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: service.SMTPSecurityStartTLS, FromEmail: "noreply@example.test", FromName: "RustDesk Test", TimeoutSeconds: 10}, 1); err != nil {
				t.Fatalf("save smtp settings: %v", err)
			}
			if err := service.AllService.SettingsService.SaveEmailVerification(tc.settings, 1); err != nil {
				t.Fatalf("save email verification settings: %v", err)
			}
			passwordHash, err := utils.EncryptPassword("secret123")
			if err != nil {
				t.Fatalf("hash password: %v", err)
			}
			user := &model.User{Username: "admin-user", Email: "admin@example.test", Password: passwordHash, Status: model.COMMON_STATUS_ENABLE}
			if tc.verified {
				verifiedAt := custom_types.AutoTime(time.Now())
				user.EmailVerifiedAt = &verifiedAt
			}
			if err := fixture.db.Create(user).Error; err != nil {
				t.Fatalf("create user: %v", err)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"username":"admin-user","password":"secret123"}`))
			request.Header.Set("Content-Type", "application/json")
			fixture.engine.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("login status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			var payload struct {
				Code int    `json:"code"`
				Msg  string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
				t.Fatalf("unmarshal login response: %v; body=%q", err, recorder.Body.String())
			}
			if payload.Code != tc.wantCode {
				t.Fatalf("login code = %d, want %d; body=%q", payload.Code, tc.wantCode, recorder.Body.String())
			}
			var tokenCount int64
			if err := fixture.db.Model(&model.UserToken{}).Where("user_id = ?", user.Id).Count(&tokenCount).Error; err != nil {
				t.Fatalf("count login tokens: %v", err)
			}
			if (tokenCount > 0) != tc.wantTokenCreated {
				t.Fatalf("token created = %v, want %v; body=%q", tokenCount > 0, tc.wantTokenCreated, recorder.Body.String())
			}
		})
	}
}
