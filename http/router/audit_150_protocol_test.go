package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func audit150Fixture(t *testing.T, limit int, bodyLimit int64) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.AuditConn{}, &model.AuditFile{}); err != nil {
		t.Fatal(err)
	}
	oldConfig, oldLogger, oldLocalizer := global.Config, global.Logger, global.Localizer
	oldDB, oldService := service.DB, service.AllService
	t.Cleanup(func() {
		global.Config, global.Logger, global.Localizer = oldConfig, oldLogger, oldLocalizer
		service.DB, service.AllService = oldDB, oldService
	})
	global.Config = config.Config{Lang: "en"}
	global.Config.Gin.ReportingRateLimitPerMin = limit
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(i18n.NewBundle(language.English)) }
	service.DB, service.AllService = db, &service.Service{AuditService: &service.AuditService{}}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join(cwd, "..", "..")); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.BodyLimit(bodyLimit), gin.Recovery())
	ApiInit(r)
	return r, db
}

func audit150Post(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/audit/"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.0.2.1:12345"
	r.ServeHTTP(w, req)
	return w
}

func audit150Empty(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("status/body = %d/%q, want 200/zero bytes", w.Code, w.Body.String())
	}
	if got := audit150ClientOutcome(w.Code, w.Body.String()); got != "success" {
		t.Errorf("real router acknowledgement consumed as %s, want success", got)
	}
}

// Predicate-only port of official RustDesk 1.5.0
// fada664df7a294d1d1a9ca3e7cd3637069122f17 src/server/connection.rs:1690-1725.
// This is not official-binary E2E, a backoff timer, or an exactly-once claim.
func audit150ClientOutcome(status int, text string) string {
	if status >= 200 && status < 300 {
		if strings.TrimSpace(text) == "" {
			return "success"
		}
		return "retry"
	}
	if status >= 500 || status == 408 || status == 429 {
		return "retry"
	}
	return "reject"
}

func TestAudit150ClientConsumptionPredicate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body, want string
	}{
		{"200_empty", 200, "", "success"},
		{"200_whitespace", 200, " 	\r\n\u0085\u00a0\u2003", "success"},
		{"201_empty", 201, "", "success"},
		{"299_whitespace", 299, " \n", "success"},
		{"200_legacy_success_envelope", 200, `{"code":0,"message":"success","data":null}`, "retry"},
		{"200_error_json", 200, `{"error":"write failed"}`, "retry"},
		{"204_nonempty", 204, "maintenance", "retry"},
		{"503_empty", 503, "", "retry"},
		{"503_error_json", 503, `{"error":"audit temporarily unavailable"}`, "retry"},
		{"408_timeout", 408, "", "retry"},
		{"429_limited", 429, `{"error":"too many reporting requests"}`, "retry"},
		{"400_rejection", 400, `{"error":"bad request"}`, "reject"},
		{"199_not_success", 199, "", "reject"},
		{"300_not_success", 300, "", "reject"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := audit150ClientOutcome(tc.status, tc.body); got != tc.want {
				t.Fatalf("consumer(%d, %q)=%s, want %s", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestAudit150AnonymousLifecycleAndLegacyNoops(t *testing.T) {
	r, db := audit150Fixture(t, 0, 4096)
	for _, tc := range []struct{ name, body string }{
		{"new", `{"id":"target","action":"new","conn_id":42,"peer":["source","Source"],"ip":"198.51.100.10","session_id":123,"type":1,"nonce":"same","unknown":{"x":1}}`},
		// Official 1.5 auth fields are numeric: TemporaryPassword=2, Totp=1.
		// They are anonymous self-reports, not trusted identity or persisted fields.
		{"update_150_primary_auth_two_factor", `{"id":"target","conn_id":42,"peer":["updated","Updated"],"session_id":456,"type":2,"ip":"203.0.113.1","uuid":"ignored","nonce":"same","primary_auth":2,"two_factor":1}`},
		{"close", `{"id":"target","action":"close","conn_id":42,"nonce":"same"}`},
		{"missing_close", `{"id":"absent","action":"close","conn_id":99}`},
		{"missing_update", `{"id":"absent","conn_id":99}`},
		{"zero_close", `{"id":"absent","action":"close"}`},
		{"zero_update", `{"id":"absent"}`},
		{"unknown_action", `{"id":"target","action":"update","conn_id":42,"peer":["do-not-save"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) { audit150Empty(t, audit150Post(r, "conn", tc.body)) })
	}
	var row model.AuditConn
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.FromPeer != "updated" || row.FromName != "Updated" || row.SessionId != "456" || row.Type != 2 || row.Ip != "198.51.100.10" || row.Uuid != "" || row.CloseTime == 0 {
		t.Fatalf("persisted row %#v", row)
	}
	var count int64
	if err := db.Model(&model.AuditConn{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	for _, table := range []string{"audit_conns", "audit_files"} {
		for _, field := range []string{"nonce", "primary_auth", "two_factor"} {
			if db.Migrator().HasColumn(table, field) {
				t.Fatalf("%s schema introduced in %s", field, table)
			}
		}
	}
}

func TestAudit150FileLegacyAndNonceAreAcceptedWithoutDeduplication(t *testing.T) {
	r, db := audit150Fixture(t, 0, 4096)
	for _, body := range []string{
		`{"id":"target","peer_id":"source","info":"{\"ip\":\"198.51.100.20\",\"name\":\"Source\",\"num\":2}","path":"/fixture.txt","is_file":true,"type":3}`,
		`{"id":"target","peer_id":"source","info":"{}","path":"/nonce.txt","uuid":"u","nonce":"repeat","future":true}`,
		`{"id":"target","peer_id":"source","info":"{}","path":"/nonce.txt","uuid":"u","nonce":"repeat"}`,
	} {
		audit150Empty(t, audit150Post(r, "file", body))
	}
	var rows []model.AuditFile
	if err := db.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Uuid != "" || rows[0].Ip != "198.51.100.20" || rows[0].FromName != "Source" || rows[0].Num != 2 || rows[0].Path != "/fixture.txt" || !rows[0].IsFile || rows[0].Type != 3 || rows[1].Uuid != "u" {
		t.Fatalf("rows %#v", rows)
	}
}

func TestAudit150DatabaseFailuresAre503AndRecover(t *testing.T) {
	for _, tc := range []struct{ name, operation, path, body string }{
		{"conn_create", "create", "conn", `{"id":"target","action":"new","conn_id":43}`},
		{"file_create", "create", "file", `{"id":"target","info":"{}"}`},
		{"close_query", "query", "conn", `{"id":"target","action":"close","conn_id":42}`},
		{"update_query", "query", "conn", `{"id":"target","conn_id":42,"peer":["updated"]}`},
		{"missing_query", "query", "conn", `{"id":"absent","action":"close"}`},
		{"close_update", "update", "conn", `{"id":"target","action":"close","conn_id":42}`},
		{"conn_update", "update", "conn", `{"id":"target","conn_id":42,"peer":["updated"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, db := audit150Fixture(t, 0, 4096)
			seed := &model.AuditConn{PeerId: "target", ConnId: 42, FromPeer: "original"}
			if err := db.Create(seed).Error; err != nil {
				t.Fatal(err)
			}
			injected := errors.New("synthetic SQL error: private DSN /fixture/path")
			fail := func(tx *gorm.DB) { tx.AddError(injected) }
			var remove func() error
			switch tc.operation {
			case "create":
				if err := db.Callback().Create().Before("gorm:create").Register("audit150:fail", fail); err != nil {
					t.Fatal(err)
				}
				remove = func() error { return db.Callback().Create().Remove("audit150:fail") }
			case "query":
				if err := db.Callback().Query().Before("gorm:query").Register("audit150:fail", fail); err != nil {
					t.Fatal(err)
				}
				remove = func() error { return db.Callback().Query().Remove("audit150:fail") }
			case "update":
				if err := db.Callback().Update().Before("gorm:update").Register("audit150:fail", fail); err != nil {
					t.Fatal(err)
				}
				remove = func() error { return db.Callback().Update().Remove("audit150:fail") }
			}
			w := audit150Post(r, tc.path, tc.body)
			if w.Code != 503 || w.Body.String() != `{"error":"audit temporarily unavailable"}` {
				t.Errorf("status/body=%d/%q want fixed redacted 503", w.Code, w.Body.String())
			}
			if got := audit150ClientOutcome(w.Code, w.Body.String()); got != "retry" {
				t.Errorf("real router DB failure consumed as %s, want retry", got)
			}
			if err := remove(); err != nil {
				t.Fatal(err)
			}
			var row model.AuditConn
			if err := db.First(&row, seed.Id).Error; err != nil {
				t.Fatal(err)
			}
			if row.FromPeer != "original" || row.CloseTime != 0 {
				t.Fatalf("failed write mutated row %#v", row)
			}
			var files, conns int64
			if err := db.Model(&model.AuditFile{}).Count(&files).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.AuditConn{}).Count(&conns).Error; err != nil {
				t.Fatal(err)
			}
			if files != 0 || conns != 1 {
				t.Fatalf("failed request wrote rows: files=%d conns=%d", files, conns)
			}
			audit150Empty(t, audit150Post(r, tc.path, tc.body))
			if err := db.First(&row, seed.Id).Error; err != nil {
				t.Fatal(err)
			}
			if tc.operation == "update" && tc.name == "close_update" && row.CloseTime == 0 {
				t.Fatal("recovered close not persisted")
			}
			if (tc.name == "conn_update" || tc.name == "update_query") && row.FromPeer != "updated" {
				t.Fatal("recovered update not persisted")
			}
			if err := db.Model(&model.AuditFile{}).Count(&files).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.AuditConn{}).Count(&conns).Error; err != nil {
				t.Fatal(err)
			}
			if tc.name == "file_create" && files != 1 {
				t.Fatal("recovered file create not persisted")
			}
			if tc.name == "conn_create" && conns != 2 {
				t.Fatal("recovered conn create not persisted")
			}
		})
	}
}

func TestAudit150ValidationAndMiddlewareRemainCompatible(t *testing.T) {
	r, db := audit150Fixture(t, 0, 128)
	for _, tc := range []struct{ path, body string }{{"conn", "{"}, {"file", "{"}, {"conn", `{"action":"new","conn_id":42}`}, {"conn", `{"id":"target","action":"new","conn_id":0}`}, {"file", `{"info":"{}"}`}} {
		w := audit150Post(r, tc.path, tc.body)
		if w.Code != 400 {
			t.Errorf("%s %s status=%d", tc.path, tc.body, w.Code)
		}
		if got := audit150ClientOutcome(w.Code, w.Body.String()); got != "reject" {
			t.Errorf("real router validation consumed as %s, want reject", got)
		}
	}
	// Existing BodyLimit supplies MaxBytesError; audit binding historically maps it to 400.
	// The middleware's standalone 413 behavior is covered by its existing tests.
	for _, path := range []string{"conn", "file"} {
		w := audit150Post(r, path, `{"id":"target","padding":"`+strings.Repeat("x", 200)+`"}`)
		if w.Code != 400 {
			t.Errorf("oversized %s status=%d want baseline 400", path, w.Code)
		}
	}
	var count int64
	for _, m := range []interface{}{&model.AuditConn{}, &model.AuditFile{}} {
		if err := db.Model(m).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("validation side effect count=%d err=%v", count, err)
		}
	}
	limited, db2 := audit150Fixture(t, 1, 4096)
	for _, path := range []string{"conn", "file"} {
		body := `{"id":"absent","info":"{}"}`
		audit150Empty(t, audit150Post(limited, path, body))
		w := audit150Post(limited, path, body)
		if w.Code != 429 || w.Body.String() != `{"error":"too many reporting requests"}` {
			t.Errorf("rate response %d/%q", w.Code, w.Body.String())
		}
		if got := audit150ClientOutcome(w.Code, w.Body.String()); got != "retry" {
			t.Errorf("real router rate limit consumed as %s, want retry", got)
		}
	}
	if err := db2.Model(&model.AuditFile{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("rate limit side effect count=%d err=%v", count, err)
	}
}
