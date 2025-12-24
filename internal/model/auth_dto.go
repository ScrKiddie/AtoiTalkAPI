package model

type GoogleLoginRequest struct {
	Code string `json:"code" validate:"required"`
}

type AuthResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
}

type SendOTPRequest struct {
	Email string `json:"email" validate:"required,email"`
	Mode  string `json:"mode" validate:"required,temp_code_mode"`
	Token string `json:"token" validate:"required"`
}
