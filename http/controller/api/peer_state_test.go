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

func createPeerStateLoginUser(t *testing.T, db *gorm.DB, username string, password string) *model.User {
	t.Helper()

	isAdmin := false
	passwordHash, err := utils.EncryptPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &model.User{Username: username, Password: passwordHash, Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func TestLoginBindsExistingPeerAndRecordsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/login", (&Login{}).Login)

	user := createPeerStateLoginUser(t, db, "alice", "secret123")
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

func TestLoginAllowsValidCredentialsWhenCaptchaThresholdReached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: 1, BanThreshold: 3})
	router := gin.New()
	router.POST("/api/login", (&Login{}).Login)
	createPeerStateLoginUser(t, db, "captcha-user", "secret123")

	bad := postPeerStateJSON(router, "/api/login", `{"username":"captcha-user","password":"bad-password","id":"peer-1","uuid":"uuid-1"}`)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad login status = %d, want %d; body=%q", bad.Code, http.StatusBadRequest, bad.Body.String())
	}

	valid := postPeerStateJSON(router, "/api/login", `{"username":"captcha-user","password":"secret123","id":"peer-1","uuid":"uuid-1"}`)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid login after captcha threshold status = %d, want %d; body=%q", valid.Code, http.StatusOK, valid.Body.String())
	}
	if !strings.Contains(valid.Body.String(), `"access_token"`) {
		t.Fatalf("body = %q, want access_token", valid.Body.String())
	}
}

func TestLoginRejectsBannedClientBeforeCredentialCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 2})
	router := gin.New()
	router.POST("/api/login", (&Login{}).Login)
	createPeerStateLoginUser(t, db, "banned-user", "secret123")

	for i := 0; i < 2; i++ {
		bad := postPeerStateJSON(router, "/api/login", `{"username":"banned-user","password":"bad-password","id":"peer-1","uuid":"uuid-1"}`)
		if bad.Code != http.StatusBadRequest {
			t.Fatalf("bad login %d status = %d, want %d; body=%q", i+1, bad.Code, http.StatusBadRequest, bad.Body.String())
		}
	}

	banned := postPeerStateJSON(router, "/api/login", `{"username":"banned-user","password":"secret123","id":"peer-1","uuid":"uuid-1"}`)
	if banned.Code != http.StatusTooManyRequests {
		t.Fatalf("banned login status = %d, want %d; body=%q", banned.Code, http.StatusTooManyRequests, banned.Body.String())
	}
	if strings.Contains(banned.Body.String(), `"access_token"`) {
		t.Fatalf("banned login returned token: body=%q", banned.Body.String())
	}

	var count int64
	if err := db.Model(&model.UserToken{}).Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count != 0 {
		t.Fatalf("token count = %d, want 0 after banned login", count)
	}
}

func TestSysInfoRejectsMissingPeerIDWithoutCreatingPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/sysinfo", (&Peer{}).SysInfo)

	recorder := postPeerStateJSON(router, "/api/sysinfo", `{"uuid":"uuid-1","hostname":"host-1","os":"Windows","username":"rd"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}

	var count int64
	if err := db.Model(&model.Peer{}).Count(&count).Error; err != nil {
		t.Fatalf("count peers: %v", err)
	}
	if count != 0 {
		t.Fatalf("peer count = %d, want 0 for invalid sysinfo", count)
	}
}

func TestSysInfoCreatesPeerWhenLegacyPayloadOmitsUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	router := gin.New()
	router.POST("/api/sysinfo", (&Peer{}).SysInfo)

	recorder := postPeerStateJSON(router, "/api/sysinfo", `{"id":"legacy-peer","hostname":"legacy-host","os":"Windows","username":"rd"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Body.String() != "SYSINFO_UPDATED" {
		t.Fatalf("body = %q, want SYSINFO_UPDATED", recorder.Body.String())
	}

	var peer model.Peer
	if err := db.Where("id = ?", "legacy-peer").First(&peer).Error; err != nil {
		t.Fatalf("find created legacy peer: %v", err)
	}
	if peer.Uuid != "" {
		t.Fatalf("legacy peer uuid = %q, want empty", peer.Uuid)
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
