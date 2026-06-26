package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
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

func setupPeerStateControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Peer{}, &model.LoginLog{}, &model.AddressBook{}, &model.User{}, &model.UserToken{}); err != nil {
		t.Fatalf("migrate peer state models: %v", err)
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
	return db
}

func postPeerStateJSON(router *gin.Engine, target string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestLoginBindsExistingPeerAndRecordsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/login", (&Login{}).Login)

	isAdmin := false
	passwordHash, err := utils.EncryptPassword("secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{Username: "alice", Password: passwordHash, Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.Peer{Id: "peer-1", Uuid: "uuid-1"}).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}

	recorder := postPeerStateJSON(router, "/api/login", `{"username":"alice","password":"secret123","id":"peer-1","uuid":"uuid-1","deviceInfo":{"type":"app","os":"Windows","name":"host-1"}}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"access_token"`) {
		t.Fatalf("body = %q, want access_token", recorder.Body.String())
	}

	var peer model.Peer
	if err := db.Where("id = ?", "peer-1").First(&peer).Error; err != nil {
		t.Fatalf("find peer: %v", err)
	}
	if peer.UserId != user.Id {
		t.Fatalf("peer UserId = %d, want %d", peer.UserId, user.Id)
	}

	var token model.UserToken
	if err := db.Where("user_id = ? and device_id = ? and device_uuid = ?", user.Id, "peer-1", "uuid-1").First(&token).Error; err != nil {
		t.Fatalf("find token: %v", err)
	}
	if token.Token == "" {
		t.Fatalf("token is empty")
	}

	var loginLog model.LoginLog
	if err := db.Where("user_id = ? and device_id = ? and uuid = ?", user.Id, "peer-1", "uuid-1").First(&loginLog).Error; err != nil {
		t.Fatalf("find login log: %v", err)
	}
	if loginLog.Client != "app" {
		t.Fatalf("login log client = %q, want app", loginLog.Client)
	}
}

func TestSysInfoCreatesUnattendedPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/sysinfo", (&Peer{}).SysInfo)

	recorder := postPeerStateJSON(router, "/api/sysinfo", `{"id":"peer-1","uuid":"uuid-1","hostname":"host-1","os":"Windows","username":"rd"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("body = %q, want SYSINFO_UPDATED", recorder.Body.String())
	}

	var peer model.Peer
	if err := db.Where("id = ?", "peer-1").First(&peer).Error; err != nil {
		t.Fatalf("find created peer: %v", err)
	}
	if peer.UserId != 0 {
		t.Fatalf("UserId = %d, want 0 for unattended peer", peer.UserId)
	}
}

func TestSysInfoIgnoresLoggedInDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/sysinfo", (&Peer{}).SysInfo)

	if err := db.Create(&model.LoginLog{UserId: 42, DeviceId: "peer-1", Uuid: "uuid-1"}).Error; err != nil {
		t.Fatalf("create login log: %v", err)
	}

	recorder := postPeerStateJSON(router, "/api/sysinfo", `{"id":"peer-1","uuid":"uuid-1","hostname":"host-1","os":"Windows","username":"rd"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Body.String() != "IGNORE" {
		t.Fatalf("body = %q, want IGNORE", recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.Peer{}).Where("id = ?", "peer-1").Count(&count).Error; err != nil {
		t.Fatalf("count peers: %v", err)
	}
	if count != 0 {
		t.Fatalf("peer count = %d, want 0 for logged-in device sysinfo", count)
	}
}

func TestSysInfoDoesNotOverwriteExistingLoggedInPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/sysinfo", (&Peer{}).SysInfo)

	if err := db.Create(&model.Peer{Id: "peer-1", Uuid: "uuid-1", Hostname: "existing-host", UserId: 42}).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if err := db.Create(&model.LoginLog{UserId: 42, DeviceId: "peer-1", Uuid: "uuid-1"}).Error; err != nil {
		t.Fatalf("create login log: %v", err)
	}

	recorder := postPeerStateJSON(router, "/api/sysinfo", `{"id":"peer-1","uuid":"uuid-1","hostname":"new-host","os":"Windows","username":"rd"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Body.String() != "IGNORE" {
		t.Fatalf("body = %q, want IGNORE", recorder.Body.String())
	}

	var peer model.Peer
	if err := db.Where("id = ?", "peer-1").First(&peer).Error; err != nil {
		t.Fatalf("find peer: %v", err)
	}
	if peer.Hostname != "existing-host" {
		t.Fatalf("Hostname = %q, want existing-host", peer.Hostname)
	}
	if peer.UserId != 42 {
		t.Fatalf("UserId = %d, want 42", peer.UserId)
	}
}

func TestHeartbeatReturnsEmptyObjectForInvalidPayloads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/heartbeat", (&Index{}).Heartbeat)

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{`},
		{name: "missing uuid", body: `{"id":"peer-1","ver":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := postPeerStateJSON(router, "/api/heartbeat", tc.body)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			if strings.TrimSpace(recorder.Body.String()) != "{}" {
				t.Fatalf("body = %q, want empty JSON object", recorder.Body.String())
			}
		})
	}
}

func TestHeartbeatReturnsEmptyObjectForUnknownPeerWithoutCreatingPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/heartbeat", (&Index{}).Heartbeat)

	recorder := postPeerStateJSON(router, "/api/heartbeat", `{"id":"missing-peer","uuid":"uuid-1","ver":1}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if strings.TrimSpace(recorder.Body.String()) != "{}" {
		t.Fatalf("body = %q, want empty JSON object", recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.Peer{}).Where("id = ?", "missing-peer").Count(&count).Error; err != nil {
		t.Fatalf("count peers: %v", err)
	}
	if count != 0 {
		t.Fatalf("peer count = %d, want 0 for unknown heartbeat", count)
	}
}

func TestSysInfoCreatedPeerHeartbeatUpdatesOnlineState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/sysinfo", (&Peer{}).SysInfo)
	router.POST("/api/heartbeat", (&Index{}).Heartbeat)

	sysinfo := postPeerStateJSON(router, "/api/sysinfo", `{"id":"peer-1","uuid":"uuid-1","hostname":"host-1","os":"Windows","username":"rd"}`)
	if sysinfo.Code != http.StatusOK {
		t.Fatalf("sysinfo status = %d, want %d; body=%q", sysinfo.Code, http.StatusOK, sysinfo.Body.String())
	}
	if sysinfo.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("sysinfo body = %q, want SYSINFO_UPDATED", sysinfo.Body.String())
	}

	heartbeat := postPeerStateJSON(router, "/api/heartbeat", `{"id":"peer-1","uuid":"uuid-1","ver":1}`)
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d; body=%q", heartbeat.Code, http.StatusOK, heartbeat.Body.String())
	}
	if strings.TrimSpace(heartbeat.Body.String()) != "{}" {
		t.Fatalf("heartbeat body = %q, want empty JSON object", heartbeat.Body.String())
	}

	var peer model.Peer
	if err := db.Where("id = ?", "peer-1").First(&peer).Error; err != nil {
		t.Fatalf("find peer: %v", err)
	}
	if peer.UserId != 0 {
		t.Fatalf("UserId = %d, want 0 for unattended peer", peer.UserId)
	}
	if peer.LastOnlineTime == 0 {
		t.Fatalf("LastOnlineTime = 0, want heartbeat to update online timestamp")
	}
	if peer.LastOnlineIp == "" {
		t.Fatalf("LastOnlineIp is empty, want heartbeat client IP")
	}
}

func TestHeartbeatDeletesLoggedInUnboundPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/heartbeat", (&Index{}).Heartbeat)

	if err := db.Create(&model.Peer{Id: "peer-1", Uuid: "uuid-1"}).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if err := db.Create(&model.LoginLog{UserId: 42, DeviceId: "peer-1", Uuid: "uuid-1"}).Error; err != nil {
		t.Fatalf("create login log: %v", err)
	}

	recorder := postPeerStateJSON(router, "/api/heartbeat", `{"id":"peer-1","uuid":"uuid-1","ver":1}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.Peer{}).Where("id = ?", "peer-1").Count(&count).Error; err != nil {
		t.Fatalf("count peers: %v", err)
	}
	if count != 0 {
		t.Fatalf("peer count = %d, want 0 after logged-in unbound heartbeat", count)
	}
}

func TestHeartbeatKeepsLoggedInPeerWithAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/heartbeat", (&Index{}).Heartbeat)

	if err := db.Create(&model.Peer{Id: "peer-1", Uuid: "uuid-1", Alias: "kept"}).Error; err != nil {
		t.Fatalf("create peer: %v", err)
	}
	if err := db.Create(&model.LoginLog{UserId: 42, DeviceId: "peer-1", Uuid: "uuid-1"}).Error; err != nil {
		t.Fatalf("create login log: %v", err)
	}

	recorder := postPeerStateJSON(router, "/api/heartbeat", `{"id":"peer-1","uuid":"uuid-1","ver":1}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var peer model.Peer
	if err := db.Where("id = ?", "peer-1").First(&peer).Error; err != nil {
		t.Fatalf("find peer: %v", err)
	}
	if peer.Alias != "kept" {
		t.Fatalf("Alias = %q, want kept", peer.Alias)
	}
}
