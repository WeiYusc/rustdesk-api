package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestAuditConnCreatesUpdatesAndClosesExistingConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}

	router := gin.New()
	router.POST("/api/audit/conn", (&Audit{}).AuditConn)

	created := postCompatibilityJSON(router, "/api/audit/conn", `{"action":"new","conn_id":42,"id":"target-peer","peer":["source-peer","Source Device"],"ip":"198.51.100.10","session_id":12345,"type":1,"uuid":"target-uuid"}`, "")
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d; body=%q", created.Code, http.StatusOK, created.Body.String())
	}
	assertAuditSuccessResponse(t, created.Body.Bytes())

	var auditConn model.AuditConn
	if err := db.Where("peer_id = ? and conn_id = ?", "target-peer", int64(42)).First(&auditConn).Error; err != nil {
		t.Fatalf("find audit conn: %v", err)
	}
	if auditConn.Action != model.AuditActionNew || auditConn.FromPeer != "source-peer" || auditConn.FromName != "Source Device" || auditConn.Ip != "198.51.100.10" || auditConn.SessionId != "12345" || auditConn.Type != 1 || auditConn.Uuid != "target-uuid" {
		t.Fatalf("created audit conn = %#v", auditConn)
	}

	updated := postCompatibilityJSON(router, "/api/audit/conn", `{"conn_id":42,"id":"target-peer","peer":["updated-source","Updated Device"],"ip":"203.0.113.20","session_id":67890,"type":2,"uuid":"ignored-uuid"}`, "")
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%q", updated.Code, http.StatusOK, updated.Body.String())
	}
	assertAuditSuccessResponse(t, updated.Body.Bytes())

	if err := db.Where("peer_id = ? and conn_id = ?", "target-peer", int64(42)).First(&auditConn).Error; err != nil {
		t.Fatalf("find updated audit conn: %v", err)
	}
	if auditConn.FromPeer != "updated-source" || auditConn.FromName != "Updated Device" || auditConn.SessionId != "67890" || auditConn.Type != 2 {
		t.Fatalf("updated audit conn = %#v", auditConn)
	}
	if auditConn.Ip != "198.51.100.10" || auditConn.Uuid != "target-uuid" || auditConn.CloseTime != 0 {
		t.Fatalf("unexpected non-mutable fields after update = %#v", auditConn)
	}

	closed := postCompatibilityJSON(router, "/api/audit/conn", `{"action":"close","conn_id":42,"id":"target-peer"}`, "")
	if closed.Code != http.StatusOK {
		t.Fatalf("close status = %d, want %d; body=%q", closed.Code, http.StatusOK, closed.Body.String())
	}
	assertAuditSuccessResponse(t, closed.Body.Bytes())

	if err := db.Where("peer_id = ? and conn_id = ?", "target-peer", int64(42)).First(&auditConn).Error; err != nil {
		t.Fatalf("find closed audit conn: %v", err)
	}
	if auditConn.CloseTime == 0 {
		t.Fatalf("close_time = 0, want set")
	}
}

func TestAuditFileCreatesFileAuditWithInfoMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}

	router := gin.New()
	router.POST("/api/audit/file", (&Audit{}).AuditFile)

	recorder := postCompatibilityJSON(router, "/api/audit/file", `{"id":"target-peer","peer_id":"source-peer","info":"{\"ip\":\"198.51.100.30\",\"name\":\"Source User\",\"num\":7}","is_file":true,"path":"/tmp/example.txt","type":3,"uuid":"file-uuid"}`, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	assertAuditSuccessResponse(t, recorder.Body.Bytes())

	var auditFile model.AuditFile
	if err := db.Where("peer_id = ? and from_peer = ?", "target-peer", "source-peer").First(&auditFile).Error; err != nil {
		t.Fatalf("find audit file: %v", err)
	}
	if !auditFile.IsFile || auditFile.Path != "/tmp/example.txt" || auditFile.Type != 3 || auditFile.Uuid != "file-uuid" {
		t.Fatalf("audit file core fields = %#v", auditFile)
	}
	if auditFile.FromName != "Source User" || auditFile.Ip != "198.51.100.30" || auditFile.Num != 7 {
		t.Fatalf("audit file derived info fields = %#v", auditFile)
	}
}

func TestAuditConnAcceptsLegacyCloseWithoutConnID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}

	router := gin.New()
	router.POST("/api/audit/conn", (&Audit{}).AuditConn)

	recorder := postCompatibilityJSON(router, "/api/audit/conn", `{"action":"close","id":"target-peer"}`, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	assertAuditSuccessResponse(t, recorder.Body.Bytes())

	var count int64
	if err := db.Model(&model.AuditConn{}).Count(&count).Error; err != nil {
		t.Fatalf("count audit conns: %v", err)
	}
	if count != 0 {
		t.Fatalf("audit conn count = %d, want 0 for legacy close without match", count)
	}
}

func TestAuditFileAcceptsLegacyPayloadWithoutUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}

	router := gin.New()
	router.POST("/api/audit/file", (&Audit{}).AuditFile)

	recorder := postCompatibilityJSON(router, "/api/audit/file", `{"id":"target-peer","peer_id":"source-peer","info":"{}","path":"/tmp/example.txt","type":3}`, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	assertAuditSuccessResponse(t, recorder.Body.Bytes())

	var auditFile model.AuditFile
	if err := db.Where("peer_id = ? and from_peer = ?", "target-peer", "source-peer").First(&auditFile).Error; err != nil {
		t.Fatalf("find legacy audit file: %v", err)
	}
	if auditFile.Uuid != "" {
		t.Fatalf("legacy audit file uuid = %q, want empty", auditFile.Uuid)
	}
}

func TestAuditConnRejectsMissingRequiredIdentityWithoutCreatingRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}

	router := gin.New()
	router.POST("/api/audit/conn", (&Audit{}).AuditConn)

	recorder := postCompatibilityJSON(router, "/api/audit/conn", `{"action":"new","conn_id":0,"peer":["source-peer"],"uuid":"target-uuid"}`, "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.AuditConn{}).Count(&count).Error; err != nil {
		t.Fatalf("count audit conns: %v", err)
	}
	if count != 0 {
		t.Fatalf("audit conn count = %d, want 0", count)
	}
}

func TestAuditFileRejectsMissingRequiredIdentityWithoutCreatingRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatalf("migrate audit models: %v", err)
	}

	router := gin.New()
	router.POST("/api/audit/file", (&Audit{}).AuditFile)

	recorder := postCompatibilityJSON(router, "/api/audit/file", `{"peer_id":"source-peer","info":"{}","path":"/tmp/example.txt","type":3}`, "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.AuditFile{}).Count(&count).Error; err != nil {
		t.Fatalf("count audit files: %v", err)
	}
	if count != 0 {
		t.Fatalf("audit file count = %d, want 0", count)
	}
}

func assertAuditSuccessResponse(t *testing.T, body []byte) {
	t.Helper()
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal success response: %v; body=%q", err, string(body))
	}
	if payload.Code != 0 || payload.Message != "success" || payload.Data != "" {
		t.Fatalf("success response = %#v", payload)
	}
}
