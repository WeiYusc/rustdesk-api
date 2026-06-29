package admin

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestAdminFileUploadStoresOnlySafeImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resourcesPath := t.TempDir()
	router := setupAdminFileFixture(t, resourcesPath)

	pngBody := encodeTinyPNG(t)
	success := adminFileUploadRequest(t, router, "avatar.png", "image/png", pngBody, "file-token")
	if success.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want %d; body=%q", success.Code, http.StatusOK, success.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(success.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	if payload.Code != 0 || !strings.HasPrefix(payload.Data.URL, "/upload/avatar/") || !strings.HasSuffix(payload.Data.URL, ".png") {
		t.Fatalf("upload payload = %#v", payload)
	}
	storedPath := filepath.Join(resourcesPath, "public", "upload", strings.TrimPrefix(payload.Data.URL, "/upload/"))
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("uploaded file missing at %s: %v", storedPath, err)
	}

	textFile := adminFileUploadRequest(t, router, "note.txt", "text/plain", []byte("not an image"), "file-token")
	assertAdminFileResponseCode(t, textFile.Body.Bytes(), 101)

	pathTraversal := adminFileUploadRequest(t, router, `..\\avatar.png`, "image/png", pngBody, "file-token")
	assertAdminFileResponseCode(t, pathTraversal.Body.Bytes(), 101)
}

func setupAdminFileFixture(t *testing.T, resourcesPath string) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.Config.Gin.ResourcesPath = resourcesPath
	global.Logger = logrus.New()
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(i18n.NewBundle(language.English)) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	global.Jwt = jwt.NewJwt("", 0)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)
	isAdmin := true
	user := &model.User{Username: "file-admin", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "file-token", ExpiredAt: serviceTokenExpiryForAdminFileFixture()}).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	router := gin.New()
	fileGroup := router.Group("/api/admin/file").Use(middleware.BackendUserAuth())
	fileGroup.POST("/upload", (&File{}).Upload)
	return router
}

func serviceTokenExpiryForAdminFileFixture() int64 {
	return time.Now().Add(time.Hour).Unix()
}

func adminFileUploadRequest(t *testing.T, router *gin.Engine, filename string, contentType string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	_ = writer.WriteField("content_type", contentType)
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/file/upload", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("api-token", token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func encodeTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func assertAdminFileResponseCode(t *testing.T, body []byte, want int) {
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
