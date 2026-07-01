package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Passkey struct{}

func (p *Passkey) LoginBegin(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetPasskey()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !settings.Enabled {
		response.Fail(c, 101, "PasskeyDisabled")
		return
	}
	response.Fail(c, 101, "PasskeyDomainVerificationPending")
}

func (p *Passkey) LoginFinish(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetPasskey()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !settings.Enabled {
		response.Fail(c, 101, "PasskeyDisabled")
		return
	}
	response.Fail(c, 101, "PasskeyDomainVerificationPending")
}

func (p *Passkey) List(c *gin.Context) {
	response.Success(c, []interface{}{})
}

func (p *Passkey) RegisterBegin(c *gin.Context) {
	response.Fail(c, 101, "PasskeyDomainVerificationPending")
}

func (p *Passkey) RegisterFinish(c *gin.Context) {
	response.Fail(c, 101, "PasskeyDomainVerificationPending")
}

func (p *Passkey) Rename(c *gin.Context) {
	response.Fail(c, 101, "PasskeyNotImplemented")
}

func (p *Passkey) Delete(c *gin.Context) {
	response.Fail(c, 101, "PasskeyNotImplemented")
}
