package service

import "github.com/lejianwen/rustdesk-api/v2/model"

func EmailVerificationRequiredForLogin(u *model.User, settings EmailVerificationSettings) bool {
	return settings.Enabled && settings.RequireForLogin && u != nil && u.EmailVerifiedAt == nil
}
