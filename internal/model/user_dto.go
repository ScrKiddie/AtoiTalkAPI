package model

import "mime/multipart"

type UserDTO struct {
	ID            int    `json:"id"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
	Avatar        string `json:"avatar"`
	Bio           string `json:"bio"`
	HasPassword   bool   `json:"has_password"`
	PrivateChatID *int   `json:"private_chat_id,omitempty"`
}

type CreateUserDTO struct {
	Email    string
	FullName string
	Avatar   string
}

type UpdateProfileRequest struct {
	FullName      string                `form:"full_name" validate:"required,min=3,max=100"`
	Bio           string                `form:"bio" validate:"max=255"`
	Avatar        *multipart.FileHeader `form:"avatar" validate:"omitempty,imagevalid=800_800_2"`
	DeleteAvatar  bool                  `form:"delete_avatar"`
}

type SearchUserRequest struct {
	Query         string `json:"query" validate:"omitempty,min=1"`
	Cursor        string `json:"cursor"`
	Limit         int    `json:"limit" validate:"min=1,max=50"`
	IncludeChatID bool   `json:"include_chat_id"`
}
