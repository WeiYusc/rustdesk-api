package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
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

type forgotPasswordFixture struct {
	db     *gorm.DB
	router *gin.Engine
	user   *model.User
	fake   *fakeForgotPasswordSender
}

type fakeForgotPasswordSender struct {
	messages []service.SMTPMessage
	err      error
}

func (f *fakeForgotPasswordSender) Send(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, message)
	return nil
}

func setupForgotPasswordFixture(t *testing.T) forgotPasswordFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open forgot password db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.LoginLog{}, &model.Setting{}, &model.EmailVerificationToken{}); err != nil {
		t.Fatalf("migrate forgot password models: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.DB = db
	global.Logger = logrus.New()
	global.Jwt = jwt.NewJwt("forgot-password-controller-secret", time.Hour)
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, locale := range []string{"en", "zh_CN"} {
		if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", locale+".toml")); err != nil {
			t.Fatalf("load %s locale: %v", locale, err)
		}
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.ApiInitValidator()
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)

	passwordHash, err := utils.EncryptPassword("old-password-123")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	isAdmin := false
	user := &model.User{Username: "reset-user", Email: "Reset@Example.Test", Password: passwordHash, Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create reset user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "existing-user-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create existing user token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	login := &Login{}
	group := engine.Group("/api/admin")
	group.POST("/forgot-password/request", login.ForgotPasswordRequest)
	group.POST("/forgot-password/reset", login.ForgotPasswordReset)

	fake := &fakeForgotPasswordSender{}
	setEmailVerificationSenderForTest(t, fake)
	return forgotPasswordFixture{db: db, router: engine, user: user, fake: fake}
}

func (f forgotPasswordFixture) request(path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func TestForgotPasswordRequestWithConfiguredSMTPSendsResetTokenWithoutExposingIt(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)

	response := fixture.request("/api/admin/forgot-password/request", `{"email":" reset@example.test "}`)

	assertForgotPasswordResponseCode(t, response, 0)
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(fixture.fake.messages))
	}
	message := fixture.fake.messages[0]
	if message.To != "reset@example.test" {
		t.Fatalf("message to = %q, want normalized reset@example.test", message.To)
	}
	resetToken := extractResetTokenFromBody(t, message.TextBody)
	if resetToken == "" || len(resetToken) < 32 {
		t.Fatalf("weak or missing reset token in body: %q", message.TextBody)
	}
	body := response.Body.String()
	if strings.Contains(body, resetToken) || strings.Contains(body, "code_hash") || strings.Contains(body, "CodeHash") {
		t.Fatalf("response exposed reset secret/hash: %q", body)
	}
	var stored model.EmailVerificationToken
	if err := fixture.db.First(&stored).Error; err != nil {
		t.Fatalf("load stored reset token: %v", err)
	}
	if stored.Purpose != model.EmailVerificationPurposePasswordReset || stored.Email != "reset@example.test" || stored.UserId != fixture.user.Id || stored.CodeHash == "" || stored.UsedAt != nil {
		t.Fatalf("stored reset token = %#v", stored)
	}
	if strings.Contains(stored.CodeHash, resetToken) {
		t.Fatalf("stored hash contains raw token: hash=%q token=%q", stored.CodeHash, resetToken)
	}
}

func TestForgotPasswordRequestDoesNotEnumerateUnknownEmail(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)

	response := fixture.request("/api/admin/forgot-password/request", `{"email":"missing@example.test"}`)

	assertForgotPasswordResponseCode(t, response, 0)
	if len(fixture.fake.messages) != 0 {
		t.Fatalf("sent messages for missing user = %d, want 0", len(fixture.fake.messages))
	}
	assertForgotPasswordTokenCount(t, fixture.db, 0)
}

func TestForgotPasswordRequestSendFailureDoesNotEnumerateExistingEmail(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	fixture.fake.err = errors.New("smtp down")

	response := fixture.request("/api/admin/forgot-password/request", `{"email":"reset@example.test"}`)

	assertForgotPasswordResponseCode(t, response, 0)
	var stored model.EmailVerificationToken
	if err := fixture.db.First(&stored).Error; err != nil {
		t.Fatalf("load reset token after send failure: %v", err)
	}
	if stored.UsedAt == nil {
		t.Fatalf("send failure left reset challenge usable: %#v", stored)
	}
}

func TestForgotPasswordRequestCooldownDoesNotEnumerateExistingEmail(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	first := fixture.request("/api/admin/forgot-password/request", `{"email":"reset@example.test"}`)
	assertForgotPasswordResponseCode(t, first, 0)
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("first send messages = %d, want 1", len(fixture.fake.messages))
	}

	second := fixture.request("/api/admin/forgot-password/request", `{"email":"reset@example.test"}`)

	assertForgotPasswordResponseCode(t, second, 0)
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("cooldown request sent another email; messages=%d", len(fixture.fake.messages))
	}
}

func TestForgotPasswordResetRejectsWeakPasswords(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	request := fixture.request("/api/admin/forgot-password/request", `{"email":"reset@example.test"}`)
	assertForgotPasswordResponseCode(t, request, 0)
	resetToken := extractResetTokenFromBody(t, fixture.fake.messages[0].TextBody)

	weak := fixture.request("/api/admin/forgot-password/reset", `{"token":"`+resetToken+`","password":"abc","confirm_password":"abc"}`)

	assertForgotPasswordResponseCode(t, weak, 101)
}

func TestForgotPasswordResetUpdatesPasswordConsumesTokenAndFlushesSessions(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	request := fixture.request("/api/admin/forgot-password/request", `{"email":"reset@example.test"}`)
	assertForgotPasswordResponseCode(t, request, 0)
	resetToken := extractResetTokenFromBody(t, fixture.fake.messages[0].TextBody)

	reset := fixture.request("/api/admin/forgot-password/reset", `{"token":"`+resetToken+`","password":"new-password-456","confirm_password":"new-password-456"}`)

	assertForgotPasswordResponseCode(t, reset, 0)
	var stored model.EmailVerificationToken
	if err := fixture.db.First(&stored).Error; err != nil {
		t.Fatalf("load stored reset token: %v", err)
	}
	if stored.UsedAt == nil {
		t.Fatalf("reset token was not consumed: %#v", stored)
	}
	var sessionCount int64
	if err := fixture.db.Model(&model.UserToken{}).Where("user_id = ?", fixture.user.Id).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count user tokens: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("existing user sessions after reset = %d, want 0", sessionCount)
	}
	if got := service.AllService.UserService.InfoByUsernamePassword("reset-user", "new-password-456"); got.Id != fixture.user.Id {
		t.Fatalf("new password did not authenticate user: %#v", got)
	}
	if got := service.AllService.UserService.InfoByUsernamePassword("reset-user", "old-password-123"); got.Id != 0 {
		t.Fatalf("old password still authenticates user: %#v", got)
	}
}

func TestForgotPasswordResetRejectsReusedInvalidAndMismatchedTokens(t *testing.T) {
	fixture := setupForgotPasswordFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	request := fixture.request("/api/admin/forgot-password/request", `{"email":"reset@example.test"}`)
	assertForgotPasswordResponseCode(t, request, 0)
	resetToken := extractResetTokenFromBody(t, fixture.fake.messages[0].TextBody)

	mismatch := fixture.request("/api/admin/forgot-password/reset", `{"token":"`+resetToken+`","password":"new-password-456","confirm_password":"different-password"}`)
	assertForgotPasswordResponseCode(t, mismatch, 101)

	first := fixture.request("/api/admin/forgot-password/reset", `{"token":"`+resetToken+`","password":"new-password-456","confirm_password":"new-password-456"}`)
	assertForgotPasswordResponseCode(t, first, 0)
	reuse := fixture.request("/api/admin/forgot-password/reset", `{"token":"`+resetToken+`","password":"another-password-789","confirm_password":"another-password-789"}`)
	assertForgotPasswordResponseCode(t, reuse, 101)
	invalid := fixture.request("/api/admin/forgot-password/reset", `{"token":"not-a-real-token","password":"another-password-789","confirm_password":"another-password-789"}`)
	assertForgotPasswordResponseCode(t, invalid, 101)
}

func assertForgotPasswordResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != want {
		t.Fatalf("response code = %d, want %d; body=%q", payload.Code, want, recorder.Body.String())
	}
}

func assertForgotPasswordTokenCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.EmailVerificationToken{}).Count(&count).Error; err != nil {
		t.Fatalf("count reset tokens: %v", err)
	}
	if count != want {
		t.Fatalf("reset token count = %d, want %d", count, want)
	}
}

func extractResetTokenFromBody(t *testing.T, body string) string {
	t.Helper()
	marker := "token="
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatalf("reset body missing token marker: %q", body)
	}
	token := body[idx+len(marker):]
	if end := strings.IndexAny(token, " \r\n"); end >= 0 {
		token = token[:end]
	}
	return strings.TrimSpace(token)
}
