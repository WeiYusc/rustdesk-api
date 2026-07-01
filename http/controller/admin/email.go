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
		response.Fail(c, 101, "EmailVerificationDisabled")
		return
	}
	response.Fail(c, 101, "EmailVerificationNotImplemented")
}

func (e *Email) ConfirmVerification(c *gin.Context) {
	response.Fail(c, 101, "EmailVerificationNotImplemented")
}

func (e *Email) BeginChange(c *gin.Context) {
	response.Fail(c, 101, "EmailVerificationNotImplemented")
}

func (e *Email) ConfirmChange(c *gin.Context) {
	response.Fail(c, 101, "EmailVerificationNotImplemented")
}
