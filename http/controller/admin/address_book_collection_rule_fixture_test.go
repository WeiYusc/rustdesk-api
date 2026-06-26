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

type adminAddressBookCollectionRuleFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	admin         *model.User
	owner         *model.User
	viewer        *model.User
	otherViewer   *model.User
	group         *model.Group
	collections   []*model.AddressBookCollection
	rules         []*model.AddressBookCollectionRule
}

func setupAdminAddressBookCollectionRuleFixture(t *testing.T) adminAddressBookCollectionRuleFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Group{}, &model.User{}, &model.UserToken{}, &model.AddressBookCollection{}, &model.AddressBookCollectionRule{}); err != nil {
		t.Fatalf("migrate admin address-book collection-rule models: %v", err)
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

	shareGroup := &model.Group{Name: "Share Group", Type: model.GroupTypeShare}
	if err := db.Create(shareGroup).Error; err != nil {
		t.Fatalf("create share group: %v", err)
	}

	adminUser := createAdminAddressBookCollectionRuleFixtureUser(t, db, "admin-abcr-user", true, "admin-abcr-token", 0)
	createAdminAddressBookCollectionRuleFixtureUser(t, db, "non-admin-abcr-user", false, "non-admin-abcr-token", 0)
	owner := createAdminAddressBookCollectionRuleFixtureUser(t, db, "owner-abcr-user", false, "owner-abcr-token", 0)
	viewer := createAdminAddressBookCollectionRuleFixtureUser(t, db, "viewer-abcr-user", false, "viewer-abcr-token", shareGroup.Id)
	otherViewer := createAdminAddressBookCollectionRuleFixtureUser(t, db, "other-viewer-abcr-user", false, "other-viewer-abcr-token", shareGroup.Id)

	collections := []*model.AddressBookCollection{
		{UserId: owner.Id, Name: "Owner Shared Collection A"},
		{UserId: owner.Id, Name: "Owner Shared Collection B"},
		{UserId: viewer.Id, Name: "Viewer Owned Collection"},
	}
	for _, collection := range collections {
		if err := db.Create(collection).Error; err != nil {
			t.Fatalf("create seed collection: %v", err)
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

	router := gin.New()
	controller := &AddressBookCollectionRule{}
	ruleGroup := router.Group("/api/admin/address_book_collection_rule").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	ruleGroup.GET("/list", controller.List)
	ruleGroup.GET("/detail/:id", controller.Detail)
	ruleGroup.POST("/create", controller.Create)
	ruleGroup.POST("/update", controller.Update)
	ruleGroup.POST("/delete", controller.Delete)

	return adminAddressBookCollectionRuleFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-abcr-token",
		nonAdminToken: "non-admin-abcr-token",
		admin:         adminUser,
		owner:         owner,
		viewer:        viewer,
		otherViewer:   otherViewer,
		group:         shareGroup,
		collections:   collections,
		rules:         rules,
	}
}

func createAdminAddressBookCollectionRuleFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string, groupId uint) *model.User {
	t.Helper()

	user := &model.User{Username: username, Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin, GroupId: groupId}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: token, ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create token %s: %v", token, err)
	}
	return user
}

func adminAddressBookCollectionRuleRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminAddressBookCollectionRuleRoutesRequireAdminAndListFiltersRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAddressBookCollectionRuleFixture(t)

	unauthenticated := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection_rule/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminAddressBookCollectionRuleResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection_rule/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminAddressBookCollectionRuleResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection_rule/list?page=1&page_size=10&user_id="+strconv.FormatUint(uint64(fixture.owner.Id), 10)+"&collection_id="+strconv.FormatUint(uint64(fixture.collections[0].Id), 10), "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Rules    []struct {
				Id           uint `json:"id"`
				UserId       uint `json:"user_id"`
				CollectionId uint `json:"collection_id"`
				Type         int  `json:"type"`
				ToId         uint `json:"to_id"`
				Rule         int  `json:"rule"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal rule list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 1 {
		t.Fatalf("rule list payload = %#v", payload)
	}
	if len(payload.Data.Rules) != 1 {
		t.Fatalf("rule list length = %d, want 1", len(payload.Data.Rules))
	}
	got := payload.Data.Rules[0]
	if got.Id != fixture.rules[0].Id || got.UserId != fixture.owner.Id || got.CollectionId != fixture.collections[0].Id || got.Type != model.ShareAddressBookRuleTypePersonal || got.ToId != fixture.viewer.Id || got.Rule != model.ShareAddressBookRuleRuleRead {
		t.Fatalf("rule list row = %#v", got)
	}
}

func TestAdminAddressBookCollectionRuleCreateValidatesOwnerTargetsAndDuplicates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAddressBookCollectionRuleFixture(t)

	ownerMismatch := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.viewer.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":1,"to_id":`+strconv.FormatUint(uint64(fixture.otherViewer.Id), 10)+`,"rule":1}`, fixture.adminToken)
	assertAdminAddressBookCollectionRuleResponseCode(t, ownerMismatch.Body.Bytes(), 101)

	selfShare := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":1,"to_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"rule":1}`, fixture.adminToken)
	assertAdminAddressBookCollectionRuleResponseCode(t, selfShare.Body.Bytes(), 101)

	missingPersonalTarget := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":1,"to_id":9999,"rule":1}`, fixture.adminToken)
	assertAdminAddressBookCollectionRuleResponseCode(t, missingPersonalTarget.Body.Bytes(), 101)

	missingGroupTarget := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":2,"to_id":9999,"rule":1}`, fixture.adminToken)
	assertAdminAddressBookCollectionRuleResponseCode(t, missingGroupTarget.Body.Bytes(), 101)

	duplicate := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":1,"to_id":`+strconv.FormatUint(uint64(fixture.viewer.Id), 10)+`,"rule":2}`, fixture.adminToken)
	assertAdminAddressBookCollectionRuleResponseCode(t, duplicate.Body.Bytes(), 101)

	createGroupRule := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/create", `{"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":2,"to_id":`+strconv.FormatUint(uint64(fixture.group.Id), 10)+`,"rule":3}`, fixture.adminToken)
	if createGroupRule.Code != http.StatusOK {
		t.Fatalf("create group rule status = %d, want %d; body=%q", createGroupRule.Code, http.StatusOK, createGroupRule.Body.String())
	}
	assertAdminAddressBookCollectionRuleResponseCode(t, createGroupRule.Body.Bytes(), 0)
	assertAdminAddressBookCollectionRuleRowCount(t, fixture.db, "collection_id = ? and type = ? and to_id = ?", []any{fixture.collections[0].Id, model.ShareAddressBookRuleTypeGroup, fixture.group.Id}, 1)
}

func TestAdminAddressBookCollectionRuleDetailUpdateAndDeleteSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAddressBookCollectionRuleFixture(t)

	detail := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodGet, "/api/admin/address_book_collection_rule/detail/"+strconv.FormatUint(uint64(fixture.rules[0].Id), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			Id           uint `json:"id"`
			UserId       uint `json:"user_id"`
			CollectionId uint `json:"collection_id"`
			Type         int  `json:"type"`
			ToId         uint `json:"to_id"`
			Rule         int  `json:"rule"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.Id != fixture.rules[0].Id || detailPayload.Data.UserId != fixture.owner.Id || detailPayload.Data.CollectionId != fixture.collections[0].Id || detailPayload.Data.Type != model.ShareAddressBookRuleTypePersonal || detailPayload.Data.ToId != fixture.viewer.Id || detailPayload.Data.Rule != model.ShareAddressBookRuleRuleRead {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	update := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/update", `{"id":`+strconv.FormatUint(uint64(fixture.rules[0].Id), 10)+`,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[0].Id), 10)+`,"type":1,"to_id":`+strconv.FormatUint(uint64(fixture.viewer.Id), 10)+`,"rule":3}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminAddressBookCollectionRuleResponseCode(t, update.Body.Bytes(), 0)
	var updated model.AddressBookCollectionRule
	if err := fixture.db.Where("id = ?", fixture.rules[0].Id).First(&updated).Error; err != nil {
		t.Fatalf("find updated rule: %v", err)
	}
	if updated.Rule != model.ShareAddressBookRuleRuleFullControl || updated.CollectionId != fixture.collections[0].Id || updated.ToId != fixture.viewer.Id {
		t.Fatalf("updated rule = %#v", updated)
	}

	duplicateUpdate := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/update", `{"id":`+strconv.FormatUint(uint64(fixture.rules[0].Id), 10)+`,"user_id":`+strconv.FormatUint(uint64(fixture.owner.Id), 10)+`,"collection_id":`+strconv.FormatUint(uint64(fixture.collections[1].Id), 10)+`,"type":1,"to_id":`+strconv.FormatUint(uint64(fixture.viewer.Id), 10)+`,"rule":1}`, fixture.adminToken)
	assertAdminAddressBookCollectionRuleResponseCode(t, duplicateUpdate.Body.Bytes(), 101)

	deleteResponse := adminAddressBookCollectionRuleRequest(fixture.router, http.MethodPost, "/api/admin/address_book_collection_rule/delete", `{"id":`+strconv.FormatUint(uint64(fixture.rules[0].Id), 10)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminAddressBookCollectionRuleResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminAddressBookCollectionRuleRowCount(t, fixture.db, "id = ?", []any{fixture.rules[0].Id}, 0)
	assertAdminAddressBookCollectionRuleRowCount(t, fixture.db, "id = ?", []any{fixture.rules[1].Id}, 1)
}

func assertAdminAddressBookCollectionRuleResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminAddressBookCollectionRuleRowCount(t *testing.T, db *gorm.DB, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.AddressBookCollectionRule{}).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count collection rules: %v", err)
	}
	if count != want {
		t.Fatalf("rule row count = %d, want %d", count, want)
	}
}
