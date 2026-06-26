package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

func createCompatibilityFixtureUser(t *testing.T, db *gorm.DB, isAdmin bool, token string) *model.User {
	t.Helper()

	user := &model.User{
		Username: "compat-user-" + token,
		Status:   model.COMMON_STATUS_ENABLE,
		IsAdmin:  &isAdmin,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserToken{
		UserId:    user.Id,
		Token:     token,
		ExpiredAt: time.Now().Add(time.Hour).Unix(),
	}).Error; err != nil {
		t.Fatalf("create user token: %v", err)
	}
	return user
}

func postCompatibilityJSON(router *gin.Engine, target string, body string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func getCompatibilityJSON(router *gin.Engine, target string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestServerConfigRoutesRequireRustAuthAndExposeConfiguredServerValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	global.Config.Rustdesk = config.Rustdesk{
		IdServer: "rd.example.test:21116",
		Key:      "fixture-public-key",
	}

	router := gin.New()
	webClient := &WebClient{}
	router.POST("/api/server-config", middleware.RustAuth(), webClient.ServerConfig)
	router.POST("/api/server-config-v2", middleware.RustAuth(), webClient.ServerConfigV2)

	unauthenticated := postCompatibilityJSON(router, "/api/server-config", `{}`, "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d; body=%q", unauthenticated.Code, http.StatusUnauthorized, unauthenticated.Body.String())
	}

	createCompatibilityFixtureUser(t, db, false, "compat-token")
	for _, route := range []string{"/api/server-config", "/api/server-config-v2"} {
		recorder := postCompatibilityJSON(router, route, `{}`, "compat-token")
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d; body=%q", route, recorder.Code, http.StatusOK, recorder.Body.String())
		}

		var payload struct {
			Code int `json:"code"`
			Data struct {
				IdServer string         `json:"id_server"`
				Key      string         `json:"key"`
				Peers    map[string]any `json:"peers"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s unmarshal response: %v; body=%q", route, err, recorder.Body.String())
		}
		if payload.Code != 0 {
			t.Fatalf("%s code = %d, want 0", route, payload.Code)
		}
		if payload.Data.IdServer != "rd.example.test:21116" {
			t.Fatalf("%s id_server = %q", route, payload.Data.IdServer)
		}
		if payload.Data.Key != "fixture-public-key" {
			t.Fatalf("%s key = %q", route, payload.Data.Key)
		}
		if route == "/api/server-config" && payload.Data.Peers == nil {
			t.Fatalf("%s peers map is nil", route)
		}
		if route == "/api/server-config-v2" && payload.Data.Peers != nil {
			t.Fatalf("%s peers = %#v, want omitted", route, payload.Data.Peers)
		}
	}
}

func TestSysInfoVerReturnsVersionAndStartTimeLines(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPeerStateControllerTestDB(t)

	router := gin.New()
	router.POST("/api/sysinfo_ver", (&Peer{}).SysInfoVer)

	recorder := postCompatibilityJSON(router, "/api/sysinfo_ver", `{}`, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := strings.TrimRight(recorder.Body.String(), "\n")
	lines := strings.Split(body, "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[len(lines)-1]) == "" {
		t.Fatalf("body = %q, want a trailing start-time line", recorder.Body.String())
	}
	if _, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(lines[len(lines)-1])); err != nil {
		t.Fatalf("last line = %q, want start time format: %v", lines[len(lines)-1], err)
	}
}

func TestDeviceGroupAccessibleRequiresAdminAndReturnsGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPeerStateControllerTestDB(t)
	if err := db.AutoMigrate(&model.DeviceGroup{}); err != nil {
		t.Fatalf("migrate device groups: %v", err)
	}
	if err := db.Create(&model.DeviceGroup{Name: "ops-devices"}).Error; err != nil {
		t.Fatalf("create device group: %v", err)
	}

	router := gin.New()
	router.GET("/api/device-group/accessible", middleware.RustAuth(), (&Group{}).Device)

	createCompatibilityFixtureUser(t, db, false, "non-admin-token")
	nonAdmin := getCompatibilityJSON(router, "/api/device-group/accessible", "non-admin-token")
	if nonAdmin.Code != http.StatusBadRequest {
		t.Fatalf("non-admin status = %d, want %d; body=%q", nonAdmin.Code, http.StatusBadRequest, nonAdmin.Body.String())
	}

	createCompatibilityFixtureUser(t, db, true, "admin-token")
	admin := getCompatibilityJSON(router, "/api/device-group/accessible", "admin-token")
	if admin.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want %d; body=%q", admin.Code, http.StatusOK, admin.Body.String())
	}
	var payload struct {
		Total int `json:"total"`
		Data  []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(admin.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal admin response: %v; body=%q", err, admin.Body.String())
	}
	if len(payload.Data) != 1 || payload.Data[0].Name != "ops-devices" {
		t.Fatalf("device groups = %#v, want ops-devices", payload.Data)
	}
}
