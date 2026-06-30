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

type adminGroupFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	seedGroups    []*model.Group
}

func setupAdminGroupFixture(t *testing.T) adminGroupFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Group{}); err != nil {
		t.Fatalf("migrate admin group models: %v", err)
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

	createAdminGroupFixtureUser(t, db, "admin-group-user", true, "admin-group-token")
	createAdminGroupFixtureUser(t, db, "non-admin-group-user", false, "non-admin-group-token")
	seedGroups := []*model.Group{
		{Name: "Default Devices", Type: model.GroupTypeDefault},
		{Name: "Shared Devices", Type: model.GroupTypeShare},
	}
	for _, group := range seedGroups {
		if err := db.Create(group).Error; err != nil {
			t.Fatalf("create seed group: %v", err)
		}
	}

	router := gin.New()
	controller := &Group{}
	group := router.Group("/api/admin/group").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	group.GET("/list", controller.List)
	group.GET("/detail/:id", controller.Detail)
	group.POST("/create", controller.Create)
	group.POST("/update", controller.Update)
	group.POST("/delete", controller.Delete)

	return adminGroupFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-group-token",
		nonAdminToken: "non-admin-group-token",
		seedGroups:    seedGroups,
	}
}

func createAdminGroupFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) {
	t.Helper()

	user := &model.User{Username: username, Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create token %s: %v", token, err)
	}
}

func adminGroupRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminGroupRoutesRequireAdminAndListGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminGroupFixture(t)

	unauthenticated := adminGroupRequest(fixture.router, http.MethodGet, "/api/admin/group/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminGroupResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminGroupRequest(fixture.router, http.MethodGet, "/api/admin/group/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminGroupResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminGroupRequest(fixture.router, http.MethodGet, "/api/admin/group/list?page=1&page_size=10", "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Groups   []struct {
				Id   uint   `json:"id"`
				Name string `json:"name"`
				Type int    `json:"type"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal admin list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("admin list payload = %#v", payload)
	}
	if len(payload.Data.Groups) != 2 {
		t.Fatalf("admin list length = %d, want 2", len(payload.Data.Groups))
	}
	if payload.Data.Groups[0].Id != fixture.seedGroups[0].Id || payload.Data.Groups[1].Id != fixture.seedGroups[1].Id {
		t.Fatalf("admin list groups = %#v, want seeded insertion order", payload.Data.Groups)
	}
}

func TestAdminGroupDetailRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminGroupFixture(t)

	invalidDetail := adminGroupRequest(fixture.router, http.MethodGet, "/api/admin/group/detail/not-a-number", "", fixture.adminToken)
	if invalidDetail.Code != http.StatusOK {
		t.Fatalf("invalid detail status = %d, want %d; body=%q", invalidDetail.Code, http.StatusOK, invalidDetail.Body.String())
	}
	assertAdminGroupResponseCode(t, invalidDetail.Body.Bytes(), 101)
	if strings.Contains(invalidDetail.Body.String(), "ItemNotFound") {
		t.Fatalf("invalid detail returned not-found instead of params error: body=%q", invalidDetail.Body.String())
	}
}

func TestAdminGroupListReportsDefaultPaginationMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminGroupFixture(t)

	adminList := adminGroupRequest(fixture.router, http.MethodGet, "/api/admin/group/list", "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Groups   []struct {
				Id uint `json:"id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal default pagination list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("default pagination payload = %#v", payload)
	}
	if len(payload.Data.Groups) != 2 {
		t.Fatalf("default pagination list length = %d, want 2", len(payload.Data.Groups))
	}
}

func TestAdminGroupCreateDetailUpdateAndDeleteSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminGroupFixture(t)

	create := adminGroupRequest(fixture.router, http.MethodPost, "/api/admin/group/create", `{"name":"QA Devices","type":2}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminGroupResponseCode(t, create.Body.Bytes(), 0)

	var created model.Group
	if err := fixture.db.Where("name = ?", "QA Devices").First(&created).Error; err != nil {
		t.Fatalf("find created group: %v", err)
	}
	if created.Type != model.GroupTypeShare {
		t.Fatalf("created group type = %d, want %d", created.Type, model.GroupTypeShare)
	}

	detail := adminGroupRequest(fixture.router, http.MethodGet, "/api/admin/group/detail/"+strconv.FormatUint(uint64(created.Id), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			Id   uint   `json:"id"`
			Name string `json:"name"`
			Type int    `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.Id != created.Id || detailPayload.Data.Name != created.Name || detailPayload.Data.Type != created.Type {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	update := adminGroupRequest(fixture.router, http.MethodPost, "/api/admin/group/update", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"name":"QA Devices Updated","type":1}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminGroupResponseCode(t, update.Body.Bytes(), 0)
	var updated model.Group
	if err := fixture.db.Where("id = ?", created.Id).First(&updated).Error; err != nil {
		t.Fatalf("find updated group: %v", err)
	}
	if updated.Name != "QA Devices Updated" || updated.Type != model.GroupTypeDefault {
		t.Fatalf("updated group = %#v", updated)
	}

	deleteResponse := adminGroupRequest(fixture.router, http.MethodPost, "/api/admin/group/delete", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"name":"ignored"}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminGroupResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminGroupRowCount(t, fixture.db, created.Id, 0)
	assertAdminGroupRowCount(t, fixture.db, fixture.seedGroups[0].Id, 1)
	assertAdminGroupRowCount(t, fixture.db, fixture.seedGroups[1].Id, 1)
}

func assertAdminGroupResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminGroupRowCount(t *testing.T, db *gorm.DB, id uint, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.Group{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count group: %v", err)
	}
	if count != want {
		t.Fatalf("group row count = %d, want %d", count, want)
	}
}
