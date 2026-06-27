package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
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

type adminSecurityFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	adminUser     *model.User
	nonAdminUser  *model.User
}

func setupAdminSecurityFixture(t *testing.T) adminSecurityFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserToken{},
		&model.LoginLog{},
		&model.AddressBook{},
		&model.AddressBookCollection{},
		&model.Group{},
		&model.Peer{},
		&model.Oauth{},
		&model.ServerCmd{},
	); err != nil {
		t.Fatalf("migrate security fixture models: %v", err)
	}

	global.Config = config.Config{Lang: "en"}
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer {
		return i18n.NewLocalizer(i18n.NewBundle(language.English))
	}
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	global.Jwt = jwt.NewJwt("", 0)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)

	adminUser := createAdminSecurityFixtureUser(t, db, "security-admin", true, "security-admin-token")
	nonAdminUser := createAdminSecurityFixtureUser(t, db, "security-user", false, "security-user-token")

	if err := db.Create(&model.AddressBookCollection{UserId: adminUser.Id, Name: "admin collection"}).Error; err != nil {
		t.Fatalf("create admin collection: %v", err)
	}
	if err := db.Create(&model.Group{Name: "security group"}).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.Create(&model.Peer{Id: "security-peer", Uuid: "security-uuid", Version: "9.9.9", UserId: adminUser.Id, Alias: "security"}).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if err := db.Create(&model.Oauth{Op: "oidc", OauthType: model.OauthTypeOidc, ClientId: "client-id", ClientSecret: "super-secret", Issuer: "https://issuer.example.test"}).Error; err != nil {
		t.Fatalf("create oauth: %v", err)
	}

	router := gin.New()
	Init(router)

	return adminSecurityFixture{
		db:            db,
		router:        router,
		adminToken:    "security-admin-token",
		nonAdminToken: "security-user-token",
		adminUser:     adminUser,
		nonAdminUser:  nonAdminUser,
	}
}

func createAdminSecurityFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
	t.Helper()
	user := &model.User{Username: username, Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create token %s: %v", token, err)
	}
	return user
}

func adminSecurityRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminSecuritySensitiveRoutesRequireAdminPrivilege(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminSecurityFixture(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "rustdesk cmd list", method: http.MethodGet, path: "/api/admin/rustdesk/cmdList?page=1&page_size=10"},
		{name: "rustdesk cmd create", method: http.MethodPost, path: "/api/admin/rustdesk/cmdCreate", body: `{"cmd":"allowlist","target":"id-server"}`},
		{name: "user groupUsers", method: http.MethodPost, path: "/api/admin/user/groupUsers", body: `{}`},
		{name: "peer simpleData", method: http.MethodPost, path: "/api/admin/peer/simpleData", body: `{"ids":["security-peer"]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nonAdmin := adminSecurityRequest(fixture.router, tc.method, tc.path, tc.body, fixture.nonAdminToken)
			if nonAdmin.Code != http.StatusOK {
				t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
			}
			assertAdminSecurityResponseCode(t, nonAdmin.Body.Bytes(), 403)
		})
	}
}

func TestAdminSecurityMyAddressBookCreateIgnoresSubmittedUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminSecurityFixture(t)

	response := adminSecurityRequest(fixture.router, http.MethodPost, "/api/admin/my/address_book/create", `{"id":"evil-peer","user_id":1,"tags":[]}`, fixture.nonAdminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
	}
	assertAdminSecurityResponseCode(t, response.Body.Bytes(), 0)

	var created model.AddressBook
	if err := fixture.db.Where("id = ?", "evil-peer").First(&created).Error; err != nil {
		t.Fatalf("find address book: %v", err)
	}
	if created.UserId != fixture.nonAdminUser.Id {
		t.Fatalf("created address book user_id = %d, want current user %d", created.UserId, fixture.nonAdminUser.Id)
	}
}

func TestAdminSecurityOauthResponsesDoNotExposeClientSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminSecurityFixture(t)

	list := adminSecurityRequest(fixture.router, http.MethodGet, "/api/admin/oauth/list?page=1&page_size=10", "", fixture.adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%q", list.Code, http.StatusOK, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "super-secret") {
		t.Fatalf("oauth list leaked client_secret value: %s", list.Body.String())
	}

	detail := adminSecurityRequest(fixture.router, http.MethodGet, "/api/admin/oauth/detail/1", "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	if strings.Contains(detail.Body.String(), "super-secret") {
		t.Fatalf("oauth detail leaked client_secret value: %s", detail.Body.String())
	}
}

func TestAdminSecurityLogoutDeletesCurrentToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminSecurityFixture(t)

	logout := adminSecurityRequest(fixture.router, http.MethodPost, "/api/admin/logout", `{}`, fixture.nonAdminToken)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d; body=%q", logout.Code, http.StatusOK, logout.Body.String())
	}
	assertAdminSecurityResponseCode(t, logout.Body.Bytes(), 0)

	var count int64
	if err := fixture.db.Model(&model.UserToken{}).Where("token = ?", fixture.nonAdminToken).Count(&count).Error; err != nil {
		t.Fatalf("count token: %v", err)
	}
	if count != 0 {
		t.Fatalf("logout left %d token row(s), want 0", count)
	}
}

func assertAdminSecurityResponseCode(t *testing.T, body []byte, want int) {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, string(body))
	}
	if payload.Code != want {
		t.Fatalf("response code = %d, want %d; body=%q", payload.Code, want, string(body))
	}
}
