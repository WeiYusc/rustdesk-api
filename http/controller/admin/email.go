package admin

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Email struct{}

type emailVerificationConfirmRequest struct {
	Code string `json:"code"`
}

type emailChangeBeginRequest struct {
	Email string `json:"email"`
}

type emailVerificationSendResponse struct {
	ChallengeID uint      `json:"challenge_id"`
	Email       string    `json:"email"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type emailSenderForVerification interface {
	Send(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error
}

type cleanupRegistrar interface {
	Cleanup(func())
}

var emailVerificationSender = struct {
	sync.RWMutex
	sender emailSenderForVerification
}{sender: service.NewSMTPSender(nil)}

func setEmailVerificationSenderForTest(t cleanupRegistrar, sender emailSenderForVerification) {
	emailVerificationSender.Lock()
	previous := emailVerificationSender.sender
	emailVerificationSender.sender = sender
	emailVerificationSender.Unlock()
	t.Cleanup(func() {
		emailVerificationSender.Lock()
		emailVerificationSender.sender = previous
		emailVerificationSender.Unlock()
	})
}

func (e *Email) SendVerification(c *gin.Context) {
	user, ok := currentEmailUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	if service.NormalizeEmailForVerification(user.Email) == "" {
		response.Fail(c, 101, "email is required")
		return
	}
	settings, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !settings.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationDisabled"))
		return
	}
	smtpSettings, err := service.AllService.SettingsService.GetSMTPForSend()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !smtpSettings.Ready() {
		response.Fail(c, 101, response.TranslateMsg(c, "SMTPNotConfigured"))
		return
	}
	challenge, code, err := service.AllService.EmailVerificationService.CreateCode(user.Id, user.Email, model.EmailVerificationPurposeVerifyCurrent, c.ClientIP(), settings)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	message := service.SMTPMessage{
		To:       challenge.Email,
		Subject:  "Verify your email address",
		TextBody: fmt.Sprintf("Your email verification code is %s. It expires at %s.", code, challenge.ExpiresAt.Format(time.RFC3339)),
	}
	if err := sendEmailVerificationMessage(c.Request.Context(), smtpSettings, message); err != nil {
		if markErr := service.AllService.EmailVerificationService.MarkChallengeUsed(challenge.ID); markErr != nil {
			response.Fail(c, 101, markErr.Error())
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, emailVerificationSendResponse{ChallengeID: challenge.ID, Email: challenge.Email, ExpiresAt: challenge.ExpiresAt})
}

func (e *Email) ConfirmVerification(c *gin.Context) {
	user, ok := currentEmailUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	if service.NormalizeEmailForVerification(user.Email) == "" {
		response.Fail(c, 101, "email is required")
		return
	}
	var form emailVerificationConfirmRequest
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if strings.TrimSpace(form.Code) == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if err := service.AllService.EmailVerificationService.VerifyCurrentEmailCode(user.Id, user.Email, form.Code); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true})
}

func sendEmailVerificationMessage(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	emailVerificationSender.RLock()
	sender := emailVerificationSender.sender
	emailVerificationSender.RUnlock()
	if sender == nil {
		sender = service.NewSMTPSender(nil)
	}
	return sender.Send(ctx, settings, message)
}

func currentEmailUser(c *gin.Context) (*model.User, bool) {
	curUser, ok := c.Get("curUser")
	if !ok {
		return nil, false
	}
	user, ok := curUser.(*model.User)
	if !ok || user == nil || user.Id == 0 {
		return nil, false
	}
	return user, true
}

func (e *Email) BeginChange(c *gin.Context) {
	user, ok := currentEmailUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	var form emailChangeBeginRequest
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	newEmail := service.NormalizeEmailForVerification(form.Email)
	parsedEmail, parseErr := mail.ParseAddress(newEmail)
	if newEmail == "" || parseErr != nil || parsedEmail.Address != newEmail {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	currentEmail := service.NormalizeEmailForVerification(user.Email)
	if newEmail == currentEmail {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	settings, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !settings.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationDisabled"))
		return
	}
	smtpSettings, err := service.AllService.SettingsService.GetSMTPForSend()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !smtpSettings.Ready() {
		response.Fail(c, 101, response.TranslateMsg(c, "SMTPNotConfigured"))
		return
	}
	if err := service.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("pending_email", newEmail).Error; err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	challenge, code, err := service.AllService.EmailVerificationService.CreateCode(user.Id, newEmail, model.EmailVerificationPurposeChangeEmail, c.ClientIP(), settings)
	if err != nil {
		_ = service.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("pending_email", "").Error
		response.Fail(c, 101, err.Error())
		return
	}
	message := service.SMTPMessage{
		To:       challenge.Email,
		Subject:  "Confirm your new email address",
		TextBody: fmt.Sprintf("Your email change verification code is %s. It expires at %s.", code, challenge.ExpiresAt.Format(time.RFC3339)),
	}
	if err := sendEmailVerificationMessage(c.Request.Context(), smtpSettings, message); err != nil {
		if markErr := service.AllService.EmailVerificationService.MarkChallengeUsed(challenge.ID); markErr != nil {
			response.Fail(c, 101, markErr.Error())
			return
		}
		if clearErr := service.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("pending_email", "").Error; clearErr != nil {
			response.Fail(c, 101, clearErr.Error())
			return
		}
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, emailVerificationSendResponse{ChallengeID: challenge.ID, Email: challenge.Email, ExpiresAt: challenge.ExpiresAt})
}

func (e *Email) ConfirmChange(c *gin.Context) {
	user, ok := currentEmailUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	var form emailVerificationConfirmRequest
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if strings.TrimSpace(form.Code) == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if _, err := service.AllService.EmailVerificationService.ConfirmEmailChange(user.Id, form.Code); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true})
}
