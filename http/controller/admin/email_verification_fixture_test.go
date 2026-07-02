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

type adminEmailVerificationFixture struct {
	db     *gorm.DB
	router *gin.Engine
	user   *model.User
	token  string
	fake   *fakeEmailVerificationSender
}

type fakeEmailVerificationSender struct {
	messages []service.SMTPMessage
	err      error
}

func (f *fakeEmailVerificationSender) Send(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	if f.err != nil {
		return f.err
	}
	f.messages = append(f.messages, message)
	return nil
}

func setupAdminEmailVerificationFixture(t *testing.T) adminEmailVerificationFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite email verification db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.Setting{}, &model.EmailVerificationToken{}); err != nil {
		t.Fatalf("migrate email verification models: %v", err)
	}

	global.Config = config.Config{Lang: "en"}
	global.DB = db
	global.Logger = logrus.New()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{CaptchaThreshold: -1, BanThreshold: 0})
	global.Jwt = jwt.NewJwt("email-verification-controller-secret", time.Hour)
	service.New(&global.Config, db, global.Logger, global.Jwt, nil)

	user := &model.User{Username: "email-user", Email: "Current@Example.Test", Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create email user: %v", err)
	}
	if err := db.Create(&model.UserToken{UserId: user.Id, Token: "email-verification-token", ExpiredAt: time.Now().Add(time.Hour).Unix()}).Error; err != nil {
		t.Fatalf("create email token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	emailGroup := engine.Group("/api/admin/email").Use(middleware.BackendUserAuth())
	controller := &Email{}
	emailGroup.POST("/verification/send", controller.SendVerification)
	emailGroup.POST("/verification/confirm", controller.ConfirmVerification)
	emailGroup.POST("/change/begin", controller.BeginChange)
	emailGroup.POST("/change/confirm", controller.ConfirmChange)

	fake := &fakeEmailVerificationSender{}
	setEmailVerificationSenderForTest(t, fake)

	return adminEmailVerificationFixture{db: db, router: engine, user: user, token: "email-verification-token", fake: fake}
}

func (f adminEmailVerificationFixture) request(method string, target string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("api-token", f.token)
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func (f adminEmailVerificationFixture) unauthenticatedRequest(method string, target string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func TestEmailVerificationEndpointsRequireAuthentication(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	for _, target := range []string{
		"/api/admin/email/verification/send",
		"/api/admin/email/verification/confirm",
		"/api/admin/email/change/begin",
		"/api/admin/email/change/confirm",
	} {
		response := fixture.unauthenticatedRequest(http.MethodPost, target, `{"email":"new@example.test","code":"123456"}`)
		assertEmailVerificationResponseCode(t, response, 403)
	}
}

func TestEmailVerificationSendFailsWhenDisabled(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)

	response := fixture.request(http.MethodPost, "/api/admin/email/verification/send", `{}`)

	assertEmailVerificationResponseCode(t, response, 101)
	if !strings.Contains(response.Body.String(), "Email verification is disabled") {
		t.Fatalf("disabled response body = %q", response.Body.String())
	}
	assertEmailVerificationTokenCount(t, fixture.db, 0)
	if len(fixture.fake.messages) != 0 {
		t.Fatalf("fake sender messages = %d, want 0", len(fixture.fake.messages))
	}
}

func TestEmailVerificationSendFailsWhenSMTPUnavailableAndCreatesNoToken(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true}, fixture.user.Id); err != nil {
		t.Fatalf("save enabled email verification: %v", err)
	}

	response := fixture.request(http.MethodPost, "/api/admin/email/verification/send", `{}`)

	assertEmailVerificationResponseCode(t, response, 101)
	if !strings.Contains(response.Body.String(), "SMTP is not enabled") {
		t.Fatalf("smtp unavailable response body = %q", response.Body.String())
	}
	assertEmailVerificationTokenCount(t, fixture.db, 0)
	if len(fixture.fake.messages) != 0 {
		t.Fatalf("fake sender messages = %d, want 0", len(fixture.fake.messages))
	}
}

func TestEmailVerificationSendSuccessHidesSecretsAndSendsFakeEmailBody(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)

	response := fixture.request(http.MethodPost, "/api/admin/email/verification/send", `{}`)

	assertEmailVerificationResponseCode(t, response, 0)
	body := response.Body.String()
	if strings.Contains(body, "code_hash") || strings.Contains(body, "CodeHash") || strings.Contains(body, "hash") {
		t.Fatalf("send response exposed hash: %q", body)
	}
	assertEmailVerificationTokenCount(t, fixture.db, 1)
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load email verification token: %v", err)
	}
	if token.Purpose != model.EmailVerificationPurposeVerifyCurrent || token.Email != "current@example.test" || token.CodeHash == "" {
		t.Fatalf("stored token = %#v", token)
	}
	if strings.Contains(body, token.CodeHash) {
		t.Fatalf("send response exposed stored hash: %q", body)
	}
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("fake sender messages = %d, want 1", len(fixture.fake.messages))
	}
	message := fixture.fake.messages[0]
	if message.To != "current@example.test" {
		t.Fatalf("message to = %q, want normalized current email", message.To)
	}
	code := extractEmailVerificationCode(t, message.TextBody)
	if code == "" || strings.Contains(body, code) {
		t.Fatalf("send response exposed code %q in body %q", code, body)
	}
}

func TestEmailVerificationSendFailureMarksChallengeUsed(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	fixture.fake.err = errors.New("smtp down")

	response := fixture.request(http.MethodPost, "/api/admin/email/verification/send", `{}`)

	assertEmailVerificationResponseCode(t, response, 101)
	if !strings.Contains(response.Body.String(), "smtp down") {
		t.Fatalf("send failure response body = %q", response.Body.String())
	}
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load token after failed send: %v", err)
	}
	if token.UsedAt == nil {
		t.Fatalf("failed send left challenge usable: %#v", token)
	}
}

func TestEmailVerificationConfirmWrongAndMissingDoNotVerify(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)

	missing := fixture.request(http.MethodPost, "/api/admin/email/verification/confirm", `{}`)
	assertEmailVerificationResponseCode(t, missing, 101)
	assertEmailNotVerified(t, fixture.db, fixture.user.Id)

	send := fixture.request(http.MethodPost, "/api/admin/email/verification/send", `{}`)
	assertEmailVerificationResponseCode(t, send, 0)
	wrong := fixture.request(http.MethodPost, "/api/admin/email/verification/confirm", `{"code":"000000"}`)
	assertEmailVerificationResponseCode(t, wrong, 101)
	assertEmailNotVerified(t, fixture.db, fixture.user.Id)
}

func TestEmailVerificationConfirmSuccessSetsEmailVerifiedAt(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	send := fixture.request(http.MethodPost, "/api/admin/email/verification/send", `{}`)
	assertEmailVerificationResponseCode(t, send, 0)
	code := extractEmailVerificationCode(t, fixture.fake.messages[0].TextBody)

	confirm := fixture.request(http.MethodPost, "/api/admin/email/verification/confirm", `{"code":"`+code+`"}`)

	assertEmailVerificationResponseCode(t, confirm, 0)
	var user model.User
	if err := fixture.db.First(&user, fixture.user.Id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.EmailVerifiedAt == nil {
		t.Fatalf("EmailVerifiedAt was not set after successful confirm")
	}
}

func TestEmailVerificationBeginChangeFailsWhenDisabled(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)

	response := fixture.request(http.MethodPost, "/api/admin/email/change/begin", `{"email":"new@example.test"}`)

	assertEmailVerificationResponseCode(t, response, 101)
	assertEmailVerificationTokenCount(t, fixture.db, 0)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "", false)
	if len(fixture.fake.messages) != 0 {
		t.Fatalf("fake sender messages = %d, want 0", len(fixture.fake.messages))
	}
}

func TestEmailVerificationBeginChangeFailsWhenSMTPUnavailableAndCreatesNoPendingOrToken(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true}, fixture.user.Id); err != nil {
		t.Fatalf("save enabled email verification: %v", err)
	}

	response := fixture.request(http.MethodPost, "/api/admin/email/change/begin", `{"email":"new@example.test"}`)

	assertEmailVerificationResponseCode(t, response, 101)
	assertEmailVerificationTokenCount(t, fixture.db, 0)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "", false)
	if len(fixture.fake.messages) != 0 {
		t.Fatalf("fake sender messages = %d, want 0", len(fixture.fake.messages))
	}
}

func TestEmailVerificationBeginChangeRejectsInvalidEmail(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)

	for _, body := range []string{`{"email":"not-an-email"}`, `{"email":"Name <new@example.test>"}`, `{"email":""}`} {
		response := fixture.request(http.MethodPost, "/api/admin/email/change/begin", body)
		assertEmailVerificationResponseCode(t, response, 101)
	}
	assertEmailVerificationTokenCount(t, fixture.db, 0)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "", false)
}

func TestEmailVerificationBeginChangeSuccessSetsPendingTokenAndHidesSecrets(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)

	response := fixture.request(http.MethodPost, "/api/admin/email/change/begin", `{"email":" New@Example.Test "}`)

	assertEmailVerificationResponseCode(t, response, 0)
	body := response.Body.String()
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "new@example.test", false)
	assertEmailVerificationTokenCount(t, fixture.db, 1)
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load change email token: %v", err)
	}
	if token.Purpose != model.EmailVerificationPurposeChangeEmail || token.Email != "new@example.test" || token.CodeHash == "" || token.UsedAt != nil {
		t.Fatalf("stored change token = %#v", token)
	}
	if strings.Contains(body, "code_hash") || strings.Contains(body, "CodeHash") || strings.Contains(body, "hash") || strings.Contains(body, token.CodeHash) {
		t.Fatalf("begin change response exposed hash: %q", body)
	}
	if len(fixture.fake.messages) != 1 {
		t.Fatalf("fake sender messages = %d, want 1", len(fixture.fake.messages))
	}
	message := fixture.fake.messages[0]
	if message.To != "new@example.test" {
		t.Fatalf("message to = %q, want new@example.test", message.To)
	}
	code := extractEmailVerificationCode(t, message.TextBody)
	if code == "" || strings.Contains(body, code) {
		t.Fatalf("begin change response exposed code %q in body %q", code, body)
	}
}

func TestEmailVerificationBeginChangeSendFailureMarksTokenUsedAndClearsPending(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	fixture.fake.err = errors.New("smtp down")

	response := fixture.request(http.MethodPost, "/api/admin/email/change/begin", `{"email":"new@example.test"}`)

	assertEmailVerificationResponseCode(t, response, 101)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "", false)
	var token model.EmailVerificationToken
	if err := fixture.db.First(&token).Error; err != nil {
		t.Fatalf("load token after failed change send: %v", err)
	}
	if token.UsedAt == nil {
		t.Fatalf("failed change send left challenge usable: %#v", token)
	}
}

func TestEmailVerificationConfirmChangeMissingAndWrongDoNotChangeOrClearPending(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	begin := fixture.request(http.MethodPost, "/api/admin/email/change/begin", `{"email":"new@example.test"}`)
	assertEmailVerificationResponseCode(t, begin, 0)

	missing := fixture.request(http.MethodPost, "/api/admin/email/change/confirm", `{}`)
	assertEmailVerificationResponseCode(t, missing, 101)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "new@example.test", false)

	wrong := fixture.request(http.MethodPost, "/api/admin/email/change/confirm", `{"code":"000000"}`)
	assertEmailVerificationResponseCode(t, wrong, 101)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "Current@Example.Test", "new@example.test", false)
}

func TestEmailVerificationConfirmChangeSuccessUpdatesEmailVerifiedAndClearsPending(t *testing.T) {
	fixture := setupAdminEmailVerificationFixture(t)
	configureEmailVerificationAndSMTP(t, fixture.user.Id)
	begin := fixture.request(http.MethodPost, "/api/admin/email/change/begin", `{"email":"new@example.test"}`)
	assertEmailVerificationResponseCode(t, begin, 0)
	code := extractEmailVerificationCode(t, fixture.fake.messages[0].TextBody)

	confirm := fixture.request(http.MethodPost, "/api/admin/email/change/confirm", `{"code":"`+code+`"}`)

	assertEmailVerificationResponseCode(t, confirm, 0)
	assertUserEmailState(t, fixture.db, fixture.user.Id, "new@example.test", "", true)
}

func configureEmailVerificationAndSMTP(t *testing.T, userID uint) {
	t.Helper()
	if err := service.AllService.SettingsService.SaveEmailVerification(service.EmailVerificationSettings{Enabled: true}, userID); err != nil {
		t.Fatalf("save enabled email verification: %v", err)
	}
	if err := service.AllService.SettingsService.SaveSMTP(service.SMTPSettings{
		Enabled:        true,
		Host:           "smtp.example.test",
		Port:           587,
		Security:       service.SMTPSecurityStartTLS,
		FromEmail:      "noreply@example.test",
		FromName:       "RustDesk Test",
		TimeoutSeconds: 10,
	}, userID); err != nil {
		t.Fatalf("save smtp settings: %v", err)
	}
}

func assertEmailVerificationResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
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

func assertEmailVerificationTokenCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.EmailVerificationToken{}).Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count != want {
		t.Fatalf("email verification token count = %d, want %d", count, want)
	}
}

func assertEmailNotVerified(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.EmailVerifiedAt != nil {
		t.Fatalf("EmailVerifiedAt = %v, want nil", user.EmailVerifiedAt)
	}
}

func assertUserEmailState(t *testing.T, db *gorm.DB, userID uint, wantEmail string, wantPending string, wantVerified bool) {
	t.Helper()
	var user model.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.Email != wantEmail || user.PendingEmail != wantPending {
		t.Fatalf("email state = email %q pending %q, want %q/%q", user.Email, user.PendingEmail, wantEmail, wantPending)
	}
	if (user.EmailVerifiedAt != nil) != wantVerified {
		t.Fatalf("EmailVerifiedAt set = %v, want %v (value=%v)", user.EmailVerifiedAt != nil, wantVerified, user.EmailVerifiedAt)
	}
}

func extractEmailVerificationCode(t *testing.T, body string) string {
	t.Helper()
	fields := strings.FieldsFunc(body, func(r rune) bool { return r < '0' || r > '9' })
	for _, field := range fields {
		if len(field) == 6 {
			return field
		}
	}
	t.Fatalf("no 6-digit verification code found in body %q", body)
	return ""
}
