package admin

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Settings struct{}

type smtpTestSender interface {
	Send(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error
}

var smtpTestSenderState = struct {
	sync.RWMutex
	sender smtpTestSender
}{sender: service.NewSMTPSender(nil)}

type settingsCleanupRegistrar interface {
	Cleanup(func())
}

func setSMTPTestSenderForTest(t settingsCleanupRegistrar, sender smtpTestSender) {
	smtpTestSenderState.Lock()
	previous := smtpTestSenderState.sender
	smtpTestSenderState.sender = sender
	smtpTestSenderState.Unlock()
	t.Cleanup(func() {
		smtpTestSenderState.Lock()
		smtpTestSenderState.sender = previous
		smtpTestSenderState.Unlock()
	})
}

func sendSMTPTestMessage(ctx context.Context, settings service.SMTPSettings, message service.SMTPMessage) error {
	smtpTestSenderState.RLock()
	sender := smtpTestSenderState.sender
	smtpTestSenderState.RUnlock()
	if sender == nil {
		sender = service.NewSMTPSender(nil)
	}
	return sender.Send(ctx, settings, message)
}

func (s *Settings) GetRegisterPolicy(c *gin.Context) {
	settings, err := service.AllService.SettingsService.GetRegisterPolicy()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, settings)
}

func (s *Settings) UpdateRegisterPolicy(c *gin.Context) {
	settings := service.RegisterPolicySettings{}
	if err := c.ShouldBindJSON(&settings); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	user := service.AllService.UserService.CurUser(c)
	var updatedBy uint
	if user != nil {
		updatedBy = user.Id
	}
	if err := service.AllService.SettingsService.SaveRegisterPolicy(settings, updatedBy); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	updated, err := service.AllService.SettingsService.GetRegisterPolicy()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, updated)
}

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
	form := &admin.SMTPTestRequest{}
	if err := c.ShouldBindJSON(form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if errList := global.Validator.ValidStruct(c, form); len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	smtpSettings, err := service.AllService.SettingsService.GetSMTPForSend()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !smtpSettings.Ready() {
		response.Fail(c, 101, response.TranslateMsg(c, "SMTPServiceUnavailable"))
		return
	}
	message := service.SMTPMessage{
		To:       form.To,
		Subject:  "RustDesk API SMTP test",
		TextBody: fmt.Sprintf("This is a RustDesk API SMTP test email sent at %s.", time.Now().UTC().Format(time.RFC3339)),
	}
	if err := sendSMTPTestMessage(c.Request.Context(), smtpSettings, message); err != nil {
		failureKey := classifySMTPTestError(err)
		if global.Logger != nil {
			global.Logger.Warnf("SMTP test email failed: %s", failureKey)
		}
		response.Fail(c, 101, response.TranslateMsg(c, failureKey))
		return
	}
	response.Success(c, nil)
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
		response.Fail(c, 101, translateSettingsError(c, err))
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
		response.Fail(c, 101, translateSettingsError(c, err))
		return
	}
	updated, err := service.AllService.SettingsService.GetAuthPolicy()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, updated)
}

func translateSettingsError(c *gin.Context, err error) string {
	switch err.Error() {
	case "PasswordLoginDisableRequiresFallback", "EmailVerificationRequiresSMTP", "EmailVerificationLoginRequiresVerifiedAdmins":
		return response.TranslateMsg(c, err.Error())
	default:
		return err.Error()
	}
}
