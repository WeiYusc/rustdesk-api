package model

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuthUpgradeModelsAutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite auth models db: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &UserPasskey{}, &AuthChallenge{}, &EmailVerificationToken{}, &Setting{}); err != nil {
		t.Fatalf("auto migrate auth upgrade models: %v", err)
	}

	user := &User{
		Username:           "passkey-user",
		Email:              "passkey@example.test",
		WebauthnUserHandle: "stable-user-handle",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user with webauthn handle: %v", err)
	}
	if user.Id == 0 {
		t.Fatalf("user id not populated")
	}

	passkey := &UserPasskey{
		UserId:       user.Id,
		Name:         "Laptop Touch ID",
		CredentialID: "credential-id",
		UserHandle:   user.WebauthnUserHandle,
		PublicKey:    "public-key-json",
		SignCount:    10,
	}
	if err := db.Create(passkey).Error; err != nil {
		t.Fatalf("create user passkey: %v", err)
	}

	challenge := &AuthChallenge{
		ChallengeID: "challenge-id",
		Type:        AuthChallengeTypePasskeyLogin,
		Data:        "{}",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		Ip:          "127.0.0.1",
	}
	if err := db.Create(challenge).Error; err != nil {
		t.Fatalf("create auth challenge: %v", err)
	}

	verification := &EmailVerificationToken{
		UserId:    user.Id,
		Email:     "passkey@example.test",
		Purpose:   EmailVerificationPurposeVerifyCurrent,
		CodeHash:  "hash",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Ip:        "127.0.0.1",
	}
	if err := db.Create(verification).Error; err != nil {
		t.Fatalf("create email verification token: %v", err)
	}
}
