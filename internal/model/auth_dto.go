package model

type GoogleLoginRequest struct {
	Code string `json:"code" validate:"required"`
}

type AuthResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
}
