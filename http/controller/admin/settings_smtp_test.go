package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recordingSMTPSender struct {
	settings service.SMTPSettings
	message  service.SMTPMessage
}

func (r *recordingSMTPSender) Send(_ context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	r.settings = settings
	r.message = message
	return nil
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
