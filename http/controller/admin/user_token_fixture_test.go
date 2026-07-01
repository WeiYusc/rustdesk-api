package admin

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
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
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

type adminUserTokenFixture struct {
	db                *gorm.DB
	router            *gin.Engine
	adminToken        string
	nonAdminToken     string
	adminUser         *model.User
	nonAdminUser      *model.User
	nonAdminAuthToken *model.UserToken
	nonAdminTokens    []*model.UserToken
	adminExtraToken   *model.UserToken
}

func setupAdminUserTokenFixture(t *testing.T) adminUserTokenFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}); err != nil {
		t.Fatalf("migrate admin user-token models: %v", err)
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

	adminUser := createAdminUserTokenFixtureUser(t, db, "admin-token-user", true)
	nonAdminUser := createAdminUserTokenFixtureUser(t, db, "normal-token-user", false)
	adminAuthToken := createAdminUserTokenFixtureToken(t, db, adminUser.Id, "admin-token-auth")
	nonAdminAuthToken := createAdminUserTokenFixtureToken(t, db, nonAdminUser.Id, "normal-token-auth")
	nonAdminTokens := []*model.UserToken{
		createAdminUserTokenFixtureToken(t, db, nonAdminUser.Id, "normal-token-old"),
		createAdminUserTokenFixtureToken(t, db, nonAdminUser.Id, "normal-token-new"),
	}
	adminExtraToken := createAdminUserTokenFixtureToken(t, db, adminUser.Id, "admin-token-extra")

	router := gin.New()
	controller := &UserToken{}
	userToken := router.Group("/api/admin/user_token").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	userToken.GET("/list", controller.List)
	userToken.POST("/delete", controller.Delete)
	userToken.POST("/batchDelete", controller.BatchDelete)

	return adminUserTokenFixture{
		db:                db,
		router:            router,
		adminToken:        adminAuthToken.Token,
		nonAdminToken:     nonAdminAuthToken.Token,
		adminUser:         adminUser,
		nonAdminUser:      nonAdminUser,
		nonAdminAuthToken: nonAdminAuthToken,
		nonAdminTokens:    nonAdminTokens,
		adminExtraToken:   adminExtraToken,
	}
}

func createAdminUserTokenFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool) *model.User {
	t.Helper()

	user := &model.User{Username: username, Nickname: username + " nick", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return user
}

func createAdminUserTokenFixtureToken(t *testing.T, db *gorm.DB, userID uint, token string) *model.UserToken {
	t.Helper()

	userToken := &model.UserToken{UserId: userID, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}
	if err := db.Create(userToken).Error; err != nil {
		t.Fatalf("create token %s: %v", token, err)
	}
	return userToken
}

func adminUserTokenRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminUserTokenListRequiresAdminAndFiltersByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserTokenFixture(t)

	unauthenticated := adminUserTokenRequest(fixture.router, http.MethodGet, "/api/admin/user_token/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminUserTokenResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminUserTokenRequest(fixture.router, http.MethodGet, "/api/admin/user_token/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminUserTokenResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminUserTokenRequest(fixture.router, http.MethodGet, "/api/admin/user_token/list?page=1&page_size=10&user_id="+uintToString(fixture.nonAdminUser.Id), "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			UserTokens []struct {
				Id     uint `json:"id"`
				UserId uint `json:"user_id"`
				User   struct {
					Id       uint   `json:"id"`
					Username string `json:"username"`
					Nickname string `json:"nickname"`
				} `json:"user"`
				Token string `json:"token"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal admin list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 3 {
		t.Fatalf("admin list payload = %#v", payload)
	}
	if len(payload.Data.UserTokens) != 3 {
		t.Fatalf("admin list length = %d, want 3", len(payload.Data.UserTokens))
	}
	wantIDs := []uint{fixture.nonAdminTokens[1].Id, fixture.nonAdminTokens[0].Id, fixture.nonAdminAuthToken.Id}
	for index, row := range payload.Data.UserTokens {
		if row.UserId != fixture.nonAdminUser.Id || row.User.Id != fixture.nonAdminUser.Id || row.User.Username != fixture.nonAdminUser.Username || row.User.Nickname != fixture.nonAdminUser.Nickname {
			t.Fatalf("row user summary = %#v, want user %d/%s", row, fixture.nonAdminUser.Id, fixture.nonAdminUser.Username)
		}
		if row.Id != wantIDs[index] {
			t.Fatalf("row ids = %#v, want id desc order %#v", payload.Data.UserTokens, wantIDs)
		}
	}
}

func TestAdminUserTokenDeleteAndBatchDeleteRemoveSelectedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserTokenFixture(t)

	deleteResponse := adminUserTokenRequest(fixture.router, http.MethodPost, "/api/admin/user_token/delete", `{"id":`+uintToString(fixture.nonAdminTokens[0].Id)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminUserTokenResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminUserTokenRowCount(t, fixture.db, fixture.nonAdminTokens[0].Id, 0)
	assertAdminUserTokenRowCount(t, fixture.db, fixture.nonAdminTokens[1].Id, 1)
	assertAdminUserTokenRowCount(t, fixture.db, fixture.adminExtraToken.Id, 1)

	batchResponse := adminUserTokenRequest(fixture.router, http.MethodPost, "/api/admin/user_token/batchDelete", `{"ids":[`+uintToString(fixture.nonAdminTokens[1].Id)+`]}`, fixture.adminToken)
	if batchResponse.Code != http.StatusOK {
		t.Fatalf("batch delete status = %d, want %d; body=%q", batchResponse.Code, http.StatusOK, batchResponse.Body.String())
	}
	assertAdminUserTokenResponseCode(t, batchResponse.Body.Bytes(), 0)
	assertAdminUserTokenRowCount(t, fixture.db, fixture.nonAdminTokens[1].Id, 0)
	assertAdminUserTokenRowCount(t, fixture.db, fixture.adminExtraToken.Id, 1)
}

func assertAdminUserTokenResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminUserTokenRowCount(t *testing.T, db *gorm.DB, id uint, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.UserToken{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count user token: %v", err)
	}
	if count != want {
		t.Fatalf("user token row count = %d, want %d", count, want)
	}
}
