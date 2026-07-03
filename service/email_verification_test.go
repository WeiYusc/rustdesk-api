package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmailVerificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite email verification test db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.EmailVerificationToken{}); err != nil {
		t.Fatalf("migrate email verification models: %v", err)
	}
	DB = db
	Jwt = jwt.NewJwt("email-verification-test-secret", time.Hour)
	return db
}

func testEmailVerificationSettings() EmailVerificationSettings {
	return EmailVerificationSettings{CodeTTLMinutes: 10, ResendCooldownSeconds: 60, DailySendLimitPerUser: 3}
}

func TestEmailVerificationCreateCodeRequiresHashSecret(t *testing.T) {
	setupEmailVerificationTestDB(t)
	Jwt = nil
	svc := &EmailVerificationService{}
	if _, _, err := svc.CreateCode(7, "user@example.com", model.EmailVerificationPurposeRegister, "", testEmailVerificationSettings()); !errors.Is(err, ErrEmailVerificationSecretMissing) {
		t.Fatalf("CreateCode without hash secret err = %v, want ErrEmailVerificationSecretMissing", err)
	}
}

func TestServiceNewInitializesEmailVerificationService(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := New(Config, db, Logger, Jwt, nil)
	if svc.EmailVerificationService == nil {
		t.Fatalf("New left EmailVerificationService nil")
	}
}

func TestEmailVerificationCreateCodeNormalizesAndStoresHashedCode(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}

	challenge, code, err := svc.CreateCode(42, " User@Example.COM ", model.EmailVerificationPurposeRegister, "203.0.113.10", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}
	if challenge.ID == 0 || challenge.Email != "user@example.com" || challenge.ExpiresAt.Before(time.Now()) {
		t.Fatalf("unexpected challenge: %#v", challenge)
	}
	if len(code) != 6 {
		t.Fatalf("code length = %d, want 6", len(code))
	}
	var token model.EmailVerificationToken
	if err := db.First(&token, challenge.ID).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token.Email != "user@example.com" || token.UserId != 42 || token.Purpose != model.EmailVerificationPurposeRegister || token.Ip != "203.0.113.10" {
		t.Fatalf("stored token fields = %#v", token)
	}
	if token.CodeHash == "" || token.CodeHash == code || strings.Contains(token.CodeHash, code) {
		t.Fatalf("token code hash exposes plain code: hash=%q code=%q", token.CodeHash, code)
	}
}

func TestEmailVerificationVerifyCodeAcceptsValidAndMarksUsed(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	challenge, code, err := svc.CreateCode(7, "User@Example.COM", model.EmailVerificationPurposeVerifyCurrent, "", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}

	token, err := svc.VerifyCode(7, " user@example.com ", model.EmailVerificationPurposeVerifyCurrent, code)
	if err != nil {
		t.Fatalf("VerifyCode valid error: %v", err)
	}
	if token.Id != challenge.ID || token.UsedAt == nil {
		t.Fatalf("verified token = %#v, want id %d with UsedAt", token, challenge.ID)
	}
	var stored model.EmailVerificationToken
	if err := db.First(&stored, challenge.ID).Error; err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if stored.UsedAt == nil {
		t.Fatalf("stored token UsedAt was not set")
	}
}

func TestEmailVerificationVerifyCodeRejectsWrongExpiredAndUsed(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	_, code, err := svc.CreateCode(7, "user@example.com", model.EmailVerificationPurposeRegister, "", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}
	if _, err := svc.VerifyCode(7, "user@example.com", model.EmailVerificationPurposeRegister, "000000"); !errors.Is(err, ErrEmailVerificationInvalidCode) {
		t.Fatalf("wrong code err = %v, want ErrEmailVerificationInvalidCode", err)
	}
	if _, err := svc.VerifyCode(7, "user@example.com", model.EmailVerificationPurposeRegister, code); err != nil {
		t.Fatalf("VerifyCode first use error: %v", err)
	}
	if _, err := svc.VerifyCode(7, "user@example.com", model.EmailVerificationPurposeRegister, code); !errors.Is(err, ErrEmailVerificationAlreadyUsed) {
		t.Fatalf("used code err = %v, want ErrEmailVerificationAlreadyUsed", err)
	}

	expiredHash, err := svc.hashCode("123456")
	if err != nil {
		t.Fatalf("hash expired code: %v", err)
	}
	expired := model.EmailVerificationToken{UserId: 8, Email: "expired@example.com", Purpose: model.EmailVerificationPurposeRegister, CodeHash: expiredHash, ExpiresAt: time.Now().Add(-time.Minute)}
	if err := db.Create(&expired).Error; err != nil {
		t.Fatalf("create expired token: %v", err)
	}
	if _, err := svc.VerifyCode(8, "expired@example.com", model.EmailVerificationPurposeRegister, "123456"); !errors.Is(err, ErrEmailVerificationExpired) {
		t.Fatalf("expired code err = %v, want ErrEmailVerificationExpired", err)
	}
}

func TestEmailVerificationCreateCodeEnforcesCooldownAndDailyLimit(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	settings := EmailVerificationSettings{CodeTTLMinutes: 10, ResendCooldownSeconds: 60, DailySendLimitPerUser: 2}
	if _, _, err := svc.CreateCode(9, "limit@example.com", model.EmailVerificationPurposeRegister, "", settings); err != nil {
		t.Fatalf("first CreateCode error: %v", err)
	}
	if _, _, err := svc.CreateCode(9, "limit@example.com", model.EmailVerificationPurposeRegister, "", settings); !errors.Is(err, ErrEmailVerificationCooldown) {
		t.Fatalf("cooldown err = %v, want ErrEmailVerificationCooldown", err)
	}
	oldCreated := custom_types.AutoTime(time.Now().Add(-2 * time.Minute))
	if err := db.Model(&model.EmailVerificationToken{}).Where("user_id = ?", 9).Updates(map[string]interface{}{"created_at": oldCreated, "used_at": time.Now()}).Error; err != nil {
		t.Fatalf("age first token: %v", err)
	}
	if _, _, err := svc.CreateCode(9, "limit@example.com", model.EmailVerificationPurposeRegister, "", settings); err != nil {
		t.Fatalf("second CreateCode after cooldown error: %v", err)
	}
	if err := db.Model(&model.EmailVerificationToken{}).Where("user_id = ? AND used_at IS NULL", 9).Update("used_at", time.Now()).Error; err != nil {
		t.Fatalf("mark second token used to bypass cooldown: %v", err)
	}
	if _, _, err := svc.CreateCode(9, "limit@example.com", model.EmailVerificationPurposeRegister, "", settings); !errors.Is(err, ErrEmailVerificationDailyLimit) {
		t.Fatalf("daily limit err = %v, want ErrEmailVerificationDailyLimit", err)
	}
}

func TestEmailVerificationVerifyCodeLocksChallengeAfterRepeatedWrongCodes(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	_, code, err := svc.CreateCode(10, "limit-guesses@example.com", model.EmailVerificationPurposeVerifyCurrent, "", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}
	for i := 0; i < maxEmailVerificationFailedAttempts; i++ {
		if _, err := svc.VerifyCode(10, "limit-guesses@example.com", model.EmailVerificationPurposeVerifyCurrent, "000000"); !errors.Is(err, ErrEmailVerificationInvalidCode) {
			t.Fatalf("wrong code attempt %d err = %v, want ErrEmailVerificationInvalidCode", i+1, err)
		}
	}
	if _, err := svc.VerifyCode(10, "limit-guesses@example.com", model.EmailVerificationPurposeVerifyCurrent, code); !errors.Is(err, ErrEmailVerificationAlreadyUsed) {
		t.Fatalf("valid code after repeated wrong attempts err = %v, want ErrEmailVerificationAlreadyUsed", err)
	}
	var token model.EmailVerificationToken
	if err := db.Where("user_id = ? AND email = ?", 10, "limit-guesses@example.com").First(&token).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	if token.UsedAt == nil || token.FailedAttempts != maxEmailVerificationFailedAttempts {
		t.Fatalf("locked token = %#v, want used with failed attempts", token)
	}
}

func TestEmailVerificationVerifyCodeReturnsAlreadyUsedWhenUpdateRaces(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	challenge, code, err := svc.CreateCode(11, "race@example.com", model.EmailVerificationPurposeVerifyCurrent, "", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}
	now := time.Now()
	if err := db.Model(&model.EmailVerificationToken{}).Where("id = ?", challenge.ID).Update("used_at", now).Error; err != nil {
		t.Fatalf("pre-mark token used: %v", err)
	}
	if _, err := svc.VerifyCode(11, "race@example.com", model.EmailVerificationPurposeVerifyCurrent, code); !errors.Is(err, ErrEmailVerificationAlreadyUsed) {
		t.Fatalf("raced used token err = %v, want ErrEmailVerificationAlreadyUsed", err)
	}
}

func TestEmailVerificationVerifyCurrentEmailCodeRollsBackTokenWhenUserEmailChanged(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	user := &model.User{Username: "rollback-current", Email: "user@example.com"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	challenge, code, err := svc.CreateCode(user.Id, "user@example.com", model.EmailVerificationPurposeVerifyCurrent, "", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.Id).Update("email", "changed@example.com").Error; err != nil {
		t.Fatalf("change user email before verify: %v", err)
	}
	if err := svc.VerifyCurrentEmailCode(user.Id, "user@example.com", code); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("VerifyCurrentEmailCode mismatched user email err = %v, want ErrRecordNotFound", err)
	}
	var stored model.EmailVerificationToken
	if err := db.First(&stored, challenge.ID).Error; err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if stored.UsedAt != nil {
		t.Fatalf("rolled-back current email verification consumed token: %#v", stored.UsedAt)
	}
}

func TestEmailVerificationConfirmEmailChangeSuccessIsAtomic(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	user := &model.User{Username: "change-user", Email: "old@example.com", PendingEmail: "New@Example.COM"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	challenge, code, err := svc.CreateCode(user.Id, "new@example.com", model.EmailVerificationPurposeChangeEmail, "", testEmailVerificationSettings())
	if err != nil {
		t.Fatalf("CreateCode error: %v", err)
	}
	changedEmail, err := svc.ConfirmEmailChange(user.Id, code)
	if err != nil {
		t.Fatalf("ConfirmEmailChange error: %v", err)
	}
	if changedEmail != "new@example.com" {
		t.Fatalf("changedEmail = %q, want new@example.com", changedEmail)
	}
	var after model.User
	if err := db.First(&after, user.Id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if after.Email != "new@example.com" || after.PendingEmail != "" || after.EmailVerifiedAt == nil {
		t.Fatalf("email change state = %#v", after)
	}
	var token model.EmailVerificationToken
	if err := db.First(&token, challenge.ID).Error; err != nil {
		t.Fatalf("reload token: %v", err)
	}
	if token.UsedAt == nil {
		t.Fatalf("confirm email change did not consume token")
	}
}

func TestEmailVerificationMarkCurrentEmailVerifiedOnlyMatchingNormalizedEmail(t *testing.T) {
	db := setupEmailVerificationTestDB(t)
	svc := &EmailVerificationService{}
	user := &model.User{Username: "verified-user", Email: "User@Example.COM", PendingEmail: "pending@example.com"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := svc.MarkCurrentEmailVerified(user.Id, "other@example.com"); err != nil {
		t.Fatalf("MarkCurrentEmailVerified non-matching error: %v", err)
	}
	var afterNonMatch model.User
	if err := db.First(&afterNonMatch, user.Id).Error; err != nil {
		t.Fatalf("load non-match user: %v", err)
	}
	if afterNonMatch.EmailVerifiedAt != nil {
		t.Fatalf("non-matching email set EmailVerifiedAt: %#v", afterNonMatch.EmailVerifiedAt)
	}
	if err := svc.MarkCurrentEmailVerified(user.Id, " user@example.com "); err != nil {
		t.Fatalf("MarkCurrentEmailVerified matching error: %v", err)
	}
	var afterMatch model.User
	if err := db.First(&afterMatch, user.Id).Error; err != nil {
		t.Fatalf("load match user: %v", err)
	}
	if afterMatch.EmailVerifiedAt == nil {
		t.Fatalf("matching email did not set EmailVerifiedAt")
	}
	if afterMatch.PendingEmail != "pending@example.com" {
		t.Fatalf("MarkCurrentEmailVerified changed PendingEmail to %q", afterMatch.PendingEmail)
	}
}
