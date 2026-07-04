package admin

import (
	"context"
	"encoding/json"
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

type registerEmailFixture struct {
	db     *gorm.DB
	router *gin.Engine
	fake   *registerEmailFakeSender
}

type registerEmailFakeSender struct {
	messages []service.SMTPMessage
	err      error
}

func (f *registerEmailFakeSender) Send(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, message)
	return nil
}

func setupRegisterEmailFixture(t *testing.T) registerEmailFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite register email db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Setting{}, &model.EmailVerificationToken{}, &model.LoginLog{}); err != nil {
		t.Fatalf("migrate register email models: %v", err)
	}
	global.Config = config.Config{Lang: "en"}
	global.Config.App.Register = true
	global.Config.App.RegisterStatus = int(model.COMMON_STATUS_DISABLED)
	global.DB = db
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.ApiInitValidator()
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.Jwt = jwt.NewJwt("register-email-verification-secret", time.Hour)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/admin")
	user := &User{}
	group.POST("/user/register", user.Register)
	group.POST("/user/register/email/send", user.SendRegisterVerification)
	fake := &registerEmailFakeSender{}
	setEmailVerificationSenderForTest(t, fake)
	resetRegisterEmailRateLimiterForTest(t)
	return registerEmailFixture{db: db, router: engine, fake: fake}
}

func (f registerEmailFixture) post(target, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func configureRegisterEmailFixture(t *testing.T) {
	t.Helper()
	if err := service.AllService.SettingsService.SaveRegisterPolicy(service.RegisterPolicySettings{Enabled: true, DefaultStatus: int(model.COMMON_STATUS_DISABLED)}, 1); err != nil {
		t.Fatalf("save register policy: %v", err)
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true, RequireForRegister: true}, 1); err != nil {
		t.Fatalf("save email verification settings: %v", err)
	}
	if err := service.AllService.SettingsService.SaveSMTP(service.SMTPSettings{Enabled: true, Host: "smtp.example.test", Port: 587, Security: service.SMTPSecurityStartTLS, FromEmail: "noreply@example.test", FromName: "RustDesk Test", TimeoutSeconds: 10}, 1); err != nil {
		t.Fatalf("save smtp settings: %v", err)
	}
}

func TestRegisterEmailVerificationSendCreatesRegisterTokenAndHidesCode(t *testing.T) {
	fixture := setupRegisterEmailFixture(t)
	configureRegisterEmailFixture(t)

	response := fixture.post("/api/admin/user/register/email/send", `{"email":" NewUser@Example.Test "}`)

	assertRegisterEmailResponseCode(t, response, 0)
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("fake sender messages = %d, want 1", len(fixture.fake.messages))
	}
	message := fixture.fake.messages[0]
	if message.To != "newuser@example.test" {
		t.Fatalf("message To = %q, want normalized email", message.To)
	}
	code := extractEmailVerificationCode(t, message.TextBody)
	body := response.Body.String()
	if strings.Contains(body, code) || strings.Contains(body, "hash") || strings.Contains(body, "code_hash") {
		t.Fatalf("send response exposed secret material: %q", body)
	}
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token.UserId != 0 || token.Email != "newuser@example.test" || token.Purpose != model.EmailVerificationPurposeRegister || token.CodeHash == "" || token.UsedAt != nil {
		t.Fatalf("stored register token = %#v", token)
	}
}

func TestRegisterRequiresAndConsumesRegisterEmailCode(t *testing.T) {
	fixture := setupRegisterEmailFixture(t)
	configureRegisterEmailFixture(t)
	send := fixture.post("/api/admin/user/register/email/send", `{"email":"verify@example.test"}`)
	assertRegisterEmailResponseCode(t, send, 0)
	code := extractEmailVerificationCode(t, fixture.fake.messages[0].TextBody)

	missing := fixture.post("/api/admin/user/register", `{"username":"verify-user","password":"pass1234","confirm_password":"pass1234","email":"verify@example.test"}`)
	assertRegisterEmailResponseCode(t, missing, 101)
	wrong := fixture.post("/api/admin/user/register", `{"username":"verify-user","password":"pass1234","confirm_password":"pass1234","email":"verify@example.test","email_code":"000000"}`)
	assertRegisterEmailResponseCode(t, wrong, 101)
	assertRegisterEmailUserCount(t, fixture.db, "verify-user", 0)

	success := fixture.post("/api/admin/user/register", `{"username":"verify-user","password":"pass1234","confirm_password":"pass1234","email":" Verify@Example.Test ","email_code":"`+code+`"}`)
	assertRegisterEmailResponseCode(t, success, 0)
	var user model.User
	if err := fixture.db.Where("username = ?", "verify-user").First(&user).Error; err != nil {
		t.Fatalf("load registered user: %v", err)
	}
	if user.Email != "verify@example.test" || user.EmailVerifiedAt == nil || user.Status != model.COMMON_STATUS_DISABLED {
		t.Fatalf("registered user = %#v, want normalized verified pending-approval user", user)
	}
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load register token: %v", err)
	}
	if token.UsedAt == nil {
		t.Fatalf("successful registration did not consume register token: %#v", token)
	}
}

func TestRegisterEmailVerificationSendFailureMarksTokenUsed(t *testing.T) {
	fixture := setupRegisterEmailFixture(t)
	configureRegisterEmailFixture(t)
	fixture.fake.err = context.Canceled

	response := fixture.post("/api/admin/user/register/email/send", `{"email":"failed@example.test"}`)

	assertRegisterEmailResponseCode(t, response, 0)
	if strings.Contains(response.Body.String(), "context canceled") || strings.Contains(response.Body.String(), "smtp") {
		t.Fatalf("send failure leaked backend detail: %q", response.Body.String())
	}
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load failed-send token: %v", err)
	}
	if token.UsedAt == nil {
		t.Fatalf("failed send left register token usable: %#v", token)
	}
}

func TestRegisterEmailVerificationSendSuppressesPerIPBurst(t *testing.T) {
	fixture := setupRegisterEmailFixture(t)
	configureRegisterEmailFixture(t)

	first := fixture.post("/api/admin/user/register/email/send", `{"email":"first@example.test"}`)
	assertRegisterEmailResponseCode(t, first, 0)
	second := fixture.post("/api/admin/user/register/email/send", `{"email":"second@example.test"}`)
	assertRegisterEmailResponseCode(t, second, 0)
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("fake sender messages = %d, want second public send suppressed by IP cooldown", len(fixture.fake.messages))
	}
	var count int64
	if err := fixture.db.Model(&model.EmailVerificationToken{}).Count(&count).Error; err != nil {
		t.Fatalf("count register tokens: %v", err)
	}
	if count != 1 {
		t.Fatalf("register token count = %d, want only first send to create a token", count)
	}
}

func TestRegisterEmailVerificationSendSuppressesGlobalBurst(t *testing.T) {
	fixture := setupRegisterEmailFixture(t)
	configureRegisterEmailFixture(t)
	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/api/admin/user/register/email/send", strings.NewReader(`{"email":"first@example.test"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("X-Forwarded-For", "192.0.2.10")
	fixture.router.ServeHTTP(first, firstReq)
	assertRegisterEmailResponseCode(t, first, 0)
	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/api/admin/user/register/email/send", strings.NewReader(`{"email":"second@example.test"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("X-Forwarded-For", "192.0.2.11")
	fixture.router.ServeHTTP(second, secondReq)
	assertRegisterEmailResponseCode(t, second, 0)
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("fake sender messages = %d, want second public send suppressed by global cooldown", len(fixture.fake.messages))
	}
}

func TestRegisterEmailVerificationSendSuppressesSettingsLookupErrors(t *testing.T) {
	fixture := setupRegisterEmailFixture(t)
	configureRegisterEmailFixture(t)
	service.DB = nil
	response := fixture.post("/api/admin/user/register/email/send", `{"email":"lookup-error@example.test"}`)
	assertRegisterEmailResponseCode(t, response, 0)
	if strings.Contains(strings.ToLower(response.Body.String()), "panic") || strings.Contains(strings.ToLower(response.Body.String()), "database") || strings.Contains(response.Body.String(), "invalid") {
		t.Fatalf("settings lookup error leaked backend detail: %q", response.Body.String())
	}
	if len(fixture.fake.messages) != 0 {
		t.Fatalf("fake sender messages = %d, want lookup error to send no email", len(fixture.fake.messages))
	}
}

func assertRegisterEmailResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
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
		t.Fatalf("code = %d, want %d; body=%q", payload.Code, want, recorder.Body.String())
	}
}

func assertRegisterEmailUserCount(t *testing.T, db *gorm.DB, username string, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != want {
		t.Fatalf("user count = %d, want %d", count, want)
	}
}
