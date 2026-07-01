package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Passkey struct{}

func (p *Passkey) LoginBegin(c *gin.Context) {
	options, err := service.AllService.PasskeyService.BeginLogin(c.ClientIP())
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, options)
}

func (p *Passkey) LoginFinish(c *gin.Context) {
	payload, err := c.GetRawData()
	if err != nil {
		response.Fail(c, 101, "PasskeyVerificationFailed")
		return
	}
	user, token, err := service.AllService.PasskeyService.FinishLogin(payload, c.ClientIP())
	if err != nil {
		response.Fail(c, 101, "PasskeyVerificationFailed")
		return
	}
	responseLoginSuccess(c, user, token)
}

func (p *Passkey) List(c *gin.Context) {
	response.Success(c, []interface{}{})
}

func (p *Passkey) RegisterBegin(c *gin.Context) {
	curUser, ok := c.Get("curUser")
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	user, ok := curUser.(*model.User)
	if !ok || user == nil || user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	options, err := service.AllService.PasskeyService.BeginRegistration(user, c.ClientIP())
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, options)
}

func (p *Passkey) RegisterFinish(c *gin.Context) {
	curUser, ok := c.Get("curUser")
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	user, ok := curUser.(*model.User)
	if !ok || user == nil || user.Id == 0 {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	payload, err := c.GetRawData()
	if err != nil {
		response.Fail(c, 101, "PasskeyVerificationFailed")
		return
	}
	if err := service.AllService.PasskeyService.FinishRegistration(user, payload, c.ClientIP()); err != nil {
		response.Fail(c, 101, "PasskeyVerificationFailed")
		return
	}
	response.Success(c, gin.H{"ok": true})
}

func (p *Passkey) Rename(c *gin.Context) {
	response.Fail(c, 101, "PasskeyNotImplemented")
}

func (p *Passkey) Delete(c *gin.Context) {
	response.Fail(c, 101, "PasskeyNotImplemented")
}
