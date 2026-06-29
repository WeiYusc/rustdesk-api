package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

type adminUserFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	adminUser     *model.User
	nonAdminUser  *model.User
}

func setupAdminUserFixture(t *testing.T) adminUserFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.LoginLog{}); err != nil {
		t.Fatalf("migrate admin user models: %v", err)
	}

	global.Config = config.Config{Lang: "en"}
	global.Config.App.Register = true
	global.Config.App.RegisterStatus = int(model.COMMON_STATUS_ENABLE)
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer {
		return i18n.NewLocalizer(i18n.NewBundle(language.English))
	}
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	global.Jwt = jwt.NewJwt("", 0)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)

	adminUser := createAdminUserFixtureUser(t, db, "admin-current", "admin@example.test", "Admin Current", true, "admin-current-token")
	nonAdminUser := createAdminUserFixtureUser(t, db, "user-current", "user@example.test", "User Current", false, "user-current-token")

	router := gin.New()
	controller := &User{}
	router.POST("/api/admin/user/register", controller.Register)
	user := router.Group("/api/admin/user").Use(middleware.BackendUserAuth())
	user.GET("/current", controller.Current)
	user.POST("/changeCurInfo", controller.ChangeCurInfo)
	user.GET("/list", middleware.AdminPrivilege(), controller.List)

	return adminUserFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-current-token",
		nonAdminToken: "user-current-token",
		adminUser:     adminUser,
		nonAdminUser:  nonAdminUser,
	}
}

func createAdminUserFixtureUser(t *testing.T, db *gorm.DB, username string, email string, nickname string, isAdmin bool, token string) *model.User {
	t.Helper()

	user := &model.User{
		Username: username,
		Email:    email,
		Nickname: nickname,
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create token %s: %v", token, err)
	}
	return user
}

func adminUserRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminUserRegisterRejectsMismatchedConfirmPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserFixture(t)

	response := adminUserRequest(fixture.router, http.MethodPost, "/api/admin/user/register", `{"username":"confirm-mismatch","email":"confirm@example.test","password":"pass1234","confirm_password":"different123"}`, "")
	if response.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
	}
	assertAdminUserResponseCode(t, response.Body.Bytes(), 101)

	var users []model.User
	if err := fixture.db.Where("username = ?", "confirm-mismatch").Find(&users).Error; err != nil {
		t.Fatalf("query registered user: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("mismatched confirm_password created users = %#v", users)
	}
}

func TestAdminUserRegisterAcceptsMatchingConfirmPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserFixture(t)

	response := adminUserRequest(fixture.router, http.MethodPost, "/api/admin/user/register", `{"username":"confirm-match","email":"match@example.test","password":"pass1234","confirm_password":"pass1234"}`, "")
	if response.Code != http.StatusOK {
		t.Fatalf("register status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Username   string   `json:"username"`
			Email      string   `json:"email"`
			Token      string   `json:"token"`
			RouteNames []string `json:"route_names"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal register response: %v; body=%q", err, response.Body.String())
	}
	if payload.Code != 0 || payload.Data.Username != "confirm-match" || payload.Data.Email != "match@example.test" || payload.Data.Token == "" {
		t.Fatalf("register payload = %#v", payload)
	}
	if !reflect.DeepEqual(payload.Data.RouteNames, model.UserRouteNames) {
		t.Fatalf("registered route_names = %#v, want %#v", payload.Data.RouteNames, model.UserRouteNames)
	}
}

func TestAdminUserCurrentUsesBackendAuthAndReturnsRoleRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserFixture(t)

	unauthenticated := adminUserRequest(fixture.router, http.MethodGet, "/api/admin/user/current", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminUserResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminUserRequest(fixture.router, http.MethodGet, "/api/admin/user/current", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin current status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	var nonAdminPayload struct {
		Code int `json:"code"`
		Data struct {
			Username   string   `json:"username"`
			Email      string   `json:"email"`
			Nickname   string   `json:"nickname"`
			Token      string   `json:"token"`
			RouteNames []string `json:"route_names"`
		} `json:"data"`
	}
	if err := json.Unmarshal(nonAdmin.Body.Bytes(), &nonAdminPayload); err != nil {
		t.Fatalf("unmarshal non-admin current: %v; body=%q", err, nonAdmin.Body.String())
	}
	if nonAdminPayload.Code != 0 || nonAdminPayload.Data.Username != fixture.nonAdminUser.Username || nonAdminPayload.Data.Email != fixture.nonAdminUser.Email || nonAdminPayload.Data.Nickname != fixture.nonAdminUser.Nickname || nonAdminPayload.Data.Token != fixture.nonAdminToken {
		t.Fatalf("non-admin current payload = %#v", nonAdminPayload)
	}
	if len(nonAdminPayload.Data.RouteNames) != len(model.UserRouteNames) || nonAdminPayload.Data.RouteNames[0] != model.UserRouteNames[0] {
		t.Fatalf("non-admin route_names = %#v, want user route names", nonAdminPayload.Data.RouteNames)
	}

	adminCurrent := adminUserRequest(fixture.router, http.MethodGet, "/api/admin/user/current", "", fixture.adminToken)
	if adminCurrent.Code != http.StatusOK {
		t.Fatalf("admin current status = %d, want %d; body=%q", adminCurrent.Code, http.StatusOK, adminCurrent.Body.String())
	}
	var adminPayload struct {
		Code int `json:"code"`
		Data struct {
			Username   string   `json:"username"`
			Token      string   `json:"token"`
			RouteNames []string `json:"route_names"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminCurrent.Body.Bytes(), &adminPayload); err != nil {
		t.Fatalf("unmarshal admin current: %v; body=%q", err, adminCurrent.Body.String())
	}
	if adminPayload.Code != 0 || adminPayload.Data.Username != fixture.adminUser.Username || adminPayload.Data.Token != fixture.adminToken {
		t.Fatalf("admin current payload = %#v", adminPayload)
	}
	if len(adminPayload.Data.RouteNames) != 1 || adminPayload.Data.RouteNames[0] != "*" {
		t.Fatalf("admin route_names = %#v, want wildcard", adminPayload.Data.RouteNames)
	}
}

func TestAdminUserListRequiresAdminPrivilegeAndReturnsUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserFixture(t)

	nonAdmin := adminUserRequest(fixture.router, http.MethodGet, "/api/admin/user/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin list status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminUserResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminUserRequest(fixture.router, http.MethodGet, "/api/admin/user/list?page=1&page_size=10&username=current", "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var listPayload struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Users    []struct {
				Id       uint   `json:"id"`
				Username string `json:"username"`
				Email    string `json:"email"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("unmarshal admin list: %v; body=%q", err, adminList.Body.String())
	}
	if listPayload.Code != 0 || listPayload.Data.Page != 1 || listPayload.Data.PageSize != 10 || listPayload.Data.Total != 2 {
		t.Fatalf("admin list payload = %#v", listPayload)
	}
	if len(listPayload.Data.Users) != 2 {
		t.Fatalf("admin list length = %d, want 2", len(listPayload.Data.Users))
	}
	seen := map[string]bool{}
	for _, user := range listPayload.Data.Users {
		seen[user.Username] = true
	}
	if !seen[fixture.adminUser.Username] || !seen[fixture.nonAdminUser.Username] {
		t.Fatalf("admin list users = %#v, want seeded users", listPayload.Data.Users)
	}
}

func TestAdminUserChangeCurInfoUpdatesOnlyCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserFixture(t)

	response := adminUserRequest(fixture.router, http.MethodPost, "/api/admin/user/changeCurInfo", `{"nickname":"Updated User","avatar":"https://example.test/avatar.png","email":"updated@example.test"}`, fixture.nonAdminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("changeCurInfo status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
	}
	assertAdminUserResponseCode(t, response.Body.Bytes(), 0)

	var updated model.User
	if err := fixture.db.First(&updated, fixture.nonAdminUser.Id).Error; err != nil {
		t.Fatalf("query updated current user: %v", err)
	}
	if updated.Nickname != "Updated User" || updated.Avatar != "https://example.test/avatar.png" || updated.Email != "updated@example.test" {
		t.Fatalf("updated current user nickname/avatar/email = %q/%q/%q", updated.Nickname, updated.Avatar, updated.Email)
	}

	var adminUser model.User
	if err := fixture.db.First(&adminUser, fixture.adminUser.Id).Error; err != nil {
		t.Fatalf("query admin user: %v", err)
	}
	if adminUser.Nickname != fixture.adminUser.Nickname || adminUser.Avatar != fixture.adminUser.Avatar {
		t.Fatalf("changeCurInfo modified another user: %#v", adminUser)
	}
}

func assertAdminUserResponseCode(t *testing.T, body []byte, want int) {
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
