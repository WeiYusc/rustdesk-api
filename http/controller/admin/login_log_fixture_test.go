package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

type adminLoginLogFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	owner         *model.User
	viewer        *model.User
	loginLogs     []*model.LoginLog
}

func setupAdminLoginLogFixture(t *testing.T) adminLoginLogFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.LoginLog{}); err != nil {
		t.Fatalf("migrate admin login-log models: %v", err)
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

	createAdminLoginLogFixtureUser(t, db, "admin-login-log-user", true, "admin-login-log-token")
	createAdminLoginLogFixtureUser(t, db, "non-admin-login-log-user", false, "non-admin-login-log-token")
	owner := createAdminLoginLogFixtureUser(t, db, "owner-login-log-user", false, "owner-login-log-token")
	viewer := createAdminLoginLogFixtureUser(t, db, "viewer-login-log-user", false, "viewer-login-log-token")

	loginLogs := []*model.LoginLog{
		{UserId: owner.Id, Client: model.LoginLogClientApp, DeviceId: "owner-old", Uuid: "uuid-owner-old", Ip: "192.0.2.10", Type: model.LoginLogTypeAccount, Platform: "linux"},
		{UserId: owner.Id, Client: model.LoginLogClientWeb, DeviceId: "owner-new", Uuid: "uuid-owner-new", Ip: "192.0.2.11", Type: model.LoginLogTypeOauth, Platform: "web"},
		{UserId: viewer.Id, Client: model.LoginLogClientWebAdmin, DeviceId: "viewer-admin", Uuid: "uuid-viewer", Ip: "192.0.2.20", Type: model.LoginLogTypeAccount, Platform: "windows"},
	}
	for _, loginLog := range loginLogs {
		if err := db.Create(loginLog).Error; err != nil {
			t.Fatalf("create seed login log: %v", err)
		}
	}

	router := gin.New()
	controller := &LoginLog{}
	loginLogGroup := router.Group("/api/admin/login_log").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	loginLogGroup.GET("/list", controller.List)
	loginLogGroup.POST("/delete", controller.Delete)
	loginLogGroup.POST("/batchDelete", controller.BatchDelete)

	return adminLoginLogFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-login-log-token",
		nonAdminToken: "non-admin-login-log-token",
		owner:         owner,
		viewer:        viewer,
		loginLogs:     loginLogs,
	}
}

func createAdminLoginLogFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
	t.Helper()

	user := &model.User{Username: username, Nickname: username + " nick", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create token %s: %v", token, err)
	}
	return user
}

func adminLoginLogRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminLoginLogRoutesRequireAdminAndListFiltersByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminLoginLogFixture(t)

	unauthenticated := adminLoginLogRequest(fixture.router, http.MethodGet, "/api/admin/login_log/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminLoginLogResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminLoginLogRequest(fixture.router, http.MethodGet, "/api/admin/login_log/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminLoginLogResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminLoginLogRequest(fixture.router, http.MethodGet, "/api/admin/login_log/list?page=1&page_size=10&user_id="+strconv.FormatUint(uint64(fixture.owner.Id), 10), "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page      int `json:"page"`
			PageSize  int `json:"page_size"`
			Total     int `json:"total"`
			LoginLogs []struct {
				Id     uint `json:"id"`
				UserId uint `json:"user_id"`
				User   struct {
					Id       uint   `json:"id"`
					Username string `json:"username"`
					Nickname string `json:"nickname"`
				} `json:"user"`
				Client   string `json:"client"`
				DeviceId string `json:"device_id"`
				Uuid     string `json:"uuid"`
				Ip       string `json:"ip"`
				Type     string `json:"type"`
				Platform string `json:"platform"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal login-log list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("login-log list payload = %#v", payload)
	}
	if len(payload.Data.LoginLogs) != 2 {
		t.Fatalf("login-log list length = %d, want 2", len(payload.Data.LoginLogs))
	}
	if payload.Data.LoginLogs[0].Id != fixture.loginLogs[1].Id || payload.Data.LoginLogs[0].DeviceId != "owner-new" || payload.Data.LoginLogs[1].Id != fixture.loginLogs[0].Id || payload.Data.LoginLogs[1].DeviceId != "owner-old" {
		t.Fatalf("login-log order = %#v", payload.Data.LoginLogs)
	}
	if payload.Data.LoginLogs[0].UserId != fixture.owner.Id || payload.Data.LoginLogs[0].User.Id != fixture.owner.Id || payload.Data.LoginLogs[0].User.Username != fixture.owner.Username || payload.Data.LoginLogs[0].User.Nickname != fixture.owner.Nickname || payload.Data.LoginLogs[0].Client != model.LoginLogClientWeb || payload.Data.LoginLogs[0].Uuid != "uuid-owner-new" || payload.Data.LoginLogs[0].Ip != "192.0.2.11" || payload.Data.LoginLogs[0].Type != model.LoginLogTypeOauth || payload.Data.LoginLogs[0].Platform != "web" {
		t.Fatalf("login-log first row = %#v", payload.Data.LoginLogs[0])
	}
}

func TestAdminLoginLogDeleteAndBatchDeleteRemoveSelectedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminLoginLogFixture(t)

	deleteResponse := adminLoginLogRequest(fixture.router, http.MethodPost, "/api/admin/login_log/delete", `{"id":`+strconv.FormatUint(uint64(fixture.loginLogs[0].Id), 10)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminLoginLogResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminLoginLogRowCount(t, fixture.db, "id = ?", []any{fixture.loginLogs[0].Id}, 0)
	assertAdminLoginLogRowCount(t, fixture.db, "id = ?", []any{fixture.loginLogs[1].Id}, 1)
	assertAdminLoginLogRowCount(t, fixture.db, "id = ?", []any{fixture.loginLogs[2].Id}, 1)

	batchDelete := adminLoginLogRequest(fixture.router, http.MethodPost, "/api/admin/login_log/batchDelete", `{"ids":[`+strconv.FormatUint(uint64(fixture.loginLogs[1].Id), 10)+`]}`, fixture.adminToken)
	if batchDelete.Code != http.StatusOK {
		t.Fatalf("batch delete status = %d, want %d; body=%q", batchDelete.Code, http.StatusOK, batchDelete.Body.String())
	}
	assertAdminLoginLogResponseCode(t, batchDelete.Body.Bytes(), 0)
	assertAdminLoginLogRowCount(t, fixture.db, "id = ?", []any{fixture.loginLogs[1].Id}, 0)
	assertAdminLoginLogRowCount(t, fixture.db, "id = ?", []any{fixture.loginLogs[2].Id}, 1)
}

func assertAdminLoginLogResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminLoginLogRowCount(t *testing.T, db *gorm.DB, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.LoginLog{}).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count login logs: %v", err)
	}
	if count != want {
		t.Fatalf("login-log row count = %d, want %d", count, want)
	}
}
