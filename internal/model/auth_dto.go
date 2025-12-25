package model

type GoogleLoginRequest struct {
	Code string `json:"code" validate:"required"`
}

type AuthResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
}

type SendOTPRequest struct {
	Email        string `json:"email" validate:"required,email"`
	Mode         string `json:"mode" validate:"required,otp_mode"`
	CaptchaToken string `json:"captcha_token" validate:"required"`
}

type RegisterUserRequest struct {
	Code         string `json:"code" validate:"required,len=6"`
	FullName     string `json:"full_name" validate:"required,min=3,max=100"`
	Password     string `json:"password" validate:"required,min=8,max=72,password_complexity"`
	CaptchaToken string `json:"captcha_token" validate:"required"`
}
