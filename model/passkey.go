package model

import "time"

const (
	AuthChallengeTypePasskeyRegister = "passkey_register"
	AuthChallengeTypePasskeyLogin    = "passkey_login"
)

type UserPasskey struct {
	IdModel
	UserId          uint       `json:"user_id" gorm:"not null;index"`
	Name            string     `json:"name" gorm:"default:'';not null;size:128"`
	CredentialID    string     `json:"credential_id" gorm:"uniqueIndex;not null;size:512"`
	UserHandle      string     `json:"-" gorm:"not null;size:128;index"`
	PublicKey       string     `json:"-" gorm:"type:text;not null"`
	AttestationType string     `json:"attestation_type" gorm:"default:'';not null;size:64"`
	AAGUID          string     `json:"aaguid" gorm:"default:'';not null;size:64"`
	SignCount       uint32     `json:"sign_count" gorm:"default:0;not null"`
	CloneWarning    bool       `json:"clone_warning" gorm:"default:0;not null"`
	Transports      string     `json:"transports" gorm:"type:text"`
	BackupEligible  bool       `json:"backup_eligible" gorm:"default:0;not null"`
	BackupState     bool       `json:"backup_state" gorm:"default:0;not null"`
	LastUsedAt      *time.Time `json:"last_used_at"`
	TimeModel
}

type AuthChallenge struct {
	IdModel
	ChallengeID string     `json:"challenge_id" gorm:"uniqueIndex;not null;size:128"`
	UserId      uint       `json:"user_id" gorm:"default:0;not null;index"`
	Type        string     `json:"type" gorm:"not null;size:32"`
	Data        string     `json:"data" gorm:"type:text;not null"`
	ExpiresAt   time.Time  `json:"expires_at" gorm:"type:timestamp;not null;index"`
	UsedAt      *time.Time `json:"used_at"`
	Ip          string     `json:"ip" gorm:"default:'';not null;size:64;index"`
	TimeModel
}
