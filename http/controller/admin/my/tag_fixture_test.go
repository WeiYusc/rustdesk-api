package my

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

func TestMyTagBatchDeleteRequiresCurrentUserOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Tag{}); err != nil {
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
	ownerTag := &model.Tag{Name: "owner-tag", UserId: owner.Id, Color: 1}
	otherTag := &model.Tag{Name: "other-tag", UserId: other.Id, Color: 2}
	if err := db.Create(ownerTag).Error; err != nil {
		t.Fatalf("create owner tag: %v", err)
	}
	if err := db.Create(otherTag).Error; err != nil {
		t.Fatalf("create other tag: %v", err)
	}

	router := gin.New()
	router.POST("/api/admin/my/tag/batchDelete", middleware.BackendUserAuth(), (&Tag{}).BatchDelete)

	mixedDelete := myTagRequest(router, `{"ids":[`+uintToString(ownerTag.Id)+`,`+uintToString(otherTag.Id)+`]}`)
	assertMyTagResponseCode(t, mixedDelete.Body.Bytes(), 101)
	assertMyTagRowCount(t, db, "id in ?", []any{[]uint{ownerTag.Id, otherTag.Id}}, 2)

	ownerDelete := myTagRequest(router, `{"ids":[`+uintToString(ownerTag.Id)+`]}`)
	assertMyTagResponseCode(t, ownerDelete.Body.Bytes(), 0)
	assertMyTagRowCount(t, db, "id = ?", []any{ownerTag.Id}, 0)
	assertMyTagRowCount(t, db, "id = ?", []any{otherTag.Id}, 1)
}

func myTagRequest(router *gin.Engine, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/my/tag/batchDelete", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("api-token", "owner-token")
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertMyTagResponseCode(t *testing.T, body []byte, want int) {
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

func assertMyTagRowCount(t *testing.T, db *gorm.DB, where string, args []any, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.Tag{}).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if count != want {
		t.Fatalf("tag row count = %d, want %d", count, want)
	}
}

func uintToString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
