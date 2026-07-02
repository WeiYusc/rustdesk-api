package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Email struct{}

func (e *Email) SendVerification(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !settings.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationDisabled"))
		return
	}
	smtpSettings, err := service.AllService.SettingsService.GetSMTP()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !smtpSettings.Ready() {
		response.Fail(c, 101, response.TranslateMsg(c, "SMTPNotConfigured"))
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationNotImplemented"))
}

func (e *Email) ConfirmVerification(c *gin.Context) {
	response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationNotImplemented"))
}

func (e *Email) BeginChange(c *gin.Context) {
	response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationNotImplemented"))
}

func (e *Email) ConfirmChange(c *gin.Context) {
	response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationNotImplemented"))
}
