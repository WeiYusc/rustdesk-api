package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/model/custom_types"
	"gorm.io/gorm"
)

var (
	ErrEmailVerificationInvalidPurpose = errors.New("email verification invalid purpose")
	ErrEmailVerificationInvalidCode    = errors.New("email verification invalid code")
	ErrEmailVerificationExpired        = errors.New("email verification code expired")
	ErrEmailVerificationAlreadyUsed    = errors.New("email verification code already used")
	ErrEmailVerificationCooldown       = errors.New("email verification resend cooldown active")
	ErrEmailVerificationDailyLimit     = errors.New("email verification daily send limit reached")
	ErrEmailVerificationSecretMissing  = errors.New("email verification hash secret missing")
)

type EmailVerificationService struct{}

type EmailVerificationChallenge struct {
	ID        uint      `json:"id"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *EmailVerificationService) CreateCode(userID uint, email, purpose, ip string, settings EmailVerificationSettings) (*EmailVerificationChallenge, string, error) {
	if err := validateEmailVerificationPurpose(purpose); err != nil {
		return nil, "", err
	}
	settings.applyDefaults()
	if err := settings.validate(); err != nil {
		return nil, "", err
	}
	normalizedEmail := NormalizeEmailForVerification(email)
	if normalizedEmail == "" {
		return nil, "", fmt.Errorf("email is required")
	}
	now := time.Now()
	cooldownSince := now.Add(-time.Duration(settings.ResendCooldownSeconds) * time.Second)
	var recentUnused int64
	if err := DB.Model(&model.EmailVerificationToken{}).
		Where("user_id = ? AND email = ? AND purpose = ? AND used_at IS NULL AND created_at > ?", userID, normalizedEmail, purpose, cooldownSince).
		Count(&recentUnused).Error; err != nil {
		return nil, "", err
	}
	if recentUnused > 0 {
		return nil, "", ErrEmailVerificationCooldown
	}

	dayStart := now.Add(-24 * time.Hour)
	var dailyCount int64
	if err := DB.Model(&model.EmailVerificationToken{}).
		Where("user_id = ? AND email = ? AND purpose = ? AND created_at > ?", userID, normalizedEmail, purpose, dayStart).
		Count(&dailyCount).Error; err != nil {
		return nil, "", err
	}
	if dailyCount >= int64(settings.DailySendLimitPerUser) {
		return nil, "", ErrEmailVerificationDailyLimit
	}

	code, err := generateEmailVerificationCode()
	if err != nil {
		return nil, "", err
	}
	codeHash, err := s.hashCode(code)
	if err != nil {
		return nil, "", err
	}
	expiresAt := now.Add(time.Duration(settings.CodeTTLMinutes) * time.Minute)
	token := &model.EmailVerificationToken{UserId: userID, Email: normalizedEmail, Purpose: purpose, CodeHash: codeHash, ExpiresAt: expiresAt, Ip: ip}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		usedAt := now
		if err := tx.Model(&model.EmailVerificationToken{}).
			Where("user_id = ? AND email = ? AND purpose = ? AND used_at IS NULL", userID, normalizedEmail, purpose).
			Update("used_at", usedAt).Error; err != nil {
			return err
		}
		return tx.Create(token).Error
	}); err != nil {
		return nil, "", err
	}
	return &EmailVerificationChallenge{ID: token.Id, Email: token.Email, ExpiresAt: token.ExpiresAt}, code, nil
}

func (s *EmailVerificationService) VerifyCode(userID uint, email, purpose, code string) (*model.EmailVerificationToken, error) {
	return s.verifyCode(DB, userID, email, purpose, code)
}

func (s *EmailVerificationService) VerifyCurrentEmailCode(userID uint, email string, code string) error {
	normalizedEmail := NormalizeEmailForVerification(email)
	return DB.Transaction(func(tx *gorm.DB) error {
		if _, err := s.verifyCode(tx, userID, normalizedEmail, model.EmailVerificationPurposeVerifyCurrent, code); err != nil {
			return err
		}
		now := custom_types.AutoTime(time.Now())
		result := tx.Model(&model.User{}).
			Where("id = ? AND lower(trim(email)) = ?", userID, normalizedEmail).
			Update("email_verified_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *EmailVerificationService) ConfirmEmailChange(userID uint, code string) (string, error) {
	var changedEmail string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		pendingEmail := NormalizeEmailForVerification(user.PendingEmail)
		if pendingEmail == "" {
			return ErrEmailVerificationInvalidCode
		}
		if _, err := s.verifyCode(tx, userID, pendingEmail, model.EmailVerificationPurposeChangeEmail, code); err != nil {
			return err
		}
		now := custom_types.AutoTime(time.Now())
		result := tx.Model(&model.User{}).Where("id = ? AND lower(trim(pending_email)) = ?", userID, pendingEmail).Updates(map[string]interface{}{
			"email":             pendingEmail,
			"email_verified_at": now,
			"pending_email":     "",
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		changedEmail = pendingEmail
		return nil
	})
	return changedEmail, err
}

func (s *EmailVerificationService) verifyCode(db *gorm.DB, userID uint, email, purpose, code string) (*model.EmailVerificationToken, error) {
	if err := validateEmailVerificationPurpose(purpose); err != nil {
		return nil, err
	}
	normalizedEmail := NormalizeEmailForVerification(email)
	var tokens []model.EmailVerificationToken
	if err := db.Where("user_id = ? AND email = ? AND purpose = ?", userID, normalizedEmail, purpose).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, ErrEmailVerificationInvalidCode
	}
	wantedHash, err := s.hashCode(strings.TrimSpace(code))
	if err != nil {
		return nil, err
	}
	var matched *model.EmailVerificationToken
	for i := range tokens {
		if hmac.Equal([]byte(tokens[i].CodeHash), []byte(wantedHash)) {
			matched = &tokens[i]
			break
		}
	}
	if matched == nil {
		return nil, ErrEmailVerificationInvalidCode
	}
	if matched.UsedAt != nil {
		return nil, ErrEmailVerificationAlreadyUsed
	}
	if time.Now().After(matched.ExpiresAt) {
		return nil, ErrEmailVerificationExpired
	}
	now := time.Now()
	result := db.Model(&model.EmailVerificationToken{}).
		Where("id = ? AND used_at IS NULL", matched.Id).
		Update("used_at", now)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrEmailVerificationAlreadyUsed
	}
	matched.UsedAt = &now
	return matched, nil
}

func (s *EmailVerificationService) MarkCurrentEmailVerified(userID uint, email string) error {
	normalizedEmail := NormalizeEmailForVerification(email)
	now := custom_types.AutoTime(time.Now())
	return DB.Model(&model.User{}).
		Where("id = ? AND lower(trim(email)) = ?", userID, normalizedEmail).
		Update("email_verified_at", now).Error
}

func (s *EmailVerificationService) MarkChallengeUsed(challengeID uint) error {
	now := time.Now()
	return DB.Model(&model.EmailVerificationToken{}).
		Where("id = ? AND used_at IS NULL", challengeID).
		Update("used_at", now).Error
}

func NormalizeEmailForVerification(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmailVerificationPurpose(purpose string) error {
	switch purpose {
	case model.EmailVerificationPurposeRegister, model.EmailVerificationPurposeChangeEmail, model.EmailVerificationPurposeVerifyCurrent:
		return nil
	default:
		return ErrEmailVerificationInvalidPurpose
	}
}

func generateEmailVerificationCode() (string, error) {
	var b strings.Builder
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

func (s *EmailVerificationService) hashCode(code string) (string, error) {
	secret, err := emailVerificationHashSecret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func emailVerificationHashSecret() ([]byte, error) {
	if Jwt != nil && len(Jwt.Key) > 0 {
		return Jwt.Key, nil
	}
	return nil, ErrEmailVerificationSecretMissing
}
