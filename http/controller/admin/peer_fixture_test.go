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

type adminPeerFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	adminUser     *model.User
	peers         []*model.Peer
}

func setupAdminPeerFixture(t *testing.T) adminPeerFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Peer{}); err != nil {
		t.Fatalf("migrate admin peer models: %v", err)
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

	adminUser := createAdminPeerFixtureUser(t, db, "admin-peer-user", true, "admin-peer-token")
	createAdminPeerFixtureUser(t, db, "non-admin-peer-user", false, "non-admin-peer-token")

	peers := []*model.Peer{
		{Id: "peer-alpha", Uuid: "uuid-alpha", Hostname: "seed-host-one", Username: "alice", Alias: "alpha", UserId: adminUser.Id, Version: "1.0.0", LastOnlineTime: time.Now().Unix() - 60, LastOnlineIp: "198.51.100.10"},
		{Id: "peer-beta", Uuid: "uuid-beta", Hostname: "seed-host-two", Username: "bob", Alias: "beta", UserId: adminUser.Id, Version: "1.0.1", LastOnlineTime: time.Now().Unix() - 30, LastOnlineIp: "198.51.100.20"},
		{Id: "peer-gamma", Uuid: "uuid-gamma", Hostname: "other-host", Username: "carol", Alias: "gamma", UserId: adminUser.Id, Version: "1.0.2", LastOnlineTime: time.Now().Unix(), LastOnlineIp: "198.51.100.30"},
	}
	for _, peer := range peers {
		if err := db.Create(peer).Error; err != nil {
			t.Fatalf("create seed peer: %v", err)
		}
	}

	router := gin.New()
	controller := &Peer{}
	peerGroup := router.Group("/api/admin/peer").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	peerGroup.GET("/list", controller.List)
	peerGroup.GET("/detail/:id", controller.Detail)
	peerGroup.POST("/create", controller.Create)
	peerGroup.POST("/update", controller.Update)
	peerGroup.POST("/delete", controller.Delete)
	peerGroup.POST("/batchDelete", controller.BatchDelete)

	return adminPeerFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-peer-token",
		nonAdminToken: "non-admin-peer-token",
		adminUser:     adminUser,
		peers:         peers,
	}
}

func createAdminPeerFixtureUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
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

func adminPeerRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminPeerRoutesRequireAdminAndListFiltersPeers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminPeerFixture(t)

	unauthenticated := adminPeerRequest(fixture.router, http.MethodGet, "/api/admin/peer/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminPeerResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminPeerRequest(fixture.router, http.MethodGet, "/api/admin/peer/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminPeerResponseCode(t, nonAdmin.Body.Bytes(), 403)

	adminList := adminPeerRequest(fixture.router, http.MethodGet, "/api/admin/peer/list?page=1&page_size=10&hostname=seed-host", "", fixture.adminToken)
	if adminList.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want %d; body=%q", adminList.Code, http.StatusOK, adminList.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Total    int `json:"total"`
			Peers    []struct {
				RowId    uint   `json:"row_id"`
				Id       string `json:"id"`
				Hostname string `json:"hostname"`
				Alias    string `json:"alias"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(adminList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal admin list: %v; body=%q", err, adminList.Body.String())
	}
	if payload.Code != 0 || payload.Data.Page != 1 || payload.Data.PageSize != 10 || payload.Data.Total != 2 {
		t.Fatalf("admin list payload = %#v", payload)
	}
	if len(payload.Data.Peers) != 2 {
		t.Fatalf("admin list length = %d, want 2", len(payload.Data.Peers))
	}
	if payload.Data.Peers[0].RowId != fixture.peers[0].RowId || payload.Data.Peers[1].RowId != fixture.peers[1].RowId {
		t.Fatalf("admin list peers = %#v, want alias ASC seed-host peers", payload.Data.Peers)
	}

	uuidList := adminPeerRequest(fixture.router, http.MethodGet, "/api/admin/peer/list?page=1&page_size=10&uuids=uuid-alpha,uuid-beta", "", fixture.adminToken)
	if uuidList.Code != http.StatusOK {
		t.Fatalf("uuid list status = %d, want %d; body=%q", uuidList.Code, http.StatusOK, uuidList.Body.String())
	}
	var uuidPayload struct {
		Code int `json:"code"`
		Data struct {
			Total int `json:"total"`
			Peers []struct {
				Uuid string `json:"uuid"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(uuidList.Body.Bytes(), &uuidPayload); err != nil {
		t.Fatalf("unmarshal uuid list: %v; body=%q", err, uuidList.Body.String())
	}
	if uuidPayload.Code != 0 || uuidPayload.Data.Total != 2 {
		t.Fatalf("uuid list payload = %#v, want two matching peers", uuidPayload)
	}
}

func TestAdminPeerCreateDetailUpdateDeleteAndBatchDeleteSelectedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminPeerFixture(t)

	create := adminPeerRequest(fixture.router, http.MethodPost, "/api/admin/peer/create", `{"id":"peer-created","uuid":"uuid-created","hostname":"created-host","username":"created-user","alias":"created-alias","version":"2.0.0","cpu":"x86_64","memory":"8G","os":"linux","group_id":7}`, fixture.adminToken)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", create.Code, http.StatusOK, create.Body.String())
	}
	assertAdminPeerResponseCode(t, create.Body.Bytes(), 0)

	var created model.Peer
	if err := fixture.db.Where("id = ?", "peer-created").First(&created).Error; err != nil {
		t.Fatalf("find created peer: %v", err)
	}
	if created.Uuid != "uuid-created" || created.Hostname != "created-host" || created.Username != "created-user" || created.Alias != "created-alias" || created.Version != "2.0.0" || created.Cpu != "x86_64" || created.Memory != "8G" || created.Os != "linux" || created.GroupId != 7 {
		t.Fatalf("created peer = %#v", created)
	}

	detail := adminPeerRequest(fixture.router, http.MethodGet, "/api/admin/peer/detail/"+strconv.FormatUint(uint64(created.RowId), 10), "", fixture.adminToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d; body=%q", detail.Code, http.StatusOK, detail.Body.String())
	}
	var detailPayload struct {
		Code int `json:"code"`
		Data struct {
			RowId    uint   `json:"row_id"`
			Id       string `json:"id"`
			Hostname string `json:"hostname"`
			Alias    string `json:"alias"`
		} `json:"data"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailPayload); err != nil {
		t.Fatalf("unmarshal detail: %v; body=%q", err, detail.Body.String())
	}
	if detailPayload.Code != 0 || detailPayload.Data.RowId != created.RowId || detailPayload.Data.Id != created.Id || detailPayload.Data.Hostname != created.Hostname || detailPayload.Data.Alias != created.Alias {
		t.Fatalf("detail payload = %#v", detailPayload)
	}

	if err := fixture.db.Model(&created).Update("user_id", fixture.adminUser.Id).Error; err != nil {
		t.Fatalf("set created peer user_id: %v", err)
	}
	update := adminPeerRequest(fixture.router, http.MethodPost, "/api/admin/peer/update", `{"row_id":`+strconv.FormatUint(uint64(created.RowId), 10)+`,"id":"peer-created","uuid":"uuid-created","hostname":"updated-host","username":"updated-user","alias":"updated-alias","version":"2.1.0","cpu":"arm64","memory":"16G","os":"linux-updated","group_id":8}`, fixture.adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", update.Code, http.StatusOK, update.Body.String())
	}
	assertAdminPeerResponseCode(t, update.Body.Bytes(), 0)
	var updated model.Peer
	if err := fixture.db.Where("row_id = ?", created.RowId).First(&updated).Error; err != nil {
		t.Fatalf("find updated peer: %v", err)
	}
	if updated.Hostname != "updated-host" || updated.Username != "updated-user" || updated.Alias != "updated-alias" || updated.Version != "2.1.0" || updated.Cpu != "arm64" || updated.Memory != "16G" || updated.Os != "linux-updated" || updated.GroupId != 8 || updated.UserId != 0 {
		t.Fatalf("updated peer = %#v", updated)
	}

	deleteResponse := adminPeerRequest(fixture.router, http.MethodPost, "/api/admin/peer/delete", `{"row_id":`+strconv.FormatUint(uint64(created.RowId), 10)+`}`, fixture.adminToken)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d; body=%q", deleteResponse.Code, http.StatusOK, deleteResponse.Body.String())
	}
	assertAdminPeerResponseCode(t, deleteResponse.Body.Bytes(), 0)
	assertAdminPeerRowCount(t, fixture.db, created.RowId, 0)
	assertAdminPeerRowCount(t, fixture.db, fixture.peers[0].RowId, 1)

	batchResponse := adminPeerRequest(fixture.router, http.MethodPost, "/api/admin/peer/batchDelete", `{"row_ids":[`+strconv.FormatUint(uint64(fixture.peers[0].RowId), 10)+`]}`, fixture.adminToken)
	if batchResponse.Code != http.StatusOK {
		t.Fatalf("batch delete status = %d, want %d; body=%q", batchResponse.Code, http.StatusOK, batchResponse.Body.String())
	}
	assertAdminPeerResponseCode(t, batchResponse.Body.Bytes(), 0)
	assertAdminPeerRowCount(t, fixture.db, fixture.peers[0].RowId, 0)
	assertAdminPeerRowCount(t, fixture.db, fixture.peers[1].RowId, 1)
	assertAdminPeerRowCount(t, fixture.db, fixture.peers[2].RowId, 1)
}

func assertAdminPeerResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminPeerRowCount(t *testing.T, db *gorm.DB, rowID uint, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.Peer{}).Where("row_id = ?", rowID).Count(&count).Error; err != nil {
		t.Fatalf("count peer: %v", err)
	}
	if count != want {
		t.Fatalf("peer row count = %d, want %d", count, want)
	}
}
