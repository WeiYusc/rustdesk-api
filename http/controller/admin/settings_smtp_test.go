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

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingSMTPSender struct {
	settings service.SMTPSettings
	message  service.SMTPMessage
	err      error
}

func (r *recordingSMTPSender) Send(_ context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	r.settings = settings
	r.message = message
	return r.err
}

func setupSettingsSMTPTest(t *testing.T) (*gin.Engine, *recordingSMTPSender) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite settings smtp db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate settings model: %v", err)
	}
	global.ApiInitValidator()
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	if _, err := bundle.LoadMessageFile(filepath.Join("..", "..", "..", "resources", "i18n", "en.toml")); err != nil {
		t.Fatalf("load en locale: %v", err)
	}
	global.Localizer = func(lang string) *i18n.Localizer { return i18n.NewLocalizer(bundle, lang) }
	service.DB = db
	service.AllService = &service.Service{SettingsService: &service.SettingsService{}}
	fakeSMTPPassword := strings.Join([]string{"smtp", "secret"}, "-")
	if err := service.AllService.SettingsService.SaveSMTP(service.SMTPSettings{
		Enabled:        true,
		Host:           "smtp.example.test",
		Port:           587,
		Security:       service.SMTPSecurityStartTLS,
		Username:       "smtp-user",
		Password:       fakeSMTPPassword,
		FromEmail:      "noreply@example.test",
		FromName:       "RustDesk API",
		TimeoutSeconds: 10,
	}, 1); err != nil {
		t.Fatalf("save SMTP settings: %v", err)
	}
	sender := &recordingSMTPSender{}
	setSMTPTestSenderForTest(t, sender)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/admin/settings/smtp/test", (&Settings{}).TestSMTP)
	return engine, sender
}

func TestSettingsTestSMTPComposesAndSendsTestMessage(t *testing.T) {
	engine, sender := setupSettingsSMTPTest(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/settings/smtp/test", strings.NewReader(`{"to":"user@example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal SMTP test response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 0 {
		t.Fatalf("SMTP test payload code = %d, want 0; body=%q", payload.Code, recorder.Body.String())
	}
	if sender.settings.Password != strings.Join([]string{"smtp", "secret"}, "-") || !sender.settings.HasPassword {
		t.Fatalf("SMTP test did not use stored send password: password=%q has=%v", sender.settings.Password, sender.settings.HasPassword)
	}
	if sender.message.To != "user@example.test" || !strings.Contains(sender.message.Subject, "RustDesk API") || !strings.Contains(sender.message.TextBody, "SMTP") {
		t.Fatalf("unexpected SMTP test message: %#v", sender.message)
	}
}

func TestSettingsTestSMTPClassifiesSendFailureWithoutLeakingRawError(t *testing.T) {
	engine, sender := setupSettingsSMTPTest(t)
	sender.err = errors.New("535 5.7.8 authentication failed for real-user@example.test with password real-secret")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/settings/smtp/test", strings.NewReader(`{"to":"user@example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d; body=%q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal SMTP test response: %v; body=%q", err, recorder.Body.String())
	}
	if payload.Code != 101 {
		t.Fatalf("SMTP test payload code = %d, want 101; body=%q", payload.Code, recorder.Body.String())
	}
	if payload.Message != "SMTP authentication failed. Check the SMTP username and password." {
		t.Fatalf("SMTP test message = %q", payload.Message)
	}
	if strings.Contains(recorder.Body.String(), "real-secret") || strings.Contains(recorder.Body.String(), "real-user@example.test") || strings.Contains(recorder.Body.String(), "535") {
		t.Fatalf("SMTP test leaked raw provider detail: %q", recorder.Body.String())
	}
}
