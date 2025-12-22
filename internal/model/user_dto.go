package model

type UserDTO struct {
	ID       int    `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Avatar   string `json:"avatar,omitempty"`
}

type CreateUserDTO struct {
	Email    string
	FullName string
	Avatar   string
}
