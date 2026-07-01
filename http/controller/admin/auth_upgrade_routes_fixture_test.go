package admin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

func setupAdminAuthUpgradeRouteFixture(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite auth upgrade route db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Setting{}, &model.UserPasskey{}, &model.AuthChallenge{}); err != nil {
		t.Fatalf("migrate auth upgrade route db: %v", err)
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
	router.PasskeyBind(group)
	router.EmailBind(group)
	return engine, db
}

func TestAdminPasskeyLoginBeginDisabledByDefault(t *testing.T) {
	engine, _ := setupAdminAuthUpgradeRouteFixture(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/login/begin", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	assertAuthUpgradeRouteCode(t, recorder, 101)
	if !strings.Contains(recorder.Body.String(), "PasskeyDisabled") {
		t.Fatalf("passkey disabled response body = %q", recorder.Body.String())
	}
}

func TestAdminPasskeyLoginBeginReturnsRequestOptionsWhenEnabled(t *testing.T) {
	engine, db := setupAdminAuthUpgradeRouteFixture(t)
	if err := service.AllService.SettingsService.SavePasskey(service.PasskeySettings{
		Enabled:                  true,
		RPName:                   "RustDesk Test Admin",
		RPID:                     "rd.plumire.cyou",
		AllowedOrigins:           []string{"https://rd.plumire.cyou"},
		UserVerification:         service.UserVerificationPreferred,
		DiscoverableLoginEnabled: true,
		ResidentKeyRequirement:   service.ResidentKeyRequired,
	}, 1); err != nil {
		t.Fatalf("save passkey settings: %v", err)
	}
	if err := db.Create(&model.UserPasskey{UserId: 1, Name: "test key", CredentialID: "credential-id", UserHandle: "stable-handle", PublicKey: "public-key"}).Error; err != nil {
		t.Fatalf("create passkey: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/login/begin", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	assertAuthUpgradeRouteCode(t, recorder, 0)
	if !strings.Contains(recorder.Body.String(), `"rpId":"rd.plumire.cyou"`) || strings.Contains(recorder.Body.String(), "PasskeyDomainVerificationPending") {
		t.Fatalf("login begin body = %q", recorder.Body.String())
	}

	finish := httptest.NewRecorder()
	finishReq := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/login/finish", strings.NewReader(`{"challenge":"bad"}`))
	finishReq.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(finish, finishReq)
	assertAuthUpgradeRouteCode(t, finish, 101)
	if !strings.Contains(finish.Body.String(), "PasskeyVerificationFailed") {
		t.Fatalf("login finish invalid body = %q", finish.Body.String())
	}
}

func TestAdminPasskeyRegisterBeginRequiresLoginAndReturnsCreationOptions(t *testing.T) {
	engine, db := setupAdminAuthUpgradeRouteFixture(t)
	if err := service.AllService.SettingsService.SavePasskey(service.PasskeySettings{
		Enabled:                  true,
		RPName:                   "RustDesk Test Admin",
		RPID:                     "rd.plumire.cyou",
		AllowedOrigins:           []string{"https://rd.plumire.cyou"},
		UserVerification:         service.UserVerificationPreferred,
		DiscoverableLoginEnabled: true,
		ResidentKeyRequirement:   service.ResidentKeyRequired,
	}, 1); err != nil {
		t.Fatalf("save passkey settings: %v", err)
	}
	isAdmin := true
	user := &model.User{Username: "admin", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "admin-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create admin token: %v", err)
	}

	unauth := httptest.NewRecorder()
	unauthReq := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/register/begin", strings.NewReader(`{}`))
	unauthReq.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(unauth, unauthReq)
	assertAuthUpgradeRouteCode(t, unauth, 403)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/register/begin", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("api-token", "admin-token")
	engine.ServeHTTP(recorder, request)
	assertAuthUpgradeRouteCode(t, recorder, 0)
	if !strings.Contains(recorder.Body.String(), `"residentKey":"required"`) || !strings.Contains(recorder.Body.String(), `"rp":{"id":"rd.plumire.cyou"`) {
		t.Fatalf("register begin body = %q", recorder.Body.String())
	}

	finish := httptest.NewRecorder()
	finishReq := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/register/finish", strings.NewReader(`{"challenge":"bad"}`))
	finishReq.Header.Set("Content-Type", "application/json")
	finishReq.Header.Set("api-token", "admin-token")
	engine.ServeHTTP(finish, finishReq)
	assertAuthUpgradeRouteCode(t, finish, 101)
	if !strings.Contains(finish.Body.String(), "PasskeyVerificationFailed") {
		t.Fatalf("register finish invalid body = %q", finish.Body.String())
	}
}

func TestAdminPasskeyListRenameDeleteUseCurrentUserCredentials(t *testing.T) {
	engine, db := setupAdminAuthUpgradeRouteFixture(t)
	isAdmin := true
	user := &model.User{Username: "admin", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	other := &model.User{Username: "other", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "admin-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create admin token: %v", err)
	}
	ownKey := &model.UserPasskey{UserId: user.Id, Name: "Laptop", CredentialID: "credential-own", UserHandle: "stable-handle", PublicKey: "public-key"}
	otherKey := &model.UserPasskey{UserId: other.Id, Name: "Other", CredentialID: "credential-other", UserHandle: "other-handle", PublicKey: "public-key"}
	if err := db.Create(ownKey).Error; err != nil {
		t.Fatalf("create own passkey: %v", err)
	}
	if err := db.Create(otherKey).Error; err != nil {
		t.Fatalf("create other passkey: %v", err)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/passkey/list", nil)
	listReq.Header.Set("api-token", "admin-token")
	engine.ServeHTTP(list, listReq)
	assertAuthUpgradeRouteCode(t, list, 0)
	body := list.Body.String()
	if !strings.Contains(body, "Laptop") || strings.Contains(body, "Other") {
		t.Fatalf("list body = %q", body)
	}

	rename := httptest.NewRecorder()
	renameReq := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/rename", strings.NewReader(`{"id":`+strconv.Itoa(int(ownKey.Id))+`,"name":"Phone"}`))
	renameReq.Header.Set("Content-Type", "application/json")
	renameReq.Header.Set("api-token", "admin-token")
	engine.ServeHTTP(rename, renameReq)
	assertAuthUpgradeRouteCode(t, rename, 0)
	var renamed model.UserPasskey
	if err := db.First(&renamed, ownKey.Id).Error; err != nil {
		t.Fatalf("load renamed passkey: %v", err)
	}
	if renamed.Name != "Phone" {
		t.Fatalf("renamed passkey name = %q", renamed.Name)
	}

	deleteOther := httptest.NewRecorder()
	deleteOtherReq := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/delete", strings.NewReader(`{"id":`+strconv.Itoa(int(otherKey.Id))+`}`))
	deleteOtherReq.Header.Set("Content-Type", "application/json")
	deleteOtherReq.Header.Set("api-token", "admin-token")
	engine.ServeHTTP(deleteOther, deleteOtherReq)
	assertAuthUpgradeRouteCode(t, deleteOther, 101)

	deleteOwn := httptest.NewRecorder()
	deleteOwnReq := httptest.NewRequest(http.MethodPost, "/api/admin/passkey/delete", strings.NewReader(`{"id":`+strconv.Itoa(int(ownKey.Id))+`}`))
	deleteOwnReq.Header.Set("Content-Type", "application/json")
	deleteOwnReq.Header.Set("api-token", "admin-token")
	engine.ServeHTTP(deleteOwn, deleteOwnReq)
	assertAuthUpgradeRouteCode(t, deleteOwn, 0)
	var count int64
	if err := db.Model(&model.UserPasskey{}).Where("id = ?", ownKey.Id).Count(&count).Error; err != nil {
		t.Fatalf("count deleted passkey: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted passkey count = %d", count)
	}
}

func TestAdminEmailVerificationRoutesRequireLogin(t *testing.T) {
	engine, _ := setupAdminAuthUpgradeRouteFixture(t)
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
