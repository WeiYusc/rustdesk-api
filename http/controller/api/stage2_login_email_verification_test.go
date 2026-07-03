package api_test

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
	controller "github.com/lejianwen/rustdesk-api/v2/http/controller/api"
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

type stage2APILoginFixture struct {
	router *gin.Engine
	db     *gorm.DB
}

func setupStage2APILoginFixture(t *testing.T) stage2APILoginFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite api login db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}, &model.Oauth{}, &model.User{}, &model.UserToken{}, &model.LoginLog{}, &model.UserPasskey{}); err != nil {
		t.Fatalf("migrate api login models: %v", err)
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
	global.ApiInitValidator()
	global.LoginLimiter = nil
	global.Jwt = jwt.NewJwt("stage2-api-secret", time.Hour)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/login", (&controller.Login{}).Login)
	return stage2APILoginFixture{router: router, db: db}
}

func TestAPILoginEmailVerificationRequireForLogin(t *testing.T) {
	cases := []struct {
		name             string
		settings         service.EmailVerificationSettings
		verified         bool
		wantStatus       int
		wantTokenCreated bool
	}{
		{name: "default off allows unverified", settings: service.DefaultEmailVerificationSettings(), verified: false, wantStatus: http.StatusOK, wantTokenCreated: true},
		{name: "enabled without login requirement allows unverified", settings: service.EmailVerificationSettings{Enabled: true, RequireForLogin: false}, verified: false, wantStatus: http.StatusOK, wantTokenCreated: true},
		{name: "enabled and required denies unverified", settings: service.EmailVerificationSettings{Enabled: true, RequireForLogin: true}, verified: false, wantStatus: http.StatusBadRequest, wantTokenCreated: false},
		{name: "enabled and required allows verified", settings: service.EmailVerificationSettings{Enabled: true, RequireForLogin: true}, verified: true, wantStatus: http.StatusOK, wantTokenCreated: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := setupStage2APILoginFixture(t)
			if err := service.AllService.SettingsService.SaveEmailVerification(tc.settings, 1); err != nil {
				t.Fatalf("save email verification settings: %v", err)
			}
			passwordHash, err := utils.EncryptPassword("secret123")
			if err != nil {
				t.Fatalf("hash password: %v", err)
			}
			user := &model.User{Username: "api-user", Email: "api@example.test", Password: passwordHash, Status: model.COMMON_STATUS_ENABLE}
			if tc.verified {
				verifiedAt := custom_types.AutoTime(time.Now())
				user.EmailVerifiedAt = &verifiedAt
			}
			if err := fixture.db.Create(user).Error; err != nil {
				t.Fatalf("create api user: %v", err)
			}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"api-user","password":"secret123","id":"dev-id","uuid":"dev-uuid"}`))
			request.Header.Set("Content-Type", "application/json")
			fixture.router.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantStatus {
				t.Fatalf("api login status = %d, want %d; body=%q", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest {
				var payload struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
					t.Fatalf("unmarshal error response: %v; body=%q", err, recorder.Body.String())
				}
				if payload.Error == "" {
					t.Fatalf("error response missing error message: %q", recorder.Body.String())
				}
			}
			var tokenCount int64
			if err := fixture.db.Model(&model.UserToken{}).Where("user_id = ?", user.Id).Count(&tokenCount).Error; err != nil {
				t.Fatalf("count api login tokens: %v", err)
			}
			if (tokenCount > 0) != tc.wantTokenCreated {
				t.Fatalf("token created = %v, want %v; body=%q", tokenCount > 0, tc.wantTokenCreated, recorder.Body.String())
			}
		})
	}
}
