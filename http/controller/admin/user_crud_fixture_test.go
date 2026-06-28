package admin

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

type adminUserCrudFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	adminUser     *model.User
	otherUser     *model.User
}

func setupAdminUserCrudFixture(t *testing.T) adminUserCrudFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserToken{},
		&model.Group{},
		&model.UserThird{},
		&model.LoginLog{},
		&model.ShareRecord{},
		&model.AddressBook{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
		&model.Peer{},
	); err != nil {
		t.Fatalf("migrate admin user CRUD models: %v", err)
	}

	global.Config = config.Config{Lang: "en"}
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	global.Jwt = jwt.NewJwt("", 0)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)
	if err := db.Create(&model.Group{Name: "Default Group", Type: model.GroupTypeDefault}).Error; err != nil {
		t.Fatalf("create default group: %v", err)
	}
	if err := db.Create(&model.Group{Name: "Second Group", Type: model.GroupTypeShare}).Error; err != nil {
		t.Fatalf("create second group: %v", err)
	}

	adminUser := createAdminUserCrudFixtureUser(t, db, "admin-crud-user", true, "admin-crud-token")
	createAdminUserCrudFixtureUser(t, db, "non-admin-crud-user", false, "non-admin-crud-token")
	otherUser := createAdminUserCrudFixtureUser(t, db, "other-crud-user", false, "other-crud-token")

	router := gin.New()
	controller := &User{}
	user := router.Group("/api/admin/user").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	user.GET("/detail/:id", controller.Detail)
	user.POST("/create", controller.Create)
	user.POST("/update", controller.Update)
	user.POST("/delete", controller.Delete)
	user.POST("/groupUsers", controller.GroupUsers)

	return adminUserCrudFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-crud-token",
		nonAdminToken: "non-admin-crud-token",
		adminUser:     adminUser,
		otherUser:     otherUser,
	}
}

func createAdminUserCrudFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
	t.Helper()

	user := &model.User{
		Username: username,
		Email:    username + "@example.test",
		Nickname: username + " nick",
		GroupId:  1,
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

func adminUserCrudRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminUserCRUDRoutesRequireAdminPrivilege(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserCrudFixture(t)

	unauthenticated := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/create", `{"username":"blocked","group_id":1,"is_admin":false,"status":1}`, "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/create", `{"username":"blocked","group_id":1,"is_admin":false,"status":1}`, fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, nonAdmin.Body.Bytes(), 403)
}

func TestAdminUserCreateDetailUpdateAndDeleteSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserCrudFixture(t)

	create := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/create", `{"username":"created-crud-user","email":"created@example.test","nickname":"Created User","avatar":"avatar.png","group_id":2,"is_admin":false,"status":1,"remark":"created remark"}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, create.Body.Bytes(), 0)

	var created model.User
	if err := fixture.db.Where("username = ?", "created-crud-user").First(&created).Error; err != nil {
		t.Fatalf("find created user: %v", err)
	}
	if created.Email != "created@example.test" || created.Nickname != "Created User" || created.Avatar != "avatar.png" || created.GroupId != 2 || created.Status != model.COMMON_STATUS_ENABLE || created.Remark != "created remark" || service.AllService.UserService.IsAdmin(&created) {
		t.Fatalf("created user = %#v", created)
	}
	if created.Password == "" {
		t.Fatalf("created user password is empty, want current default password behavior")
	}

	detail := adminUserCrudRequest(fixture.router, http.MethodGet, "/api/admin/user/detail/"+strconv.FormatUint(uint64(created.Id), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			Id       uint             `json:"id"`
			Username string           `json:"username"`
			Email    string           `json:"email"`
			GroupId  uint             `json:"group_id"`
			Status   model.StatusCode `json:"status"`
			Remark   string           `json:"remark"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.Id != created.Id || detailPayload.Data.Username != created.Username || detailPayload.Data.Email != created.Email || detailPayload.Data.GroupId != created.GroupId || detailPayload.Data.Status != created.Status || detailPayload.Data.Remark != created.Remark {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	update := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/update", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"username":"created-crud-user","email":"updated@example.test","nickname":"Updated User","avatar":"updated.png","group_id":3,"is_admin":false,"status":2,"remark":"updated remark"}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, update.Body.Bytes(), 0)
	var updated model.User
	if err := fixture.db.Where("id = ?", created.Id).First(&updated).Error; err != nil {
		t.Fatalf("find updated user: %v", err)
	}
	if updated.Email != "updated@example.test" || updated.Nickname != "Updated User" || updated.Avatar != "updated.png" || updated.GroupId != 3 || updated.Status != model.COMMON_STATUS_DISABLED || updated.Remark != "updated remark" || service.AllService.UserService.IsAdmin(&updated) {
		t.Fatalf("updated user = %#v", updated)
	}

	deleteResponse := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/delete", `{"id":`+strconv.FormatUint(uint64(created.Id), 10)+`,"username":"created-crud-user","group_id":3,"is_admin":false,"status":2}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminUserCRUDRowCount(t, fixture.db, created.Id, 0)
	assertAdminUserCRUDRowCount(t, fixture.db, fixture.adminUser.Id, 1)
	assertAdminUserCRUDRowCount(t, fixture.db, fixture.otherUser.Id, 1)
}

func TestAdminUserLastAdminErrorsDoNotExposeI18nKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserCrudFixture(t)

	if err := fixture.db.Where("id <> ?", fixture.adminUser.Id).Delete(&model.User{}).Error; err != nil {
		t.Fatalf("remove non-admin users: %v", err)
	}

	deleteResponse := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/delete", `{"id":`+strconv.FormatUint(uint64(fixture.adminUser.Id), 10)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete last admin status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, deleteResponse.Body.Bytes(), 101)
	assertAdminUserCRUDResponseMessageNotRawKey(t, deleteResponse.Body.Bytes(), "LastAdminCannotDelete")

	updateResponse := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/update", `{"id":`+strconv.FormatUint(uint64(fixture.adminUser.Id), 10)+`,"username":"admin-crud-user","group_id":1,"is_admin":false,"status":1}`, fixture.adminToken)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update last admin status = %d, want %d; body=%q", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, updateResponse.Body.Bytes(), 101)
	assertAdminUserCRUDResponseMessageNotRawKey(t, updateResponse.Body.Bytes(), "LastAdminCannotUpdate")
}

func TestAdminUserGroupUsersSupportsGroupIdFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminUserCrudFixture(t)

	groupOneUser := createAdminUserCrudFixtureUser(t, fixture.db, "group-one-user", false, "group-one-token")
	groupTwoUser := createAdminUserCrudFixtureUser(t, fixture.db, "group-two-user", false, "group-two-token")
	if err := fixture.db.Model(groupTwoUser).Update("group_id", 2).Error; err != nil {
		t.Fatalf("set group two user group: %v", err)
	}

	unfiltered := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/groupUsers", `{}`, fixture.adminToken)
	if unfiltered.Code != http.StatusOK {
		t.Fatalf("unfiltered groupUsers status = %d, want %d; body=%q", unfiltered.Code, http.StatusOK, unfiltered.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, unfiltered.Body.Bytes(), 0)
	assertAdminUserCRUDGroupUsersGroupCount(t, unfiltered.Body.Bytes(), 2)
	assertAdminUserCRUDGroupUsersContains(t, unfiltered.Body.Bytes(), groupOneUser.Id)
	assertAdminUserCRUDGroupUsersContains(t, unfiltered.Body.Bytes(), groupTwoUser.Id)

	emptyBody := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/groupUsers", ``, fixture.adminToken)
	if emptyBody.Code != http.StatusOK {
		t.Fatalf("empty-body groupUsers status = %d, want %d; body=%q", emptyBody.Code, http.StatusOK, emptyBody.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, emptyBody.Body.Bytes(), 0)
	assertAdminUserCRUDGroupUsersGroupCount(t, emptyBody.Body.Bytes(), 2)
	assertAdminUserCRUDGroupUsersContains(t, emptyBody.Body.Bytes(), groupOneUser.Id)
	assertAdminUserCRUDGroupUsersContains(t, emptyBody.Body.Bytes(), groupTwoUser.Id)

	filtered := adminUserCrudRequest(fixture.router, http.MethodPost, "/api/admin/user/groupUsers", `{"group_id":2}`, fixture.adminToken)
	if filtered.Code != http.StatusOK {
		t.Fatalf("filtered groupUsers status = %d, want %d; body=%q", filtered.Code, http.StatusOK, filtered.Body.String())
	}
	assertAdminUserCRUDResponseCode(t, filtered.Body.Bytes(), 0)
	assertAdminUserCRUDGroupUsersGroupCount(t, filtered.Body.Bytes(), 2)
	assertAdminUserCRUDGroupUsersContains(t, filtered.Body.Bytes(), groupTwoUser.Id)
	assertAdminUserCRUDGroupUsersMissing(t, filtered.Body.Bytes(), groupOneUser.Id)
}

func assertAdminUserCRUDResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminUserCRUDResponseMessageNotRawKey(t *testing.T, body []byte, key string) {
	t.Helper()
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal response message: %v; body=%q", err, string(body))
	}
	if payload.Message == key || strings.Contains(payload.Message, key) {
		t.Fatalf("response message exposes raw key %q: body=%q", key, string(body))
	}
}

func assertAdminUserCRUDGroupUsersGroupCount(t *testing.T, body []byte, want int) {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Groups []struct {
				Id uint `json:"id"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal groupUsers groups: %v; body=%q", err, string(body))
	}
	if len(payload.Data.Groups) != want {
		t.Fatalf("groupUsers group count = %d, want %d; body=%q", len(payload.Data.Groups), want, string(body))
	}
}

func assertAdminUserCRUDGroupUsersContains(t *testing.T, body []byte, userId uint) {
	t.Helper()
	if !adminUserCRUDGroupUsersHasUser(t, body, userId) {
		t.Fatalf("groupUsers response missing user %d: body=%q", userId, string(body))
	}
}

func assertAdminUserCRUDGroupUsersMissing(t *testing.T, body []byte, userId uint) {
	t.Helper()
	if adminUserCRUDGroupUsersHasUser(t, body, userId) {
		t.Fatalf("groupUsers response unexpectedly includes user %d: body=%q", userId, string(body))
	}
}

func adminUserCRUDGroupUsersHasUser(t *testing.T, body []byte, userId uint) bool {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Users []struct {
				Id uint `json:"id"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal groupUsers response: %v; body=%q", err, string(body))
	}
	for _, user := range payload.Data.Users {
		if user.Id == userId {
			return true
		}
	}
	return false
}

func assertAdminUserCRUDRowCount(t *testing.T, db *gorm.DB, id uint, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.User{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count user: %v", err)
	}
	if count != want {
		t.Fatalf("user row count = %d, want %d", count, want)
	}
}
