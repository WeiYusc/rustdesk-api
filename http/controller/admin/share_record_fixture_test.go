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

type adminShareRecordFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	owner         *model.User
	viewer        *model.User
	shareRecords  []*model.ShareRecord
}

func setupAdminShareRecordFixture(t *testing.T) adminShareRecordFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.ShareRecord{}); err != nil {
		t.Fatalf("migrate admin share-record models: %v", err)
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

	createAdminShareRecordFixtureUser(t, db, "admin-share-record-user", true, "admin-share-record-token")
	createAdminShareRecordFixtureUser(t, db, "non-admin-share-record-user", false, "non-admin-share-record-token")
	owner := createAdminShareRecordFixtureUser(t, db, "owner-share-record-user", false, "owner-share-record-token")
	viewer := createAdminShareRecordFixtureUser(t, db, "viewer-share-record-user", false, "viewer-share-record-token")

	shareRecords := []*model.ShareRecord{
		{UserId: owner.Id, PeerId: "owner-peer-old", ShareToken: "token-owner-old", PasswordType: "once", Password: "pw-old", Expire: 1000},
		{UserId: owner.Id, PeerId: "owner-peer-new", ShareToken: "token-owner-new", PasswordType: "fixed", Password: "pw-new", Expire: 2000},
		{UserId: viewer.Id, PeerId: "viewer-peer", ShareToken: "token-viewer", PasswordType: "once", Password: "pw-viewer", Expire: 3000},
	}
	for _, shareRecord := range shareRecords {
		if err := db.Create(shareRecord).Error; err != nil {
			t.Fatalf("create seed share record: %v", err)
		}
	}

	router := gin.New()
	controller := &ShareRecord{}
	shareRecordGroup := router.Group("/api/admin/share_record").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	shareRecordGroup.GET("/list", controller.List)
	shareRecordGroup.POST("/delete", controller.Delete)
	shareRecordGroup.POST("/batchDelete", controller.BatchDelete)

	return adminShareRecordFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-share-record-token",
		nonAdminToken: "non-admin-share-record-token",
		owner:         owner,
		viewer:        viewer,
		shareRecords:  shareRecords,
	}
}

func createAdminShareRecordFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
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

func adminShareRecordRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminShareRecordRoutesRequireAdminAndListFiltersByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminShareRecordFixture(t)

	unauthenticated := adminShareRecordRequest(fixture.router, http.MethodGet, "/api/admin/share_record/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminShareRecordResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminShareRecordRequest(fixture.router, http.MethodGet, "/api/admin/share_record/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminShareRecordResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminShareRecordRequest(fixture.router, http.MethodGet, "/api/admin/share_record/list?page=1&page_size=10&user_id="+strconv.FormatUint(uint64(fixture.owner.Id), 10), "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page         int `json:"page"`
			PageSize     int `json:"page_size"`
			Total        int `json:"total"`
			ShareRecords []struct {
				Id           uint   `json:"id"`
				UserId       uint   `json:"user_id"`
				PeerId       string `json:"peer_id"`
				ShareToken   string `json:"share_token"`
				PasswordType string `json:"password_type"`
				Password     string `json:"password"`
				Expire       int64  `json:"expire"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal share-record list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("share-record list payload = %#v", payload)
	}
	if len(payload.Data.ShareRecords) != 2 {
		t.Fatalf("share-record list length = %d, want 2", len(payload.Data.ShareRecords))
	}
	first := payload.Data.ShareRecords[0]
	if first.Id != fixture.shareRecords[0].Id || first.UserId != fixture.owner.Id || first.PeerId != "owner-peer-old" || first.ShareToken != "token-owner-old" || first.PasswordType != "once" || first.Password != "pw-old" || first.Expire != 1000 {
		t.Fatalf("share-record first row = %#v", first)
	}
	second := payload.Data.ShareRecords[1]
	if second.Id != fixture.shareRecords[1].Id || second.UserId != fixture.owner.Id || second.PeerId != "owner-peer-new" || second.ShareToken != "token-owner-new" || second.PasswordType != "fixed" || second.Password != "pw-new" || second.Expire != 2000 {
		t.Fatalf("share-record second row = %#v", second)
	}
}

func TestAdminShareRecordDeleteAndBatchDeleteRemoveSelectedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminShareRecordFixture(t)

	deleteResponse := adminShareRecordRequest(fixture.router, http.MethodPost, "/api/admin/share_record/delete", `{"id":`+strconv.FormatUint(uint64(fixture.shareRecords[0].Id), 10)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminShareRecordResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminShareRecordRowCount(t, fixture.db, "id = ?", []any{fixture.shareRecords[0].Id}, 0)
	assertAdminShareRecordRowCount(t, fixture.db, "id = ?", []any{fixture.shareRecords[1].Id}, 1)
	assertAdminShareRecordRowCount(t, fixture.db, "id = ?", []any{fixture.shareRecords[2].Id}, 1)

	batchDelete := adminShareRecordRequest(fixture.router, http.MethodPost, "/api/admin/share_record/batchDelete", `{"ids":[`+strconv.FormatUint(uint64(fixture.shareRecords[1].Id), 10)+`]}`, fixture.adminToken)
	if batchDelete.Code != http.StatusOK {
		t.Fatalf("batch delete status = %d, want %d; body=%q", batchDelete.Code, http.StatusOK, batchDelete.Body.String())
	}
	assertAdminShareRecordResponseCode(t, batchDelete.Body.Bytes(), 0)
	assertAdminShareRecordRowCount(t, fixture.db, "id = ?", []any{fixture.shareRecords[1].Id}, 0)
	assertAdminShareRecordRowCount(t, fixture.db, "id = ?", []any{fixture.shareRecords[2].Id}, 1)
}

func assertAdminShareRecordResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminShareRecordRowCount(t *testing.T, db *gorm.DB, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.ShareRecord{}).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count share records: %v", err)
	}
	if count != want {
		t.Fatalf("share-record row count = %d, want %d", count, want)
	}
}
