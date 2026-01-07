package model

import "mime/multipart"

type CreateGroupChatRequest struct {
	Name            string                `form:"name" validate:"required,min=3,max=100"`
	Description     string                `form:"description" validate:"max=255"`
	MemberUsernames []string              `form:"member_usernames" validate:"required,min=1,dive,min=3,max=50"`
	Avatar          *multipart.FileHeader `form:"avatar" validate:"omitempty"`
}

type UpdateGroupChatRequest struct {
	Name        *string               `form:"name" validate:"omitempty,min=3,max=100"`
	Description *string               `form:"description" validate:"omitempty,max=255"`
	Avatar      *multipart.FileHeader `form:"avatar" validate:"omitempty"`
}

type SearchGroupMembersRequest struct {
	GroupID int    `json:"group_id" validate:"required"`
	Query   string `json:"query" validate:"omitempty,min=1"`
	Cursor  string `json:"cursor" validate:"omitempty"`
	Limit   int    `json:"limit" validate:"omitempty,gt=0,max=50"`
}

type AddGroupMemberRequest struct {
	Usernames []string `json:"usernames" validate:"required,min=1,dive,min=3,max=50"`
}

type UpdateGroupMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member"`
}

type TransferGroupOwnershipRequest struct {
	NewOwnerID int `json:"new_owner_id" validate:"required,gt=0"`
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
