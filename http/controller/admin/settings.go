package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Settings struct{}

func (s *Settings) GetSMTP(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetSMTP()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, settings)
}

func (s *Settings) UpdateSMTP(c *gin.Context) {
	settings := service.SMTPSettings{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	user := service.AllService.UserService.CurUser(c)
	var updatedBy uint
	if user != nil {
		updatedBy = user.Id
	}
	if err := service.AllService.SettingsService.SaveSMTP(settings, updatedBy); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	masked, err := service.AllService.SettingsService.GetSMTP()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, masked)
}

func (s *Settings) TestSMTP(c *gin.Context) {
	response.Fail(c, 101, "SmtpSendNotImplemented")
}

func (s *Settings) GetEmailVerification(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, settings)
}

func (s *Settings) UpdateEmailVerification(c *gin.Context) {
	settings := service.EmailVerificationSettings{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	user := service.AllService.UserService.CurUser(c)
	var updatedBy uint
	if user != nil {
		updatedBy = user.Id
	}
	if err := service.AllService.SettingsService.SaveEmailVerification(settings, updatedBy); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	updated, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, updated)
}

func (s *Settings) GetPasskey(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetPasskey()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, settings)
}

func (s *Settings) UpdatePasskey(c *gin.Context) {
	settings := service.PasskeySettings{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	user := service.AllService.UserService.CurUser(c)
	var updatedBy uint
	if user != nil {
		updatedBy = user.Id
	}
	if err := service.AllService.SettingsService.SavePasskey(settings, updatedBy); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	updated, err := service.AllService.SettingsService.GetPasskey()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, updated)
}

func (s *Settings) GetAuthPolicy(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetAuthPolicy()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, settings)
}

func (s *Settings) UpdateAuthPolicy(c *gin.Context) {
	settings := service.AuthPolicySettings{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	user := service.AllService.UserService.CurUser(c)
	var updatedBy uint
	if user != nil {
		updatedBy = user.Id
	}
	if err := service.AllService.SettingsService.SaveAuthPolicy(settings, updatedBy); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	updated, err := service.AllService.SettingsService.GetAuthPolicy()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, updated)
}
