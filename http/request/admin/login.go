package admin

type Login struct {
	Username  string `json:"username" validate:"required" label:"用户名"`
	Password  string `json:"password,omitempty" validate:"required" label:"密码"`
	Platform  string `json:"platform" label:"平台"`
	Captcha   string `json:"captcha,omitempty" label:"验证码"`
	CaptchaId string `json:"captcha_id,omitempty"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email" label:"邮箱"`
}

type ForgotPasswordResetRequest struct {
	Token           string `json:"token" validate:"required" label:"重置令牌"`
	Password        string `json:"password" validate:"required,gte=4,lte=32" label:"密码"`
	ConfirmPassword string `json:"confirm_password" validate:"required,gte=4,lte=32" label:"确认密码"`
}

type SMTPTestRequest struct {
	To string `json:"to" validate:"required,email" label:"收件邮箱"`
}

type LoginLogQuery struct {
	UserId int `form:"user_id"`
	IsMy   int `form:"is_my"`
	PageQuery
}
type LoginTokenQuery struct {
	UserId int `form:"user_id"`
	PageQuery
}

type LoginLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
