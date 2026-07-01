package admin

import (
	"encoding/json"

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
	response.Success(c, gin.H{"challenge_id": options.Challenge, "public_key": options})
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
	curUser, ok := currentPasskeyUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	items, err := service.AllService.PasskeyService.List(curUser.Id)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, items)
}

func (p *Passkey) RegisterBegin(c *gin.Context) {
	user, ok := currentPasskeyUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	options, err := service.AllService.PasskeyService.BeginRegistration(user, c.ClientIP())
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"challenge_id": options.Challenge, "public_key": options})
}

func (p *Passkey) RegisterFinish(c *gin.Context) {
	user, ok := currentPasskeyUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	payload, err := c.GetRawData()
	if err != nil {
		response.Fail(c, 101, "PasskeyVerificationFailed")
		return
	}
	if err := service.AllService.PasskeyService.FinishRegistration(user, passkeyNameFromPayload(payload), payload, c.ClientIP()); err != nil {
		response.Fail(c, 101, "PasskeyVerificationFailed")
		return
	}
	response.Success(c, gin.H{"ok": true})
}

func (p *Passkey) Rename(c *gin.Context) {
	user, ok := currentPasskeyUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	var form struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if err := service.AllService.PasskeyService.Rename(user.Id, form.ID, form.Name); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, nil)
}

func (p *Passkey) Delete(c *gin.Context) {
	user, ok := currentPasskeyUser(c)
	if !ok {
		response.Fail(c, 403, response.TranslateMsg(c, "NeedLogin"))
		return
	}
	var form struct {
		ID uint `json:"id"`
	}
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if err := service.AllService.PasskeyService.Delete(user.Id, form.ID); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, nil)
}

func currentPasskeyUser(c *gin.Context) (*model.User, bool) {
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

func passkeyNameFromPayload(payload []byte) string {
	var form struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &form); err != nil {
		return ""
	}
	return form.Name
}
