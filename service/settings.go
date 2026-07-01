package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	SettingKeySMTP              = "smtp"
	SettingKeyEmailVerification = "email_verification"
	SettingKeyPasskey           = "passkey"
	SettingKeyAuthPolicy        = "auth_policy"

	SMTPSecurityNone     = "none"
	SMTPSecurityStartTLS = "starttls"
	SMTPSecurityTLS      = "tls"

	UserVerificationPreferred   = "preferred"
	UserVerificationRequired    = "required"
	UserVerificationDiscouraged = "discouraged"
	ResidentKeyRequired         = "required"
	ResidentKeyPreferred        = "preferred"
	ResidentKeyDiscouraged      = "discouraged"
)

type SettingsService struct{}

type SMTPSettings struct {
	Enabled            bool   `json:"enabled"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Security           string `json:"security"`
	Username           string `json:"username"`
	Password           string `json:"password,omitempty"`
	HasPassword        bool   `json:"has_password"`
	FromEmail          string `json:"from_email"`
	FromName           string `json:"from_name"`
	ReplyTo            string `json:"reply_to"`
	TimeoutSeconds     int    `json:"timeout_seconds"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	ClearPassword      bool   `json:"clear_password,omitempty"`
}

type EmailVerificationSettings struct {
	Enabled               bool `json:"enabled"`
	RequireForRegister    bool `json:"require_for_register"`
	RequireForEmailChange bool `json:"require_for_email_change"`
	RequireForLogin       bool `json:"require_for_login"`
	CodeTTLMinutes        int  `json:"code_ttl_minutes"`
	ResendCooldownSeconds int  `json:"resend_cooldown_seconds"`
	DailySendLimitPerUser int  `json:"daily_send_limit_per_user"`
}

type PasskeySettings struct {
	Enabled                  bool     `json:"enabled"`
	RPName                   string   `json:"rp_name"`
	RPID                     string   `json:"rp_id"`
	AllowedOrigins           []string `json:"allowed_origins"`
	UserVerification         string   `json:"user_verification"`
	DiscoverableLoginEnabled bool     `json:"discoverable_login_enabled"`
	ResidentKeyRequirement   string   `json:"resident_key_requirement"`
}

type AuthPolicySettings struct {
	DisablePasswordLogin bool `json:"disable_password_login"`
}

func DefaultSMTPSettings() SMTPSettings {
	return SMTPSettings{
		Port:           587,
		Security:       SMTPSecurityStartTLS,
		TimeoutSeconds: 10,
	}
}

func DefaultEmailVerificationSettings() EmailVerificationSettings {
	return EmailVerificationSettings{
		RequireForEmailChange: true,
		CodeTTLMinutes:        10,
		ResendCooldownSeconds: 60,
		DailySendLimitPerUser: 10,
	}
}

func DefaultPasskeySettings() PasskeySettings {
	return PasskeySettings{
		RPName:                   "RustDesk API Admin",
		UserVerification:         UserVerificationPreferred,
		DiscoverableLoginEnabled: true,
		ResidentKeyRequirement:   ResidentKeyRequired,
	}
}

func DefaultAuthPolicySettings() AuthPolicySettings {
	return AuthPolicySettings{}
}

func (s *SettingsService) GetSMTP() (SMTPSettings, error) {
	stored, err := s.getSMTPStored()
	if err != nil {
		return SMTPSettings{}, err
	}
	stored.HasPassword = stored.Password != ""
	stored.Password = ""
	stored.ClearPassword = false
	return stored, nil
}

func (s *SettingsService) SaveSMTP(settings SMTPSettings, updatedBy uint) error {
	current, err := s.getSMTPStored()
	if err != nil {
		return err
	}
	if settings.Password == "" && !settings.ClearPassword {
		settings.Password = current.Password
	}
	if settings.ClearPassword {
		settings.Password = ""
	}
	settings.HasPassword = settings.Password != ""
	settings.ClearPassword = false
	settings.applyDefaults()
	if err := settings.validate(); err != nil {
		return err
	}
	return s.saveJSONSetting(SettingKeySMTP, settings, true, updatedBy)
}

func (s *SettingsService) getSMTPStored() (SMTPSettings, error) {
	settings := DefaultSMTPSettings()
	if err := s.loadJSONSetting(SettingKeySMTP, &settings); err != nil {
		return SMTPSettings{}, err
	}
	settings.applyDefaults()
	settings.HasPassword = settings.Password != ""
	settings.ClearPassword = false
	return settings, nil
}

func (s *SettingsService) GetEmailVerification() (EmailVerificationSettings, error) {
	settings := DefaultEmailVerificationSettings()
	if err := s.loadJSONSetting(SettingKeyEmailVerification, &settings); err != nil {
		return EmailVerificationSettings{}, err
	}
	settings.applyDefaults()
	return settings, nil
}

func (s *SettingsService) SaveEmailVerification(settings EmailVerificationSettings, updatedBy uint) error {
	settings.applyDefaults()
	if err := settings.validate(); err != nil {
		return err
	}
	return s.saveJSONSetting(SettingKeyEmailVerification, settings, false, updatedBy)
}

func (s *SettingsService) GetPasskey() (PasskeySettings, error) {
	settings := DefaultPasskeySettings()
	if err := s.loadJSONSetting(SettingKeyPasskey, &settings); err != nil {
		return PasskeySettings{}, err
	}
	settings.applyDefaults()
	return settings, nil
}

func (s *SettingsService) SavePasskey(settings PasskeySettings, updatedBy uint) error {
	settings.applyDefaults()
	if err := settings.validate(); err != nil {
		return err
	}
	return s.saveJSONSetting(SettingKeyPasskey, settings, false, updatedBy)
}

func (s *SettingsService) GetAuthPolicy() (AuthPolicySettings, error) {
	settings := DefaultAuthPolicySettings()
	if err := s.loadJSONSetting(SettingKeyAuthPolicy, &settings); err != nil {
		return AuthPolicySettings{}, err
	}
	return settings, nil
}

func (s *SettingsService) SaveAuthPolicy(settings AuthPolicySettings, updatedBy uint) error {
	return s.saveJSONSetting(SettingKeyAuthPolicy, settings, false, updatedBy)
}

func PasswordLoginDisabled(configDisabled bool) bool {
	if configDisabled {
		return true
	}
	if AllService == nil {
		return false
	}
	policy, err := AllService.SettingsService.GetAuthPolicy()
	if err != nil {
		return true
	}
	return policy.DisablePasswordLogin
}

func (s *SettingsService) loadJSONSetting(key string, out interface{}) error {
	setting := &model.Setting{}
	err := DB.Where("key = ?", key).First(setting).Error
	if err != nil {
		return ignoreRecordNotFound(err)
	}
	if setting.Value == "" {
		return nil
	}
	return json.Unmarshal([]byte(setting.Value), out)
}

func (s *SettingsService) saveJSONSetting(key string, value interface{}, isSecret bool, updatedBy uint) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	setting := &model.Setting{
		Key:       key,
		Value:     string(payload),
		IsSecret:  isSecret,
		UpdatedBy: updatedBy,
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "is_secret", "updated_by", "updated_at"}),
	}).Create(setting).Error
}

func ignoreRecordNotFound(err error) error {
	if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *SMTPSettings) applyDefaults() {
	if s.Port == 0 {
		s.Port = 587
	}
	if s.Security == "" {
		s.Security = SMTPSecurityStartTLS
	}
	if s.TimeoutSeconds == 0 {
		s.TimeoutSeconds = 10
	}
}

func (s SMTPSettings) validate() error {
	if s.Enabled && strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("smtp host is required when enabled")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("smtp port must be between 1 and 65535")
	}
	switch s.Security {
	case SMTPSecurityNone, SMTPSecurityStartTLS, SMTPSecurityTLS:
	default:
		return fmt.Errorf("unsupported smtp security: %s", s.Security)
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > 300 {
		return fmt.Errorf("smtp timeout must be between 1 and 300 seconds")
	}
	if s.FromEmail != "" && !strings.Contains(s.FromEmail, "@") {
		return fmt.Errorf("smtp from_email must be an email address")
	}
	if s.ReplyTo != "" && !strings.Contains(s.ReplyTo, "@") {
		return fmt.Errorf("smtp reply_to must be an email address")
	}
	return nil
}

func (s *EmailVerificationSettings) applyDefaults() {
	if s.CodeTTLMinutes == 0 {
		s.CodeTTLMinutes = 10
	}
	if s.ResendCooldownSeconds == 0 {
		s.ResendCooldownSeconds = 60
	}
	if s.DailySendLimitPerUser == 0 {
		s.DailySendLimitPerUser = 10
	}
}

func (s EmailVerificationSettings) validate() error {
	if s.CodeTTLMinutes < 1 || s.CodeTTLMinutes > 60 {
		return fmt.Errorf("email verification code ttl must be between 1 and 60 minutes")
	}
	if s.ResendCooldownSeconds < 1 || s.ResendCooldownSeconds > 3600 {
		return fmt.Errorf("email verification resend cooldown must be between 1 and 3600 seconds")
	}
	if s.DailySendLimitPerUser < 1 || s.DailySendLimitPerUser > 1000 {
		return fmt.Errorf("email verification daily limit must be between 1 and 1000")
	}
	return nil
}

func (s *PasskeySettings) applyDefaults() {
	if s.RPName == "" {
		s.RPName = "RustDesk API Admin"
	}
	if s.UserVerification == "" {
		s.UserVerification = UserVerificationPreferred
	}
	if s.ResidentKeyRequirement == "" {
		s.ResidentKeyRequirement = ResidentKeyRequired
	}
}

func (s PasskeySettings) validate() error {
	if strings.TrimSpace(s.RPName) == "" {
		return fmt.Errorf("passkey rp_name is required")
	}
	switch s.UserVerification {
	case UserVerificationPreferred, UserVerificationRequired, UserVerificationDiscouraged:
	default:
		return fmt.Errorf("unsupported passkey user_verification: %s", s.UserVerification)
	}
	switch s.ResidentKeyRequirement {
	case ResidentKeyRequired, ResidentKeyPreferred, ResidentKeyDiscouraged:
	default:
		return fmt.Errorf("unsupported passkey resident_key_requirement: %s", s.ResidentKeyRequirement)
	}
	if !s.Enabled {
		return nil
	}
	if strings.TrimSpace(s.RPID) == "" {
		return fmt.Errorf("passkey rp_id is required when passkey is enabled")
	}
	if len(s.AllowedOrigins) == 0 {
		return fmt.Errorf("passkey allowed_origins is required when passkey is enabled")
	}
	for _, origin := range s.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Errorf("invalid passkey allowed origin: %s", origin)
		}
		if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return fmt.Errorf("passkey allowed origin must be a canonical origin: %s", origin)
		}
		if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
			return fmt.Errorf("passkey allowed origin must use https except localhost: %s", origin)
		}
	}
	return nil
}
