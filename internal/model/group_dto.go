package model

import (
	"mime/multipart"

	"github.com/google/uuid"
)

type CreateGroupChatRequest struct {
	Name        string                `form:"name" validate:"required,min=3,max=100"`
	Description string                `form:"description" validate:"max=255"`
	MemberIDs   []uuid.UUID           `form:"member_ids" validate:"required,min=1,dive"`
	Avatar      *multipart.FileHeader `form:"avatar" validate:"omitempty"`
	IsPublic    bool                  `form:"is_public"`
}

type UpdateGroupChatRequest struct {
	Name        *string               `form:"name" validate:"omitempty,min=3,max=100"`
	Description *string               `form:"description" validate:"omitempty,max=255"`
	Avatar      *multipart.FileHeader `form:"avatar" validate:"omitempty"`
	IsPublic    *bool                 `form:"is_public"`
}

type SearchGroupMembersRequest struct {
	GroupID uuid.UUID `json:"group_id" validate:"required"`
	Query   string    `json:"query" validate:"omitempty,min=1"`
	Cursor  string    `json:"cursor" validate:"omitempty"`
	Limit   int       `json:"limit" validate:"omitempty,gt=0,max=50"`
}

type SearchPublicGroupsRequest struct {
	Query  string `json:"query" validate:"omitempty,min=1"`
	Cursor string `json:"cursor" validate:"omitempty"`
	Limit  int    `json:"limit" validate:"omitempty,gt=0,max=50"`
}

type AddGroupMemberRequest struct {
	UserIDs []uuid.UUID `json:"user_ids" validate:"required,min=1,dive"`
}

type UpdateGroupMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member"`
}

type TransferGroupOwnershipRequest struct {
	NewOwnerID uuid.UUID `json:"new_owner_id" validate:"required"`
}

type JoinGroupByInviteRequest struct {
	InviteCode string `json:"invite_code" validate:"required"`
}

type GroupMemberDTO struct {
	ID       uuid.UUID `json:"id"`
	UserID   uuid.UUID `json:"user_id"`
	FullName string    `json:"full_name"`
	Avatar   string    `json:"avatar"`
	Role     string    `json:"role"`
	JoinedAt string    `json:"joined_at"`
	IsOnline bool      `json:"is_online"`
	IsBanned bool      `json:"is_banned"`
}

type PublicGroupDTO struct {
	ID          uuid.UUID `json:"id"`
	ChatID      uuid.UUID `json:"chat_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Avatar      string    `json:"avatar"`
	MemberCount int       `json:"member_count"`
	IsMember    bool      `json:"is_member"`
}

type GroupInviteResponse struct {
	InviteCode string  `json:"invite_code"`
	ExpiresAt  *string `json:"expires_at,omitempty"`
}

type GroupPreviewDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Avatar      string    `json:"avatar"`
	MemberCount int       `json:"member_count"`
	IsPublic    bool      `json:"is_public"`
}
