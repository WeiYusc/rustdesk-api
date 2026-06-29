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

type adminTagFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	owner         *model.User
	viewer        *model.User
	collections   []*model.AddressBookCollection
	tags          []*model.Tag
}

func setupAdminTagFixture(t *testing.T) adminTagFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.AddressBookCollection{}, &model.Tag{}); err != nil {
		t.Fatalf("migrate admin tag models: %v", err)
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

	createAdminTagFixtureUser(t, db, "admin-tag-user", true, "admin-tag-token")
	createAdminTagFixtureUser(t, db, "non-admin-tag-user", false, "non-admin-tag-token")
	owner := createAdminTagFixtureUser(t, db, "owner-tag-user", false, "owner-tag-token")
	viewer := createAdminTagFixtureUser(t, db, "viewer-tag-user", false, "viewer-tag-token")

	collections := []*model.AddressBookCollection{
		{UserId: owner.Id, Name: "Owner Tag Collection A"},
		{UserId: owner.Id, Name: "Owner Tag Collection B"},
	}
	for _, collection := range collections {
		if err := db.Create(collection).Error; err != nil {
			t.Fatalf("create seed collection: %v", err)
		}
	}

	tags := []*model.Tag{
		{Name: "owner-red", UserId: owner.Id, Color: 0xFFFF0000, CollectionId: collections[0].Id},
		{Name: "owner-blue", UserId: owner.Id, Color: 0xFF0000FF, CollectionId: collections[1].Id},
		{Name: "viewer-green", UserId: viewer.Id, Color: 0xFF00FF00, CollectionId: 0},
	}
	for _, tag := range tags {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create seed tag: %v", err)
		}
	}

	router := gin.New()
	controller := &Tag{}
	tagGroup := router.Group("/api/admin/tag").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	tagGroup.GET("/list", controller.List)
	tagGroup.GET("/detail/:id", controller.Detail)
	tagGroup.POST("/create", controller.Create)
	tagGroup.POST("/update", controller.Update)
	tagGroup.POST("/delete", controller.Delete)

	return adminTagFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-tag-token",
		nonAdminToken: "non-admin-tag-token",
		owner:         owner,
		viewer:        viewer,
		collections:   collections,
		tags:          tags,
	}
}

func createAdminTagFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
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

func adminTagRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminTagRoutesRequireAdminAndListFiltersTags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminTagFixture(t)

	unauthenticated := adminTagRequest(fixture.router, http.MethodGet, "/api/admin/tag/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminTagResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminTagRequest(fixture.router, http.MethodGet, "/api/admin/tag/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminTagResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminTagRequest(fixture.router, http.MethodGet, "/api/admin/tag/list?page=1&page_size=10&user_id="+strconv.FormatUint(uint64(fixture.owner.Id), 10)+"&collection_id="+strconv.FormatUint(uint64(fixture.collections[0].Id), 10), "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Tags     []struct {
				Id           uint   `json:"id"`
				Name         string `json:"name"`
				UserId       uint   `json:"user_id"`
				Color        uint   `json:"color"`
				CollectionId uint   `json:"collection_id"`
				Collection   *struct {
					Id   uint   `json:"id"`
					Name string `json:"name"`
				} `json:"collection"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal tag list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 1 {
		t.Fatalf("tag list payload = %#v", payload)
	}
	if len(payload.Data.Tags) != 1 {
		t.Fatalf("tag list length = %d, want 1", len(payload.Data.Tags))
	}
	got := payload.Data.Tags[0]
	if got.Id != fixture.tags[0].Id || got.Name != "owner-red" || got.UserId != fixture.owner.Id || got.Color != 0xFFFF0000 || got.CollectionId != fixture.collections[0].Id {
		t.Fatalf("tag list row = %#v", got)
	}
	if got.Collection == nil || got.Collection.Id != fixture.collections[0].Id || got.Collection.Name != fixture.collections[0].Name {
		t.Fatalf("tag collection preload = %#v", got.Collection)
	}
}

func TestAdminTagCreateDetailUpdateAndDeleteSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminTagFixture(t)

	missingUser := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/create", `{"name":"missing-user","color":4278190080}`, fixture.adminToken)
	if missingUser.Code != http.StatusOK {
		t.Fatalf("missing-user status = %d, want %d; body=%q", missingUser.Code, http.StatusOK, missingUser.Body.String())
	}
	assertAdminTagResponseCode(t, missingUser.Body.Bytes(), 101)

	missingColor := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/create", `{"name":"missing-color","user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`}`, fixture.adminToken)
	if missingColor.Code != http.StatusOK {
		t.Fatalf("missing-color status = %d, want %d; body=%q", missingColor.Code, http.StatusOK, missingColor.Body.String())
	}
	assertAdminTagResponseCode(t, missingColor.Body.Bytes(), 101)

	nullColor := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/create", `{"name":"null-color","color":null,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`}`, fixture.adminToken)
	if nullColor.Code != http.StatusOK {
		t.Fatalf("null-color status = %d, want %d; body=%q", nullColor.Code, http.StatusOK, nullColor.Body.String())
	}
	assertAdminTagResponseCode(t, nullColor.Body.Bytes(), 101)

	create := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/create", `{"name":"created-tag","color":4289379276,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminTagResponseCode(t, create.Body.Bytes(), 0)
	var created model.Tag
	if err := fixture.db.Where("user_id = ? and collection_id = ? and name = ?", fixture.owner.Id, fixture.collections[0].Id, "created-tag").First(&created).Error; err != nil {
		t.Fatalf("find created tag: %v", err)
	}

	detail := adminTagRequest(fixture.router, http.MethodGet, "/api/admin/tag/detail/"+strconv.FormatUint(uint64(created.Id), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			Id           uint   `json:"id"`
			Name         string `json:"name"`
			UserId       uint   `json:"user_id"`
			Color        uint   `json:"color"`
			CollectionId uint   `json:"collection_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.Id != created.Id || detailPayload.Data.Name != "created-tag" || detailPayload.Data.UserId != fixture.owner.Id || detailPayload.Data.Color != 4289379276 || detailPayload.Data.CollectionId != fixture.collections[0].Id {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	update := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/update", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"name":"updated-tag","color":4294901760,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[1].Id), 10)+`}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminTagResponseCode(t, update.Body.Bytes(), 0)
	var updated model.Tag
	if err := fixture.db.Where("id = ?", created.Id).First(&updated).Error; err != nil {
		t.Fatalf("find updated tag: %v", err)
	}
	if updated.Name != "updated-tag" || updated.Color != 4294901760 || updated.UserId != fixture.owner.Id || updated.CollectionId != fixture.collections[1].Id {
		t.Fatalf("updated tag = %#v", updated)
	}

	updateZero := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/update", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"name":"zero-updated-tag","color":0,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[1].Id), 10)+`}`, fixture.adminToken)
	if updateZero.Code != http.StatusOK {
		t.Fatalf("update-zero status = %d, want %d; body=%q", updateZero.Code, http.StatusOK, updateZero.Body.String())
	}
	assertAdminTagResponseCode(t, updateZero.Body.Bytes(), 0)
	var zeroUpdated model.Tag
	if err := fixture.db.Where("id = ?", created.Id).First(&zeroUpdated).Error; err != nil {
		t.Fatalf("find zero-updated tag: %v", err)
	}
	if zeroUpdated.Name != "zero-updated-tag" || zeroUpdated.Color != 0 || zeroUpdated.UserId != fixture.owner.Id || zeroUpdated.CollectionId != fixture.collections[1].Id {
		t.Fatalf("zero-updated tag = %#v", zeroUpdated)
	}

	deleteResponse := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/delete", `{"id":`+strconv.FormatUint(uint64(fixture.tags[0].Id), 10)+`,"name":"owner-red","color":4278190080}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminTagResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminTagRowCount(t, fixture.db, "id = ?", []any{fixture.tags[0].Id}, 0)
	assertAdminTagRowCount(t, fixture.db, "id = ?", []any{fixture.tags[1].Id}, 1)
}

func TestAdminTagCreateAcceptsColorZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminTagFixture(t)

	create := adminTagRequest(fixture.router, http.MethodPost, "/api/admin/tag/create", `{"name":"zero-color-tag","color":0,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminTagResponseCode(t, create.Body.Bytes(), 0)

	var created model.Tag
	if err := fixture.db.Where("user_id = ? and name = ?", fixture.owner.Id, "zero-color-tag").First(&created).Error; err != nil {
		t.Fatalf("find zero-color tag: %v", err)
	}
	if created.Color != 0 {
		t.Fatalf("created color = %d, want 0", created.Color)
	}
}

func assertAdminTagResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminTagRowCount(t *testing.T, db *gorm.DB, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.Tag{}).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if count != want {
		t.Fatalf("tag row count = %d, want %d", count, want)
	}
}
