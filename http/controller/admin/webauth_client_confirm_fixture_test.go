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
	apiController "github.com/lejianwen/rustdesk-api/v2/http/controller/api"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/http/router"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebauthClientConfirmFixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite webauth confirm db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Oauth{}, &model.UserThird{}); err != nil {
		t.Fatalf("migrate webauth confirm db: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.Config.Rustdesk.ApiServer = "https://rd.example.test"
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.Jwt = jwt.NewJwt("webauth-confirm-test-secret", 0)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	adminGroup := engine.Group("/api/admin")
	adminGroup.Use(middleware.BackendUserAuth())
	router.OauthBind(adminGroup)
	engine.GET("/api/oidc/auth-query", (&apiController.Oauth{}).OidcAuthQuery)
	return engine, db
}

func TestWebauthClientConfirmAllowsBrowserLoginToReleaseClientToken(t *testing.T) {
	engine, db := setupWebauthClientConfirmFixture(t)
	isAdmin := true
	user := &model.User{Username: "admin", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "browser-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create browser token: %v", err)
	}
	service.AllService.OauthService.SetOauthCache("client-state", &service.OauthCacheItem{
		Action:     service.OauthActionTypeLogin,
		Op:         model.OauthTypeWebauth,
		Id:         "client-id",
		Uuid:       "client-uuid",
		DeviceType: model.LoginLogClientApp,
		DeviceOs:   "windows",
	}, 0)

	confirm := httptest.NewRecorder()
	confirmReq := httptest.NewRequest(http.MethodPost, "/api/admin/oauth/confirm", strings.NewReader(`{"code":"client-state"}`))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmReq.Header.Set("api-token", "browser-token")
	engine.ServeHTTP(confirm, confirmReq)
	assertWebauthConfirmCode(t, confirm, 0)

	query := httptest.NewRecorder()
	queryReq := httptest.NewRequest(http.MethodGet, "/api/oidc/auth-query?code=client-state&id=client-id&uuid=client-uuid", nil)
	engine.ServeHTTP(query, queryReq)
	if query.Code != http.StatusOK {
		t.Fatalf("query status=%d body=%q", query.Code, query.Body.String())
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		Type        string `json:"type"`
		User        struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(query.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal query response: %v; body=%q", err, query.Body.String())
	}
	if payload.Type != "access_token" || payload.AccessToken == "" || payload.User.Name != "admin" {
		t.Fatalf("query payload = %#v", payload)
	}
}

func assertWebauthConfirmCode(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
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
