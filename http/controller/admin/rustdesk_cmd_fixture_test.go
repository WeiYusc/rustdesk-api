package admin

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

type adminRustdeskCmdFixture struct {
	db         *gorm.DB
	router     *gin.Engine
	adminToken string
}

func setupAdminRustdeskCmdFixture(t *testing.T) adminRustdeskCmdFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.ServerCmd{}); err != nil {
		t.Fatalf("migrate rustdesk cmd models: %v", err)
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

	isAdmin := true
	user := &model.User{Username: "admin-rustdesk-cmd", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "admin-rustdesk-cmd-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create admin token: %v", err)
	}
	for _, cmd := range []*model.ServerCmd{
		{Cmd: "custom-one", Option: "", Explain: "custom one", Target: model.ServerCmdTargetIdServer},
		{Cmd: "custom-two", Option: "", Explain: "custom two", Target: model.ServerCmdTargetRelayServer},
	} {
		if err := db.Create(cmd).Error; err != nil {
			t.Fatalf("create custom command %s: %v", cmd.Cmd, err)
		}
	}

	router := gin.New()
	controller := &Rustdesk{}
	router.GET("/api/admin/rustdesk/cmdList", middleware.BackendUserAuth(), middleware.AdminPrivilege(), controller.CmdList)

	return adminRustdeskCmdFixture{db: db, router: router, adminToken: "admin-rustdesk-cmd-token"}
}

func adminRustdeskCmdRequest(router *gin.Engine, method string, target string, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(""))
	if token != "" {
		request.Header.Set("api-token", token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestAdminRustdeskCmdListPaginatesSystemAndCustomCommandsTogether(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := setupAdminRustdeskCmdFixture(t)

	systemCount := len(model.SysIdServerCmds) + len(model.SysRelayServerCmds)
	firstPage := adminRustdeskCmdRequest(fixture.router, http.MethodGet, "/api/admin/rustdesk/cmdList?page=1&page_size=3", fixture.adminToken)
	secondPage := adminRustdeskCmdRequest(fixture.router, http.MethodGet, "/api/admin/rustdesk/cmdList?page=2&page_size=3", fixture.adminToken)

	first := decodeAdminRustdeskCmdListResponse(t, firstPage.Body.Bytes())
	second := decodeAdminRustdeskCmdListResponse(t, secondPage.Body.Bytes())

	if first.Code != 0 || second.Code != 0 {
		t.Fatalf("cmdList response codes = %d/%d; first=%q second=%q", first.Code, second.Code, firstPage.Body.String(), secondPage.Body.String())
	}
	if first.Data.Total != systemCount+2 || second.Data.Total != systemCount+2 {
		t.Fatalf("cmdList totals = %d/%d, want %d", first.Data.Total, second.Data.Total, systemCount+2)
	}
	if len(first.Data.Commands) != 3 || len(second.Data.Commands) != 3 {
		t.Fatalf("cmdList page lengths = %d/%d, want 3/3", len(first.Data.Commands), len(second.Data.Commands))
	}
	if first.Data.Commands[0].Cmd != model.SysIdServerCmds[0].Cmd {
		t.Fatalf("first command = %q, want first system id command %q", first.Data.Commands[0].Cmd, model.SysIdServerCmds[0].Cmd)
	}
	if second.Data.Commands[0].Cmd == first.Data.Commands[0].Cmd {
		t.Fatalf("second page repeated first page system command %q", second.Data.Commands[0].Cmd)
	}
}

type adminRustdeskCmdListPayload struct {
	Code int `json:"code"`
	Data struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
		Commands []struct {
			Cmd    string `json:"cmd"`
			Target string `json:"target"`
		} `json:"list"`
	} `json:"data"`
}

func decodeAdminRustdeskCmdListResponse(t *testing.T, body []byte) adminRustdeskCmdListPayload {
	t.Helper()
	var payload adminRustdeskCmdListPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal cmdList response: %v; body=%q", err, string(body))
	}
	return payload
}
