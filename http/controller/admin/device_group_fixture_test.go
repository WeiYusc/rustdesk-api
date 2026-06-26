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

type adminDeviceGroupFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	deviceGroups  []*model.DeviceGroup
}

func setupAdminDeviceGroupFixture(t *testing.T) adminDeviceGroupFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.DeviceGroup{}); err != nil {
		t.Fatalf("migrate admin device-group models: %v", err)
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

	createAdminDeviceGroupFixtureUser(t, db, "admin-device-group-user", true, "admin-device-group-token")
	createAdminDeviceGroupFixtureUser(t, db, "non-admin-device-group-user", false, "non-admin-device-group-token")

	deviceGroups := []*model.DeviceGroup{
		{Name: "ops-devices"},
		{Name: "lab-devices"},
	}
	for _, deviceGroup := range deviceGroups {
		if err := db.Create(deviceGroup).Error; err != nil {
			t.Fatalf("create seed device group: %v", err)
		}
	}

	router := gin.New()
	controller := &DeviceGroup{}
	deviceGroup := router.Group("/api/admin/device_group").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	deviceGroup.GET("/list", controller.List)
	deviceGroup.GET("/detail/:id", controller.Detail)
	deviceGroup.POST("/create", controller.Create)
	deviceGroup.POST("/update", controller.Update)
	deviceGroup.POST("/delete", controller.Delete)

	return adminDeviceGroupFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-device-group-token",
		nonAdminToken: "non-admin-device-group-token",
		deviceGroups:  deviceGroups,
	}
}

func createAdminDeviceGroupFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
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

func adminDeviceGroupRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminDeviceGroupRoutesRequireAdminAndListGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminDeviceGroupFixture(t)

	unauthenticated := adminDeviceGroupRequest(fixture.router, http.MethodGet, "/api/admin/device_group/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminDeviceGroupResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminDeviceGroupRequest(fixture.router, http.MethodGet, "/api/admin/device_group/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminDeviceGroupResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminDeviceGroupRequest(fixture.router, http.MethodGet, "/api/admin/device_group/list?page=1&page_size=10", "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page         int `json:"page"`
			PageSize     int `json:"page_size"`
			Total        int `json:"total"`
			DeviceGroups []struct {
				Id   uint   `json:"id"`
				Name string `json:"name"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal device-group list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("device-group list payload = %#v", payload)
	}
	if len(payload.Data.DeviceGroups) != 2 {
		t.Fatalf("device-group list length = %d, want 2", len(payload.Data.DeviceGroups))
	}
	if payload.Data.DeviceGroups[0].Id != fixture.deviceGroups[0].Id || payload.Data.DeviceGroups[0].Name != "ops-devices" || payload.Data.DeviceGroups[1].Id != fixture.deviceGroups[1].Id || payload.Data.DeviceGroups[1].Name != "lab-devices" {
		t.Fatalf("device-group list rows = %#v", payload.Data.DeviceGroups)
	}
}

func TestAdminDeviceGroupCreateDetailUpdateAndDeleteSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminDeviceGroupFixture(t)

	create := adminDeviceGroupRequest(fixture.router, http.MethodPost, "/api/admin/device_group/create", `{"name":"created-devices"}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminDeviceGroupResponseCode(t, create.Body.Bytes(), 0)
	var created model.DeviceGroup
	if err := fixture.db.Where("name = ?", "created-devices").First(&created).Error; err != nil {
		t.Fatalf("find created device group: %v", err)
	}

	detail := adminDeviceGroupRequest(fixture.router, http.MethodGet, "/api/admin/device_group/detail/"+strconv.FormatUint(uint64(created.Id), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			Id   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.Id != created.Id || detailPayload.Data.Name != "created-devices" {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	update := adminDeviceGroupRequest(fixture.router, http.MethodPost, "/api/admin/device_group/update", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"name":"updated-devices"}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminDeviceGroupResponseCode(t, update.Body.Bytes(), 0)
	var updated model.DeviceGroup
	if err := fixture.db.Where("id = ?", created.Id).First(&updated).Error; err != nil {
		t.Fatalf("find updated device group: %v", err)
	}
	if updated.Name != "updated-devices" {
		t.Fatalf("updated device group = %#v", updated)
	}

	deleteResponse := adminDeviceGroupRequest(fixture.router, http.MethodPost, "/api/admin/device_group/delete", `{"id":`+strconv.FormatUint(uint64(fixture.deviceGroups[0].Id), 10)+`,"name":"ops-devices"}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminDeviceGroupResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminDeviceGroupRowCount(t, fixture.db, "id = ?", []any{fixture.deviceGroups[0].Id}, 0)
	assertAdminDeviceGroupRowCount(t, fixture.db, "id = ?", []any{fixture.deviceGroups[1].Id}, 1)
}

func assertAdminDeviceGroupResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminDeviceGroupRowCount(t *testing.T, db *gorm.DB, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.DeviceGroup{}).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count device groups: %v", err)
	}
	if count != want {
		t.Fatalf("device-group row count = %d, want %d", count, want)
	}
}
