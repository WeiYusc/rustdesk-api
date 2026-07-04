package model

import "time"

const (
	EmailVerificationPurposeRegister      = "register"
	EmailVerificationPurposeChangeEmail   = "change_email"
	EmailVerificationPurposeVerifyCurrent = "verify_current"
	EmailVerificationPurposePasswordReset = "password_reset"
)

type EmailVerificationToken struct {
	IdModel
	UserId         uint       `json:"user_id" gorm:"default:0;not null;index"`
	Email          string     `json:"email" gorm:"not null;size:255;index"`
	Purpose        string     `json:"purpose" gorm:"not null;size:64"`
	CodeHash       string     `json:"-" gorm:"not null;size:255"`
	ExpiresAt      time.Time  `json:"expires_at" gorm:"type:timestamp;not null;index"`
	FailedAttempts int        `json:"-" gorm:"default:0;not null"`
	UsedAt         *time.Time `json:"used_at"`
	Ip             string     `json:"ip" gorm:"default:'';not null;size:64"`
	TimeModel
}
