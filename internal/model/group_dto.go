package model

import "mime/multipart"

type CreateGroupChatRequest struct {
	Name        string                `form:"name" validate:"required,min=3,max=100"`
	Description string                `form:"description" validate:"max=255"`
	MemberIDs   []int                 `form:"member_ids" validate:"required,min=1,dive,gt=0"`
	Avatar      *multipart.FileHeader `form:"avatar" validate:"omitempty"`
}

type SearchGroupMembersRequest struct {
	GroupID int    `json:"group_id" validate:"required"`
	Query   string `json:"query" validate:"omitempty,min=1"`
	Cursor  string `json:"cursor" validate:"omitempty"`
	Limit   int    `json:"limit" validate:"omitempty,gt=0,max=50"`
}

type AddGroupMemberRequest struct {
	UserID int `json:"user_id" validate:"required,gt=0"`
}

type GroupMemberDTO struct {
	ID       int     `json:"id"`
	UserID   int     `json:"user_id"`
	Username string  `json:"username"`
	FullName string  `json:"full_name"`
	Avatar   string  `json:"avatar"`
	Role     string  `json:"role"`
	JoinedAt string  `json:"joined_at"`
	IsOnline bool    `json:"is_online"`
}
