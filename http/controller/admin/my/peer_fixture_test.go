package my

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

func TestMyPeerListFiltersByCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Peer{}); err != nil {
		t.Fatalf("migrate: %v", err)
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

	isAdmin := false
	owner := &model.User{Username: "owner", IsAdmin: &isAdmin, Status: model.COMMON_STATUS_ENABLE}
	other := &model.User{Username: "other", IsAdmin: &isAdmin, Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("create other: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: owner.Id, Token: "owner-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	peers := []*model.Peer{
		{Id: "owner-peer", Uuid: "owner-uuid", UserId: owner.Id, Hostname: "owner-host"},
		{Id: "other-peer", Uuid: "other-uuid", UserId: other.Id, Hostname: "other-host"},
		{Id: "unassigned-peer", Uuid: "unassigned-uuid", Hostname: "unassigned-host"},
	}
	for _, peer := range peers {
		if err := db.Create(peer).Error; err != nil {
			t.Fatalf("create peer %s: %v", peer.Id, err)
		}
	}

	router := gin.New()
	router.GET("/api/admin/my/peer/list", middleware.BackendUserAuth(), (&Peer{}).List)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/my/peer/list", strings.NewReader(""))
	request.Header.Set("api-token", "owner-token")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				Id     uint   `json:"row_id"`
				PeerId string `json:"id"`
				UserId uint   `json:"user_id"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 0 {
		t.Fatalf("response code = %d; body=%q", payload.Code, recorder.Body.String())
	}
	if len(payload.Data.List) != 1 || payload.Data.List[0].PeerId != "owner-peer" {
		t.Fatalf("my peer list = %#v, want only owner peer", payload.Data.List)
	}
}
