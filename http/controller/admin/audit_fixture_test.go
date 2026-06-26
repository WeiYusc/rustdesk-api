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

type adminAuditFixture struct {
	db            *gorm.DB
	router        *gin.Engine
	adminToken    string
	nonAdminToken string
	connRows      []*model.AuditConn
	fileRows      []*model.AuditFile
}

func setupAdminAuditFixture(t *testing.T) adminAuditFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate admin audit models: %v", err)
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

	adminUser := createAdminAuditUser(t, db, "admin-audit", true, "admin-audit-token")
	_ = adminUser
	createAdminAuditUser(t, db, "non-admin-audit", false, "non-admin-audit-token")

	connRows := []*model.AuditConn{
		{PeerId: "target-alpha", FromPeer: "source-one", ConnId: 101, Type: 1},
		{PeerId: "target-beta", FromPeer: "source-two", ConnId: 102, Type: 2},
		{PeerId: "other-peer", FromPeer: "source-one", ConnId: 103, Type: 3},
	}
	for _, row := range connRows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create audit conn: %v", err)
		}
	}
	fileRows := []*model.AuditFile{
		{PeerId: "file-alpha", FromPeer: "client-one", Path: "/tmp/a.txt", IsFile: true},
		{PeerId: "file-beta", FromPeer: "client-two", Path: "/tmp/b.txt", IsFile: true},
		{PeerId: "file-gamma", FromPeer: "client-one", Path: "/tmp/c.txt", IsFile: false},
	}
	for _, row := range fileRows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create audit file: %v", err)
		}
	}

	router := gin.New()
	controller := &Audit{}
	conn := router.Group("/api/admin/audit_conn").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	conn.GET("/list", controller.ConnList)
	conn.POST("/delete", controller.ConnDelete)
	conn.POST("/batchDelete", controller.BatchConnDelete)
	file := router.Group("/api/admin/audit_file").Use(middleware.BackendUserAuth(), middleware.AdminPrivilege())
	file.GET("/list", controller.FileList)
	file.POST("/delete", controller.FileDelete)
	file.POST("/batchDelete", controller.BatchFileDelete)

	return adminAuditFixture{
		db:            db,
		router:        router,
		adminToken:    "admin-audit-token",
		nonAdminToken: "non-admin-audit-token",
		connRows:      connRows,
		fileRows:      fileRows,
	}
}

func createAdminAuditUser(t *testing.T, db *gorm.DB, username string, isAdmin bool, token string) *model.User {
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

func adminAuditRequest(router *gin.Engine, method string, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminAuditRoutesRequireAdminAndListWithFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAuditFixture(t)

	unauthenticated := adminAuditRequest(fixture.router, http.MethodGet, "/api/admin/audit_conn/list?page=1&page_size=10", "", "")
	if unauthenticated.Code != http.StatusOK {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusOK, unauthenticated.Body.String())
	}
	assertAdminAuditResponseCode(t, unauthenticated.Body.Bytes(), 403)

	nonAdmin := adminAuditRequest(fixture.router, http.MethodGet, "/api/admin/audit_conn/list?page=1&page_size=10", "", fixture.nonAdminToken)
	if nonAdmin.Code != http.StatusOK {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusOK, nonAdmin.Body.String())
	}
	assertAdminAuditResponseCode(t, nonAdmin.Body.Bytes(), 403)

	connList := adminAuditRequest(fixture.router, http.MethodGet, "/api/admin/audit_conn/list?page=1&page_size=2&peer_id=target", "", fixture.adminToken)
	if connList.Code != http.StatusOK {
		t.Fatalf("conn list status = %d, want %d; body=%q", connList.Code, http.StatusOK, connList.Body.String())
	}
	var connPayload struct {
		Code int `json:"code"`
		Data struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			AuditConns []struct {
				Id       uint   `json:"id"`
				PeerId   string `json:"peer_id"`
				FromPeer string `json:"from_peer"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(connList.Body.Bytes(), &connPayload); err != nil {
		t.Fatalf("unmarshal conn list: %v; body=%q", err, connList.Body.String())
	}
	if connPayload.Code != 0 || connPayload.Data.Page != 1 || connPayload.Data.PageSize != 2 || connPayload.Data.Total != 2 {
		t.Fatalf("conn list payload = %#v", connPayload)
	}
	if len(connPayload.Data.AuditConns) != 2 {
		t.Fatalf("conn list length = %d, want 2", len(connPayload.Data.AuditConns))
	}
	if connPayload.Data.AuditConns[0].Id != fixture.connRows[1].Id || connPayload.Data.AuditConns[1].Id != fixture.connRows[0].Id {
		t.Fatalf("conn list ids = %#v, want id desc for target rows", connPayload.Data.AuditConns)
	}
	for _, row := range connPayload.Data.AuditConns {
		if !strings.Contains(row.PeerId, "target") {
			t.Fatalf("conn row peer_id = %q, want target filter", row.PeerId)
		}
	}

	fileList := adminAuditRequest(fixture.router, http.MethodGet, "/api/admin/audit_file/list?page=1&page_size=1&from_peer=client-one", "", fixture.adminToken)
	if fileList.Code != http.StatusOK {
		t.Fatalf("file list status = %d, want %d; body=%q", fileList.Code, http.StatusOK, fileList.Body.String())
	}
	var filePayload struct {
		Code int `json:"code"`
		Data struct {
			Page       int `json:"page"`
			PageSize   int `json:"page_size"`
			Total      int `json:"total"`
			AuditFiles []struct {
				Id       uint   `json:"id"`
				PeerId   string `json:"peer_id"`
				FromPeer string `json:"from_peer"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(fileList.Body.Bytes(), &filePayload); err != nil {
		t.Fatalf("unmarshal file list: %v; body=%q", err, fileList.Body.String())
	}
	if filePayload.Code != 0 || filePayload.Data.Page != 1 || filePayload.Data.PageSize != 1 || filePayload.Data.Total != 2 {
		t.Fatalf("file list payload = %#v", filePayload)
	}
	if len(filePayload.Data.AuditFiles) != 1 || filePayload.Data.AuditFiles[0].FromPeer != "client-one" || filePayload.Data.AuditFiles[0].Id != fixture.fileRows[2].Id {
		t.Fatalf("file list rows = %#v, want newest client-one row", filePayload.Data.AuditFiles)
	}
}

func TestAdminAuditDeleteAndBatchDeleteRemoveSelectedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminAuditFixture(t)

	deleteConn := adminAuditRequest(fixture.router, http.MethodPost, "/api/admin/audit_conn/delete", `{"id":`+uintToString(fixture.connRows[0].Id)+`}`, fixture.adminToken)
	if deleteConn.Code != http.StatusOK {
		t.Fatalf("conn delete status = %d, want %d; body=%q", deleteConn.Code, http.StatusOK, deleteConn.Body.String())
	}
	assertAdminAuditResponseCode(t, deleteConn.Body.Bytes(), 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditConn{}, "id = ?", fixture.connRows[0].Id, 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditConn{}, "id = ?", fixture.connRows[1].Id, 1)

	batchConn := adminAuditRequest(fixture.router, http.MethodPost, "/api/admin/audit_conn/batchDelete", `{"ids":[`+uintToString(fixture.connRows[1].Id)+`]}`, fixture.adminToken)
	if batchConn.Code != http.StatusOK {
		t.Fatalf("conn batch delete status = %d, want %d; body=%q", batchConn.Code, http.StatusOK, batchConn.Body.String())
	}
	assertAdminAuditResponseCode(t, batchConn.Body.Bytes(), 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditConn{}, "id = ?", fixture.connRows[1].Id, 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditConn{}, "id = ?", fixture.connRows[2].Id, 1)

	deleteFile := adminAuditRequest(fixture.router, http.MethodPost, "/api/admin/audit_file/delete", `{"id":`+uintToString(fixture.fileRows[0].Id)+`}`, fixture.adminToken)
	if deleteFile.Code != http.StatusOK {
		t.Fatalf("file delete status = %d, want %d; body=%q", deleteFile.Code, http.StatusOK, deleteFile.Body.String())
	}
	assertAdminAuditResponseCode(t, deleteFile.Body.Bytes(), 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditFile{}, "id = ?", fixture.fileRows[0].Id, 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditFile{}, "id = ?", fixture.fileRows[1].Id, 1)

	batchFile := adminAuditRequest(fixture.router, http.MethodPost, "/api/admin/audit_file/batchDelete", `{"ids":[`+uintToString(fixture.fileRows[1].Id)+`]}`, fixture.adminToken)
	if batchFile.Code != http.StatusOK {
		t.Fatalf("file batch delete status = %d, want %d; body=%q", batchFile.Code, http.StatusOK, batchFile.Body.String())
	}
	assertAdminAuditResponseCode(t, batchFile.Body.Bytes(), 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditFile{}, "id = ?", fixture.fileRows[1].Id, 0)
	assertAdminAuditRowCount(t, fixture.db, &model.AuditFile{}, "id = ?", fixture.fileRows[2].Id, 1)
}

func assertAdminAuditResponseCode(t *testing.T, body []byte, want int) {
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

func assertAdminAuditRowCount(t *testing.T, db *gorm.DB, modelValue any, where string, arg any, want int64) {
	t.Helper()
	query := db.Model(modelValue)
	if where != "" {
		query = query.Where(where, arg)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != want {
		t.Fatalf("row count = %d, want %d", count, want)
	}
}

func uintToString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
