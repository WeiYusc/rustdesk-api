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
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type adminAddressBookCollectionFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	owner         *model.User
	viewer        *model.User
	collections   []*model.AddressBookCollection
}

func setupAdminAddressBookCollectionFixture(t *testing.T) adminAddressBookCollectionFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.AddressBook{}, &model.AddressBookCollection{}, &model.AddressBookCollectionRule{}); err != nil {
		t.Fatalf("migrate admin address-book collection models: %v", err)
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

	adminUser := createAdminAddressBookCollectionFixtureUser(t, db, "admin-abc-user", true, "admin-abc-token")
	createAdminAddressBookCollectionFixtureUser(t, db, "non-admin-abc-user", false, "non-admin-abc-token")
	owner := createAdminAddressBookCollectionFixtureUser(t, db, "owner-abc-user", false, "owner-abc-token")
	viewer := createAdminAddressBookCollectionFixtureUser(t, db, "viewer-abc-user", false, "viewer-abc-token")
	_ = adminUser

	collections := []*model.AddressBookCollection{
		{UserId: owner.Id, Name: "Owner Collection A"},
		{UserId: owner.Id, Name: "Owner Collection B"},
		{UserId: viewer.Id, Name: "Viewer Collection"},
	}
	for _, collection := range collections {
		if err := db.Create(collection).Error; err != nil {
			t.Fatalf("create seed collection: %v", err)
		}
	}
	seedCollectionRows(t, db, owner, viewer, collections)

	router := gin.New()
	controller := &AddressBookCollection{}
	collectionGroup := router.Group("/api/admin/address_book_collection").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	collectionGroup.GET("/list", controller.List)
	collectionGroup.GET("/detail/:id", controller.Detail)
	collectionGroup.POST("/create", controller.Create)
	collectionGroup.POST("/update", controller.Update)
	collectionGroup.POST("/delete", controller.Delete)

	return adminAddressBookCollectionFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-abc-token",
		nonAdminToken: "non-admin-abc-token",
		owner:         owner,
		viewer:        viewer,
		collections:   collections,
	}
}

func createAdminAddressBookCollectionFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
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

func seedCollectionRows(t *testing.T, db *gorm.DB, owner *model.User, viewer *model.User, collections []*model.AddressBookCollection) {
	t.Helper()

	addressBooks := []*model.AddressBook{
		{Id: "ab-target", UserId: owner.Id, CollectionId: collections[0].Id, Tags: custom_types.AutoJson([]byte(`[]`))},
		{Id: "ab-other", UserId: owner.Id, CollectionId: collections[1].Id, Tags: custom_types.AutoJson([]byte(`[]`))},
	}
	for _, addressBook := range addressBooks {
		if err := db.Create(addressBook).Error; err != nil {
			t.Fatalf("create seed address book: %v", err)
		}
	}
	rules := []*model.AddressBookCollectionRule{
		{UserId: owner.Id, CollectionId: collections[0].Id, Type: model.ShareAddressBookRuleTypePersonal, ToId: viewer.Id, Rule: model.ShareAddressBookRuleRuleRead},
		{UserId: owner.Id, CollectionId: collections[1].Id, Type: model.ShareAddressBookRuleTypePersonal, ToId: viewer.Id, Rule: model.ShareAddressBookRuleRuleReadWrite},
	}
	for _, rule := range rules {
		if err := db.Create(rule).Error; err != nil {
			t.Fatalf("create seed collection rule: %v", err)
		}
	}
}

func adminAddressBookCollectionRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminAddressBookCollectionRoutesRequireAdminAndListByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAddressBookCollectionFixture(t)

	unauthenticated := adminAddressBookCollectionRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminAddressBookCollectionResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminAddressBookCollectionRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminAddressBookCollectionResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminAddressBookCollectionRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection/list?page=1&page_size=10&user_id="+strconv.FormatUint(uint64(fixture.owner.Id), 10), "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page        int `json:"page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			Collections []struct {
				Id     uint   `json:"id"`
				UserId uint   `json:"user_id"`
				Name   string `json:"name"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal collection list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("collection list payload = %#v", payload)
	}
	if len(payload.Data.Collections) != 2 {
		t.Fatalf("collection list length = %d, want 2", len(payload.Data.Collections))
	}
	if payload.Data.Collections[0].Id != fixture.collections[0].Id || payload.Data.Collections[1].Id != fixture.collections[1].Id {
		t.Fatalf("collection list rows = %#v, want owner collections", payload.Data.Collections)
	}
}

func TestAdminAddressBookCollectionCreateDetailUpdateAndDeleteCascadesSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAddressBookCollectionFixture(t)

	create := adminAddressBookCollectionRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"name":"Created Collection"}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminAddressBookCollectionResponseCode(t, create.Body.Bytes(), 0)
	var created model.AddressBookCollection
	if err := fixture.db.Where("user_id = ? and name = ?", fixture.owner.Id, "Created Collection").First(&created).Error; err != nil {
		t.Fatalf("find created collection: %v", err)
	}

	detail := adminAddressBookCollectionRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection/detail/"+strconv.FormatUint(uint64(created.Id), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			Id     uint   `json:"id"`
			UserId uint   `json:"user_id"`
			Name   string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.Id != created.Id || detailPayload.Data.UserId != fixture.owner.Id || detailPayload.Data.Name != "Created Collection" {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	update := adminAddressBookCollectionRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection/update", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"name":"Updated Collection"}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminAddressBookCollectionResponseCode(t, update.Body.Bytes(), 0)
	var updated model.AddressBookCollection
	if err := fixture.db.Where("id = ?", created.Id).First(&updated).Error; err != nil {
		t.Fatalf("find updated collection: %v", err)
	}
	if updated.Name != "Updated Collection" || updated.UserId != fixture.owner.Id {
		t.Fatalf("updated collection = %#v", updated)
	}

	deleteResponse := adminAddressBookCollectionRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection/delete", `{"id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminAddressBookCollectionResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminAddressBookCollectionRowCount(t, fixture.db, &model.AddressBookCollection{}, "id = ?", fixture.collections[0].Id, 0)
	assertAdminAddressBookCollectionRowCount(t, fixture.db, &model.AddressBook{}, "collection_id = ?", fixture.collections[0].Id, 0)
	assertAdminAddressBookCollectionRowCount(t, fixture.db, &model.AddressBookCollectionRule{}, "collection_id = ?", fixture.collections[0].Id, 0)
	assertAdminAddressBookCollectionRowCount(t, fixture.db, &model.AddressBookCollection{}, "id = ?", fixture.collections[1].Id, 1)
	assertAdminAddressBookCollectionRowCount(t, fixture.db, &model.AddressBook{}, "collection_id = ?", fixture.collections[1].Id, 1)
	assertAdminAddressBookCollectionRowCount(t, fixture.db, &model.AddressBookCollectionRule{}, "collection_id = ?", fixture.collections[1].Id, 1)
}

func assertAdminAddressBookCollectionResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminAddressBookCollectionRowCount(t *testing.T, db *gorm.DB, modelValue any, where string, arg any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(modelValue).Where(where, arg).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != want {
		t.Fatalf("row count = %d, want %d", count, want)
	}
}
