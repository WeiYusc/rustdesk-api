package service

import (
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSettingsServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite settings test db: %v", err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatalf("migrate settings model: %v", err)
	}
	DB = db
	return db
}

func TestSettingsServiceReturnsSMTPDefaultsWithoutPersistedRow(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	settings, err := svc.GetSMTP()
	if err != nil {
		t.Fatalf("GetSMTP error: %v", err)
	}

	if settings.Enabled {
		t.Fatalf("default SMTP enabled = true, want false")
	}
	if settings.Port != 587 {
		t.Fatalf("default SMTP port = %d, want 587", settings.Port)
	}
	if settings.Security != SMTPSecurityStartTLS {
		t.Fatalf("default SMTP security = %q, want %q", settings.Security, SMTPSecurityStartTLS)
	}
	if settings.TimeoutSeconds != 10 {
		t.Fatalf("default SMTP timeout = %d, want 10", settings.TimeoutSeconds)
	}
	if settings.Password != "" || settings.HasPassword {
		t.Fatalf("default SMTP password/has_password = %q/%v, want empty/false", settings.Password, settings.HasPassword)
	}
}

func TestSettingsServiceMasksSMTPPasswordAndPreservesWhenBlank(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	initial := SMTPSettings{
		Enabled:        true,
		Host:           "smtp.example.test",
		Port:           465,
		Security:       SMTPSecurityTLS,
		Username:       "smtp-user",
		Password:       "secret-password",
		FromEmail:      "noreply@example.test",
		FromName:       "RustDesk API",
		TimeoutSeconds: 15,
	}
	if err := svc.SaveSMTP(initial, 7); err != nil {
		t.Fatalf("SaveSMTP initial error: %v", err)
	}

	got, err := svc.GetSMTP()
	if err != nil {
		t.Fatalf("GetSMTP after save error: %v", err)
	}
	if got.Password != "" || !got.HasPassword {
		t.Fatalf("masked SMTP password/has_password = %q/%v, want empty/true", got.Password, got.HasPassword)
	}
	if got.Host != initial.Host || got.Port != initial.Port || got.Username != initial.Username || got.FromEmail != initial.FromEmail {
		t.Fatalf("SMTP settings did not round trip: %#v", got)
	}

	update := got
	update.Host = "smtp2.example.test"
	update.Password = ""
	if err := svc.SaveSMTP(update, 8); err != nil {
		t.Fatalf("SaveSMTP update error: %v", err)
	}

	stored, err := svc.getSMTPStored()
	if err != nil {
		t.Fatalf("getSMTPStored error: %v", err)
	}
	if stored.Password != "secret-password" {
		t.Fatalf("blank password update changed stored password to %q", stored.Password)
	}
}

func TestSettingsServiceClearSMTPPassword(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	if err := svc.SaveSMTP(SMTPSettings{Host: "smtp.example.test", Password: "secret-password"}, 1); err != nil {
		t.Fatalf("SaveSMTP initial error: %v", err)
	}
	if err := svc.SaveSMTP(SMTPSettings{Host: "smtp.example.test", ClearPassword: true}, 2); err != nil {
		t.Fatalf("SaveSMTP clear error: %v", err)
	}

	got, err := svc.GetSMTP()
	if err != nil {
		t.Fatalf("GetSMTP after clear error: %v", err)
	}
	if got.HasPassword || got.Password != "" {
		t.Fatalf("cleared SMTP password/has_password = %q/%v, want empty/false", got.Password, got.HasPassword)
	}
}

func TestSettingsServiceRejectsInvalidSMTPSettings(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	cases := []SMTPSettings{
		{Port: -1, Security: SMTPSecurityStartTLS},
		{Port: 70000, Security: SMTPSecurityStartTLS},
		{Port: 587, Security: "invalid"},
		{Enabled: true, Port: 587, Security: SMTPSecurityStartTLS, Host: ""},
	}
	for _, tc := range cases {
		if err := svc.SaveSMTP(tc, 1); err == nil {
			t.Fatalf("SaveSMTP(%#v) succeeded, want validation error", tc)
		}
	}
}

func TestSettingsServiceRejectsInvalidEmailVerificationSettings(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	cases := []EmailVerificationSettings{
		{CodeTTLMinutes: 61, ResendCooldownSeconds: 60, DailySendLimitPerUser: 10},
		{CodeTTLMinutes: 10, ResendCooldownSeconds: -1, DailySendLimitPerUser: 10},
		{CodeTTLMinutes: 10, ResendCooldownSeconds: 60, DailySendLimitPerUser: -1},
	}
	for _, tc := range cases {
		if err := svc.SaveEmailVerification(tc, 1); err == nil {
			t.Fatalf("SaveEmailVerification(%#v) succeeded, want validation error", tc)
		}
	}
}

func TestSettingsServiceRejectsInvalidPasskeySettings(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	cases := []PasskeySettings{
		{Enabled: true, RPName: "RustDesk", RPID: "", AllowedOrigins: []string{"https://admin.example.test"}, UserVerification: UserVerificationPreferred, ResidentKeyRequirement: ResidentKeyRequired},
		{Enabled: true, RPName: "RustDesk", RPID: "admin.example.test", AllowedOrigins: nil, UserVerification: UserVerificationPreferred, ResidentKeyRequirement: ResidentKeyRequired},
		{Enabled: true, RPName: "RustDesk", RPID: "admin.example.test", AllowedOrigins: []string{"https://admin.example.test/callback"}, UserVerification: UserVerificationPreferred, ResidentKeyRequirement: ResidentKeyRequired},
		{Enabled: false, RPName: "RustDesk", UserVerification: "invalid", ResidentKeyRequirement: ResidentKeyRequired},
		{Enabled: false, RPName: "RustDesk", UserVerification: UserVerificationPreferred, ResidentKeyRequirement: "sometimes"},
	}
	for _, tc := range cases {
		if err := svc.SavePasskey(tc, 1); err == nil {
			t.Fatalf("SavePasskey(%#v) succeeded, want validation error", tc)
		}
	}
}

func TestSettingsServicePasswordLoginDisabledHelper(t *testing.T) {
	db := setupSettingsServiceTestDB(t)
	svc := &SettingsService{}
	AllService = &Service{SettingsService: svc}

	if !PasswordLoginDisabled(true) {
		t.Fatalf("PasswordLoginDisabled(true) = false, want true")
	}
	if PasswordLoginDisabled(false) {
		t.Fatalf("PasswordLoginDisabled(false) with no persisted policy = true, want false")
	}
	if err := svc.SaveAuthPolicy(AuthPolicySettings{DisablePasswordLogin: true}, 1); err != nil {
		t.Fatalf("SaveAuthPolicy disable error: %v", err)
	}
	if !PasswordLoginDisabled(false) {
		t.Fatalf("PasswordLoginDisabled(false) ignored persisted disable policy")
	}

	if err := db.Model(&model.Setting{}).Where("key = ?", SettingKeyAuthPolicy).Update("value", "{").Error; err != nil {
		t.Fatalf("corrupt auth policy setting: %v", err)
	}
	if !PasswordLoginDisabled(false) {
		t.Fatalf("PasswordLoginDisabled(false) with corrupt persisted policy = false, want fail-closed true")
	}
}

func TestSettingsServiceDefaultAuthFeatureSettings(t *testing.T) {
	setupSettingsServiceTestDB(t)
	svc := &SettingsService{}

	email, err := svc.GetEmailVerification()
	if err != nil {
		t.Fatalf("GetEmailVerification error: %v", err)
	}
	if email.Enabled || !email.RequireForEmailChange || email.RequireForLogin {
		t.Fatalf("email verification defaults = %#v", email)
	}
	if email.CodeTTLMinutes != 10 || email.ResendCooldownSeconds != 60 || email.DailySendLimitPerUser != 10 {
		t.Fatalf("email verification limit defaults = %#v", email)
	}

	passkey, err := svc.GetPasskey()
	if err != nil {
		t.Fatalf("GetPasskey error: %v", err)
	}
	if passkey.Enabled || !passkey.DiscoverableLoginEnabled || passkey.ResidentKeyRequirement != ResidentKeyRequired {
		t.Fatalf("passkey defaults = %#v", passkey)
	}
	if passkey.UserVerification != UserVerificationPreferred {
		t.Fatalf("passkey user verification = %q", passkey.UserVerification)
	}

	auth, err := svc.GetAuthPolicy()
	if err != nil {
		t.Fatalf("GetAuthPolicy error: %v", err)
	}
	if auth.DisablePasswordLogin {
		t.Fatalf("auth policy disable password default = true, want false")
	}
}
