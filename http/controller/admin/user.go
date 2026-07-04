package admin

import (
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	adResp "github.com/lejianwen/rustdesk-api/v2/http/response/admin"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"gorm.io/gorm"
)

type User struct {
}

// Detail 管理员
// @Tags 用户
// @Summary 管理员详情
// @Description 管理员详情
// @Accept  json
// @Produce  json
// @Param id path int true "ID"
// @Success 200 {object} response.Response{data=model.User}
// @Failure 500 {object} response.Response
// @Router /admin/user/detail/{id} [get]
// @Security token
func (ct *User) Detail(c *gin.Context) {
	iid, ok := parsePositiveIDParam(c)
	if !ok {
		return
	}
	u := service.AllService.UserService.InfoById(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
	return
}

// Create 管理员
// @Tags 用户
// @Summary 创建管理员
// @Description 创建管理员
// @Accept  json
// @Produce  json
// @Param body body admin.UserForm true "管理员信息"
// @Success 200 {object} response.Response{data=model.User}
// @Failure 500 {object} response.Response
// @Router /admin/user/create [post]
// @Security token
func (ct *User) Create(c *gin.Context) {
	f := &admin.UserForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if f.Password == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+"PasswordRequired")
		return
	}
	if errList := global.Validator.ValidVar(c, f.Password, "gte=4,lte=32"); len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToUser()
	u.Password = f.Password
	err := service.AllService.UserService.Create(u)
	if err != nil {
		response.Fail(c, 101, translateUserServiceError(c, "OperationFailed", err))
		return
	}
	response.Success(c, nil)
}

// List 列表
// @Tags 用户
// @Summary 管理员列表
// @Description 管理员列表
// @Accept  json
// @Produce  json
// @Param page query int false "页码"
// @Param page_size query int false "页大小"
// @Param username query int false "账户"
// @Success 200 {object} response.Response{data=model.UserList}
// @Failure 500 {object} response.Response
// @Router /admin/user/list [get]
// @Security token
func (ct *User) List(c *gin.Context) {
	query := &admin.UserQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.UserService.List(query.Page, query.PageSize, func(tx *gorm.DB) {
		if query.Username != "" {
			tx.Where("username like ?", "%"+query.Username+"%")
		}
	})
	response.Success(c, res)
}

// Update 编辑
// @Tags 用户
// @Summary 管理员编辑
// @Description 管理员编辑
// @Accept  json
// @Produce  json
// @Param body body admin.UserForm true "用户信息"
// @Success 200 {object} response.Response{data=model.User}
// @Failure 500 {object} response.Response
// @Router /admin/user/update [post]
// @Security token
func (ct *User) Update(c *gin.Context) {
	f := &admin.UserForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := f.ToUser()
	err := service.AllService.UserService.Update(u)
	if err != nil {
		response.Fail(c, 101, translateUserServiceError(c, "OperationFailed", err))
		return
	}
	response.Success(c, nil)
}

// Delete 删除
// @Tags 用户
// @Summary 管理员删除
// @Description 管理员编删除
// @Accept  json
// @Produce  json
// @Param body body admin.UserForm true "用户信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user/delete [post]
// @Security token
func (ct *User) Delete(c *gin.Context) {
	f := &admin.UserForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id := f.Id
	errList := global.Validator.ValidVar(c, id, "required,gt=0")
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.InfoById(f.Id)
	if u.Id > 0 {
		err := service.AllService.UserService.Delete(u)
		if err == nil {
			response.Success(c, nil)
			return
		}
		response.Fail(c, 101, translateUserServiceError(c, "", err))
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

func translateUserServiceError(c *gin.Context, fallbackPrefix string, err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "PasswordRequired":
		return response.TranslateMsg(c, "ParamsError") + err.Error()
	case "LastAdminCannotDelete", "LastAdminCannotUpdate":
		return response.TranslateMsg(c, err.Error())
	default:
		if fallbackPrefix == "" {
			return err.Error()
		}
		return response.TranslateMsg(c, fallbackPrefix) + err.Error()
	}
}

// UpdatePassword 修改密码
// @Tags 用户
// @Summary 修改密码
// @Description 修改密码
// @Accept  json
// @Produce  json
// @Param body body admin.UserPasswordForm true "用户信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user/updatePassword [post]
// @Security token
func (ct *User) UpdatePassword(c *gin.Context) {
	f := &admin.UserPasswordForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.InfoById(f.Id)
	if u.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.UserService.UpdatePassword(u, f.Password)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// Current 当前用户
// @Tags 用户
// @Summary 当前用户
// @Description 当前用户
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response{data=adResp.LoginPayload}
// @Failure 500 {object} response.Response
// @Router /admin/user/current [get]
// @Security token
func (ct *User) Current(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	token, _ := c.Get("token")
	t := token.(string)
	responseLoginSuccess(c, u, t)
}

// ChangeCurPwd 修改当前用户密码
// @Tags 用户
// @Summary 修改当前用户密码
// @Description 修改当前用户密码
// @Accept  json
// @Produce  json
// @Param body body admin.ChangeCurPasswordForm true "用户信息"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/user/changeCurPwd [post]
// @Security token
func (ct *User) ChangeCurPwd(c *gin.Context) {
	f := &admin.ChangeCurPasswordForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}

	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.CurUser(c)
	// Verify the old password only when the account already has one set
	if !service.AllService.UserService.IsPasswordEmptyByUser(u) {
		ok, _, err := utils.VerifyPassword(u.Password, f.OldPassword)
		if err != nil || !ok {
			response.Fail(c, 101, response.TranslateMsg(c, "OldPasswordError"))
			return
		}
	}
	err := service.AllService.UserService.UpdatePassword(u, f.NewPassword)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// ChangeCurInfo 修改当前用户资料
func (ct *User) ChangeCurInfo(c *gin.Context) {
	f := &admin.ChangeCurInfoForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	u := service.AllService.UserService.CurUser(c)
	requestedEmail := service.NormalizeEmailForVerification(f.Email)
	currentEmail := service.NormalizeEmailForVerification(u.Email)
	settings, settingsErr := service.AllService.SettingsService.GetEmailVerification()
	if settingsErr != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+settingsErr.Error())
		return
	}
	if settings.Enabled && settings.RequireForEmailChange && requestedEmail != "" && requestedEmail != currentEmail {
		response.Fail(c, 101, response.TranslateMsg(c, "DirectEmailChangeRequiresVerification"))
		return
	}
	if settings.Enabled && settings.RequireForEmailChange && requestedEmail == "" {
		f.Email = u.Email
	}
	err := service.AllService.UserService.UpdateCurrentInfo(u, f.Nickname, f.Avatar, f.Email)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

// MyOauth
// @Tags 用户
// @Summary 我的授权
// @Description 我的授权
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response{data=[]adResp.UserOauthItem}
// @Failure 500 {object} response.Response
// @Router /admin/user/myOauth [get]
// @Security token
func (ct *User) MyOauth(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	oal := service.AllService.OauthService.List(1, 100, nil)
	ops := make([]string, 0)
	for _, oa := range oal.Oauths {
		ops = append(ops, oa.Op)
	}
	uts := service.AllService.UserService.UserThirdsByUserId(u.Id)
	var res []*adResp.UserOauthItem
	for _, oa := range oal.Oauths {
		item := &adResp.UserOauthItem{
			Op: oa.Op,
		}
		for _, ut := range uts {
			if ut.Op == oa.Op {
				item.Status = 1
				break
			}
		}
		res = append(res, item)
	}
	response.Success(c, res)
}

// groupUsers
func (ct *User) GroupUsers(c *gin.Context) {
	query := &admin.GroupUsersQuery{}
	if err := c.ShouldBindJSON(query); err != nil && !errors.Is(err, io.EOF) {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	aG := service.AllService.GroupService.List(1, 999, nil)
	aU := service.AllService.UserService.List(1, 9999, func(tx *gorm.DB) {
		if query.GroupId > 0 {
			tx.Where("group_id = ?", query.GroupId)
		}
	})
	response.Success(c, gin.H{
		"groups": aG.Groups,
		"users":  aU.Users,
	})
}

type registerEmailSendRequest struct {
	Email string `json:"email"`
}

type registerEmailRateLimitBucket struct {
	lastSend time.Time
	sends    []time.Time
}

type registerEmailRateLimitSnapshot struct {
	global registerEmailRateLimitBucket
	ips    map[string]registerEmailRateLimitBucket
}

var registerEmailRateLimitState = struct {
	sync.Mutex
	global  registerEmailRateLimitBucket
	buckets map[string]registerEmailRateLimitBucket
}{buckets: map[string]registerEmailRateLimitBucket{}}

func resetRegisterEmailRateLimiterForTest(t cleanupRegistrar) {
	registerEmailRateLimitState.Lock()
	previous := registerEmailRateLimitSnapshot{global: registerEmailRateLimitState.global, ips: registerEmailRateLimitState.buckets}
	registerEmailRateLimitState.global = registerEmailRateLimitBucket{}
	registerEmailRateLimitState.buckets = map[string]registerEmailRateLimitBucket{}
	registerEmailRateLimitState.Unlock()
	t.Cleanup(func() {
		registerEmailRateLimitState.Lock()
		registerEmailRateLimitState.global = previous.global
		registerEmailRateLimitState.buckets = previous.ips
		registerEmailRateLimitState.Unlock()
	})
}

func allowRegisterEmailSend(ip string, now time.Time) bool {
	const ipCooldown = 60 * time.Second
	const ipDailyLimit = 50
	const globalCooldown = time.Second
	const globalDailyLimit = 1000
	key := strings.TrimSpace(ip)
	if key == "" {
		key = "unknown"
	}
	registerEmailRateLimitState.Lock()
	defer registerEmailRateLimitState.Unlock()
	globalBucket := pruneRegisterEmailRateLimitBucket(registerEmailRateLimitState.global, now)
	if !globalBucket.lastSend.IsZero() && now.Sub(globalBucket.lastSend) < globalCooldown {
		registerEmailRateLimitState.global = globalBucket
		return false
	}
	if len(globalBucket.sends) >= globalDailyLimit {
		registerEmailRateLimitState.global = globalBucket
		return false
	}
	bucket := pruneRegisterEmailRateLimitBucket(registerEmailRateLimitState.buckets[key], now)
	if !bucket.lastSend.IsZero() && now.Sub(bucket.lastSend) < ipCooldown {
		registerEmailRateLimitState.buckets[key] = bucket
		registerEmailRateLimitState.global = globalBucket
		return false
	}
	if len(bucket.sends) >= ipDailyLimit {
		registerEmailRateLimitState.buckets[key] = bucket
		registerEmailRateLimitState.global = globalBucket
		return false
	}
	globalBucket.lastSend = now
	globalBucket.sends = append(globalBucket.sends, now)
	bucket.lastSend = now
	bucket.sends = append(bucket.sends, now)
	registerEmailRateLimitState.global = globalBucket
	registerEmailRateLimitState.buckets[key] = bucket
	pruneRegisterEmailRateLimitIPs(now)
	return true
}

func pruneRegisterEmailRateLimitBucket(bucket registerEmailRateLimitBucket, now time.Time) registerEmailRateLimitBucket {
	dayStart := now.Add(-24 * time.Hour)
	kept := bucket.sends[:0]
	for _, sentAt := range bucket.sends {
		if sentAt.After(dayStart) {
			kept = append(kept, sentAt)
		}
	}
	bucket.sends = kept
	if !bucket.lastSend.IsZero() && bucket.lastSend.Before(dayStart) {
		bucket.lastSend = time.Time{}
	}
	return bucket
}

func pruneRegisterEmailRateLimitIPs(now time.Time) {
	if len(registerEmailRateLimitState.buckets) <= 4096 {
		return
	}
	dayStart := now.Add(-24 * time.Hour)
	for key, bucket := range registerEmailRateLimitState.buckets {
		if bucket.lastSend.Before(dayStart) {
			delete(registerEmailRateLimitState.buckets, key)
		}
	}
}

func respondRegisterEmailSendSuppressed(c *gin.Context) {
	response.Success(c, gin.H{"ok": true})
}

func (ct *User) SendRegisterVerification(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			if global.Logger != nil {
				global.Logger.Warnf("register email verification send recovered: %v", r)
			}
			respondRegisterEmailSendSuppressed(c)
		}
	}()
	policy, err := service.AllService.SettingsService.GetRegisterPolicy()
	if err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("register email verification policy lookup failed: %v", err)
		}
		respondRegisterEmailSendSuppressed(c)
		return
	}
	if !policy.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "RegisterClosed"))
		return
	}
	settings, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("register email verification settings lookup failed: %v", err)
		}
		respondRegisterEmailSendSuppressed(c)
		return
	}
	if !settings.Enabled || !settings.RequireForRegister {
		response.Fail(c, 101, response.TranslateMsg(c, "EmailVerificationDisabled"))
		return
	}
	var form registerEmailSendRequest
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	email := service.NormalizeEmailForVerification(form.Email)
	parsedEmail, parseErr := mail.ParseAddress(email)
	if email == "" || parseErr != nil || parsedEmail.Address != email {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	smtpSettings, err := service.AllService.SettingsService.GetSMTPForSend()
	if err != nil {
		if global.Logger != nil {
			global.Logger.Warnf("register email verification SMTP lookup failed: %v", err)
		}
		respondRegisterEmailSendSuppressed(c)
		return
	}
	if !smtpSettings.Ready() {
		response.Fail(c, 101, response.TranslateMsg(c, "SMTPNotConfigured"))
		return
	}
	if !allowRegisterEmailSend(c.ClientIP(), time.Now()) {
		respondRegisterEmailSendSuppressed(c)
		return
	}
	challenge, code, err := service.AllService.EmailVerificationService.CreateCode(0, email, model.EmailVerificationPurposeRegister, c.ClientIP(), settings)
	if err != nil {
		respondRegisterEmailSendSuppressed(c)
		return
	}
	message := service.SMTPMessage{
		To:       challenge.Email,
		Subject:  "Verify your registration email address",
		TextBody: fmt.Sprintf("Your registration verification code is %s. It expires at %s.", code, challenge.ExpiresAt.Format(time.RFC3339)),
	}
	if err := sendEmailVerificationMessage(c.Request.Context(), smtpSettings, message); err != nil {
		if markErr := service.AllService.EmailVerificationService.MarkChallengeUsed(challenge.ID); markErr != nil && global.Logger != nil {
			global.Logger.Warnf("register email verification challenge cleanup failed: %v", markErr)
		}
		failureKey := classifySMTPDeliveryError(err)
		if global.Logger != nil {
			global.Logger.Warnf("register email verification send failed: %s", failureKey)
		}
		respondRegisterEmailSendSuppressed(c)
		return
	}
	response.Success(c, emailVerificationSendResponse{ChallengeID: challenge.ID, Email: challenge.Email, ExpiresAt: challenge.ExpiresAt})
}

// Register
func (ct *User) Register(c *gin.Context) {
	policy, err := service.AllService.SettingsService.GetRegisterPolicy()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if !policy.Enabled {
		response.Fail(c, 101, response.TranslateMsg(c, "RegisterClosed"))
		return
	}
	f := &admin.RegisterForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	if f.Password != f.ConfirmPassword {
		response.Fail(c, 101, response.TranslateMsg(c, "PasswordMismatch"))
		return
	}
	emailSettings, err := service.AllService.SettingsService.GetEmailVerification()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	requireEmail := emailSettings.Enabled && emailSettings.RequireForRegister
	f.Email = strings.TrimSpace(f.Email)
	if requireEmail && f.Email == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+"EmailRequired")
		return
	}
	if f.Email != "" {
		if errList := global.Validator.ValidVar(c, f.Email, "email,lte=128"); len(errList) > 0 {
			response.Fail(c, 101, errList[0])
			return
		}
	}
	regStatus := model.StatusCode(policy.DefaultStatus)
	// 注册状态可能未配置，默认启用
	if regStatus != model.COMMON_STATUS_DISABLED && regStatus != model.COMMON_STATUS_ENABLE {
		regStatus = model.COMMON_STATUS_ENABLE
	}

	var u *model.User
	if requireEmail {
		if strings.TrimSpace(f.EmailCode) == "" {
			response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+"EmailCodeRequired")
			return
		}
		var registerErr error
		u, registerErr = service.AllService.UserService.RegisterWithVerifiedEmail(f.Username, f.Email, f.Password, regStatus, f.EmailCode)
		if registerErr != nil {
			response.Fail(c, 101, translateUserServiceError(c, "OperationFailed", registerErr))
			return
		}
	} else {
		u = service.AllService.UserService.Register(f.Username, f.Email, f.Password, regStatus)
		if u == nil || u.Id == 0 {
			response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed"))
			return
		}
	}
	if regStatus == model.COMMON_STATUS_DISABLED {
		// 需要管理员审核
		response.Success(c, gin.H{"pending_approval": true})
		return
	}
	// 注册成功后自动登录
	ut := service.AllService.UserService.Login(u, &model.LoginLog{
		UserId: u.Id,
		Client: model.LoginLogClientWebAdmin,
		Uuid:   "",
		Ip:     c.ClientIP(),
		Type:   model.LoginLogTypeAccount,
	})
	responseLoginSuccess(c, u, ut.Token)
}
